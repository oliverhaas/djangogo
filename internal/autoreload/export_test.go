package autoreload

// ChildCommand exposes Supervisor.command to the tests.
var ChildCommand = (*Supervisor).command

// GoFiles exposes goFiles to the tests.
var GoFiles = goFiles

// PollSources returns the check of the supervisor for .go files added to or
// removed from the package directories dirs since its last call.
func PollSources(dirs ...string) func() []string {
	w := &sources{dirs: dirs}
	return w.poll
}
