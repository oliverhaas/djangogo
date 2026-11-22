package migrations_test

import (
	"context"

	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// This file stands in for the migration module 0002_second of Django's
// test_migrations_manual_porting. Its name starts with a digit, as the name
// of a migration file does, so the writer asks to copy the functions defined
// here.

// portingForwards is the data function of the fixture.
func portingForwards(context.Context, *orm.DB, *migrations.ProjectState) error { return nil }
