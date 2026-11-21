package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/schema/models.py that the ported tests use.
// Django's tests call the schema editor directly; here each change is an
// AlterField operation, which runs the same schema editor code.
var (
	schemaAuthorName   = migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 255}
	schemaAuthorHeight = migrations.FieldState{
		Name: "Height", Column: "height", Kind: orm.KindPositiveInt, Null: true, Blank: true,
	}
	schemaAuthorFields = []migrations.FieldState{
		idField,
		schemaAuthorName,
		schemaAuthorHeight,
		{Name: "Weight", Column: "weight", Kind: orm.KindInt, Null: true, Blank: true},
		{Name: "UUID", Column: "uuid", Kind: orm.KindUUID, Null: true},
	}
	schemaNoteInfo    = migrations.FieldState{Name: "Info", Column: "info", Kind: orm.KindText}
	schemaNoteAddress = migrations.FieldState{Name: "Address", Column: "address", Kind: orm.KindText, Null: true}
	schemaNoteFields  = []migrations.FieldState{idField, schemaNoteInfo, schemaNoteAddress}
)

// createSchemaModel creates the table of a model with the given fields
// (Django's editor.create_model) and returns the state that holds it.
func createSchemaModel(t *testing.T, db *orm.DB, app, name string, fields ...migrations.FieldState) *migrations.ProjectState {
	t.Helper()
	return applyOps(t, db, app, migrations.NewProjectState(), migrations.CreateModel{Name: name, Fields: fields})
}

// alterSchemaField changes a field of a model to field (Django's
// editor.alter_field) and returns the new state.
func alterSchemaField(t *testing.T, db *orm.DB, app string, state *migrations.ProjectState, model string, field migrations.FieldState) *migrations.ProjectState {
	t.Helper()
	return applyOps(t, db, app, state, migrations.AlterField{Model: model, Field: field})
}

// columnType returns the type of a column as the database reports it, in
// lower case: PostgreSQL's format_type and the declared type on SQLite. It
// plays the role of the field class in Django's column_classes.
func columnType(t *testing.T, db *orm.DB, table, column string) string {
	t.Helper()
	query := `SELECT type FROM pragma_table_info(?) WHERE name = ?`
	if db.Dialect().Name() == "postgres" {
		query = `SELECT format_type(a.atttypid, a.atttypmod) FROM pg_attribute a ` +
			`JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace ` +
			`WHERE n.nspname = current_schema() AND c.relname = $1 AND a.attname = $2`
	}
	var typ string
	if err := db.SQL().QueryRowContext(context.Background(), query, table, column).Scan(&typ); err != nil {
		t.Fatalf("type of %s.%s: %v", table, column, err)
	}
	return strings.ToLower(typ)
}

// wantColumnType returns the type the dialect of db reports for a column
// declared with the SQLite type sqlite or the PostgreSQL type postgres.
func wantColumnType(db *orm.DB, sqlite, postgres string) string {
	if db.Dialect().Name() == "postgres" {
		return postgres
	}
	return sqlite
}

// assertNotNullViolation asserts that err is a NOT NULL violation (Django's
// assertRaises(IntegrityError) on a NULL insert).
func assertNotNullViolation(t *testing.T, db *orm.DB, err error) {
	t.Helper()
	ec, ok := db.Dialect().(orm.ErrorClassifier)
	if err == nil || !ok || ec.ClassifyError(err) != orm.IntegrityNotNull {
		t.Errorf("got error %v, want a NOT NULL violation", err)
	}
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter
func TestSchemaAlter(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		// Ensure the field is right to begin with
		djangotest.AssertEqual(t, columnType(t, db, "schema_author", "name"),
			wantColumnType(db, "varchar(255)", "character varying(255)"))
		djangotest.AssertColumnNotNull(t, db, "schema_author", "name")
		// Alter the name field to a TextField
		newField := migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindText, Null: true}
		state = alterSchemaField(t, db, "schema", state, "Author", newField)
		djangotest.AssertEqual(t, columnType(t, db, "schema_author", "name"), "text")
		djangotest.AssertColumnNull(t, db, "schema_author", "name")
		// Change nullability again
		newField2 := migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindText}
		alterSchemaField(t, db, "schema", state, "Author", newField2)
		djangotest.AssertEqual(t, columnType(t, db, "schema_author", "name"), "text")
		djangotest.AssertColumnNotNull(t, db, "schema_author", "name")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_text_field
