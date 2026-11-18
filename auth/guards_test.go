package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/sessions"
)

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
	auth.Login(sess, u)
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
	if loc := rec.Header().Get("Location"); loc != "/login/?next=/secret/" {
		t.Fatalf("Location = %q, want /login/?next=/secret/", loc)
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

func TestPermissionRequired(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	granted := createUser(t, db, "permu", "pw")
	must(t, granted.AddPermissions(ctx, db, groupPerm(t, db, "Can do it", "do_it")))
	denied := createUser(t, db, "noperm", "pw")

	guard := auth.PermissionRequired("auth.do_it", auth.WithLoginURL("/login/"), auth.WithGuardDB(db))

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
