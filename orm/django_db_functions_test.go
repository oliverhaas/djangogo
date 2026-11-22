package orm_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/db_functions/tests.py::FunctionTests.test_nested_function_ordering
func TestFunctionNestedFunctionOrdering(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith"})
		create(t, db, &fnAuthor{Name: "Rhonda Simpson", Alias: new("ronny")})
		authors := orm.Query[fnAuthor](db)
		length := orm.Length(orm.Coalesce("alias", "name"))
		assertFlat(t, authors.OrderBy(length).ValuesList("name"), []any{"Rhonda Simpson", "John Smith"})
		assertFlat(t, authors.OrderBy(length.Desc()).ValuesList("name"), []any{"John Smith", "Rhonda Simpson"})
	})
}

// Ported from Django: tests/db_functions/tests.py::FunctionTests.test_func_transform_bilateral
func TestFunctionFuncTransformBilateral(t *testing.T) {
	t.Skip("needs bilateral transforms, which the ORM does not have")
}

// Ported from Django: tests/db_functions/tests.py::FunctionTests.test_func_transform_bilateral_multivalue
func TestFunctionFuncTransformBilateralMultivalue(t *testing.T) {
	t.Skip("needs bilateral transforms, which the ORM does not have")
}

// Ported from Django: tests/db_functions/tests.py::FunctionTests.test_function_as_filter
func TestFunctionFunctionAsFilter(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("SMITHJ")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db)
		smithj := orm.Upper(orm.Value("smithj"))
		assertFlat(t, authors.Filter("alias", smithj).ValuesList("name"), []any{"John Smith"})
		assertFlat(t, authors.Exclude("alias", smithj).ValuesList("name"), []any{"Rhonda"})
	})
}
