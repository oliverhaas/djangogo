package migrations_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// appliedFor returns the applied migrations of one app, in order.
func appliedFor(t *testing.T, r *migrations.Recorder, app string) []migrations.Key {
	t.Helper()
	applied, err := r.AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrations: %v", err)
	}
	var out []migrations.Key
	for key := range applied {
		if key.App == app {
			out = append(out, key)
		}
	}
	slices.SortFunc(out, func(a, b migrations.Key) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// fieldNames returns the field names of a model in the state, in order.
func fieldNames(ms *migrations.ModelState) []string {
	names := make([]string, 0, len(ms.Fields))
	for _, f := range ms.Fields {
		names = append(names, f.Name)
	}
	return names
}

// Django checks that a second configured database is unaffected. The Go port
// uses a second, independent test database for that.
//
// Ported from Django: tests/migrations/test_loader.py::RecorderTests.test_apply
func TestApply(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		recorder := migrations.NewRecorder(db)
		djangotest.AssertEqual(t, len(appliedFor(t, recorder, "myapp")), 0)
		if err := recorder.RecordApplied(ctx, k("myapp", "0432_ponies")); err != nil {
			t.Fatalf("RecordApplied: %v", err)
		}
		djangotest.AssertSequenceEqual(t, appliedFor(t, recorder, "myapp"), []migrations.Key{k("myapp", "0432_ponies")})
		// That does not affect the records of another database.
		other := migrations.NewRecorder(djangotest.NewDB(t))
		djangotest.AssertEqual(t, len(appliedFor(t, other, "myapp")), 0)
		if err := recorder.RecordUnapplied(ctx, k("myapp", "0432_ponies")); err != nil {
			t.Fatalf("RecordUnapplied: %v", err)
		}
		djangotest.AssertEqual(t, len(appliedFor(t, recorder, "myapp")), 0)
	})
}

// Django also checks that unmigrated apps are among the "real apps" of the
// state. A Go project state holds no model classes, so there is no such list.
//
// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_load
func TestLoad(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, testMigrations())
		djangotest.AssertSequenceEqual(t, plan(t, l.Graph.ForwardsPlan, k("migrations", "0002_second")), []migrations.Key{
			k("migrations", "0001_initial"),
			k("migrations", "0002_second"),
		})
		// Now render it out.
		state, err := l.ProjectState([]migrations.Key{k("migrations", "0002_second")}, true)
		if err != nil {
			t.Fatalf("ProjectState: %v", err)
		}
		djangotest.AssertEqual(t, len(state.Models), 2)
		djangotest.AssertSequenceEqual(t, fieldNames(modelOf(t, state, "migrations", "Author")),
			[]string{"ID", "Name", "Slug", "Age", "Rating"})
		djangotest.AssertSequenceEqual(t, fieldNames(modelOf(t, state, "migrations", "Book")),
			[]string{"ID", "Author"})
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_plan_handles_repeated_migrations
func TestPlanHandlesRepeatedMigrations(t *testing.T) {
	l := newLoader(t, nil, testMigrations(), testMigrations2())
	nodes := []migrations.Key{k("migrations", "0002_second"), k("migrations2", "0001_initial")}
	got, err := l.Graph.GeneratePlan(nodes, true)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	djangotest.AssertSequenceEqual(t, got, []migrations.Key{
		k("migrations", "0001_initial"),
		k("migrations", "0002_second"),
		k("migrations2", "0001_initial"),
	})
}