func TestSchemaAlterTextField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		state := createSchemaModel(t, db, "schema", "Note", schemaNoteFields...)
		newField := schemaNoteInfo
		newField.Blank = true
		alterSchemaField(t, db, "schema", state, "Note", newField)
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_text_field_to_not_null_with_default_value
func TestSchemaAlterTextFieldToNotNullWithDefaultValue(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Note", schemaNoteFields...)
		mustExec(t, db, `INSERT INTO schema_note (info, address) VALUES (?, NULL)`, "")
		newField := migrations.FieldState{Name: "Address", Column: "address", Kind: orm.KindText, Blank: true, Default: ""}
		alterSchemaField(t, db, "schema", state, "Note", newField)
		djangotest.AssertEqual(t, queryInt(t, db, `SELECT count(*) FROM schema_note WHERE address = ''`), int64(1))
	})
}

// testAlterTextFieldTo creates a Note whose info holds value and changes info
// to a field of the given kind, which must convert the value.
func testAlterTextFieldTo(t *testing.T, kind orm.Kind, value string) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Note", schemaNoteFields...)
		mustExec(t, db, `INSERT INTO schema_note (info) VALUES (?)`, value)
		newField := migrations.FieldState{Name: "Info", Column: "info", Kind: kind, Blank: true}
		alterSchemaField(t, db, "schema", state, "Note", newField)
		// Make sure the field isn't nullable
		djangotest.AssertColumnNotNull(t, db, "schema_note", "info")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_text_field_to_date_field
