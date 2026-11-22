package orm_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// thThing is Django's transaction_hooks.Thing.
type thThing struct {
	ID  int64
	Num int
}

// thModels returns the models of Django's transaction_hooks test app.
func thModels() []any {
	return []any{djangotest.App("transaction_hooks", &thThing{})}
}

// errForced is Django's ForcedError.
var errForced = errors.New("forced error")

// thCase is the state of one TestConnectionOnCommit test: the numbers that the
// callbacks reported, in the order they ran.
type thCase struct {
	t        *testing.T
	db       *orm.DB
	notified []int
}

// notify records that the callback for num ran, Django's notify().
func (c *thCase) notify(num int) {
	c.notified = append(c.notified, num)
}

// do creates a Thing and registers a callback that reports it, Django's do().
func (c *thCase) do(ctx context.Context, num int) {
	c.t.Helper()
	if err := orm.Query[thThing](c.db).Create(ctx, &thThing{Num: num}); err != nil {
		c.t.Fatalf("create thing %d: %v", num, err)
	}
	err := c.db.OnCommit(ctx, func(context.Context) error {
		c.notify(num)
		return nil
	})
	if err != nil {
		c.t.Fatalf("OnCommit: %v", err)
	}
}

// assertNotified checks the numbers the callbacks reported.
func (c *thCase) assertNotified(nums ...int) {
	c.t.Helper()
	if !slices.Equal(c.notified, nums) {
		c.t.Errorf("notified = %v, want %v", c.notified, nums)
	}
}

// assertDone checks the reported numbers and the stored Things, Django's
// assertDone().
func (c *thCase) assertDone(nums ...int) {
	c.t.Helper()
	c.assertNotified(nums...)
	things, err := orm.Query[thThing](c.db).All(context.Background())
	if err != nil {
		c.t.Fatalf("All: %v", err)
	}
	got := make([]int, 0, len(things))
	for _, th := range things {
		got = append(got, th.Num)
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(nums))
	if !slices.Equal(got, want) {
		c.t.Errorf("stored things = %v, want %v", got, want)
	}
}

// thRun runs body on every dialect with a fresh thCase.
func thRun(t *testing.T, body func(ctx context.Context, c *thCase)) {
	t.Helper()
	djangotest.EachDB(t, thModels(), func(t *testing.T, db *orm.DB) {
		body(context.Background(), &thCase{t: t, db: db})
	})
}

// atomic runs fn in an Atomic block and fails the test on an error.
func (c *thCase) atomic(ctx context.Context, fn func(ctx context.Context) error, opts ...orm.AtomicOption) {
	c.t.Helper()
	if err := c.db.Atomic(ctx, fn, opts...); err != nil {
		c.t.Fatalf("Atomic: %v", err)
	}
}

// atomicForced runs fn in an Atomic block and checks that it fails with
// errForced.
func (c *thCase) atomicForced(ctx context.Context, fn func(ctx context.Context) error, opts ...orm.AtomicOption) {
	c.t.Helper()
	djangotest.AssertErrorIs(c.t, c.db.Atomic(ctx, fn, opts...), errForced)
}

// thLogs collects the records logged through the default slog logger.
type thLogs struct {
	mu      sync.Mutex
	records []slog.Record
}

// Enabled accepts every level.
func (l *thLogs) Enabled(context.Context, slog.Level) bool { return true }

// Handle keeps the record.
func (l *thLogs) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, r.Clone())
	return nil
}

// WithAttrs returns the collector itself.
func (l *thLogs) WithAttrs([]slog.Attr) slog.Handler { return l }

// WithGroup returns the collector itself.
func (l *thLogs) WithGroup(string) slog.Handler { return l }

