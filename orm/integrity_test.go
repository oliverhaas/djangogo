package orm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// intItem has a unique column and a NOT NULL column for the integrity tests.
type intItem struct {
	ID   int64
	Code string `orm:"max_length=10;unique"`
	Name string `orm:"max_length=10"`
}

// Meta names the table that the raw statements of the tests use.
func (intItem) Meta() orm.Meta { return orm.Meta{Table: "integrity_item"} }

// intChild references an item.
type intChild struct {
	ID   int64
	Item orm.FK[intItem] `orm:"on_delete=cascade"`
}

// Meta names the table that the raw statements of the tests use.
func (intChild) Meta() orm.Meta { return orm.Meta{Table: "integrity_child"} }

// intModels returns the models of the integrity tests.
func intModels() []any {
	return []any{djangotest.App("integrity", &intItem{}, &intChild{})}
}

// intKinds lists every kind sentinel, to check that an error matches exactly
// one of them.
var intKinds = map[orm.IntegrityKind]error{
	orm.IntegrityUnique:     orm.ErrUniqueViolation,
	orm.IntegrityForeignKey: orm.ErrForeignKeyViolation,
	orm.IntegrityNotNull:    orm.ErrNotNullViolation,
	orm.IntegrityCheck:      orm.ErrCheckViolation,
}

// assertIntegrity checks that err is an IntegrityError of the given kind: it
// matches ErrIntegrity and the sentinel of its kind, no other kind sentinel,
// and keeps the driver's message.
func assertIntegrity(t *testing.T, err error, kind orm.IntegrityKind) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want an integrity error")
	}
	var ie *orm.IntegrityError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v (%T), want an *orm.IntegrityError", err, err)
	}
	djangotest.AssertEqual(t, ie.Kind, kind)
	djangotest.AssertErrorIs(t, err, orm.ErrIntegrity)
	for k, sentinel := range intKinds {
		if got, want := errors.Is(err, sentinel), k == kind; got != want {
			t.Errorf("errors.Is(err, %v) = %v, want %v", sentinel, got, want)
		}
	}
	if ie.Err == nil || err.Error() == "" || ie.Error() != ie.Err.Error() {
		t.Errorf("IntegrityError does not keep the driver's message: %q, driver %v", ie.Error(), ie.Err)
	}
}

// TestIntegrityUniqueOnCreate checks that Create of a duplicate unique value
// reports a unique violation. On PostgreSQL the INSERT runs with RETURNING,
// so this covers the query path as well as the exec path of SQLite.
func TestIntegrityUniqueOnCreate(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &intItem{Code: "a", Name: "first"})
		err := orm.Query[intItem](db).Create(context.Background(), &intItem{Code: "a", Name: "second"})
		assertIntegrity(t, err, orm.IntegrityUnique)
	})
}

// TestIntegrityPrimaryKeyOnExec checks that a duplicate primary key written
// with Exec is a unique violation.
func TestIntegrityPrimaryKeyOnExec(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		item := create(t, db, &intItem{Code: "a", Name: "first"})
		_, err := db.Exec(context.Background(),
			"INSERT INTO integrity_item (id, code, name) VALUES (?, ?, ?)", item.ID, "b", "second")
		assertIntegrity(t, err, orm.IntegrityUnique)
	})
}

// TestIntegrityNotNull checks that a NULL in a NOT NULL column is a not-null
// violation.
func TestIntegrityNotNull(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		_, err := db.Exec(context.Background(),
			"INSERT INTO integrity_item (code, name) VALUES (?, NULL)", "a")
		assertIntegrity(t, err, orm.IntegrityNotNull)
	})
}

// TestIntegrityCheck checks that a value a CHECK constraint rejects is a check
// violation. The ORM declares no CHECK constraints, so the table is created
// by hand.
func TestIntegrityCheck(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if _, err := db.Exec(ctx, "CREATE TABLE integrity_positive (n INTEGER CHECK (n > 0))"); err != nil {
			t.Fatalf("create table: %v", err)
		}
		_, err := db.Exec(ctx, "INSERT INTO integrity_positive (n) VALUES (?)", -1)
		assertIntegrity(t, err, orm.IntegrityCheck)
	})
}

// TestIntegrityForeignKey checks that a reference to a missing row is a
// foreign key violation on both SQLite and PostgreSQL.
func TestIntegrityForeignKey(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		err := orm.Query[intChild](db).Create(context.Background(), &intChild{Item: fkTo[intItem](12345)})
		assertIntegrity(t, err, orm.IntegrityForeignKey)
	})
}

// TestIntegrityInsideAtomic checks that a violation inside Atomic reaches the
// caller unchanged and rolls the block back.
func TestIntegrityInsideAtomic(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &intItem{Code: "a", Name: "first"})
		err := db.Atomic(ctx, func(ctx context.Context) error {
			if err := orm.Query[intItem](db).Create(ctx, &intItem{Code: "b", Name: "second"}); err != nil {
				return err
			}
			return orm.Query[intItem](db).Create(ctx, &intItem{Code: "a", Name: "third"})
		})
		assertIntegrity(t, err, orm.IntegrityUnique)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[intItem](db)), 1)
	})
}

// TestIntegrityOtherErrorsAreNotIntegrity checks that an error that is no
// constraint violation, such as a syntax error, does not match ErrIntegrity.
func TestIntegrityOtherErrorsAreNotIntegrity(t *testing.T) {
	djangotest.EachDB(t, intModels(), func(t *testing.T, db *orm.DB) {
		_, err := db.Exec(context.Background(), "INSERT INTO integrity_missing_table (n) VALUES (1)")
		if err == nil {
			t.Fatal("err = nil, want the error of the missing table")
		}
		if errors.Is(err, orm.ErrIntegrity) {
			t.Errorf("errors.Is(%v, ErrIntegrity) = true, want false", err)
		}
		var ie *orm.IntegrityError
		if errors.As(err, &ie) {
			t.Errorf("err is an IntegrityError of kind %v, want none", ie.Kind)
		}
	})
}