func TestSchemaAlterTextFieldToDateField(t *testing.T) {
	testAlterTextFieldTo(t, orm.KindDate, "1988-05-05")
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_text_field_to_datetime_field
func TestSchemaAlterTextFieldToDatetimeField(t *testing.T) {
	testAlterTextFieldTo(t, orm.KindDateTime, "1988-05-05 3:16:17.4567")
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_text_field_to_time_field
func TestSchemaAlterTextFieldToTimeField(t *testing.T) {
	testAlterTextFieldTo(t, orm.KindTime, "3:16:17.4567")
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_textual_field_keep_null_status
func TestSchemaAlterTextualFieldKeepNullStatus(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := createSchemaModel(t, db, "schema", "Note", schemaNoteFields...)
		assertNotNullViolation(t, db, execSQL(db, `INSERT INTO schema_note (info) VALUES (NULL)`))
		newField := migrations.FieldState{Name: "Info", Column: "info", Kind: orm.KindChar, MaxLength: 50}
		alterSchemaField(t, db, "schema", state, "Note", newField)
		assertNotNullViolation(t, db, execSQL(db, `INSERT INTO schema_note (info) VALUES (NULL)`))
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_numeric_field_keep_null_status
func TestSchemaAlterNumericFieldKeepNullStatus(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := applyOps(t, db, "schema", migrations.NewProjectState(), migrations.CreateModel{
			Name: "UniqueTest",
			Fields: []migrations.FieldState{
				idField,
				{Name: "Year", Column: "year", Kind: orm.KindInt},
				{Name: "Slug", Column: "slug", Kind: orm.KindSlug, MaxLength: 50, DBIndex: true},
			},
			UniqueTogether: [][]string{{"Year", "Slug"}},
		})
		insert := `INSERT INTO schema_uniquetest (year, slug) VALUES (NULL, ?)`
		assertNotNullViolation(t, db, execSQL(db, insert, "aaa"))
		newField := migrations.FieldState{Name: "Year", Column: "year", Kind: orm.KindBigInt}
		alterSchemaField(t, db, "schema", state, "UniqueTest", newField)
		assertNotNullViolation(t, db, execSQL(db, insert, "bbb"))
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_null_to_not_null
func TestSchemaAlterNullToNotNull(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		// Ensure the field is right to begin with
		djangotest.AssertColumnNull(t, db, "schema_author", "height")
		// Create some test data
		mustExec(t, db, `INSERT INTO schema_author (name, height) VALUES (?, ?)`, "Not null author", 12)
		mustExec(t, db, `INSERT INTO schema_author (name) VALUES (?)`, "Null author")
		height := `SELECT coalesce(height, -1) FROM schema_author WHERE name = ?`
		// Verify null value
		djangotest.AssertEqual(t, queryInt(t, db, height, "Not null author"), int64(12))
		djangotest.AssertEqual(t, queryInt(t, db, height, "Null author"), int64(-1))
		// Alter the height field to NOT NULL with default
		newField := migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindPositiveInt, Default: int64(42)}
		alterSchemaField(t, db, "schema", state, "Author", newField)
		djangotest.AssertColumnNotNull(t, db, "schema_author", "height")
		// Verify default value
		djangotest.AssertEqual(t, queryInt(t, db, height, "Not null author"), int64(12))
		djangotest.AssertEqual(t, queryInt(t, db, height, "Null author"), int64(42))
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_charfield_to_null
func TestSchemaAlterCharFieldToNull(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		// Change the CharField to null
		newField := schemaAuthorName
		newField.Null = true
		alterSchemaField(t, db, "schema", state, "Author", newField)
		djangotest.AssertColumnNull(t, db, "schema_author", "name")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_char_field_decrease_length
//
// The test is PostgreSQL specific in Django too: SQLite does not enforce the
// length of a varchar.
func TestSchemaAlterCharFieldDecreaseLength(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "postgres" {
			return
		}
		// Create the table.
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		mustExec(t, db, `INSERT INTO schema_author (name) VALUES (?)`, strings.Repeat("x", 255))
		// Change max_length of CharField.
		newField := migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 254}
		_, err := tryApplyOps(db, "schema", state, migrations.AlterField{Model: "Author", Field: newField})
		msg := "value too long for type character varying(254)"
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("got error %v, want %q", err, msg)
		}
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_textfield_to_null
func TestSchemaAlterTextFieldToNull(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		state := createSchemaModel(t, db, "schema", "Note", schemaNoteFields...)
		// Change the TextField to null
		newField := schemaNoteInfo
		newField.Null = true
		alterSchemaField(t, db, "schema", state, "Note", newField)
		djangotest.AssertColumnNull(t, db, "schema_note", "info")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_alter_null_to_not_null_keeping_default
func TestSchemaAlterNullToNotNullKeepingDefault(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Create the table
		height := schemaAuthorHeight
		height.Default = int64(42)
		state := createSchemaModel(t, db, "schema", "AuthorWithDefaultHeight", idField, schemaAuthorName, height)
		// Ensure the field is right to begin with
		djangotest.AssertColumnNull(t, db, "schema_authorwithdefaultheight", "height")
		// Alter the height field to NOT NULL keeping the previous default
		newField := migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindPositiveInt, Default: int64(42)}
		alterSchemaField(t, db, "schema", state, "AuthorWithDefaultHeight", newField)
		djangotest.AssertColumnNotNull(t, db, "schema_authorwithdefaultheight", "height")
	})
}

// Ported from Django: tests/schema/tests.py::SchemaTests.test_check_constraints
//
// The constraint is found by its effect: a negative height is rejected while
// it exists. Django finds it through introspection.
func TestSchemaCheckConstraints(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		hasCheck := func() bool {
			t.Helper()
			err := execSQL(db, `INSERT INTO schema_author (name, height) VALUES (?, -1)`, "check")
			if err == nil {
				mustExec(t, db, `DELETE FROM schema_author`)
				return false
			}
			ec, ok := db.Dialect().(orm.ErrorClassifier)
			if !ok || ec.ClassifyError(err) != orm.IntegrityCheck {
				t.Fatalf("insert a negative height: %v", err)
			}
			return true
		}
		// Create the tables
		state := createSchemaModel(t, db, "schema", "Author", schemaAuthorFields...)
		// Ensure the constraint exists
		if !hasCheck() {
			t.Error("No check constraint for height found")
		}
		// Alter the column to remove it
		newField := migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindInt, Null: true, Blank: true}
		state = alterSchemaField(t, db, "schema", state, "Author", newField)
		if hasCheck() {
			t.Error("Check constraint for height found")
		}
		// Alter the column to re-add it
		alterSchemaField(t, db, "schema", state, "Author", schemaAuthorHeight)
		if !hasCheck() {
			t.Error("No check constraint for height found")
		}
	})
}

