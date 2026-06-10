package scaffold

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the djangogo repository root, derived
// from this test file's location (scaffold/ is one level below the root).
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Dir(wd)
}

// parseGo asserts that path parses as syntactically valid Go.
func parseGo(t *testing.T, path string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors); err != nil {
		t.Errorf("parse %s: %v", path, err)
	}
}

func TestProjectWritesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	root := repoRoot(t)
	if err := Project(dir, "myproj", root); err != nil {
		t.Fatalf("Project: %v", err)
	}

	goFiles := []string{
		"settings.go",
		"main.go",
		filepath.Join("myprojapp", "app.go"),
		filepath.Join("myprojapp", "models.go"),
		filepath.Join("myprojapp", "admin.go"),
		filepath.Join("myprojapp", "urls.go"),
	}
	for _, rel := range goFiles {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", rel, err)
			continue
		}
		parseGo(t, path)
	}

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	src := string(gomod)
	if !strings.Contains(src, "module myproj") {
		t.Errorf("go.mod missing module line:\n%s", src)
	}
	if !strings.Contains(src, "go 1.26") {
		t.Errorf("go.mod missing go version:\n%s", src)
	}
	if !strings.Contains(src, "require github.com/oliverhaas/djangogo") {
		t.Errorf("go.mod missing require:\n%s", src)
	}
	wantReplace := "replace github.com/oliverhaas/djangogo => " + root
	if !strings.Contains(src, wantReplace) {
		t.Errorf("go.mod missing replace %q:\n%s", wantReplace, src)
	}
}

func TestProjectMainWiresApp(t *testing.T) {
	dir := t.TempDir()
	if err := Project(dir, "myproj", ""); err != nil {
		t.Fatalf("Project: %v", err)
	}
	main, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(main)
	if !strings.Contains(src, `"myproj/myprojapp"`) {
		t.Errorf("main.go missing app import:\n%s", src)
	}
	if !strings.Contains(src, "&myprojapp.App{}") {
		t.Errorf("main.go missing app wiring:\n%s", src)
	}
	if !strings.Contains(src, "auth.App{}") {
		t.Errorf("main.go missing auth app wiring (admin needs it):\n%s", src)
	}
	// Without a replace path, no replace directive is emitted.
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if strings.Contains(string(gomod), "replace ") {
		t.Errorf("go.mod should not contain a replace directive:\n%s", gomod)
	}
}

func TestAppWritesPackageFiles(t *testing.T) {
	dir := t.TempDir()
	if err := App(dir, "blog", ""); err != nil {
		t.Fatalf("App: %v", err)
	}
	for _, rel := range []string{"app.go", "models.go", "admin.go", "urls.go"} {
		path := filepath.Join(dir, "blogapp", rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", rel, err)
			continue
		}
		parseGo(t, path)
	}
}

func TestAppNameAlreadyEndingInApp(t *testing.T) {
	dir := t.TempDir()
	if err := App(dir, "blogapp", ""); err != nil {
		t.Fatalf("App: %v", err)
	}
	// The package directory must not double the "app" suffix.
	if _, err := os.Stat(filepath.Join(dir, "blogapp", "app.go")); err != nil {
		t.Errorf("expected blogapp/app.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blogappapp")); err == nil {
		t.Error("package directory should not double the app suffix")
	}
}
