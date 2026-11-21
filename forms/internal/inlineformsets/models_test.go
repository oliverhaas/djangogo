// Package inlineformsets_test ports Django's tests/inline_formsets. It lives
// in its own package because tests/model_formsets declares models with the
// same names.
package inlineformsets_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// School is the School model of Django's tests/inline_formsets/models.py.
type School struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Parent is the Parent model of Django's tests/inline_formsets/models.py.
type Parent struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Child is the Child model of Django's tests/inline_formsets/models.py. The
// Go ORM has no Meta.constraints, so its UniqueConstraint unique_parents on
// (mother, father) is a unique_together.
type Child struct {
	ID     int64
	Mother orm.FK[Parent] `orm:"related_name=mothers_children"`
	Father orm.FK[Parent] `orm:"related_name=fathers_children"`
	School orm.FK[School]
	Name   string `orm:"max_length=100"`
}

// Meta declares unique_together on (mother, father).
func (Child) Meta() orm.Meta { return orm.Meta{UniqueTogether: [][]string{{"Mother", "Father"}}} }

// Poet is the Poet model of Django's tests/inline_formsets/models.py.
type Poet struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// String returns the name.
func (p Poet) String() string { return p.Name }

// Poem is the Poem model of Django's tests/inline_formsets/models.py.
type Poem struct {
	ID   int64
	Poet orm.FK[Poet]
	Name string `orm:"max_length=100"`
}

// Meta declares unique_together on (poet, name).
func (Poem) Meta() orm.Meta { return orm.Meta{UniqueTogether: [][]string{{"Poet", "Name"}}} }

// String returns the name.
func (p Poem) String() string { return p.Name }

// models returns the models of the ported tests.
func models() []any {
	return []any{djangotest.App("inline_formsets", &School{}, &Parent{}, &Child{}, &Poet{}, &Poem{})}
}

// eachDB runs fn on every database with the models of the ported tests.
func eachDB(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	djangotest.EachDB(t, models(), fn)
}

// create inserts obj and returns it, Django's Model.objects.create.
func create[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
	return obj
}

// count returns the number of rows of the queryset.
func count[T any](t *testing.T, qs *orm.QuerySet[T]) int64 {
	t.Helper()
	n, err := qs.Count(context.Background())
	if err != nil {
		t.Fatalf("count %T: %v", *new(T), err)
	}
	return n
}

// postData builds submitted data from alternating keys and values.
func postData(kv ...string) url.Values {
	data := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		data.Set(kv[i], kv[i+1])
	}
	return data
}

// assertErrContains fails the test unless err contains want.
func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error %v, want one containing %q", err, want)
	}
}

// fk returns a foreign key to the row with the primary key pk.
func fk[T any](pk int64) orm.FK[T] {
	var f orm.FK[T]
	f.SetPK(pk)
	return f
}