// TestAlterFieldKindKeepsData changes a field to another kind with a row in
// the table, and checks the column type and the converted value. Django's
// schema tests cover these changes without reading the value back.
func TestAlterFieldKindKeepsData(t *testing.T) {
	cases := []struct {
		name             string
		from, to         migrations.FieldState
		value            any
		read             string
		want             string
		sqlite, postgres string
	}{
		{
			name:  "char to text",
			from:  migrations.FieldState{Kind: orm.KindChar, MaxLength: 20},
			to:    migrations.FieldState{Kind: orm.KindText},
			value: "Ada", read: "value", want: "Ada",
			sqlite: "text", postgres: "text",
		},
		{
			name:  "text to char",
			from:  migrations.FieldState{Kind: orm.KindText},
			to:    migrations.FieldState{Kind: orm.KindChar, MaxLength: 20},
			value: "Ada", read: "value", want: "Ada",
			sqlite: "varchar(20)", postgres: "character varying(20)",
		},
		{
			name:  "int to bigint",
			from:  migrations.FieldState{Kind: orm.KindInt},
			to:    migrations.FieldState{Kind: orm.KindBigInt},
			value: 2147483647, read: "CAST(value AS TEXT)", want: "2147483647",
			sqlite: "bigint", postgres: "bigint",
		},
		{
			name:  "smallint to bigint",
			from:  migrations.FieldState{Kind: orm.KindSmallInt},
			to:    migrations.FieldState{Kind: orm.KindBigInt},
			value: 32767, read: "CAST(value AS TEXT)", want: "32767",
			sqlite: "bigint", postgres: "bigint",
		},
		{
			name:  "int to decimal",
			from:  migrations.FieldState{Kind: orm.KindInt},
			to:    migrations.FieldState{Kind: orm.KindDecimal, MaxDigits: 10, DecimalPlaces: 2},
			value: 42, read: "CAST(value * 100 AS INTEGER)", want: "4200",
			sqlite: "decimal", postgres: "numeric(10,2)",
		},
		{
			name:  "char to date",
			from:  migrations.FieldState{Kind: orm.KindChar, MaxLength: 10},
			to:    migrations.FieldState{Kind: orm.KindDate},
			value: "2024-03-20", read: "CAST(value AS TEXT)", want: "2024-03-20",
			sqlite: "date", postgres: "date",
		},
		{
			name:  "nullable bigint to not null int",
			from:  migrations.FieldState{Kind: orm.KindBigInt, Null: true},
			to:    migrations.FieldState{Kind: orm.KindInt},
			value: 7, read: "CAST(value AS TEXT)", want: "7",
			sqlite: "integer", postgres: "bigint",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
				from, to := tc.from, tc.to
				from.Name, from.Column = "Value", "value"
				to.Name, to.Column = "Value", "value"
				state := createSchemaModel(t, db, "kinds", "Row", idField, from)
				before := columnType(t, db, "kinds_row", "value")
				mustExec(t, db, `INSERT INTO kinds_row (value) VALUES (?)`, tc.value)
				next := alterSchemaField(t, db, "kinds", state, "Row", to)
				djangotest.AssertEqual(t, columnType(t, db, "kinds_row", "value"), wantColumnType(db, tc.sqlite, tc.postgres))
				var got string
				query := `SELECT ` + tc.read + ` FROM kinds_row`
				if err := db.SQL().QueryRowContext(context.Background(), query).Scan(&got); err != nil {
					t.Fatalf("read the value: %v", err)
				}
				djangotest.AssertEqual(t, got, tc.want)
				// And the reverse change restores the column.
				databaseBackwards(t, db, "kinds", migrations.AlterField{Model: "Row", Field: to}, next, state)
				djangotest.AssertEqual(t, columnType(t, db, "kinds_row", "value"), before)
			})
		})
	}
}