// thCaptureLogs routes the default slog logger into the returned collector for
// the rest of the test.
func thCaptureLogs(t *testing.T) *thLogs {
	t.Helper()
	previous := slog.Default()
	logs := &thLogs{}
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

// assertRobustLog checks that logs hold one error record for the robust
// callback of test, with Django's message ending in suffix, and that the
// record carries the error of the callback.
func assertRobustLog(t *testing.T, logs *thLogs, test, suffix string, want error) {
	t.Helper()
	logs.mu.Lock()
	defer logs.mu.Unlock()
	if len(logs.records) != 1 {
		t.Fatalf("got %d log records, want 1", len(logs.records))
	}
	r := logs.records[0]
	if r.Level != slog.LevelError {
		t.Errorf("level = %v, want ERROR", r.Level)
	}
	if !strings.HasPrefix(r.Message, "Error calling ") || !strings.Contains(r.Message, test) ||
		!strings.HasSuffix(r.Message, suffix) {
		t.Errorf("message = %q, want Error calling <%s callback>%s", r.Message, test, suffix)
	}
	var logged error
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			logged, _ = a.Value.Any().(error)
		}
		return true
	})
	if !errors.Is(logged, want) {
		t.Errorf("logged error = %v, want %v", logged, want)
	}
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_executes_immediately_if_no_transaction
func TestTransactionHooksExecutesImmediatelyIfNoTransaction(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.do(ctx, 1)
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_robust_if_no_transaction
func TestTransactionHooksRobustIfNoTransaction(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		robustErr := errors.New("robust callback")
		robustCallback := func(context.Context) error { return robustErr }
		logs := thCaptureLogs(c.t)
		if err := c.db.OnCommitRobust(ctx, robustCallback); err != nil {
			c.t.Errorf("OnCommitRobust = %v, want nil", err)
		}
		c.do(ctx, 1)
		c.assertDone(1)
		assertRobustLog(c.t, logs, "TestTransactionHooksRobustIfNoTransaction", " in OnCommit() (robust callback).", robustErr)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_robust_transaction
func TestTransactionHooksRobustTransaction(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		robustErr := errors.New("robust callback")
		robustCallback := func(context.Context) error { return robustErr }
		logs := thCaptureLogs(c.t)
		c.atomic(ctx, func(ctx context.Context) error {
			if err := c.db.OnCommitRobust(ctx, robustCallback); err != nil {
				return err
			}
			c.do(ctx, 1)
			return nil
		})
		c.assertDone(1)
		assertRobustLog(c.t, logs, "TestTransactionHooksRobustTransaction",
			" in OnCommit() during transaction (robust callback).", robustErr)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_delays_execution_until_after_transaction_commit
func TestTransactionHooksDelaysExecutionUntilAfterTransactionCommit(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			c.assertNotified()
			return nil
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_does_not_execute_if_transaction_rolled_back
func TestTransactionHooksDoesNotExecuteIfTransactionRolledBack(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomicForced(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			return errForced
		})
		c.assertDone()
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_executes_only_after_final_transaction_committed
func TestTransactionHooksExecutesOnlyAfterFinalTransactionCommitted(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 1)
				c.assertNotified()
				return nil
			})
			c.assertNotified()
			return nil
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_discards_hooks_from_rolled_back_savepoint
func TestTransactionHooksDiscardsHooksFromRolledBackSavepoint(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			// One successful savepoint.
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 1)
				return nil
			})
			// One failed savepoint.
			c.atomicForced(ctx, func(ctx context.Context) error {
				c.do(ctx, 2)
				return errForced
			})
			// Another successful savepoint.
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 3)
				return nil
			})
			return nil
		})
		// Only the callbacks of the successful savepoints run.
		c.assertDone(1, 3)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_no_hooks_run_from_failed_transaction
func TestTransactionHooksNoHooksRunFromFailedTransaction(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomicForced(ctx, func(ctx context.Context) error {
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 1)
				return nil
			})
			return errForced
		})
		c.assertDone()
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_inner_savepoint_rolled_back_with_outer
func TestTransactionHooksInnerSavepointRolledBackWithOuter(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.atomicForced(ctx, func(ctx context.Context) error {
				c.atomic(ctx, func(ctx context.Context) error {
					c.do(ctx, 1)
					return nil
				})
				return errForced
			})
			c.do(ctx, 2)
			return nil
		})
		c.assertDone(2)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_no_savepoints_atomic_merged_with_outer
