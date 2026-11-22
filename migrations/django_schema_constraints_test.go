package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Django's schema tests add and remove constraints with the schema editor
// and read them back through introspection. Here AddConstraint and
// RemoveConstraint run the same schema editor code, the SQL comes from
// CollectSQL, and a constraint is found by its name or by its effect.

// addSchemaConstraint adds c to model and returns the new state and the SQL
// that adds it.
func addSchemaConstraint(
	t *testing.T, db *orm.DB, app string, state *migrations.ProjectState, model string, c orm.Constraint,
) (*migrations.ProjectState, string) {
	t.Helper()
	op := migrations.AddConstraint{Model: model, Constraint: c}
	next := stateForwards(t, app, state, op)
	stmts, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
		return op.DatabaseForwards(context.Background(), app, e, state, next)
	})
	if err != nil {
		t.Fatalf("collect the SQL of %s: %v", op.Describe(), err)
	}
	databaseForwards(t, db, app, op, state, next)
	return next, strings.Join(stmts, "\n")
}

// removeSchemaConstraint removes the constraint name from model.
func removeSchemaConstraint(
	t *testing.T, db *orm.DB, app string, state *migrations.ProjectState, model, name string,
) *migrations.ProjectState {
	t.Helper()
	return applyOps(t, db, app, state, migrations.RemoveConstraint{Model: model, Name: name})
}

// assertSQLContains fails the test when sql lacks any of the parts.
func assertSQLContains(t *testing.T, sql string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(sql, part) {
			t.Errorf("the SQL does not contain %s:\n%s", part, sql)
		}
	}
}

// insertSchemaAuthor inserts a row into schema_author. A nil height or
// weight is NULL.
func insertSchemaAuthor(db *orm.DB, name string, height, weight any) error {
	return execSQL(db, "INSERT INTO schema_author (name, height, weight) VALUES (?, ?, ?)", name, height, weight)
}

// supportsNullsNotDistinct reports Django's supports_nulls_distinct_unique_constraints.
func supportsNullsNotDistinct(db *orm.DB) bool {
	fd, ok := db.Dialect().(orm.FeatureDialect)
	return ok && fd.Features().UniqueNullsNotDistinct
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint
func TestSchemaUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{Fields: []string{"Name"}, Name: "name_uq"}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		d := db.Dialect()
		assertSQLContains(t, sql, d.Quote("schema_author"), d.Quote("name"))
		djangotest.AssertConstraintExists(t, db, "schema_author", "name_uq")
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "Author", "name_uq")
		djangotest.AssertConstraintNotExists(t, db, "schema_author", "name_uq")
	})
}

// Django orders the expression with desc() and checks the order through
// introspection. A Go constraint expression has no order. The test checks
// that the index is unique by its effect.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_func_unique_constraint
func TestSchemaFuncUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.Upper(orm.F("name"))}, Name: "func_upper_uq"}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		djangotest.AssertIndexNameExists(t, db, "schema_author", "func_upper_uq")
		mustExec(t, db, "INSERT INTO schema_author (name) VALUES (?)", "ada")
		assertRejected(t, db, "INSERT INTO schema_author (name) VALUES (?)", "ADA")
		// SQL contains a database function.
		assertSQLContains(t, sql, "UPPER("+db.Dialect().Quote("name")+")")
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "Author", "func_upper_uq")
		djangotest.AssertIndexNameNotExists(t, db, "schema_author", "func_upper_uq")
	})
}

// Django's BookWithSlug also has an author and a publication date, which
// the constraint does not use.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_composite_func_unique_constraint
func TestSchemaCompositeFuncUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "BookWithSlug", idField,
			migrations.FieldState{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 100},
			migrations.FieldState{Name: "Slug", Column: "slug", Kind: orm.KindChar, MaxLength: 20})
		constraint := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.Upper(orm.F("title")), orm.Lower(orm.F("slug"))},
			Name:        "func_upper_lower_unq",
		}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "BookWithSlug", constraint)
		djangotest.AssertIndexNameExists(t, db, "schema_bookwithslug", "func_upper_lower_unq")
		mustExec(t, db, "INSERT INTO schema_bookwithslug (title, slug) VALUES (?, ?)", "go", "Go")
		assertRejected(t, db, "INSERT INTO schema_bookwithslug (title, slug) VALUES (?, ?)", "GO", "go")
		// SQL contains database functions.
		d := db.Dialect()
		assertSQLContains(t, sql, "UPPER("+d.Quote("title")+")", "LOWER("+d.Quote("slug")+")")
		if strings.Index(sql, "UPPER") > strings.Index(sql, "LOWER") {
			t.Errorf("UPPER comes after LOWER:\n%s", sql)
		}
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "BookWithSlug", "func_upper_lower_unq")
		djangotest.AssertIndexNameNotExists(t, db, "schema_bookwithslug", "func_upper_lower_unq")
	})
}

