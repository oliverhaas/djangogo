package migrations

// Migration is one ordered set of operations for an app, with linear dependencies.
type Migration struct {
	App          string
	Name         string   // e.g. "0001_initial"
	Dependencies []string // names of prior migrations in the same app
	Operations   []Operation
}

// StateFromMigrations replays every operation of migs (in slice order) onto an empty
// ProjectState and returns the resulting cumulative state.
func StateFromMigrations(migs []Migration) *ProjectState {
	ps := NewProjectState()
	for _, mig := range migs {
		for _, op := range mig.Operations {
			op.Apply(ps)
		}
	}
	return ps
}