func TestTransactionHooksNoSavepointsAtomicMergedWithOuter(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 1)
				c.atomicForced(ctx, func(context.Context) error {
					return errForced
				}, orm.WithoutSavepoint())
				return nil
			})
			return nil
		})
		c.assertDone()
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_inner_savepoint_does_not_affect_outer
func TestTransactionHooksInnerSavepointDoesNotAffectOuter(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 1)
				c.atomicForced(ctx, func(context.Context) error {
					return errForced
				})
				return nil
			})
			return nil
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_runs_hooks_in_order_registered
func TestTransactionHooksRunsHooksInOrderRegistered(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			c.atomic(ctx, func(ctx context.Context) error {
				c.do(ctx, 2)
				return nil
			})
			c.do(ctx, 3)
			return nil
		})
		c.assertDone(1, 2, 3)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_hooks_cleared_after_successful_commit
func TestTransactionHooksHooksClearedAfterSuccessfulCommit(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			return nil
		})
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 2)
			return nil
		})
		c.assertDone(1, 2) // not [1, 1, 2]
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_hooks_cleared_after_rollback
func TestTransactionHooksHooksClearedAfterRollback(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomicForced(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			return errForced
		})
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 2)
			return nil
		})
		c.assertDone(2)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_hooks_cleared_on_reconnect
func TestTransactionHooksHooksClearedOnReconnect(t *testing.T) {
	t.Skip("closing the connection of an atomic block is not possible")
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_error_in_hook_doesnt_prevent_clearing_hooks
func TestTransactionHooksErrorInHookDoesntPreventClearingHooks(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomicForced(ctx, func(ctx context.Context) error {
			return c.db.OnCommit(ctx, func(context.Context) error {
				return errForced
			})
		})
		c.atomic(ctx, func(ctx context.Context) error {
			c.do(ctx, 1)
			return nil
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_db_query_in_hook
func TestTransactionHooksDBQueryInHook(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		c.atomic(ctx, func(ctx context.Context) error {
			if err := orm.Query[thThing](c.db).Create(ctx, &thThing{Num: 1}); err != nil {
				return err
			}
			return c.db.OnCommit(ctx, func(ctx context.Context) error {
				things, err := orm.Query[thThing](c.db).All(ctx)
				for _, th := range things {
					c.notify(th.Num)
				}
				return err
			})
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_transaction_in_hook
func TestTransactionHooksTransactionInHook(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		onCommit := func(ctx context.Context) error {
			return c.db.Atomic(ctx, func(ctx context.Context) error {
				th := thThing{Num: 1}
				if err := orm.Query[thThing](c.db).Create(ctx, &th); err != nil {
					return err
				}
				c.notify(th.Num)
				return nil
			})
		}
		c.atomic(ctx, func(ctx context.Context) error {
			return c.db.OnCommit(ctx, onCommit)
		})
		c.assertDone(1)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_hook_in_hook
func TestTransactionHooksHookInHook(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		var onCommit func(ctx context.Context, i int, addHook bool) error
		onCommit = func(ctx context.Context, i int, addHook bool) error {
			return c.db.Atomic(ctx, func(ctx context.Context) error {
				if addHook {
					err := c.db.OnCommit(ctx, func(ctx context.Context) error {
						return onCommit(ctx, i+10, false)
					})
					if err != nil {
						return err
					}
				}
				th := thThing{Num: i}
				if err := orm.Query[thThing](c.db).Create(ctx, &th); err != nil {
					return err
				}
				c.notify(th.Num)
				return nil
			})
		}
		c.atomic(ctx, func(ctx context.Context) error {
			for _, i := range []int{1, 2} {
				err := c.db.OnCommit(ctx, func(ctx context.Context) error {
					return onCommit(ctx, i, true)
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		c.assertDone(1, 11, 2, 12)
	})
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_raises_exception_non_autocommit_mode
func TestTransactionHooksRaisesExceptionNonAutocommitMode(t *testing.T) {
	t.Skip("needs set_autocommit, which the ORM does not have")
}

// Ported from Django: tests/transaction_hooks/tests.py::TestConnectionOnCommit.test_raises_exception_non_callable
func TestTransactionHooksRaisesExceptionNonCallable(t *testing.T) {
	thRun(t, func(ctx context.Context, c *thCase) {
		assertErrorContains(c.t, c.db.OnCommit(ctx, nil), "OnCommit's callback must not be nil")
	})
}
