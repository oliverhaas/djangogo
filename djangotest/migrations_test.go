package djangotest_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// schemaDDL is plain DDL valid on SQLite and PostgreSQL. It gives the tests a
// foreign key, nullable and NOT NULL columns, a multi-column index, a unique
// index and a plain index.
var schemaDDL = []string{
	`CREATE TABLE t_parent (id INTEGER PRIMARY KEY)`,
	`CREATE TABLE t_child (id INTEGER PRIMARY KEY, ` +
		`parent_id INTEGER NOT NULL REFERENCES t_parent (id), ` +
		`note VARCHAR(10) NULL, a INTEGER, b INTEGER, u INTEGER, s INTEGER)`,
	`CREATE INDEX t_child_ab ON t_child (a, b)`,
	`CREATE UNIQUE INDEX t_child_u ON t_child (u)`,
	`CREATE INDEX t_child_s ON t_child (s)`,
}

// withSchema runs fn on both dialects against a database that holds the tables
// of schemaDDL.
func withSchema(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		for _, stmt := range schemaDDL {
			if _, err := db.SQL().ExecContext(context.Background(), stmt); err != nil {
				t.Fatalf("create schema: %v", err)
			}
		}
		fn(t, db)
	})
}

// schemaCase is one assertion call that must fail with the given message
// fragment.
type schemaCase struct {
	name string
	want string
	fn   func(t djangotest.TB, db *orm.DB)
}

// runFailing checks that every case fails with its message.
func runFailing(t *testing.T, db *orm.DB, cases []schemaCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustFail(t, c.want, func(tb djangotest.TB) { c.fn(tb, db) })
		})
	}
}

// TestAssertTableExists covers AssertTableExists and AssertTableNotExists.
func TestAssertTableExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertTableExists(t, db, "t_child")
		djangotest.AssertTableNotExists(t, db, "t_missing")
		runFailing(t, db, []schemaCase{
			{"exists", `table "t_missing" does not exist`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertTableExists(tb, db, "t_missing")
			}},
			{"not exists", `table "t_child" exists`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertTableNotExists(tb, db, "t_child")
			}},
		})
	})
}

// TestAssertColumnExists covers AssertColumnExists and AssertColumnNotExists.
func TestAssertColumnExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertColumnExists(t, db, "t_child", "note")
		djangotest.AssertColumnNotExists(t, db, "t_child", "missing")
		runFailing(t, db, []schemaCase{
			{"exists", `column "missing" does not exist in table "t_child"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnExists(tb, db, "t_child", "missing")
			}},
			{"not exists", `column "note" exists in table "t_child"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNotExists(tb, db, "t_child", "note")
			}},
		})
	})
}

// TestAssertColumnNull covers AssertColumnNull and AssertColumnNotNull,
// including the primary key and a missing column.
func TestAssertColumnNull(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertColumnNull(t, db, "t_child", "note")
		djangotest.AssertColumnNotNull(t, db, "t_child", "parent_id")
		djangotest.AssertColumnNotNull(t, db, "t_child", "id")
		runFailing(t, db, []schemaCase{
			{"null on NOT NULL", `column "parent_id" of table "t_child" is NOT NULL`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNull(tb, db, "t_child", "parent_id")
			}},
			{"null on primary key", `column "id" of table "t_child" is NOT NULL`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNull(tb, db, "t_child", "id")
			}},
			{"not null on nullable", `column "note" of table "t_child" accepts NULL`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNotNull(tb, db, "t_child", "note")
			}},
			{"null on missing", `column "missing" does not exist in table "t_child"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNull(tb, db, "t_child", "missing")
			}},
			{"not null on missing", `column "missing" does not exist in table "t_child"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertColumnNotNull(tb, db, "t_child", "missing")
			}},
		})
	})
}

