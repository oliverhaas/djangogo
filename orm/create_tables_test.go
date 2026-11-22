package orm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// cycleTeam and cyclePlayer reference each other, so neither table can be
// created with all of its foreign keys before the other one exists.
type cycleTeam struct {
	ID      int64
	Name    string              `orm:"max_length=50"`
	Captain orm.FK[cyclePlayer] `orm:"null"`
}

type cyclePlayer struct {
	ID   int64
	Name string `orm:"max_length=50"`
	Team orm.FK[cycleTeam]
}

// TestCreateTablesMutualForeignKeys creates two tables that reference each
// other, and both foreign keys are enforced afterwards.
//
// No Django test exists: Django's schema editor adds the foreign keys of a
// PostgreSQL table after the tables are created (deferred_sql), which every
// test database with such models relies on.
func TestCreateTablesMutualForeignKeys(t *testing.T) {
	djangotest.EachDB(t, []any{&cycleTeam{}, &cyclePlayer{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		team := cycleTeam{Name: "Rovers"}
		if err := orm.Query[cycleTeam](db).Create(ctx, &team); err != nil {
			t.Fatal(err)
		}
		player := cyclePlayer{Name: "Ada"}
		player.Team.SetPK(team.ID)
		if err := orm.Query[cyclePlayer](db).Create(ctx, &player); err != nil {
			t.Fatal(err)
		}
		if _, err := orm.Query[cycleTeam](db).Filter("id", team.ID).Update(ctx, "captain", player.ID); err != nil {
			t.Fatal(err)
		}
		got, err := orm.Query[cycleTeam](db).Get(ctx, "captain__name", "Ada")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got.ID, team.ID)

		_, err = orm.Query[cycleTeam](db).Filter("id", team.ID).Update(ctx, "captain", player.ID+100)
		if !errors.Is(err, orm.ErrForeignKeyViolation) {
			t.Errorf("a captain that does not exist: %v, want a foreign key violation", err)
		}
	})
}
