package migrations

import "errors"

// ErrNoChanges is returned by MakeMigrations when the models match the prior state.
var ErrNoChanges = errors.New("migrations: no changes detected")