// TestAssertIndexExists covers AssertIndexExists and AssertIndexNotExists:
// the index of a unique constraint counts, and column order matters.
func TestAssertIndexExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertIndexExists(t, db, "t_child", "a", "b")
		djangotest.AssertIndexExists(t, db, "t_child", "s")
		djangotest.AssertIndexExists(t, db, "t_child", "u")
		djangotest.AssertIndexNotExists(t, db, "t_child", "b", "a")
		djangotest.AssertIndexNotExists(t, db, "t_child", "a")
		djangotest.AssertIndexNotExists(t, db, "t_child", "note")
		runFailing(t, db, []schemaCase{
			{"exists on missing", `table "t_child" has no index on (note)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexExists(tb, db, "t_child", "note")
			}},
			{"exists reversed", `table "t_child" has no index on (b, a)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexExists(tb, db, "t_child", "b", "a")
			}},
			{"exists on prefix", `table "t_child" has no index on (a)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexExists(tb, db, "t_child", "a")
			}},
			{"not exists on index", `table "t_child" has an index on (a, b)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexNotExists(tb, db, "t_child", "a", "b")
			}},
			{"not exists on unique", `table "t_child" has an index on (u)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexNotExists(tb, db, "t_child", "u")
			}},
		})
	})
}

// TestAssertIndexNameExists covers AssertIndexNameExists and
// AssertIndexNameNotExists.
func TestAssertIndexNameExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertIndexNameExists(t, db, "t_child", "t_child_ab")
		djangotest.AssertIndexNameExists(t, db, "t_child", "t_child_u")
		djangotest.AssertIndexNameNotExists(t, db, "t_child", "t_child_missing")
		runFailing(t, db, []schemaCase{
			{"exists", `table "t_child" has no index named "t_child_missing"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexNameExists(tb, db, "t_child", "t_child_missing")
			}},
			{"not exists", `table "t_child" has an index named "t_child_ab"`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertIndexNameNotExists(tb, db, "t_child", "t_child_ab")
			}},
		})
	})
}

// TestAssertUniqueConstraintExists covers AssertUniqueConstraintExists and
// AssertUniqueConstraintNotExists. A plain index must not count as unique.
func TestAssertUniqueConstraintExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertUniqueConstraintExists(t, db, "t_child", "u")
		djangotest.AssertUniqueConstraintNotExists(t, db, "t_child", "s")
		djangotest.AssertUniqueConstraintNotExists(t, db, "t_child", "a", "b")
		djangotest.AssertUniqueConstraintNotExists(t, db, "t_child", "note")
		runFailing(t, db, []schemaCase{
			{"exists on plain index", `table "t_child" has no unique constraint on (s)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertUniqueConstraintExists(tb, db, "t_child", "s")
			}},
			{"exists on multi-column plain index", `table "t_child" has no unique constraint on (a, b)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertUniqueConstraintExists(tb, db, "t_child", "a", "b")
			}},
			{"exists on missing", `table "t_child" has no unique constraint on (note)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertUniqueConstraintExists(tb, db, "t_child", "note")
			}},
			{"not exists on unique", `table "t_child" has a unique constraint on (u)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertUniqueConstraintNotExists(tb, db, "t_child", "u")
			}},
		})
	})
}

// TestAssertFKExists covers AssertFKExists and AssertFKNotExists. The target
// table and column must both match.
func TestAssertFKExists(t *testing.T) {
	withSchema(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertFKExists(t, db, "t_child", "parent_id", "t_parent", "id")
		djangotest.AssertFKNotExists(t, db, "t_child", "note", "t_parent", "id")
		djangotest.AssertFKNotExists(t, db, "t_child", "parent_id", "t_child", "id")
		djangotest.AssertFKNotExists(t, db, "t_child", "parent_id", "t_parent", "other")
		runFailing(t, db, []schemaCase{
			{"exists on non-key column", `table "t_child" has no foreign key note -> t_parent(id)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertFKExists(tb, db, "t_child", "note", "t_parent", "id")
			}},
			{"exists on wrong table", `table "t_child" has no foreign key parent_id -> t_child(id)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertFKExists(tb, db, "t_child", "parent_id", "t_child", "id")
			}},
			{"exists on wrong column", `table "t_child" has no foreign key parent_id -> t_parent(other)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertFKExists(tb, db, "t_child", "parent_id", "t_parent", "other")
			}},
			{"not exists on existing key", `table "t_child" has a foreign key parent_id -> t_parent(id)`, func(tb djangotest.TB, db *orm.DB) {
				djangotest.AssertFKNotExists(tb, db, "t_child", "parent_id", "t_parent", "id")
			}},
		})
	})
}
