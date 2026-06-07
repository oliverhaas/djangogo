package migrations

import (
	"context"
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// Apply runs every migration in migs that is not yet recorded as applied, in slice
// order, each inside its own transaction (schema changes plus the tracking-row insert
// are atomic). It returns the keys ("<app>/<name>") that were applied this call.
func Apply(ctx context.Context, db *orm.DB, migs []Migration) ([]string, error) {
	if err := EnsureTable(ctx, db); err != nil {
		return nil, err
	}
	applied, err := AppliedSet(ctx, db)
	if err != nil {
		return nil, err
	}

	d := db.Dialect()
	ps := NewProjectState()
	var done []string

	for _, mig := range migs {
		key := mig.App + "/" + mig.Name
		if applied[key] {
			// Already applied: advance the state without executing any DDL so later
			// pending ops compute their SQL against the correct schema.
			for _, op := range mig.Operations {
				op.Apply(ps)
			}
			continue
		}

		tx, err := db.SQL().BeginTx(ctx, nil)
		if err != nil {
			return done, err
		}
		for _, op := range mig.Operations {
			stmts, err := op.SQL(d, ps)
			if err != nil {
				_ = tx.Rollback()
				return done, fmt.Errorf("migrations: %s/%s: %s: %w", mig.App, mig.Name, op.Describe(), err)
			}
			for _, s := range stmts {
				if _, err := tx.ExecContext(ctx, s); err != nil {
					_ = tx.Rollback()
					return done, fmt.Errorf("migrations: %s/%s: exec %q: %w", mig.App, mig.Name, s, err)
				}
			}
			op.Apply(ps)
		}
		if err := recordApplied(ctx, tx, mig.App, mig.Name); err != nil {
			_ = tx.Rollback()
			return done, err
		}
		if err := tx.Commit(); err != nil {
			return done, err
		}
		done = append(done, key)
	}
	return done, nil
}
