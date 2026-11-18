package orm

// CompileSelectForTest exposes the compiled SELECT to external tests so they
// can assert on the generated SQL, such as the joins of select_related.
func CompileSelectForTest[T any](qs *QuerySet[T]) (string, []any, error) {
	return qs.compileSelect()
}

// compileSelect returns the SELECT statement of the queryset.
func (qs *QuerySet[T]) compileSelect() (string, []any, error) {
	sql, args, _, err := qs.q.selectSQL()
	return sql, args, err
}

// compileCount returns the statement Count runs.
func (qs *QuerySet[T]) compileCount() (string, []any, error) {
	if qs.q.err != nil {
		return "", nil, qs.q.err
	}
	sql, args, _, err := qs.q.aggregateSQL([]annotation{{name: countAlias, expr: Count("*")}})
	return sql, args, err
}

// compileDelete returns the statement Delete runs.
func (qs *QuerySet[T]) compileDelete() (string, []any, error) {
	return qs.q.deleteSQL()
}

// compileUpdate returns the statement Update runs for the assignments.
func (qs *QuerySet[T]) compileUpdate(assignments ...any) (string, []any, error) {
	return qs.q.updateSQL(assignments)
}
