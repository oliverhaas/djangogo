package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// authModels is the registry of the auth tests: contenttypes, auth and the
// session table.
func authModels() []any {
	return []any{contenttypes.App{}, auth.App{}, &sessions.Record{}}
}

// newTestDB returns an SQLite database with the tables of authModels.
func newTestDB(t *testing.T) *orm.DB {
	t.Helper()
	return djangotest.NewDB(t, authModels()...)
}

// eachDB runs fn on every available dialect with the tables of authModels.
func eachDB(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	djangotest.EachDB(t, authModels(), fn)
}

// createUser inserts an active user with the given password and returns it.
func createUser(t *testing.T, db *orm.DB, username, password string) *auth.User {
	t.Helper()
	u, err := auth.CreateUser(context.Background(), db, username, username+"@example.com", password, false, false, true)
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", username, err)
	}
	return u
}

// reload returns a fresh copy of the user with the given id, which has an
// empty permission cache like a user Django fetched again.
func reload(t *testing.T, db *orm.DB, id int64) *auth.User {
	t.Helper()
	u, err := orm.Query[auth.User](db).Get(context.Background(), "id", id)
	if err != nil {
		t.Fatalf("reload user %d: %v", id, err)
	}
	return &u
}

// saveFlags writes the status flags of u to the database (Django's
// user.save() in the ported tests).
func saveFlags(t *testing.T, db *orm.DB, u *auth.User) {
	t.Helper()
	_, err := orm.Query[auth.User](db).Filter("id", u.ID).Update(context.Background(),
		"is_staff", u.IsStaff, "is_superuser", u.IsSuperuser, "is_active", u.IsActive)
	if err != nil {
		t.Fatalf("save user %d: %v", u.ID, err)
	}
}

// groupPerm creates a permission on the content type of auth.Group, the way
// the Django tests build their permissions. Its permission string is
// "auth.<codename>".
func groupPerm(t *testing.T, db *orm.DB, name, codename string) *auth.Permission {
	t.Helper()
	ctx := context.Background()
	ct, err := contenttypes.GetForInstance(ctx, db, &auth.Group{})
	if err != nil {
		t.Fatalf("content type of Group: %v", err)
	}
	p := &auth.Permission{Name: name, ContentTypeID: ct.ID, Codename: codename}
	if err := orm.Query[auth.Permission](db).Create(ctx, p); err != nil {
		t.Fatalf("create permission %q: %v", codename, err)
	}
	return p
}

// createGroup inserts a group with the given name.
func createGroup(t *testing.T, db *orm.DB, name string) *auth.Group {
	t.Helper()
	g := &auth.Group{Name: name}
	if err := orm.Query[auth.Group](db).Create(context.Background(), g); err != nil {
		t.Fatalf("create group %q: %v", name, err)
	}
	return g
}

// must fails the test when err is not nil.
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// okHandler writes 200 and records whether it ran.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}
