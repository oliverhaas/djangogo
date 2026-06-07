package scaffold

import (
	"os/exec"
	"testing"
)

// TestGeneratedProjectBuilds is the exit-criterion test: it generates a project
// pointed at the local repo via a replace directive, runs go mod tidy, then go
// build ./..., and asserts both succeed. This proves startproject produces a
// runnable project. It is skipped under -short because it invokes the Go
// toolchain and resolves modules.
func TestGeneratedProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping generated-project build under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}

	dir := t.TempDir()
	root := repoRoot(t)
	if err := Project(dir, "myproj", root); err != nil {
		t.Fatalf("Project: %v", err)
	}

	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
		}
	}

	run("go", "mod", "tidy")
	run("go", "build", "./...")
	// vet catches issues a build alone may miss (e.g. printf misuse).
	run("go", "vet", "./...")
}
