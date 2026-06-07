package orm

// CompileSelectForTest exposes compileSelect to external tests so they can assert
// on the generated SQL (e.g. that select_related produces a LEFT JOIN query).
func CompileSelectForTest[T any](q *QuerySet[T]) (string, []any, error) {
	return q.compileSelect()
}