// The Django fixture depends on ("auth", "__first__"), and the auth app in turn
// on contenttypes. The Go port defines both apps as small fixtures.
//
// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_load_unmigrated_dependency
func TestLoadUnmigratedDependency(t *testing.T) {
	user := fkField("User", "user_id", "auth.User")
	user.Null = true
	user.RelOnDelete = orm.OnDeleteSetNull
	l := newLoader(t, nil, []migrations.Migration{
		createOnly("contenttypes", "0001_initial", "ContentType", nil),
		createOnly("auth", "0001_initial", "User", []migrations.Key{k("contenttypes", migrations.FirstMigration)}),
		createOnly("migrations", "0001_initial", "Book", []migrations.Key{k("auth", migrations.FirstMigration)}, user),
	})
	djangotest.AssertSequenceEqual(t, plan(t, l.Graph.ForwardsPlan, k("migrations", "0001_initial")), []migrations.Key{
		k("contenttypes", "0001_initial"),
		k("auth", "0001_initial"),
		k("migrations", "0001_initial"),
	})
	// Now render it out.
	state, err := l.ProjectState([]migrations.Key{k("migrations", "0001_initial")}, true)
	if err != nil {
		t.Fatalf("ProjectState: %v", err)
	}
	own := 0
	for _, ms := range state.Models {
		if ms.App == "migrations" {
			own++
		}
	}
	djangotest.AssertEqual(t, own, 1)
	djangotest.AssertSequenceEqual(t, fieldNames(modelOf(t, state, "migrations", "Book")), []string{"ID", "User"})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_run_before
func TestRunBefore(t *testing.T) {
	author := fkField("Author", "author_id", "migrations.Author")
	author.Null = true
	author.RelOnDelete = orm.OnDeleteSetNull
	third := createOnly("migrations", "0003_third", "Author", []migrations.Key{k("migrations", "0001_initial")})
	third.RunBefore = []migrations.Key{k("migrations", "0002_second")}
	l := newLoader(t, nil, []migrations.Migration{
		createOnly("migrations", "0001_initial", "Salamander", nil),
		createOnly("migrations", "0002_second", "Book", []migrations.Key{k("migrations", "0001_initial")}, author),
		third,
	})
	djangotest.AssertSequenceEqual(t, plan(t, l.Graph.ForwardsPlan, k("migrations", "0002_second")), []migrations.Key{
		k("migrations", "0001_initial"),
		k("migrations", "0003_third"),
		k("migrations", "0002_second"),
	})
}

// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_first
func TestFirst(t *testing.T) {
	l := newLoader(t, nil, []migrations.Migration{
		createOnly("migrations", "thefirst", "Author", nil),
		createOnly("migrations", "second", "Book",
			[]migrations.Key{k("migrations", "thefirst"), k("migrations2", "0002_second")}),
		createOnly("migrations2", "0001_initial", "OtherAuthor",
			[]migrations.Key{k("migrations", migrations.FirstMigration)}),
		createOnly("migrations2", "0002_second", "Bookstore", []migrations.Key{k("migrations2", "0001_initial")}),
	})
	djangotest.AssertSequenceEqual(t, plan(t, l.Graph.ForwardsPlan, k("migrations", "second")), []migrations.Key{
		k("migrations", "thefirst"),
		k("migrations2", "0001_initial"),
		k("migrations2", "0002_second"),
		k("migrations", "second"),
	})
}

// Django raises a KeyError for an unknown prefix. Go returns an error that
// wraps ErrMigrationNotFound and carries Django's message.
//
// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_name_match
func TestNameMatch(t *testing.T) {
	l := newLoader(t, nil, testMigrations())
	m, err := l.MigrationByPrefix("migrations", "0001")
	if err != nil {
		t.Fatalf("MigrationByPrefix: %v", err)
	}
	djangotest.AssertEqual(t, m.Name, "0001_initial")

	_, err = l.MigrationByPrefix("migrations", "0")
	var ambiguous *migrations.AmbiguityError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want AmbiguityError", err)
	}
	djangotest.AssertEqual(t, ambiguous.Error(), "There is more than one migration for 'migrations' with the prefix '0'")

	_, err = l.MigrationByPrefix("migrations", "blarg")
	djangotest.AssertErrorIs(t, err, migrations.ErrMigrationNotFound)
	if !strings.Contains(err.Error(), "There is no migration for 'migrations' with the prefix 'blarg'") {
		t.Errorf("error = %q, want Django's message in it", err)
	}
}

// A Go loader reads the applied migrations when it is built, so the port builds
// a second loader after recording the migration.
//
// Ported from Django: tests/migrations/test_loader.py::LoaderTests.test_check_consistent_history
func TestCheckConsistentHistory(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		l := newLoader(t, db, testMigrations())
		if err := l.CheckConsistentHistory(); err != nil {
			t.Fatalf("CheckConsistentHistory on an empty database: %v", err)
		}
		if err := migrations.NewRecorder(db).RecordApplied(context.Background(), k("migrations", "0002_second")); err != nil {
			t.Fatalf("RecordApplied: %v", err)
		}
		err := newLoader(t, db, testMigrations()).CheckConsistentHistory()
		var inconsistent *migrations.InconsistentMigrationHistoryError
		if !errors.As(err, &inconsistent) {
			t.Fatalf("error = %v, want InconsistentMigrationHistoryError", err)
		}
		djangotest.AssertEqual(t, inconsistent.Error(),
			"Migration migrations.0002_second is applied before its dependency "+
				"migrations.0001_initial on database 'default'.")
	})
}
