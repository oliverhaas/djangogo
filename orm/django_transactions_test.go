package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// trReporter is Django's transactions.Reporter, without its Meta.ordering.
// Tests that rely on that ordering sort with OrderBy("first_name", "last_name")
// through trAll.
type trReporter struct {
	ID        int64
	FirstName string `orm:"max_length=30"`
	LastName  string `orm:"max_length=30"`
	Email     string `orm:"max_length=254"`
}

// trModels returns the models of Django's transactions test app.
func trModels() []any {
	return []any{djangotest.App("transactions", &trReporter{})}
}

// trBody is the body of a Django test that several test classes run. ctx is
// the context every query of the body must use.
type trBody func(ctx context.Context, t *testing.T, db *orm.DB)

// trCreate inserts a reporter on the transaction of ctx.
func trCreate(ctx context.Context, t *testing.T, db *orm.DB, r trReporter) *trReporter {
	t.Helper()
	if err := orm.Query[trReporter](db).Create(ctx, &r); err != nil {
		t.Fatalf("create reporter: %v", err)
	}
	return &r
}

// trAll returns every reporter ordered like Django's Meta.ordering.
func trAll(ctx context.Context, t *testing.T, db *orm.DB) []trReporter {
	t.Helper()
	rows, err := orm.Query[trReporter](db).OrderBy("first_name", "last_name").All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	return rows
}

// trAssertAll checks that the table holds exactly the wanted reporters, in
// the order of Django's Meta.ordering.
func trAssertAll(ctx context.Context, t *testing.T, db *orm.DB, want ...*trReporter) {
	t.Helper()
	got := trAll(ctx, t, db)
	if len(got) != len(want) {
		t.Errorf("got %d reporters %+v, want %d", len(got), got, len(want))
		return
	}
	for i := range got {
		if got[i] != *want[i] {
			t.Errorf("reporter %d = %+v, want %+v", i, got[i], *want[i])
		}
	}
}

// trOops is the error that Django's tests raise as Exception("Oops, ...").
func trOops(msg string) error { return errors.New("Oops, " + msg) }

// trAssertOops checks that err is one of the Oops errors.
func trAssertOops(t *testing.T, err error) {
	t.Helper()
	assertErrorContains(t, err, "Oops")
}

// trRun runs the body on every dialect, in autocommit mode (Django's
// AtomicTests).
func trRun(t *testing.T, body trBody) {
	t.Helper()
	djangotest.EachDB(t, trModels(), func(t *testing.T, db *orm.DB) {
		body(context.Background(), t, db)
	})
}

// trRunInside runs the body inside an outer atomic block that commits at the
// end (Django's AtomicInsideTransactionTests).
func trRunInside(t *testing.T, body trBody) {
	t.Helper()
	djangotest.EachDB(t, trModels(), func(t *testing.T, db *orm.DB) {
		err := db.Atomic(context.Background(), func(ctx context.Context) error {
			body(ctx, t, db)
			return nil
		})
		if err != nil {
			t.Errorf("outer Atomic: %v", err)
		}
	})
}

// trBodyDecoratorSyntaxCommit is the body of AtomicTests.test_decorator_syntax_commit.
// Go has no decorators: a named function run through Atomic stands in for a
// function decorated with transaction.atomic.
func trBodyDecoratorSyntaxCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter *trReporter
	makeReporter := func(ctx context.Context) error {
		reporter = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return nil
	}
	if err := db.Atomic(ctx, makeReporter); err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter)
}

// trBodyDecoratorSyntaxRollback is the body of AtomicTests.test_decorator_syntax_rollback.
func trBodyDecoratorSyntaxRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	makeReporter := func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
		return trOops("that's his last name")
	}
	trAssertOops(t, db.Atomic(ctx, makeReporter))
	trAssertAll(ctx, t, db)
}

// trBodyCommit is the body of AtomicTests.test_commit.
func trBodyCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter *trReporter
	err := db.Atomic(ctx, func(ctx context.Context) error {
		reporter = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return nil
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter)
}

// trBodyRollback is the body of AtomicTests.test_rollback.
func trBodyRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	err := db.Atomic(ctx, func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
		return trOops("that's his last name")
	})
	trAssertOops(t, err)
	trAssertAll(ctx, t, db)
}

