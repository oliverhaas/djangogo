package scaffold

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedProjectBuilds generates a project pointed at the local
// repository through a replace directive, adds a second app as startapp
// does, and checks that go mod tidy, go build, go vet and the tests of the
// generated project succeed. It is skipped under -short because it runs the
// Go toolchain.
func TestGeneratedProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the generated-project build under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}

	dir := t.TempDir()
	if err := Project(dir, "myproj", Options{ReplacePath: repoRoot(t)}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	if err := App(filepath.Join(dir, "shop"), "shop", Options{}); err != nil {
		t.Fatalf("App: %v", err)
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
	run("go", "vet", "./...")
	run("go", "test", "./...")
}
