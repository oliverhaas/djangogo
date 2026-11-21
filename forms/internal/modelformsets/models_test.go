// Package modelformsets_test ports Django's tests/model_formsets and
// tests/model_formsets_regress. It lives in its own package because the forms
// tests already declare models with the names that these Django tests use.
package modelformsets_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Author is the Author model of Django's tests/model_formsets/models.py.
type Author struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Meta orders the authors by name.
func (Author) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// String returns the name.
func (a Author) String() string { return a.Name }

// Book is the Book model of Django's tests/model_formsets/models.py. Its
// clean() assertion has no Go counterpart.
type Book struct {
	ID     int64
	Author orm.FK[Author]
	Title  string `orm:"max_length=100"`
}

// Meta declares unique_together on (author, title) and orders by id.
func (Book) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"Author", "Title"}}, Ordering: []string{"id"}}
}

// String returns the title.
func (b Book) String() string { return b.Title }

// BookWithCustomPK is the BookWithCustomPK model of Django's
// tests/model_formsets/models.py.
type BookWithCustomPK struct {
	MyPK   orm.Decimal `orm:"pk;max_digits=5;decimal_places=0"`
	Author orm.FK[Author]
	Title  string `orm:"max_length=100"`
}

// Editor is the Editor model of Django's tests/model_formsets/models.py.
type Editor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// BookWithOptionalAltEditor is the BookWithOptionalAltEditor model of
// Django's tests/model_formsets/models.py.
type BookWithOptionalAltEditor struct {
	ID        int64
	Author    orm.FK[Author]
	AltEditor orm.FK[Editor] `orm:"blank;null;on_delete=set_null"`
	Title     string         `orm:"max_length=100"`
}

// Meta declares unique_together on (author, title, alt_editor).
func (BookWithOptionalAltEditor) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"Author", "Title", "AltEditor"}}}
}

// AuthorMeeting is the AuthorMeeting model of Django's
// tests/model_formsets/models.py.
type AuthorMeeting struct {
	ID      int64
	Name    string `orm:"max_length=100"`
	Authors orm.M2M[Author]
	Created orm.Date `orm:"type=date;editable=false"`
}

// CustomPrimaryKey is the CustomPrimaryKey model of Django's
// tests/model_formsets/models.py.
type CustomPrimaryKey struct {
	MyPK      string `orm:"pk;max_length=10"`
	SomeField string `orm:"max_length=100"`
}

// Place is the Place model of Django's tests/model_formsets/models.py.
type Place struct {
	ID   int64
	Name string `orm:"max_length=50"`
	City string `orm:"max_length=50"`
}

// String returns the name.
func (p Place) String() string { return p.Name }

// Owner is the Owner model of Django's tests/model_formsets/models.py.
type Owner struct {
	AutoID int64  `orm:"pk"`
	Name   string `orm:"max_length=100"`
	Place  orm.FK[Place]
}

// Meta orders the owners by auto_id.
func (Owner) Meta() orm.Meta { return orm.Meta{Ordering: []string{"auto_id"}} }

// String returns the name. Django's __str__ adds the place, which a Go
// String method cannot load.
func (o Owner) String() string { return o.Name }

// Location is the Location model of Django's tests/model_formsets/models.py.
type Location struct {
	ID    int64
	Place orm.FK[Place] `orm:"unique"`
	Lat   string        `orm:"max_length=100"`
	Lon   string        `orm:"max_length=100"`
}

// OwnerProfile is the OwnerProfile model of Django's
// tests/model_formsets/models.py.
type OwnerProfile struct {
	Owner orm.O2O[Owner] `orm:"pk"`
	Age   int            `orm:"type=positive_int"`
}

// Product is the Product model of Django's tests/model_formsets/models.py.
type Product struct {
	ID   int64
	Slug string `orm:"type=slug;unique"`
}

// String returns the slug.
func (p Product) String() string { return p.Slug }

// Price is the Price model of Django's tests/model_formsets/models.py.
type Price struct {
	ID       int64
	Price    orm.Decimal `orm:"max_digits=10;decimal_places=2"`
	Quantity int         `orm:"type=positive_int"`
}

// Meta declares unique_together on (price, quantity).
func (Price) Meta() orm.Meta { return orm.Meta{UniqueTogether: [][]string{{"Price", "Quantity"}}} }

// Repository is the Repository model of Django's
// tests/model_formsets/models.py.
type Repository struct {
	ID   int64
	Name string `orm:"max_length=25"`
}

// String returns the name.
func (r Repository) String() string { return r.Name }

// Revision is the Revision model of Django's tests/model_formsets/models.py.
type Revision struct {
	ID         int64
	Repository orm.FK[Repository]
	Revision   string `orm:"max_length=40"`
}

// Meta declares unique_together on (repository, revision).
func (Revision) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"Repository", "Revision"}}}
}