// Django orders the expressions and reads the columns of the index through
// introspection. The Go test checks the SQL.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint_field_and_expression
func TestSchemaUniqueConstraintFieldAndExpression(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.F("height"), orm.F("uuid"), orm.Lower(orm.F("name"))},
			Name:        "func_f_lower_field_unq",
		}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		djangotest.AssertIndexNameExists(t, db, "schema_author", "func_f_lower_field_unq")
		// SQL contains database functions and columns.
		d := db.Dialect()
		assertSQLContains(t, sql, d.Quote("height"), d.Quote("uuid"), "LOWER("+d.Quote("name")+")")
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "Author", "func_f_lower_field_unq")
		djangotest.AssertIndexNameNotExists(t, db, "schema_author", "func_f_lower_field_unq")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_func_unique_constraint_partial
func TestSchemaFuncUniqueConstraintPartial(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.Upper(orm.F("name"))},
			Name:        "func_upper_cond_weight_uq",
			Condition:   orm.Q("weight__isnull", false),
		}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		djangotest.AssertIndexNameExists(t, db, "schema_author", "func_upper_cond_weight_uq")
		d := db.Dialect()
		assertSQLContains(t, sql, "UPPER("+d.Quote("name")+")", "WHERE "+d.Quote("weight")+" IS NOT NULL")
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "Author", "func_upper_cond_weight_uq")
		djangotest.AssertIndexNameNotExists(t, db, "schema_author", "func_upper_cond_weight_uq")
	})
}

// No Go counterpart: SchemaTests.test_func_unique_constraint_covering and
// test_func_unique_constraint_collate test the include option and Collate,
// and test_func_unique_constraint_nondeterministic uses Random. The orm has
// none of them. test_func_unique_constraint_unsupported tests a database
// without expression indexes; SQLite and PostgreSQL both have them.

// Django registers Lower on CharField and Abs on IntegerField. The orm has no
// Abs, so the weight is used as it is.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_func_unique_constraint_lookups
func TestSchemaFuncUniqueConstraintLookups(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		t.Cleanup(orm.RegisterTransform(orm.KindChar, "lower", orm.Lower))
		constraint := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.F("name__lower"), orm.F("weight")},
			Name:        "func_lower_abs_lookup_uq",
		}
		// Add constraint.
		state, sql := addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		djangotest.AssertIndexNameExists(t, db, "schema_author", "func_lower_abs_lookup_uq")
		// SQL contains columns.
		d := db.Dialect()
		assertSQLContains(t, sql, d.Quote("name"), d.Quote("weight"))
		if err := insertSchemaAuthor(db, "Ada", nil, 1); err != nil {
			t.Fatal(err)
		}
		if err := insertSchemaAuthor(db, "ADA", nil, 1); err == nil {
			t.Error("the database accepted a second row with the same lowercased name and weight")
		}
		// Remove constraint.
		removeSchemaConstraint(t, db, "schema", state, "Author", "func_lower_abs_lookup_uq")
		djangotest.AssertIndexNameNotExists(t, db, "schema_author", "func_lower_abs_lookup_uq")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_func_unique_constraint_nonexistent_field
func TestSchemaFuncUniqueConstraintNonexistentField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.Lower(orm.F("nonexistent"))}, Name: "func_nonexistent_uq"}
		op := migrations.AddConstraint{Model: "Author", Constraint: constraint}
		next := stateForwards(t, "schema", state, op)
		_, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
			return op.DatabaseForwards(context.Background(), "schema", e, state, next)
		})
		const msg = "annot resolve keyword 'nonexistent' into field. Choices are: height, id, name, uuid, weight"
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("got error %v, want one containing %q", err, msg)
		}
	})
}