// trBodyNestedCommitCommit is the body of AtomicTests.test_nested_commit_commit.
func trBodyNestedCommitCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter1, reporter2 *trReporter
	err := db.Atomic(ctx, func(ctx context.Context) error {
		reporter1 = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return db.Atomic(ctx, func(ctx context.Context) error {
			reporter2 = trCreate(ctx, t, db, trReporter{FirstName: "Archibald", LastName: "Haddock"})
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter2, reporter1)
}

// trBodyNestedCommitRollback is the body of AtomicTests.test_nested_commit_rollback.
func trBodyNestedCommitRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter *trReporter
	err := db.Atomic(ctx, func(ctx context.Context) error {
		reporter = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		inner := db.Atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
			return trOops("that's his last name")
		})
		trAssertOops(t, inner)
		return nil
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter)
}

// trBodyNestedRollbackCommit is the body of AtomicTests.test_nested_rollback_commit.
func trBodyNestedRollbackCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	err := db.Atomic(ctx, func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{LastName: "Tintin"})
		err := db.Atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{LastName: "Haddock"})
			return nil
		})
		if err != nil {
			t.Errorf("inner Atomic: %v", err)
		}
		return trOops("that's his first name")
	})
	trAssertOops(t, err)
	trAssertAll(ctx, t, db)
}

// trBodyNestedRollbackRollback is the body of AtomicTests.test_nested_rollback_rollback.
func trBodyNestedRollbackRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	err := db.Atomic(ctx, func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{LastName: "Tintin"})
		inner := db.Atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
			return trOops("that's his last name")
		})
		trAssertOops(t, inner)
		return trOops("that's his first name")
	})
	trAssertOops(t, err)
	trAssertAll(ctx, t, db)
}

// trAtomic is the one reusable atomic block of the reuse_* tests: every
// level runs through this same func value.
func trAtomic(db *orm.DB) func(context.Context, func(context.Context) error) error {
	return func(ctx context.Context, fn func(context.Context) error) error {
		return db.Atomic(ctx, fn)
	}
}

// trBodyReuseCommitCommit is the body of AtomicTests.test_reuse_commit_commit.
func trBodyReuseCommitCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	atomic := trAtomic(db)
	var reporter1, reporter2 *trReporter
	err := atomic(ctx, func(ctx context.Context) error {
		reporter1 = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return atomic(ctx, func(ctx context.Context) error {
			reporter2 = trCreate(ctx, t, db, trReporter{FirstName: "Archibald", LastName: "Haddock"})
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter2, reporter1)
}

// trBodyReuseCommitRollback is the body of AtomicTests.test_reuse_commit_rollback.
func trBodyReuseCommitRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	atomic := trAtomic(db)
	var reporter *trReporter
	err := atomic(ctx, func(ctx context.Context) error {
		reporter = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		inner := atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
			return trOops("that's his last name")
		})
		trAssertOops(t, inner)
		return nil
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	trAssertAll(ctx, t, db, reporter)
}

// trBodyReuseRollbackCommit is the body of AtomicTests.test_reuse_rollback_commit.
func trBodyReuseRollbackCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	atomic := trAtomic(db)
	err := atomic(ctx, func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{LastName: "Tintin"})
		err := atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{LastName: "Haddock"})
			return nil
		})
		if err != nil {
			t.Errorf("inner Atomic: %v", err)
		}
		return trOops("that's his first name")
	})
	trAssertOops(t, err)
	trAssertAll(ctx, t, db)
}

