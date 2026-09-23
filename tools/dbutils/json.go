package dbutils

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
)

// TODO: replace json with `jsonb` everywhere in the codebase
// TODO: Use PostgreSQL's native JSON functions directly.

// JSONEach returns a PostgreSQL string expression expanding json elements
// with some normalizations for non-json columns.
func JSONEach(column string) string {
	return fmt.Sprintf(
		`jsonb_array_elements_text(CASE WHEN ([[%s]] IS JSON OR json_valid([[%s]]::text)) AND jsonb_typeof([[%s]]::jsonb) = 'array' THEN [[%s]]::jsonb ELSE jsonb_build_array([[%s]]) END)`,
		column, column, column, column, column,
	)
}

// JSONEachByPlaceholder expands a given user input json array to multiple rows.
// Use [JSONEach] if you want to expand a column value instead.
// The [placeholder] is the parameter placeholder in SQL prepared statements.
// We assume the parameter value is a marshalled JSON array.
func JSONEachByPlaceholder(placeholder string) string {
	return fmt.Sprintf(
		`jsonb_array_elements({:%s}::jsonb)`,
		placeholder,
	)
}

// JsonArrayExistsStr is used to determine whether a JSON string array contains a string element.
// Right now it only used to determine whether a JSON string ID array contains a specific ID.
// Operation "?" definition: Does the string exist as a top-level key within the JSON value?
// The type of the key is only supported to be string.
// If we want to support other types, we may need to use `@>` operator instead.
func JsonArrayExistsStr(column string, strValue string) dbx.Expression {
	return dbx.NewExp(fmt.Sprintf("[[%s]] ? {:value}::text", column), dbx.Params{
		"value": strValue,
	})
}

// JSONArrayLength returns a PostgreSQL string expression for json array length
// with some normalizations for non-json columns.
//
// It works with both json and non-json column values.
//
// Returns 0 for empty string or NULL column values.
func JSONArrayLength(column string) string {
	return fmt.Sprintf(
		`(CASE WHEN ([[%s]] IS JSON OR JSON_VALID([[%s]]::text)) AND jsonb_typeof([[%s]]::jsonb) = 'array' THEN jsonb_array_length([[%s]]::jsonb) ELSE 0 END)`,
		column, column, column, column,
	)
}

// JSONExtract returns a PostgreSQL JSON extract string expression with
// some normalizations for non-json columns.
func JSONExtract(column string, path string) string {
	// prefix the path with dot if it is not starting with array notation
	if path != "" && !strings.HasPrefix(path, "[") {
		path = "." + path
	}

	// Using `json_value::text` will get a string with double quotes. Using `json_value #>> '{}'` to get string content instead.
	// Adding `::jsonb` at the end as a hint to `typeAwareJoin` to convert the other value to text while comparing the data (only if the other type is not determined).
	return fmt.Sprintf(
		`JSON_QUERY_OR_NULL([[%s]], '$%s')::jsonb`,
		column,
		path,
	)
}
