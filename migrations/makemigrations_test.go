package migrations

import (
	"errors"
	"os"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// personOrmRegistry returns an orm.Registry containing a Person model.
func personOrmRegistry(t *testing.T) *orm.Registry {
	t.Helper()
	type Person struct {
		ID   int    `orm:"pk"`
		Name string `orm:"max_length=100"`
		Age  int
	}
	r := orm.NewRegistry()
	if _, err := r.Register(&Person{}); err != nil {
		t.Fatalf("orm.Registry.Register: %v", err)
	}
	return r
}

func TestMakeMigrations_Initial(t *testing.T) {
	t.Parallel()

	r := personOrmRegistry(t)
	current := StateFromRegistry(r)

	dir := t.TempDir()
	mig, path, err := MakeMigrations("myapp", current, nil, dir, "myapp0001")
	if err != nil {
		t.Fatalf("MakeMigrations error: %v", err)
	}
	if mig == nil {
		t.Fatal("expected non-nil Migration")
	}
	if mig.Name != "0001_initial" {
		t.Errorf("Name = %q, want %q", mig.Name, "0001_initial")
	}
	if mig.App != "myapp" {
		t.Errorf("App = %q, want %q", mig.App, "myapp")
	}
	if len(mig.Dependencies) != 0 {
		t.Errorf("Dependencies = %v, want empty", mig.Dependencies)
	}
	if len(mig.Operations) != 1 {
		t.Errorf("Operations len = %d, want 1", len(mig.Operations))
	} else if _, ok := mig.Operations[0].(CreateModel); !ok {
		t.Errorf("Operations[0] is %T, want CreateModel", mig.Operations[0])
	}
	if path == "" {
		t.Error("expected non-empty file path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found at path %q: %v", path, err)
	}
}

func TestMakeMigrations_NoChanges(t *testing.T) {
	t.Parallel()

	r := personOrmRegistry(t)
	current := StateFromRegistry(r)

	// Generate the first migration.
	mig1, _, err := MakeMigrations("myapp", current, nil, "", "")
	if err != nil {
		t.Fatalf("first MakeMigrations error: %v", err)
	}

	// Running again with the same state should yield ErrNoChanges.
	_, _, err = MakeMigrations("myapp", current, []Migration{*mig1}, "", "")
	if !errors.Is(err, ErrNoChanges) {
		t.Errorf("err = %v, want ErrNoChanges", err)
	}
}

func TestMakeMigrations_Incremental(t *testing.T) {
	t.Parallel()

	// Build a current state with Person (ID, Name, Age).
	r := personOrmRegistry(t)
	current1 := StateFromRegistry(r)

	mig1, _, err := MakeMigrations("myapp", current1, nil, "", "")
	if err != nil {
		t.Fatalf("first MakeMigrations: %v", err)
	}

	// Build a new current state with Person + Bio field added.
	current2 := current1.Clone()
	personModel := current2.Models["Person"]
	personModel.Fields = append(personModel.Fields, FieldState{
		Name: "Bio", Column: "bio", Kind: orm.KindText, Null: true,
	})

	mig2, _, err := MakeMigrations("myapp", current2, []Migration{*mig1}, "", "")
	if err != nil {
		t.Fatalf("second MakeMigrations: %v", err)
	}
	if mig2 == nil {
		t.Fatal("expected non-nil Migration")
	}
	if mig2.Name != "0002_auto" {
		t.Errorf("Name = %q, want %q", mig2.Name, "0002_auto")
	}
	if len(mig2.Dependencies) != 1 || mig2.Dependencies[0] != "0001_initial" {
		t.Errorf("Dependencies = %v, want [\"0001_initial\"]", mig2.Dependencies)
	}
	if len(mig2.Operations) != 1 {
		t.Errorf("Operations len = %d, want 1", len(mig2.Operations))
	} else if _, ok := mig2.Operations[0].(AddField); !ok {
		t.Errorf("Operations[0] is %T, want AddField", mig2.Operations[0])
	}
}
