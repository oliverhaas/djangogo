package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// TestM2MRegistrationAndMetadata covers M2M metadata accessors, db_table=,
// cross-app same-name through column naming (#11226), and registration errors.
func TestM2MRegistrationAndMetadata(t *testing.T) {
	type customTag struct {
		ID   int64
		Name string `orm:"max_length=20"`
	}
	type customPost struct {
		ID   int64
		Tags orm.M2M[customTag] `orm:"db_table=custom_post_tags"`
	}

	djangotest.EachDB(t, []any{djangotest.App("blog", &customTag{}, &customPost{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		postM, ok := db.Registry().ModelOf(customPost{})
		if !ok {
			t.Fatal("customPost not registered")
		}
		m2ms := postM.ManyToMany()
		djangotest.AssertEqual(t, len(m2ms), 1)
		f, ok := postM.ManyToManyField("tags")
		if !ok {
			t.Fatal("ManyToManyField(tags) not found")
		}
		djangotest.AssertEqual(t, f.Rel.Kind, orm.RelM2M)
		djangotest.AssertEqual(t, f.Rel.AutoCreated, true)
		djangotest.AssertEqual(t, f.Rel.Through.AutoCreated(), true)
		djangotest.AssertEqual(t, f.Rel.Through.Table(), "custom_post_tags")
		djangotest.AssertTableExists(t, db, "custom_post_tags")

		// Dynamic helpers ManyValues / SetManyPKs for forms and admin.
		t1 := create(t, db, &customTag{Name: "go"})
		t2 := create(t, db, &customTag{Name: "django"})
		p := create(t, db, &customPost{})
		if err := orm.SetManyPKs(ctx, db, p, f, []any{t1.ID, t2.ID}); err != nil {
			t.Fatalf("SetManyPKs: %v", err)
		}
		pks, err := orm.ManyValues(ctx, db, p, f)
		if err != nil {
			t.Fatalf("ManyValues: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pks, []any{t1.ID, t2.ID}, djangotest.Unordered())
	})
}

// TestM2MValidationErrors verifies that invalid M2M tags and through models
// are rejected during registration or Resolve.
func TestM2MValidationErrors(t *testing.T) {
	type target struct {
		ID int64
	}
	type badSym struct {
		ID   int64
		Link orm.M2M[target] `orm:"symmetrical=true"`
	}
	type badBoth struct {
		ID   int64
		Link orm.M2M[target] `orm:"through=something;db_table=custom"`
	}
	type badTag struct {
		ID   int64
		Link orm.M2M[target] `orm:"on_delete=cascade"`
	}
	type missingThrough struct {
		ID   int64
		Link orm.M2M[target] `orm:"through=NoSuchModel"`
	}

	// Reject invalid tag option at Register time.
	r0 := orm.NewRegistry()
	if _, err := r0.Register(&badTag{}); err == nil {
		t.Error("expected error for on_delete= on M2M")
	}

	// Reject symmetrical=true on non-self M2M at Resolve time.
	r1 := orm.NewRegistry()
	_, _ = r1.Register(&target{})
	_, _ = r1.Register(&badSym{})
	if err := r1.Resolve(); err == nil || !strings.Contains(err.Error(), "symmetrical=true") {
		t.Errorf("Resolve badSym = %v, want symmetrical error", err)
	}

	// Reject through= combined with db_table=.
	r2 := orm.NewRegistry()
	_, _ = r2.Register(&target{})
	_, _ = r2.Register(&badBoth{})
	if err := r2.Resolve(); err == nil || !strings.Contains(err.Error(), "both through and db_table") {
		t.Errorf("Resolve badBoth = %v, want through+db_table error", err)
	}

	// Reject unregistered through= model.
	r3 := orm.NewRegistry()
	_, _ = r3.Register(&target{})
	_, _ = r3.Register(&missingThrough{})
	if err := r3.Resolve(); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("Resolve missingThrough = %v, want unregistered through error", err)
	}
}

// TestM2MSignalThroughModelAndRollback verifies OnM2MChangedThrough,
// OnM2MChangedModel, and transaction rollback when a signal handler fails.
func TestM2MSignalThroughModelAndRollback(t *testing.T) {
	djangotest.EachDB(t, m2mtModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mtSetUp(t, db)

		var actions []orm.M2MAction
		cancelThrough := orm.OnM2MChangedThrough[m2mtMembership](func(_ context.Context, ev orm.M2MChangedEvent) error {
			actions = append(actions, ev.Action)
			return nil
		})
		defer cancelThrough()

		if err := orm.Many[m2mtPerson](db, f.rock, "members").Add(ctx, f.bob); err != nil {
			t.Fatalf("Add: %v", err)
		}
		djangotest.AssertSequenceEqual(t, actions, []orm.M2MAction{orm.M2MPreAdd, orm.M2MPostAdd})

		// Returning an error from post_add rolls back the insert.
		sentinel := errors.New("abort m2m add")
		groupM, _ := db.Registry().ModelOf(m2mtGroup{})
		membersField, _ := groupM.ManyToManyField("members")
		cancelModel := orm.OnM2MChangedModel(membersField.Rel.Through, func(_ context.Context, ev orm.M2MChangedEvent) error {
			if ev.Action == orm.M2MPostAdd {
				return sentinel
			}
			return nil
		})
		defer cancelModel()

		err := orm.Many[m2mtPerson](db, f.rock, "members").Add(ctx, f.jane)
		if !errors.Is(err, sentinel) {
			t.Fatalf("Add error = %v, want sentinel", err)
		}
		members, err := orm.Many[m2mtPerson](db, f.rock, "members").All(ctx)
		if err != nil {
			t.Fatalf("All after rollback: %v", err)
		}
		djangotest.AssertSequenceEqual(t, personNames(members), []string{"Bob"})
	})
}
