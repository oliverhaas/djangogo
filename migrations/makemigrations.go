package migrations

import "fmt"

// MakeMigrations diffs the prior migrations' cumulative state against current and, if
// there are changes, builds the next Migration for app (number = len(prior)+1, name
// "NNNN_initial" for the first else "NNNN_auto", Dependencies = the previous migration
// name if any). When dir != "", it also writes the Go file (package pkgName) and
// returns its path. Returns ErrNoChanges when there is nothing to do.
func MakeMigrations(app string, current *ProjectState, prior []Migration, dir, pkgName string) (mig *Migration, path string, err error) {
	priorState := StateFromMigrations(prior)
	ops := Diff(priorState, current)
	if len(ops) == 0 {
		return nil, "", ErrNoChanges
	}

	n := len(prior) + 1
	label := "auto"
	if n == 1 {
		label = "initial"
	}
	name := fmt.Sprintf("%04d_%s", n, label)

	var deps []string
	if len(prior) > 0 {
		deps = []string{prior[len(prior)-1].Name}
	}

	m := Migration{
		App:          app,
		Name:         name,
		Dependencies: deps,
		Operations:   ops,
	}

	if dir != "" {
		path, err = WriteMigration(dir, pkgName, m)
		if err != nil {
			return nil, "", err
		}
	}

	return &m, path, nil
}
