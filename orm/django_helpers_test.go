package orm_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// create inserts obj through the ORM and fails the test when that fails. It
// returns obj so that a fixture can be built in one expression.
func create[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("create %T: %v", *obj, err)
	}
	return obj
}

// fkTo returns a foreign key that points at the row with the given primary key.
func fkTo[T any](pk int64) orm.FK[T] {
	var f orm.FK[T]
	f.SetPK(pk)
	return f
}

// day returns midnight UTC of the given date, the Go form of a naive Python
// datetime(year, month, day).
func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

// mustAll runs the queryset and fails the test on an error.
func mustAll[T any](t *testing.T, qs *orm.QuerySet[T]) []T {
	t.Helper()
	rows, err := qs.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	return rows
}

// mustCount returns the row count of the queryset and fails the test on an
// error.
func mustCount[T any](t *testing.T, qs *orm.QuerySet[T]) int64 {
	t.Helper()
	n, err := qs.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	return n
}

// equalValue reports whether two values of a Values row are equal. Times are
// compared as instants, because a driver may return them in another location.
func equalValue(got, want any) bool {
	gt, gotTime := got.(time.Time)
	wt, wantTime := want.(time.Time)
	if gotTime && wantTime {
		return gt.Equal(wt)
	}
	return reflect.DeepEqual(got, want)
}

// assertValues runs a Values queryset and checks its rows, in order.
func assertValues(t *testing.T, vs *orm.ValuesQuerySet, want []map[string]any) {
	t.Helper()
	got, err := vs.All(context.Background())
	if err != nil {
		t.Errorf("Values: %v", err)
		return
	}
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = len(got[i]) == len(want[i])
		for key, w := range want[i] {
			g, ok := got[i][key]
			equal = equal && ok && equalValue(g, w)
		}
	}
	if !equal {
		t.Errorf("values differ:\n got: %v\nwant: %v", got, want)
	}
}

// assertValuesList runs a ValuesList queryset and checks its rows, in order.
func assertValuesList(t *testing.T, vl *orm.ValuesListQuerySet, want [][]any) {
	t.Helper()
	got, err := vl.All(context.Background())
	if err != nil {
		t.Errorf("ValuesList: %v", err)
		return
	}
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = len(got[i]) == len(want[i])
		for k := 0; equal && k < len(got[i]); k++ {
			equal = equalValue(got[i][k], want[i][k])
		}
	}
	if !equal {
		t.Errorf("values lists differ:\n got: %v\nwant: %v", got, want)
	}
}

// assertFlat runs a one-key ValuesList queryset and checks its values, in
// order.
func assertFlat(t *testing.T, vl *orm.ValuesListQuerySet, want []any) {
	t.Helper()
	got, err := vl.Flat(context.Background())
	if err != nil {
		t.Errorf("Flat: %v", err)
		return
	}
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = equalValue(got[i], want[i])
	}
	if !equal {
		t.Errorf("flat values differ:\n got: %v\nwant: %v", got, want)
	}
}

// assertFieldError checks that err is an orm.FieldError whose message contains
// want. Django's FieldError messages are kept word for word, but start in
// lower case behind the "orm: <Model>: " prefix of a Go error.
func assertFieldError(t *testing.T, err error, want string) {
	t.Helper()
	if !errors.Is(err, orm.ErrFieldError) {
		t.Errorf("error = %v, want a FieldError", err)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// assertErrorContains checks that err is set and its message contains want.
func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Errorf("error = nil, want one containing %q", want)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}