// trBodyReuseRollbackRollback is the body of AtomicTests.test_reuse_rollback_rollback.
func trBodyReuseRollbackRollback(ctx context.Context, t *testing.T, db *orm.DB) {
	atomic := trAtomic(db)
	err := atomic(ctx, func(ctx context.Context) error {
		trCreate(ctx, t, db, trReporter{LastName: "Tintin"})
		inner := atomic(ctx, func(ctx context.Context) error {
			trCreate(ctx, t, db, trReporter{FirstName: "Haddock"})
			return trOops("that's his last name")
		})
		trAssertOops(t, inner)
		return trOops("that's his first name")
	})
	trAssertOops(t, err)
	trAssertAll(ctx, t, db)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_decorator_syntax_commit
func TestTransactionsAtomicDecoratorSyntaxCommit(t *testing.T) {
	trRun(t, trBodyDecoratorSyntaxCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_decorator_syntax_rollback
func TestTransactionsAtomicDecoratorSyntaxRollback(t *testing.T) {
	trRun(t, trBodyDecoratorSyntaxRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_alternate_decorator_syntax_commit
func TestTransactionsAtomicAlternateDecoratorSyntaxCommit(t *testing.T) {
	trRun(t, trBodyDecoratorSyntaxCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_alternate_decorator_syntax_rollback
func TestTransactionsAtomicAlternateDecoratorSyntaxRollback(t *testing.T) {
	trRun(t, trBodyDecoratorSyntaxRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_commit
func TestTransactionsAtomicCommit(t *testing.T) {
	trRun(t, trBodyCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_rollback
func TestTransactionsAtomicRollback(t *testing.T) {
	trRun(t, trBodyRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_nested_commit_commit
func TestTransactionsAtomicNestedCommitCommit(t *testing.T) {
	trRun(t, trBodyNestedCommitCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_nested_commit_rollback
func TestTransactionsAtomicNestedCommitRollback(t *testing.T) {
	trRun(t, trBodyNestedCommitRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_nested_rollback_commit
func TestTransactionsAtomicNestedRollbackCommit(t *testing.T) {
	trRun(t, trBodyNestedRollbackCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_nested_rollback_rollback
func TestTransactionsAtomicNestedRollbackRollback(t *testing.T) {
	trRun(t, trBodyNestedRollbackRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_reuse_commit_commit
func TestTransactionsAtomicReuseCommitCommit(t *testing.T) {
	trRun(t, trBodyReuseCommitCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_reuse_commit_rollback
func TestTransactionsAtomicReuseCommitRollback(t *testing.T) {
	trRun(t, trBodyReuseCommitRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_reuse_rollback_commit
func TestTransactionsAtomicReuseRollbackCommit(t *testing.T) {
	trRun(t, trBodyReuseRollbackCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_reuse_rollback_rollback
func TestTransactionsAtomicReuseRollbackRollback(t *testing.T) {
	trRun(t, trBodyReuseRollbackRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_merged_commit_commit
func TestTransactionsAtomicMergedCommitCommit(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_merged_commit_rollback
func TestTransactionsAtomicMergedCommitRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_merged_rollback_commit
func TestTransactionsAtomicMergedRollbackCommit(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_merged_rollback_rollback
func TestTransactionsAtomicMergedRollbackRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_force_rollback
func TestTransactionsAtomicForceRollback(t *testing.T) {
	t.Skip("needs set_rollback/get_rollback and the savepoint API, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_prevent_rollback
func TestTransactionsAtomicPreventRollback(t *testing.T) {
	t.Skip("needs set_rollback/get_rollback and the savepoint API, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicTests.test_failure_on_exit_transaction
func TestTransactionsAtomicFailureOnExitTransaction(t *testing.T) {
	t.Skip("needs access to the savepoint ids of the connection")
}

// AtomicInsideTransactionTests inherits every AtomicTests test and runs it
// inside an outer atomic block that commits at the end.

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_decorator_syntax_commit
func TestTransactionsInsideTransactionDecoratorSyntaxCommit(t *testing.T) {
	trRunInside(t, trBodyDecoratorSyntaxCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_decorator_syntax_rollback
func TestTransactionsInsideTransactionDecoratorSyntaxRollback(t *testing.T) {
	trRunInside(t, trBodyDecoratorSyntaxRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_alternate_decorator_syntax_commit
func TestTransactionsInsideTransactionAlternateDecoratorSyntaxCommit(t *testing.T) {
	trRunInside(t, trBodyDecoratorSyntaxCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_alternate_decorator_syntax_rollback
func TestTransactionsInsideTransactionAlternateDecoratorSyntaxRollback(t *testing.T) {
	trRunInside(t, trBodyDecoratorSyntaxRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_commit
func TestTransactionsInsideTransactionCommit(t *testing.T) {
	trRunInside(t, trBodyCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_rollback
func TestTransactionsInsideTransactionRollback(t *testing.T) {
	trRunInside(t, trBodyRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_nested_commit_commit
func TestTransactionsInsideTransactionNestedCommitCommit(t *testing.T) {
	trRunInside(t, trBodyNestedCommitCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_nested_commit_rollback
func TestTransactionsInsideTransactionNestedCommitRollback(t *testing.T) {
	trRunInside(t, trBodyNestedCommitRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_nested_rollback_commit
func TestTransactionsInsideTransactionNestedRollbackCommit(t *testing.T) {
	trRunInside(t, trBodyNestedRollbackCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_nested_rollback_rollback
func TestTransactionsInsideTransactionNestedRollbackRollback(t *testing.T) {
	trRunInside(t, trBodyNestedRollbackRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_reuse_commit_commit
func TestTransactionsInsideTransactionReuseCommitCommit(t *testing.T) {
	trRunInside(t, trBodyReuseCommitCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_reuse_commit_rollback
func TestTransactionsInsideTransactionReuseCommitRollback(t *testing.T) {
	trRunInside(t, trBodyReuseCommitRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_reuse_rollback_commit
func TestTransactionsInsideTransactionReuseRollbackCommit(t *testing.T) {
	trRunInside(t, trBodyReuseRollbackCommit)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_reuse_rollback_rollback
func TestTransactionsInsideTransactionReuseRollbackRollback(t *testing.T) {
	trRunInside(t, trBodyReuseRollbackRollback)
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_merged_commit_commit
func TestTransactionsInsideTransactionMergedCommitCommit(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_merged_commit_rollback
func TestTransactionsInsideTransactionMergedCommitRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_merged_rollback_commit
func TestTransactionsInsideTransactionMergedRollbackCommit(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_merged_rollback_rollback
func TestTransactionsInsideTransactionMergedRollbackRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_force_rollback
func TestTransactionsInsideTransactionForceRollback(t *testing.T) {
	t.Skip("needs set_rollback/get_rollback and the savepoint API, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_prevent_rollback
func TestTransactionsInsideTransactionPreventRollback(t *testing.T) {
	t.Skip("needs set_rollback/get_rollback and the savepoint API, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicInsideTransactionTests.test_failure_on_exit_transaction
func TestTransactionsInsideTransactionFailureOnExitTransaction(t *testing.T) {
	t.Skip("needs access to the savepoint ids of the connection")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_decorator_syntax_commit
func TestTransactionsWithoutAutocommitDecoratorSyntaxCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_decorator_syntax_rollback
func TestTransactionsWithoutAutocommitDecoratorSyntaxRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_alternate_decorator_syntax_commit
func TestTransactionsWithoutAutocommitAlternateDecoratorSyntaxCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_alternate_decorator_syntax_rollback
func TestTransactionsWithoutAutocommitAlternateDecoratorSyntaxRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_commit
func TestTransactionsWithoutAutocommitCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_rollback
func TestTransactionsWithoutAutocommitRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_nested_commit_commit
func TestTransactionsWithoutAutocommitNestedCommitCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_nested_commit_rollback
func TestTransactionsWithoutAutocommitNestedCommitRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_nested_rollback_commit
func TestTransactionsWithoutAutocommitNestedRollbackCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_nested_rollback_rollback
func TestTransactionsWithoutAutocommitNestedRollbackRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_reuse_commit_commit
func TestTransactionsWithoutAutocommitReuseCommitCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_reuse_commit_rollback
func TestTransactionsWithoutAutocommitReuseCommitRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_reuse_rollback_commit
func TestTransactionsWithoutAutocommitReuseRollbackCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_reuse_rollback_rollback
func TestTransactionsWithoutAutocommitReuseRollbackRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_merged_commit_commit
func TestTransactionsWithoutAutocommitMergedCommitCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_merged_commit_rollback
func TestTransactionsWithoutAutocommitMergedCommitRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_merged_rollback_commit
func TestTransactionsWithoutAutocommitMergedRollbackCommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_merged_rollback_rollback
func TestTransactionsWithoutAutocommitMergedRollbackRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_force_rollback
func TestTransactionsWithoutAutocommitForceRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_prevent_rollback
func TestTransactionsWithoutAutocommitPreventRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicWithoutAutocommitTests.test_failure_on_exit_transaction
func TestTransactionsWithoutAutocommitFailureOnExitTransaction(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicMergeTests.test_merged_outer_rollback
func TestTransactionsMergedOuterRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicMergeTests.test_merged_inner_savepoint_rollback
func TestTransactionsMergedInnerSavepointRollback(t *testing.T) {
	t.Skip("needs atomic(savepoint=False), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicErrorsTests.test_atomic_prevents_setting_autocommit
func TestTransactionsAtomicPreventsSettingAutocommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicErrorsTests.test_atomic_prevents_calling_transaction_methods
func TestTransactionsAtomicPreventsCallingTransactionMethods(t *testing.T) {
	t.Skip("needs the transaction commit and rollback functions, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicErrorsTests.test_atomic_prevents_queries_in_broken_transaction
func TestTransactionsAtomicPreventsQueriesInBrokenTransaction(t *testing.T) {
	t.Skip("needs broken-transaction tracking (TransactionManagementError), which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicErrorsTests.test_atomic_allows_queries_after_fixing_transaction
func TestTransactionsAtomicAllowsQueriesAfterFixingTransaction(t *testing.T) {
	t.Skip("Django skips it on databases with atomic_transactions, which SQLite and PostgreSQL have")
}

// Ported from Django: tests/transactions/tests.py::AtomicErrorsTests.test_atomic_prevents_queries_in_broken_transaction_after_client_close
func TestTransactionsAtomicPreventsQueriesInBrokenTransactionAfterClientClose(t *testing.T) {
	t.Skip("closing the connection of an atomic block is not possible")
}

// Ported from Django: tests/transactions/tests.py::AtomicMySQLTests.test_implicit_savepoint_rollback
func TestTransactionsImplicitSavepointRollback(t *testing.T) {
	t.Skip("MySQL only")
}

// Go has no callable instances: a method value of a struct type stands in for
// Django's Callable() instance, and Atomic must accept it as fn.
type trCallable struct{}

// Call is the method whose value the test passes to Atomic.
func (trCallable) Call(context.Context) error { return nil }

// Ported from Django: tests/transactions/tests.py::AtomicMiscTests.test_wrap_callable_instance
func TestTransactionsAtomicAcceptsAMethodValue(t *testing.T) {
	djangotest.EachDB(t, trModels(), func(t *testing.T, db *orm.DB) {
		if err := db.Atomic(context.Background(), trCallable{}.Call); err != nil {
			t.Errorf("Atomic with a method value: %v", err)
		}
	})
}

// The savepoint name comes from the captured "SAVEPOINT <name>" statement,
// where Django reads connection.savepoint_ids[-1].
//
// Ported from Django: tests/transactions/tests.py::AtomicMiscTests.test_atomic_does_not_leak_savepoints_on_failure
func TestTransactionsAtomicDoesNotLeakSavepointsOnFailure(t *testing.T) {
	djangotest.EachDB(t, trModels(), func(t *testing.T, db *orm.DB) {
		qc := djangotest.CaptureQueries(db)
		defer qc.Stop()
		err := db.Atomic(context.Background(), func(ctx context.Context) error {
			inner := db.Atomic(ctx, func(context.Context) error {
				return trOops("intentional")
			})
			trAssertOops(t, inner)
			name := ""
			for _, q := range qc.Queries() {
				if after, ok := strings.CutPrefix(q.SQL, "SAVEPOINT "); ok {
					name = after
				}
			}
			if name == "" {
				t.Fatal("the inner block did not create a savepoint")
			}
			// Expected to fail: the savepoint no longer exists.
			_, err := db.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name)
			if err == nil {
				t.Error("ROLLBACK TO SAVEPOINT succeeded, want an error for a released savepoint")
			}
			return err
		})
		if err == nil {
			t.Error("outer Atomic = nil, want the error of the rollback to the released savepoint")
		}
	})
}

// Ported from Django: tests/transactions/tests.py::AtomicMiscTests.test_mark_for_rollback_on_error_in_transaction
func TestTransactionsMarkForRollbackOnErrorInTransaction(t *testing.T) {
	t.Skip("needs mark_for_rollback_on_error, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::AtomicMiscTests.test_mark_for_rollback_on_error_in_autocommit
func TestTransactionsMarkForRollbackOnErrorInAutocommit(t *testing.T) {
	t.Skip("needs mark_for_rollback_on_error, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::NonAutocommitTests.test_orm_query_after_error_and_rollback
func TestTransactionsOrmQueryAfterErrorAndRollback(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transactions/tests.py::NonAutocommitTests.test_orm_query_without_autocommit
func TestTransactionsOrmQueryWithoutAutocommit(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// The durable tests run the same way in Go for DurableTransactionTests and
// DurableTests: djangotest does not wrap a test in a transaction.

const trDurableNestedMsg = "a durable atomic block cannot be nested within another atomic block"

// trBodyDurableCommit is the body of DurableTestsBase.test_commit.
func trBodyDurableCommit(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter *trReporter
	err := db.AtomicDurable(ctx, func(ctx context.Context) error {
		reporter = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return nil
	})
	if err != nil {
		t.Fatalf("AtomicDurable: %v", err)
	}
	trAssertAll(ctx, t, db, reporter)
}

// trBodyDurableNestedOuterDurable is the body of DurableTestsBase.test_nested_outer_durable.
func trBodyDurableNestedOuterDurable(ctx context.Context, t *testing.T, db *orm.DB) {
	var reporter1, reporter2 *trReporter
	err := db.AtomicDurable(ctx, func(ctx context.Context) error {
		reporter1 = trCreate(ctx, t, db, trReporter{FirstName: "Tintin"})
		return db.Atomic(ctx, func(ctx context.Context) error {
			reporter2 = trCreate(ctx, t, db, trReporter{FirstName: "Archibald", LastName: "Haddock"})
			return nil
		})
	})
	if err != nil {
		t.Fatalf("AtomicDurable: %v", err)
	}
	trAssertAll(ctx, t, db, reporter2, reporter1)
}

// trBodyDurableNestedBothDurable is the body of DurableTestsBase.test_nested_both_durable.
func trBodyDurableNestedBothDurable(ctx context.Context, t *testing.T, db *orm.DB) {
	err := db.AtomicDurable(ctx, func(ctx context.Context) error {
		inner := db.AtomicDurable(ctx, func(context.Context) error {
			t.Error("the nested durable block ran")
			return nil
		})
		djangotest.AssertErrorIs(t, inner, orm.ErrDurableNested)
		assertErrorContains(t, inner, trDurableNestedMsg)
		return nil
	})
	if err != nil {
		t.Errorf("outer AtomicDurable: %v", err)
	}
}

// trBodyDurableNestedInnerDurable is the body of DurableTestsBase.test_nested_inner_durable.
func trBodyDurableNestedInnerDurable(ctx context.Context, t *testing.T, db *orm.DB) {
	err := db.Atomic(ctx, func(ctx context.Context) error {
		inner := db.AtomicDurable(ctx, func(context.Context) error {
			t.Error("the nested durable block ran")
			return nil
		})
		djangotest.AssertErrorIs(t, inner, orm.ErrDurableNested)
		assertErrorContains(t, inner, trDurableNestedMsg)
		return nil
	})
	if err != nil {
		t.Errorf("outer Atomic: %v", err)
	}
}

// trBodyDurableSequenceOfDurables is the body of DurableTestsBase.test_sequence_of_durables.
func trBodyDurableSequenceOfDurables(ctx context.Context, t *testing.T, db *orm.DB) {
	for _, name := range []string{"Tintin 1", "Tintin 2"} {
		var reporter *trReporter
		err := db.AtomicDurable(ctx, func(ctx context.Context) error {
			reporter = trCreate(ctx, t, db, trReporter{FirstName: name})
			return nil
		})
		if err != nil {
			t.Fatalf("AtomicDurable: %v", err)
		}
		got, err := orm.Query[trReporter](db).Get(ctx, "first_name", name)
		if err != nil {
			t.Fatalf("Get %q: %v", name, err)
		}
		if got != *reporter {
			t.Errorf("Get %q = %+v, want %+v", name, got, *reporter)
		}
	}
}

// Ported from Django: tests/transactions/tests.py::DurableTransactionTests.test_commit
func TestTransactionsDurableTransactionCommit(t *testing.T) {
	trRun(t, trBodyDurableCommit)
}

// Ported from Django: tests/transactions/tests.py::DurableTests.test_commit
func TestTransactionsDurableCommit(t *testing.T) {
	trRun(t, trBodyDurableCommit)
}

// Ported from Django: tests/transactions/tests.py::DurableTransactionTests.test_nested_outer_durable
func TestTransactionsDurableTransactionNestedOuterDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedOuterDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTests.test_nested_outer_durable
func TestTransactionsDurableNestedOuterDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedOuterDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTransactionTests.test_nested_both_durable
func TestTransactionsDurableTransactionNestedBothDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedBothDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTests.test_nested_both_durable
func TestTransactionsDurableNestedBothDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedBothDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTransactionTests.test_nested_inner_durable
func TestTransactionsDurableTransactionNestedInnerDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedInnerDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTests.test_nested_inner_durable
func TestTransactionsDurableNestedInnerDurable(t *testing.T) {
	trRun(t, trBodyDurableNestedInnerDurable)
}

// Ported from Django: tests/transactions/tests.py::DurableTransactionTests.test_sequence_of_durables
func TestTransactionsDurableTransactionSequenceOfDurables(t *testing.T) {
	trRun(t, trBodyDurableSequenceOfDurables)
}

// Ported from Django: tests/transactions/tests.py::DurableTests.test_sequence_of_durables
func TestTransactionsDurableSequenceOfDurables(t *testing.T) {
	trRun(t, trBodyDurableSequenceOfDurables)
}

// Ported from Django: tests/transactions/tests.py::SavepointTests.test_deprecation_warning
func TestTransactionsSavepointDeprecationWarning(t *testing.T) {
	t.Skip("needs savepoint(), which the ORM does not have")
}