// Django's nulls_distinct=True asks for the default explicitly. Go has no
// such setting: a constraint without NullsNotDistinct treats NULLs as
// distinct.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint_index_nulls_distinct
func TestSchemaUniqueConstraintIndexNullsDistinct(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		if !supportsNullsNotDistinct(db) {
			t.Skip("the database cannot treat NULLs in a unique constraint as equal")
		}
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		nullsDistinct := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("height")}, Name: "distinct_height"}
		nullsNotDistinct := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.F("weight")}, Name: "not_distinct_weight", NullsNotDistinct: true,
		}
		state, _ = addSchemaConstraint(t, db, "schema", state, "Author", nullsDistinct)
		state, _ = addSchemaConstraint(t, db, "schema", state, "Author", nullsNotDistinct)
		if err := insertSchemaAuthor(db, "", nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := insertSchemaAuthor(db, "", nil, 1); err != nil {
			t.Fatal(err)
		}
		if err := insertSchemaAuthor(db, "", 1, nil); err == nil {
			t.Error("the database accepted a second NULL weight")
		}
		state = removeSchemaConstraint(t, db, "schema", state, "Author", "distinct_height")
		removeSchemaConstraint(t, db, "schema", state, "Author", "not_distinct_weight")
		djangotest.AssertConstraintNotExists(t, db, "schema_author", "distinct_height")
		djangotest.AssertConstraintNotExists(t, db, "schema_author", "not_distinct_weight")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint_nulls_distinct
func TestSchemaUniqueConstraintNullsDistinct(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		if !supportsNullsNotDistinct(db) {
			t.Skip("the database cannot treat NULLs in a unique constraint as equal")
		}
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{Fields: []string{"Height", "Weight"}, Name: "constraint", NullsNotDistinct: true}
		state, _ = addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		for _, row := range [][2]any{{nil, nil}, {1, nil}, {nil, 1}} {
			if err := insertSchemaAuthor(db, "", row[0], row[1]); err != nil {
				t.Fatal(err)
			}
		}
		for _, row := range [][2]any{{nil, nil}, {1, nil}, {nil, 1}} {
			if err := insertSchemaAuthor(db, "", row[0], row[1]); err == nil {
				t.Errorf("the database accepted the height and weight %v twice", row)
			}
		}
		removeSchemaConstraint(t, db, "schema", state, "Author", "constraint")
		djangotest.AssertConstraintNotExists(t, db, "schema_author", "constraint")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint_nulls_distinct_condition
func TestSchemaUniqueConstraintNullsDistinctCondition(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		if !supportsNullsNotDistinct(db) {
			t.Skip("the database cannot treat NULLs in a unique constraint as equal")
		}
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{
			Fields:           []string{"Height", "Weight"},
			Name:             "un_height_weight_start_A",
			Condition:        orm.Q("name__startswith", "A"),
			NullsNotDistinct: true,
		}
		state, _ = addSchemaConstraint(t, db, "schema", state, "Author", constraint)
		accept := func(name string, height, weight any) {
			t.Helper()
			if err := insertSchemaAuthor(db, name, height, weight); err != nil {
				t.Fatal(err)
			}
		}
		reject := func(name string, height, weight any) {
			t.Helper()
			if err := insertSchemaAuthor(db, name, height, weight); err == nil {
				t.Errorf("the database accepted %s with the height %v and the weight %v", name, height, weight)
			}
		}
		accept("Adam", nil, nil)
		accept("Avocado", 1, nil)
		accept("Adrian", nil, 1)
		reject("Alex", nil, nil)
		accept("Bob", nil, nil)
		reject("Alex", 1, nil)
		accept("Bill", nil, nil)
		reject("Alex", nil, 1)
		accept("Celine", nil, 1)
		removeSchemaConstraint(t, db, "schema", state, "Author", "un_height_weight_start_A")
		djangotest.AssertConstraintNotExists(t, db, "schema_author", "un_height_weight_start_A")
	})
}

// UniqueConstraint is ignored on databases that don't support NULLS [NOT]
// DISTINCT. Django's constraint here is an expression with
// nulls_distinct=True; Go has no explicit "distinct" setting, so the test
// uses NullsNotDistinct, which the database cannot build either.
//
// Ported from Django: tests/schema/tests.py::SchemaTests.test_unique_constraint_nulls_distinct_unsupported
func TestSchemaUniqueConstraintNullsDistinctUnsupported(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		if supportsNullsNotDistinct(db) {
			t.Skip("the database supports NULLS NOT DISTINCT")
		}
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("name")}, Name: "func_name_uq", NullsNotDistinct: true}
		add := migrations.AddConstraint{Model: "Author", Constraint: constraint}
		next := stateForwards(t, "schema", state, add)
		remove := migrations.RemoveConstraint{Model: "Author", Name: "func_name_uq"}
		ctx := context.Background()
		stmts, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
			if err := add.DatabaseForwards(ctx, "schema", e, state, next); err != nil {
				return err
			}
			return remove.DatabaseForwards(ctx, "schema", e, next, state)
		})
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(stmts), 0)
	})
}
