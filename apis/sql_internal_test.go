package apis

import "testing"

func TestRequiresNonTransactionalExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  bool
	}{
		{query: "CREATE INDEX CONCURRENTLY test_idx ON test_table (created)", want: true},
		{query: "CREATE UNIQUE INDEX CONCURRENTLY test_idx ON test_table (id)", want: true},
		{query: "DROP INDEX CONCURRENTLY test_idx", want: true},
		{query: "REINDEX TABLE CONCURRENTLY test_table", want: true},
		{query: "VACUUM (ANALYZE) test_table", want: true},
		{query: "CREATE INDEX test_idx ON test_table (created)", want: false},
		{query: "DELETE FROM test_table", want: false},
		{query: "SELECT 1", want: false},
	}

	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			if got := requiresNonTransactionalExecution(test.query); got != test.want {
				t.Fatalf("requiresNonTransactionalExecution(%q) = %t, want %t", test.query, got, test.want)
			}
		})
	}
}
