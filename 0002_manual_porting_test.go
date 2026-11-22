package djangogo_test

import (
	"context"

	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// This file stands in for the migration modules of Django's
// test_migrations_manual_porting. Its name starts with a digit, as the name of
// a migration file does, so squashmigrations asks to copy the functions
// defined here.

// ManualPortingForwards is the data function of the fixture.
func ManualPortingForwards(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }
