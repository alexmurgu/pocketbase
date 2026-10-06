package core

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/inflector"
)

// updateQueryWithSortedDeduplication selects the first matching joined row
// for each record before sorting and pagination. PostgreSQL cannot ORDER BY
// related columns in SELECT DISTINCT base.*, and adding those columns to the
// projection would allow duplicate records with different related values.
func (r *RecordFieldResolver) updateQueryWithSortedDeduplication(query *dbx.SelectQuery) {
	info := query.Info()
	if !info.Distinct || len(info.OrderBy) == 0 {
		return
	}

	baseAlias := r.baseCollectionAlias
	if baseAlias == "" {
		baseAlias = r.baseCollection.Name
	}
	// Ordinary base-column sorts already satisfy SELECT DISTINCT. Keep that
	// path unchanged instead of introducing an extra join for filter-only joins.
	if len(info.Selects) == 1 && inflector.Columnify(info.Selects[0]) == baseAlias+".*" {
		allBaseColumns := true
		for _, order := range info.OrderBy {
			expression, _ := splitRecordSortDirection(order)
			prefix := "[[" + baseAlias + "."
			if !strings.HasPrefix(expression, prefix) || !strings.HasSuffix(expression, "]]") {
				allBaseColumns = false
				break
			}
			field := expression[len(prefix) : len(expression)-2]
			if r.baseCollection.Fields.GetByName(field) == nil {
				allBaseColumns = false
				break
			}
		}
		if allBaseColumns {
			return
		}
	}
	id := "[[" + baseAlias + ".id]]"
	const sortAlias = "__pb_sorted_records"

	selects := []string{id + " AS __pb_record_id"}
	orderBy := make([]string, 0, len(info.OrderBy)+1)
	for i, order := range info.OrderBy {
		expression, direction := splitRecordSortDirection(order)
		alias := fmt.Sprintf("__pb_sort_%d", i)
		selects = append(selects, expression+" AS "+alias)
		orderBy = append(orderBy, "[["+sortAlias+"."+alias+"]]"+direction)
	}
	// Break ties consistently across pages.
	orderBy = append(orderBy, id+" ASC")

	inner := *query
	inner.WithBuildHook(nil).
		Distinct(false).
		SelectOption("DISTINCT ON (" + id + ")").
		Select(selects...).
		OrderBy(append([]string{id}, info.OrderBy...)...).
		Limit(-1).
		Offset(0)
	compiled := inner.Build()

	// Join the deduplicated ids back to the base table so the original
	// projection and record-loading hooks stay intact. Sorting columns remain
	// internal and never become record fields. Count queries can still count
	// the base table's id or ctid without counting duplicate joined rows.
	outer := info.Builder.Select(info.Selects...).From(info.From...).
		InnerJoin("("+compiled.SQL()+") "+sortAlias,
			dbx.NewExp(id+" = [["+sortAlias+".__pb_record_id]]")).
		Bind(compiled.Params()).
		OrderBy(orderBy...).
		Limit(info.Limit).
		Offset(info.Offset).
		WithContext(info.Context).
		WithBuildHook(info.BuildHook)
	outer.FieldMapper = query.FieldMapper
	outer.TableMapper = query.TableMapper
	*query = *outer
}

func splitRecordSortDirection(order string) (string, string) {
	order = strings.TrimSpace(order)
	var nulls string
	for _, suffix := range []string{" NULLS FIRST", " NULLS LAST"} {
		if strings.HasSuffix(strings.ToUpper(order), suffix) {
			nulls = suffix
			order = strings.TrimSpace(order[:len(order)-len(suffix)])
			break
		}
	}
	upper := strings.ToUpper(order)
	for _, direction := range []string{" ASC", " DESC"} {
		if strings.HasSuffix(upper, direction) {
			return strings.TrimSpace(order[:len(order)-len(direction)]), direction + nulls
		}
	}
	return order, nulls
}
