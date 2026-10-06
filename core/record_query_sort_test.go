package core_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/search"
)

func TestRecordRelationSort(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	target := core.NewBaseCollection("sort_targets")
	target.Fields.Add(&core.TextField{Name: "name"}, &core.NumberField{Name: "rank"})
	if err := app.Save(target); err != nil {
		t.Fatal(err)
	}
	target.Fields.Add(&core.RelationField{Name: "parent", CollectionId: target.Id, MaxSelect: 1})
	if err := app.Save(target); err != nil {
		t.Fatal(err)
	}
	base := core.NewBaseCollection("sort_records")
	base.Fields.Add(
		&core.RelationField{Name: "single", CollectionId: target.Id, MaxSelect: 1},
		&core.RelationField{Name: "many", CollectionId: target.Id, MaxSelect: 5},
	)
	if err := app.Save(base); err != nil {
		t.Fatal(err)
	}
	newTarget := func(name string, rank int) string {
		record := core.NewRecord(target)
		record.Set("name", name)
		record.Set("rank", rank)
		if err := app.Save(record); err != nil {
			t.Fatal(err)
		}
		return record.Id
	}
	a9 := newTarget("alpha", 9)
	a3 := newTarget("alpha", 3)
	b0 := newTarget("beta", 0)
	b5 := newTarget("beta", 5)
	for child, parent := range map[string]string{a9: b5, a3: a9, b0: a3, b5: a3} {
		record, err := app.FindRecordById(target, child)
		if err != nil {
			t.Fatal(err)
		}
		record.Set("parent", parent)
		if err := app.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]string{}
	for _, fixture := range []struct {
		name   string
		single string
		many   []string
	}{
		{"a", a9, []string{a9, b0}},
		{"b", a3, []string{a3}},
		{"c", "", nil},
		{"d", b5, []string{b5}},
	} {
		record := core.NewRecord(base)
		record.Set("single", fixture.single)
		record.Set("many", fixture.many)
		if err := app.Save(record); err != nil {
			t.Fatal(err)
		}
		ids[fixture.name] = record.Id
	}

	for _, scenario := range []struct {
		name, filter, sort string
		want               []string
	}{
		{"single ascending", "", "single.name,single.rank", []string{"b", "a", "d", "c"}},
		{"single descending", "", "-single.name,-single.rank", []string{"c", "d", "a", "b"}},
		{"multiple ascending", "", "many.name,many.rank", []string{"b", "a", "d", "c"}},
		{"multiple descending", "", "-many.name,-many.rank", []string{"c", "d", "a", "b"}},
		{"nested relation", "", "single.parent.name,single.rank", []string{"b", "d", "a", "c"}},
		{"computed sort", "", "single.name:lower,single.rank", []string{"b", "a", "d", "c"}},
		{"filtered single", "single.name = {:name}", "single.name,single.rank", []string{"b", "a"}},
		{"filtered multiple", "many.name ?= {:name}", "many.name,many.rank", []string{"b", "a"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			records, err := app.FindRecordsByFilter(base, scenario.filter, scenario.sort, 0, 0,
				map[string]any{"name": "alpha"})
			if err != nil {
				t.Fatal(err)
			}
			want := make([]string, len(scenario.want))
			for i, name := range scenario.want {
				want[i] = ids[name]
			}
			got := make([]string, len(records))
			for i, record := range records {
				got[i] = record.Id
				if record.Get("__pb_sort_0") != nil {
					t.Fatal("internal sorting column leaked into a record")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}

	t.Run("explicit null ordering", func(t *testing.T) {
		resolver := core.NewRecordFieldResolver(app, base, nil, true)
		name, err := resolver.Resolve("single.name")
		if err != nil {
			t.Fatal(err)
		}
		rank, err := resolver.Resolve("single.rank")
		if err != nil {
			t.Fatal(err)
		}
		query := app.RecordQuery(base).OrderBy(name.Identifier+" ASC NULLS FIRST", rank.Identifier+" ASC")
		if err := resolver.UpdateQuery(query); err != nil {
			t.Fatal(err)
		}
		var records []*core.Record
		if err := query.All(&records); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, record := range records {
			got = append(got, record.Id)
		}
		if want := []string{ids["c"], ids["b"], ids["a"], ids["d"]}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("view collection", func(t *testing.T) {
		view := core.NewViewCollection("sort_view")
		view.ViewQuery = "SELECT id, single, many FROM sort_records"
		if err := app.Save(view); err != nil {
			t.Fatal(err)
		}
		records, err := app.FindRecordsByFilter(view, "", "single.name,single.rank", 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 || records[0].Id != ids["a"] || records[1].Id != ids["d"] {
			t.Fatalf("unexpected view page: %v", records)
		}
	})

	// Exercise the same paginated search provider as the records API, including
	// its ctid-based count query and the preserved RecordQuery loading hooks.
	var pageIds []string
	for page := 1; page <= 4; page++ {
		resolver := core.NewRecordFieldResolver(app, base, nil, true)
		provider := search.NewProvider(resolver).Query(app.RecordQuery(base)).CountCol("ctid")
		var records []*core.Record
		result, err := provider.ParseAndExec("sort=many.name,many.rank&perPage=1&page="+strconv.Itoa(page), &records)
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalItems != 4 || len(records) != 1 {
			t.Fatalf("unexpected page result: %+v", result)
		}
		pageIds = append(pageIds, records[0].Id)
	}
	if want := []string{ids["b"], ids["a"], ids["d"], ids["c"]}; !reflect.DeepEqual(pageIds, want) {
		t.Fatalf("paginated ids %v, want %v", pageIds, want)
	}
}
