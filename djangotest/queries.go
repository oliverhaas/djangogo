package djangotest

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/oliverhaas/djangogo/orm"
)

// CapturedQuery is one SQL statement observed by a QueryCapture.
type CapturedQuery struct {
	SQL  string
	Args []any
}

// QueryCapture records the SQL statements a database executes, the counterpart
// of Django's CaptureQueriesContext. Create one with CaptureQueries and call
// Stop when done.
type QueryCapture struct {
	mu      sync.Mutex
	queries []CapturedQuery
	remove  func()
}

// CaptureQueries starts recording every statement db executes. Statements are
// counted as Django counts them: each SELECT, INSERT, UPDATE, DELETE, and DDL
// statement, and the SAVEPOINT statements of a nested Atomic block. The BEGIN
// and COMMIT of an outermost Atomic block are not seen.
func CaptureQueries(db *orm.DB) *QueryCapture {
	qc := &QueryCapture{}
	qc.remove = db.AddQueryHook(func(_ context.Context, query string, args []any) {
		qc.mu.Lock()
		defer qc.mu.Unlock()
		qc.queries = append(qc.queries, CapturedQuery{SQL: query, Args: args})
	})
	return qc
}

// Stop ends the capture. The recorded queries stay available.
func (qc *QueryCapture) Stop() { qc.remove() }

// Queries returns the statements recorded so far, in execution order.
func (qc *QueryCapture) Queries() []CapturedQuery {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	out := make([]CapturedQuery, len(qc.queries))
	copy(out, qc.queries)
	return out
}

// Len returns the number of statements recorded so far.
func (qc *QueryCapture) Len() int {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	return len(qc.queries)
}

// AssertNumQueries asserts that running fn executes exactly want statements on
// db (Django's assertNumQueries). On failure the captured statements are
// listed. See CaptureQueries for what counts as a statement.
func AssertNumQueries(t TB, db *orm.DB, want int, fn func()) {
	t.Helper()
	qc := CaptureQueries(db)
	func() {
		defer qc.Stop()
		fn()
	}()
	queries := qc.Queries()
	if len(queries) == want {
		return
	}
	var b strings.Builder
	for i, q := range queries {
		fmt.Fprintf(&b, "\n%d. %s", i+1, q.SQL)
	}
	t.Errorf("%d queries executed, %d expected\nCaptured queries were:%s", len(queries), want, b.String())
}

// querySetConfig collects the optional arguments of AssertQuerySetEqual.
type querySetConfig struct {
	unordered bool
}

// QuerySetOption is an optional argument of AssertQuerySetEqual.
type QuerySetOption func(*querySetConfig)

// Unordered makes AssertQuerySetEqual ignore the order of the rows (Django's
// ordered=False): the rows must match the expected values as a multiset.
func Unordered() QuerySetOption {
	return func(c *querySetConfig) { c.unordered = true }
}

// AssertQuerySetEqual runs the queryset and asserts that its rows, each mapped
// through transform, equal want (Django's assertQuerySetEqual). Rows are
// compared in order unless Unordered is given. transform plays the role of
// Django's transform argument; map each row to the value the Django test
// compares, for example its name or its primary key. A nil transform compares
// the rows themselves, so want must then be a slice of the model type.
func AssertQuerySetEqual[T, V any](t TB, qs *orm.QuerySet[T], want []V, transform func(T) V, opts ...QuerySetOption) {
	t.Helper()
	rows, err := qs.All(context.Background())
	if err != nil {
		t.Errorf("AssertQuerySetEqual: run queryset: %v", err)
		return
	}
	got := make([]V, len(rows))
	for i, row := range rows {
		if transform != nil {
			got[i] = transform(row)
			continue
		}
		v, ok := any(row).(V)
		if !ok {
			t.Errorf("AssertQuerySetEqual: without a transform, want must hold %T values, not %T", row, got[i])
			return
		}
		got[i] = v
	}
	AssertSequenceEqual(t, got, want, opts...)
}

// AssertSequenceEqual asserts that two slices hold deeply equal elements, in
// order unless Unordered is given (Django's assertSequenceEqual and, with
// Unordered, assertCountEqual). A nil slice equals an empty one.
func AssertSequenceEqual[V any](t TB, got, want []V, opts ...QuerySetOption) {
	t.Helper()
	c := &querySetConfig{}
	for _, opt := range opts {
		opt(c)
	}
	equal := len(got) == len(want)
	if equal && c.unordered {
		equal = sameMultiset(got, want)
	} else if equal {
		for i := range got {
			if !reflect.DeepEqual(got[i], want[i]) {
				equal = false
				break
			}
		}
	}
	if !equal {
		t.Errorf("sequences differ:\n got: %#v\nwant: %#v", got, want)
	}
}

// sameMultiset reports whether a and b (of equal length) hold the same
// elements regardless of order, pairing each element of a with one unused
// deeply equal element of b.
func sameMultiset[V any](a, b []V) bool {
	used := make([]bool, len(b))
	for _, x := range a {
		found := false
		for j, y := range b {
			if !used[j] && reflect.DeepEqual(x, y) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
