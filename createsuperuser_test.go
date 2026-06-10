package djangogo

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/orm"
)

// newSuperuserApp builds a DB-backed Application with the auth models migrated,
// ready to run the createsuperuser command. The DSN is unique per test so the
// in-memory database is not shared across tests.
func newSuperuserApp(t *testing.T) *Application {
	t.Helper()
	app, err := New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:" + t.Name() + "?mode=memory&cache=shared"},
	}, auth.App{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	for _, m := range app.Models.Models() {
		if err := app.DB.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	return app
}

func TestCreatesuperuserMetadata(t *testing.T) {
	c := &createsuperuserCommand{}
	if c.Name() != "createsuperuser" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Help() == "" {
		t.Error("Help() should be non-empty")
	}
}

func TestCreatesuperuserInteractive(t *testing.T) {
	app := newSuperuserApp(t)
	var out bytes.Buffer
	app.Out = &out
	app.Commands.Out = &out
	app.In = strings.NewReader("root\nroot@example.com\nhunter2\nhunter2\n")

	if err := app.Execute([]string{"createsuperuser"}); err != nil {
		t.Fatalf("createsuperuser: %v", err)
	}

	u, err := orm.Query[auth.User](app.DB).Get(context.Background(), "username", "root")
	if err != nil {
		t.Fatalf("loading created superuser: %v", err)
	}
	if !u.IsSuperuser || !u.IsStaff || !u.IsActive {
		t.Errorf("flags = super:%v staff:%v active:%v, want all true", u.IsSuperuser, u.IsStaff, u.IsActive)
	}
	if u.Email != "root@example.com" {
		t.Errorf("email = %q, want root@example.com", u.Email)
	}
	if !u.CheckPassword("hunter2") {
		t.Error("CheckPassword failed for created superuser")
	}
}

func TestCreatesuperuserNoInput(t *testing.T) {
	app := newSuperuserApp(t)
	var out bytes.Buffer
	app.Out = &out
	app.Commands.Out = &out
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "envpass")

	err := app.Execute([]string{"createsuperuser", "--username", "envroot", "--email", "e@example.com", "--noinput"})
	if err != nil {
		t.Fatalf("createsuperuser --noinput: %v", err)
	}

	u, err := orm.Query[auth.User](app.DB).Get(context.Background(), "username", "envroot")
	if err != nil {
		t.Fatalf("loading created superuser: %v", err)
	}
	if !u.CheckPassword("envpass") {
		t.Error("CheckPassword failed for --noinput superuser")
	}
}

func TestCreatesuperuserNoInputRequiresPassword(t *testing.T) {
	app := newSuperuserApp(t)
	var out bytes.Buffer
	app.Out = &out
	app.Commands.Out = &out
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "")

	err := app.Execute([]string{"createsuperuser", "--username", "x", "--noinput"})
	if err == nil {
		t.Error("expected an error when DJANGO_SUPERUSER_PASSWORD is unset under --noinput")
	}
}

func TestCreatesuperuserRequiresDB(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.Execute([]string{"createsuperuser", "--username", "x", "--noinput"}); err == nil {
		t.Error("expected an error when no database is configured")
	}
}