// Person is the Person model of Django's tests/model_formsets/models.py.
type Person struct {
	ID   int64
	Name string `orm:"max_length=128"`
}

// Team is the Team model of Django's tests/model_formsets/models.py.
type Team struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Player is the Player model of Django's tests/model_formsets/models.py.
type Player struct {
	ID   int64
	Team orm.FK[Team] `orm:"null;on_delete=set_null"`
	Name string       `orm:"max_length=100"`
}

// String returns the name.
func (p Player) String() string { return p.Name }

// Poet is the Poet model of Django's tests/model_formsets/models.py.
type Poet struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// String returns the name.
func (p Poet) String() string { return p.Name }

// Poem is the Poem model of Django's tests/model_formsets/models.py.
type Poem struct {
	ID   int64
	Poet orm.FK[Poet]
	Name string `orm:"max_length=100"`
}

// String returns the name.
func (p Poem) String() string { return p.Name }

// Post is the Post model of Django's tests/model_formsets/models.py,
// without unique_for_date, unique_for_year and unique_for_month, which the
// Go ORM does not have.
type Post struct {
	ID       int64
	Title    string   `orm:"max_length=50;blank"`
	Slug     string   `orm:"max_length=50;blank"`
	Subtitle string   `orm:"max_length=50;blank"`
	Posted   orm.Date `orm:"type=date"`
}

// AutoPKParent is the AutoPKParent model of Django's
// tests/model_formsets/models.py.
type AutoPKParent struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// UUIDPKChildOfAutoPKParent is the UUIDPKChildOfAutoPKParent model of
// Django's tests/model_formsets/models.py.
type UUIDPKChildOfAutoPKParent struct {
	UUID   orm.UUID `orm:"pk;default=uuid4;editable=false"`
	Name   string   `orm:"max_length=255"`
	Parent orm.FK[AutoPKParent]
}

// User is the User model of Django's tests/model_formsets_regress/models.py.
type User struct {
	ID       int64
	Username string `orm:"max_length=12;unique"`
	Serial   int
}

// UserSite is the UserSite model of Django's
// tests/model_formsets_regress/models.py. Its foreign key points at the
// primary key of User: the Go ORM has no to_field="username".
type UserSite struct {
	ID   int64
	User orm.FK[User]
	Data int
}

// Network is the Network model of Django's
// tests/model_formsets_regress/models.py.
type Network struct {
	ID   int64
	Name string `orm:"max_length=15"`
}

// Host is the Host model of Django's tests/model_formsets_regress/models.py.
type Host struct {
	ID       int64
	Network  orm.FK[Network]
	Hostname string `orm:"max_length=25"`
}

// String returns the hostname.
func (h Host) String() string { return h.Hostname }

// models returns the models of the ported tests.
func models() []any {
	return []any{
		djangotest.App("model_formsets",
			&Author{}, &Book{}, &BookWithCustomPK{}, &Editor{}, &BookWithOptionalAltEditor{},
			&AuthorMeeting{}, &CustomPrimaryKey{}, &Place{}, &Owner{}, &Location{}, &OwnerProfile{},
			&Product{}, &Price{}, &Repository{}, &Revision{}, &Person{}, &Team{}, &Player{},
			&Poet{}, &Poem{}, &Post{}, &AutoPKParent{}, &UUIDPKChildOfAutoPKParent{},
		),
		djangotest.App("model_formsets_regress", &User{}, &UserSite{}, &Network{}, &Host{}),
	}
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

// count returns the number of rows of T.
func count[T any](t *testing.T, db *orm.DB) int64 {
	t.Helper()
	n, err := orm.Query[T](db).Count(context.Background())
	if err != nil {
		t.Fatalf("count %T: %v", *new(T), err)
	}
	return n
}

// get returns the row of T matching the filter arguments.
func get[T any](t *testing.T, db *orm.DB, args ...any) T {
	t.Helper()
	obj, err := orm.Query[T](db).Get(context.Background(), args...)
	if err != nil {
		t.Fatalf("get %T %v: %v", obj, args, err)
	}
	return obj
}

// all returns the rows of the queryset.
func all[T any](t *testing.T, qs *orm.QuerySet[T]) []T {
	t.Helper()
	rows, err := qs.All(context.Background())
	if err != nil {
		t.Fatalf("all %T: %v", *new(T), err)
	}
	return rows
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

// o2o returns a one-to-one relation to the row with the primary key pk.
func o2o[T any](pk int64) orm.O2O[T] {
	var f orm.O2O[T]
	f.SetPK(pk)
	return f
}

// errorMessages returns the messages of each form's errors, Django's
// formset.errors compared with a list of dicts.
func errorMessages(errs []*forms.ErrorDict) []map[string][]string {
	out := make([]map[string][]string, len(errs))
	for i, e := range errs {
		out[i] = e.Messages()
	}
	return out
}
