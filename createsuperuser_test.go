package djangogo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	_ "github.com/oliverhaas/djangogo/djangotest" // makes the fast MD5 hasher the preferred one
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// newSuperuserApp builds a DB-backed Application with the auth and session
// tables created, ready to run the auth and session commands. The DSN is
// unique per test so the in-memory database is not shared across tests.
func newSuperuserApp(t *testing.T) *Application {
	t.Helper()
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:" + t.Name() + "?mode=memory&cache=shared"},
	}, auth.App{}, sessions.App{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	for _, m := range app.Models.Models() {
		if err := app.DB.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	app.Out = &bytes.Buffer{}
	app.Commands.Out = app.Out
	return app
}

// unsetEnv removes the environment variable for the duration of the test.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// loadUser returns the user with the given username.
func loadUser(t *testing.T, app *Application, username string) auth.User {
	t.Helper()
	u, err := orm.Query[auth.User](app.DB).Get(context.Background(), "username", username)
	if err != nil {
		t.Fatalf("load user %q: %v", username, err)
	}
	return u
}

func TestCreatesuperuserMetadata(t *testing.T) {
	c := &createsuperuserCommand{}
	if c.Name() != "createsuperuser" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Help() != "Used to create a superuser." {
		t.Errorf("Help() = %q", c.Help())
	}
}

// TestCreatesuperuserInteractiveLineInput answers the prompts through the
// application's input, which is read line by line when it is not a terminal.
func TestCreatesuperuserInteractiveLineInput(t *testing.T) {
	app := newSuperuserApp(t)
	var stderr bytes.Buffer
	app.In = strings.NewReader("root\nroot@EXAMPLE.com\nshort\nshort\nn\ncorrect-horse-battery\ncorrect-horse-battery\n")
	c := &createsuperuserCommand{app: app, stderr: &stderr, systemUsername: func() string { return "" }}

	if err := c.Run(nil); err != nil {
		t.Fatalf("createsuperuser: %v", err)
	}

	u := loadUser(t, app, "root")
	if !u.IsSuperuser || !u.IsStaff || !u.IsActive {
		t.Errorf("flags = super:%v staff:%v active:%v, want all true", u.IsSuperuser, u.IsStaff, u.IsActive)
	}
	if u.Email != "root@example.com" {
		t.Errorf("email = %q, want the normalized root@example.com", u.Email)
	}
	if !u.CheckPassword("correct-horse-battery") {
		t.Error("the password was not set")
	}
	wantOut := "Username: Email address: Password: Password (again): " +
		"Bypass password validation and create user anyway? [y/N]: " +
		"Password: Password (again): Superuser created successfully.\n"
	if got := app.Out.(*bytes.Buffer).String(); got != wantOut {
		t.Errorf("stdout = %q, want %q", got, wantOut)
	}
	wantErr := "This password is too short. It must contain at least 8 characters.\n"
	if got := stderr.String(); got != wantErr {
		t.Errorf("stderr = %q, want %q", got, wantErr)
	}
}

// TestCreatesuperuserInputEnds checks that the end of the input stops the
// prompts with an error instead of looping.
func TestCreatesuperuserInputEnds(t *testing.T) {
	app := newSuperuserApp(t)
	app.In = strings.NewReader("root\n")
	c := &createsuperuserCommand{app: app, stderr: &bytes.Buffer{}, systemUsername: func() string { return "" }}
	err := c.Run(nil)
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("Run = %v, want an error about the end of the input", err)
	}
}

func TestCreatesuperuserNoInput(t *testing.T) {
	app := newSuperuserApp(t)
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "envpass")
	c := &createsuperuserCommand{app: app}

	if err := c.Run([]string{"--username", "envroot", "--email", "e@example.com", "--noinput"}); err != nil {
		t.Fatalf("createsuperuser --noinput: %v", err)
	}
	if u := loadUser(t, app, "envroot"); !u.CheckPassword("envpass") {
		t.Error("the password of DJANGO_SUPERUSER_PASSWORD was not set")
	}
}

// TestCreatesuperuserNoInputWithoutPassword checks Django's rule that a
// superuser created with --noinput and without DJANGO_SUPERUSER_PASSWORD
// gets an unusable password.
func TestCreatesuperuserNoInputWithoutPassword(t *testing.T) {
	app := newSuperuserApp(t)
	unsetEnv(t, "DJANGO_SUPERUSER_PASSWORD")
	c := &createsuperuserCommand{app: app}

	if err := c.Run([]string{"--username", "x", "--email", "", "--no-input"}); err != nil {
		t.Fatalf("createsuperuser --no-input: %v", err)
	}
	if u := loadUser(t, app, "x"); u.HasUsablePassword() {
		t.Error("the password should be unusable")
	}
}

