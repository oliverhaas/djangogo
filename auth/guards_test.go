package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// okHandler writes 200 and records whether it ran.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddlewareExposesCurrentUser(t *testing.T) {
	db := newTestDB(t)
	u := createUser(t, db, "dave", "pw")

	var sawUser *auth.User
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if cu, ok := auth.CurrentUser(r.Context()); ok {
			sawUser = cu
		}
	})

	mw := auth.Middleware(db)

	// Authenticated request: a session carrying the user id is in context.
	sess := &sessions.Session{}
	sess.Set("_auth_user_id", u.ID)
	ctx := sessions.NewContext(context.Background(), sess)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	mw(handler).ServeHTTP(httptest.NewRecorder(), req)
	if sawUser == nil || sawUser.Username != "dave" {
		t.Fatalf("authenticated request: CurrentUser = %+v, want dave", sawUser)
	}

	// Anonymous request: empty session, no user in context.
	sawUser = nil
	anon := &sessions.Session{}
	anonCtx := sessions.NewContext(context.Background(), anon)
	anonReq := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(anonCtx)
	mw(handler).ServeHTTP(httptest.NewRecorder(), anonReq)
	if sawUser != nil {
		t.Fatalf("anonymous request: CurrentUser = %+v, want none", sawUser)
	}
}

func TestLoginRequired(t *testing.T) {
	guard := auth.LoginRequired("/login/")

	// Anonymous: 302 to the login URL with ?next=.
	var ran bool
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secret/", nil)
	guard(okHandler(&ran)).ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("anonymous status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/login/?next=%2Fsecret%2F" {
		t.Fatalf("Location = %q, want /login/?next=%%2Fsecret%%2F", loc)
	}
	if ran {
		t.Fatal("wrapped handler ran for an anonymous request")
	}

	// Authenticated: 200 and the handler runs.
	ran = false
	rec = httptest.NewRecorder()
	ctx := auth.WithUser(context.Background(), &auth.User{ID: 1, Username: "x"})
	authReq := httptest.NewRequest(http.MethodGet, "/secret/", nil).WithContext(ctx)
	guard(okHandler(&ran)).ServeHTTP(rec, authReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !ran {
		t.Fatal("wrapped handler did not run for an authenticated request")
	}
}

// grantUserPermission inserts a Permission with the codename and links it directly
// to the user, returning the permission.
func grantUserPermission(t *testing.T, db *orm.DB, u *auth.User, codename string) {
	t.Helper()
	ctx := context.Background()
	p := &auth.Permission{Codename: codename, Name: codename, ContentType: "thing"}
	if err := orm.Query[auth.Permission](db).Create(ctx, p); err != nil {
		t.Fatalf("Create(permission): %v", err)
	}
	link := &auth.UserPermission{}
	link.User.SetPK(u.ID)
	link.Permission.SetPK(p.ID)
	if err := orm.Query[auth.UserPermission](db).Create(ctx, link); err != nil {
		t.Fatalf("Create(user permission): %v", err)
	}
}

// grantGroupPermission creates a group, adds the user to it, creates a permission
// with the codename, and grants it to the group.
func grantGroupPermission(t *testing.T, db *orm.DB, u *auth.User, codename string) {
	t.Helper()
	ctx := context.Background()
	g := &auth.Group{Name: codename + "-group"}
	if err := orm.Query[auth.Group](db).Create(ctx, g); err != nil {
		t.Fatalf("Create(group): %v", err)
	}
	ug := &auth.UserGroup{}
	ug.User.SetPK(u.ID)
	ug.Group.SetPK(g.ID)
	if err := orm.Query[auth.UserGroup](db).Create(ctx, ug); err != nil {
		t.Fatalf("Create(user group): %v", err)
	}
	p := &auth.Permission{Codename: codename, Name: codename, ContentType: "thing"}
	if err := orm.Query[auth.Permission](db).Create(ctx, p); err != nil {
		t.Fatalf("Create(permission): %v", err)
	}
	gp := &auth.GroupPermission{}
	gp.Group.SetPK(g.ID)
	gp.Permission.SetPK(p.ID)
	if err := orm.Query[auth.GroupPermission](db).Create(ctx, gp); err != nil {
		t.Fatalf("Create(group permission): %v", err)
	}
}

func TestHasPerm(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Superuser: always granted, even with no permission rows.
	super := &auth.User{Username: "root", IsSuperuser: true}
	if err := orm.Query[auth.User](db).Create(ctx, super); err != nil {
		t.Fatalf("Create(super): %v", err)
	}
	if ok, err := auth.HasPerm(ctx, db, super, "anything"); err != nil || !ok {
		t.Fatalf("HasPerm(super) = %v, %v; want true, nil", ok, err)
	}

	// Direct user permission.
	direct := createUser(t, db, "directu", "pw")
	grantUserPermission(t, db, direct, "edit_thing")
	if ok, err := auth.HasPerm(ctx, db, direct, "edit_thing"); err != nil || !ok {
		t.Fatalf("HasPerm(direct, edit_thing) = %v, %v; want true, nil", ok, err)
	}

	// Group-based permission.
	grouped := createUser(t, db, "groupu", "pw")
	grantGroupPermission(t, db, grouped, "view_thing")
	if ok, err := auth.HasPerm(ctx, db, grouped, "view_thing"); err != nil || !ok {
		t.Fatalf("HasPerm(grouped, view_thing) = %v, %v; want true, nil", ok, err)
	}

	// Missing permission: a user with neither direct nor group grant.
	plain := createUser(t, db, "plainu", "pw")
	if ok, err := auth.HasPerm(ctx, db, plain, "delete_thing"); err != nil || ok {
		t.Fatalf("HasPerm(plain, delete_thing) = %v, %v; want false, nil", ok, err)
	}
	// A user who has a different permission still lacks this one.
	if ok, err := auth.HasPerm(ctx, db, direct, "delete_thing"); err != nil || ok {
		t.Fatalf("HasPerm(direct, delete_thing) = %v, %v; want false, nil", ok, err)
	}
}

func TestPermissionRequired(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	granted := createUser(t, db, "permu", "pw")
	grantUserPermission(t, db, granted, "do_it")
	denied := createUser(t, db, "noperm", "pw")

	guard := auth.PermissionRequired(db, "do_it", "/login/")

	// Granted: 200.
	var ran bool
	rec := httptest.NewRecorder()
	grantedReq := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(auth.WithUser(ctx, granted))
	guard(okHandler(&ran)).ServeHTTP(rec, grantedReq)
	if rec.Code != http.StatusOK || !ran {
		t.Fatalf("granted: status %d ran %v, want 200 true", rec.Code, ran)
	}

	// Lacking the permission: 403.
	ran = false
	rec = httptest.NewRecorder()
	deniedReq := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(auth.WithUser(ctx, denied))
	guard(okHandler(&ran)).ServeHTTP(rec, deniedReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	if ran {
		t.Fatal("handler ran for a user lacking the permission")
	}

	// Anonymous: redirect to login.
	ran = false
	rec = httptest.NewRecorder()
	anonReq := httptest.NewRequest(http.MethodGet, "/", nil)
	guard(okHandler(&ran)).ServeHTTP(rec, anonReq)
	if rec.Code != http.StatusFound {
		t.Fatalf("anonymous: status %d, want %d", rec.Code, http.StatusFound)
	}
}