// TestCreatesuperuserNoInputEmptyPassword checks that an empty
// DJANGO_SUPERUSER_PASSWORD becomes the password, as Django hashes it.
func TestCreatesuperuserNoInputEmptyPassword(t *testing.T) {
	app := newSuperuserApp(t)
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "")
	c := &createsuperuserCommand{app: app}

	if err := c.Run([]string{"--username", "x", "--email", "", "--noinput"}); err != nil {
		t.Fatalf("createsuperuser --noinput: %v", err)
	}
	if u := loadUser(t, app, "x"); !u.CheckPassword("") {
		t.Error("the empty password should be set")
	}
}

func TestCreatesuperuserRequiresDB(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := &createsuperuserCommand{app: app}
	if err := c.Run([]string{"--username", "x", "--noinput"}); !errors.Is(err, errNoDatabase) {
		t.Errorf("Run = %v, want errNoDatabase", err)
	}
}

// TestChangepasswordLineInput changes a password through the application's
// input and checks the output.
func TestChangepasswordLineInput(t *testing.T) {
	app := newSuperuserApp(t)
	if _, err := auth.CreateUser(context.Background(), app.DB, "joe", "", "qwerty", false, false, true); err != nil {
		t.Fatal(err)
	}
	app.In = strings.NewReader("one\ntwo\nnot-qwerty\nnot-qwerty\n")
	c := &changepasswordCommand{app: app, stderr: &bytes.Buffer{}}

	if err := c.Run([]string{"joe"}); err != nil {
		t.Fatalf("changepassword: %v", err)
	}
	if u := loadUser(t, app, "joe"); !u.CheckPassword("not-qwerty") {
		t.Error("the password was not changed")
	}
	want := "Changing password for user 'joe'\nPassword: Password (again): " +
		"Passwords do not match. Please try again.\nPassword: Password (again): " +
		"Password changed successfully for user 'joe'\n"
	if got := app.Out.(*bytes.Buffer).String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestChangepasswordMetadata(t *testing.T) {
	c := &changepasswordCommand{}
	if c.Name() != "changepassword" || c.Help() != "Change a user's password for django.contrib.auth." {
		t.Errorf("Name() = %q, Help() = %q", c.Name(), c.Help())
	}
}

// TestClearsessionsDeletesExpired checks that clearsessions deletes the
// expired rows of the database store and keeps the others.
func TestClearsessionsDeletesExpired(t *testing.T) {
	app := newSuperuserApp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for key, expiry := range map[string]time.Time{"old": now.Add(-time.Hour), "new": now.Add(time.Hour)} {
		if err := orm.Query[sessions.Record](app.DB).Create(ctx, &sessions.Record{SessionKey: key, SessionData: "{}", ExpireDate: expiry}); err != nil {
			t.Fatal(err)
		}
	}

	if err := (&clearsessionsCommand{app: app}).Run(nil); err != nil {
		t.Fatalf("clearsessions: %v", err)
	}
	rows, err := orm.Query[sessions.Record](app.DB).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.SessionKey)
	}
	if len(keys) != 1 || keys[0] != "new" {
		t.Errorf("sessions after clearsessions = %v, want [new]", keys)
	}
}

// noClearStore is a session store without ClearExpired.
type noClearStore struct{ sessions.Store }

// TestClearsessionsUnsupportedStore checks Django's message for a session
// store that cannot clear expired sessions.
func TestClearsessionsUnsupportedStore(t *testing.T) {
	app := newSuperuserApp(t)
	err := (&clearsessionsCommand{app: app, store: noClearStore{}}).Run(nil)
	want := "Session engine 'djangogo.noClearStore' doesn't support clearing expired sessions."
	var ce *commandError
	if !errors.As(err, &ce) || ce.Error() != want {
		t.Errorf("Run = %v, want the command error %q", err, want)
	}
}

func TestClearsessionsMetadata(t *testing.T) {
	c := &clearsessionsCommand{}
	want := "Can be run as a cronjob or directly to clean out expired sessions when the backend supports it."
	if c.Name() != "clearsessions" || c.Help() != want {
		t.Errorf("Name() = %q, Help() = %q", c.Name(), c.Help())
	}
}
