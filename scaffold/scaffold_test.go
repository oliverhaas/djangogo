package scaffold

import (
	"errors"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the djangogo repository root, derived
// from the directory of the test (scaffold/ is one level below the root).
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Dir(wd)
}

// readFile returns the content of the file rel under dir.
func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// listFiles returns the files under dir, relative and with slashes, sorted.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	slices.Sort(files)
	return files
}

// checkGoFiles checks that every Go file under dir parses and is gofmt-clean.
func checkGoFiles(t *testing.T, dir string) {
	t.Helper()
	for _, rel := range listFiles(t, dir) {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		src := readFile(t, dir, rel)
		if _, err := parser.ParseFile(token.NewFileSet(), rel, src, parser.AllErrors); err != nil {
			t.Errorf("parse %s: %v", rel, err)
			continue
		}
		formatted, err := format.Source([]byte(src))
		if err != nil || string(formatted) != src {
			t.Errorf("%s is not gofmt-formatted (err %v)", rel, err)
		}
	}
}

// appFiles are the files of an app, relative to its directory.
var appFiles = []string{
	"admin.go",
	"app.go",
	"models.go",
	"static/blog/style.css",
	"templates/blog/base.html",
	"templates/blog/post_detail.html",
	"templates/blog/post_list.html",
	"urls.go",
	"views.go",
	"views_test.go",
}

func TestProjectWritesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Project(dir, "myproj", Options{}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	want := make([]string, 0, 4+len(appFiles))
	want = append(want, "go.mod", "main.go", "migrations/myprojapp/doc.go", "settings.go")
	for _, f := range appFiles {
		want = append(want, "myprojapp/"+strings.ReplaceAll(f, "blog", "myprojapp"))
	}
	slices.Sort(want)
	if got := listFiles(t, dir); !slices.Equal(got, want) {
		t.Errorf("files:\n got %q\nwant %q", got, want)
	}
	checkGoFiles(t, dir)
}

func TestProjectGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := Project(dir, "myproj", Options{}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	// Without a replace path "go mod tidy" adds the latest release.
	if got, want := readFile(t, dir, "go.mod"), "module myproj\n\ngo "+goVersion+"\n"; got != want {
		t.Errorf("go.mod = %q, want %q", got, want)
	}

	dir = t.TempDir()
	root := repoRoot(t)
	if err := Project(dir, "myproj", Options{ReplacePath: root}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	src := readFile(t, dir, "go.mod")
	for _, want := range []string{
		"module myproj\n",
		"require github.com/oliverhaas/djangogo v0.0.0-00010101000000-000000000000\n",
		"replace github.com/oliverhaas/djangogo => " + root + "\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("go.mod lacks %q:\n%s", want, src)
		}
	}
}

func TestProjectMainWiresAppAndMigrations(t *testing.T) {
	dir := t.TempDir()
	if err := Project(dir, "myproj", Options{}); err != nil {
		t.Fatalf("Project: %v", err)
	}
	src := readFile(t, dir, "main.go")
	for _, want := range []string{
		`"myproj/myprojapp"`,
		`_ "myproj/migrations/myprojapp"`,
		"&myprojapp.App{}",
		"auth.App{}",
		"settings()",
		"defer func() { _ = app.Close() }()",
		"return app.Run(os.Args[1:])",
		"os.Exit(run())",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %q:\n%s", want, src)
		}
	}
	if got := readFile(t, dir, "migrations/myprojapp/doc.go"); !strings.Contains(got, "\npackage myprojapp\n") {
		t.Errorf("the migrations package is not named after the app label:\n%s", got)
	}
}

// TestProjectSettings starts from Django's defaults, applies the environment
// and writes a new insecure secret key for every project, as Django's
// startproject does.
func TestProjectSettings(t *testing.T) {
	keyRe := regexp.MustCompile(`s\.SecretKey = "(django-insecure-[a-z0-9!@#$%^&*(\-_=+)]{50})"`)
	keys := make([]string, 0, 2)
	for range 2 {
		dir := t.TempDir()
		if err := Project(dir, "myproj", Options{}); err != nil {
			t.Fatalf("Project: %v", err)
		}
		src := readFile(t, dir, "settings.go")
		for _, want := range []string{"conf.Defaults()", "conf.LoadDotEnv()", `s.ApplyEnv("DJANGOGO_")`, "s.Debug = true"} {
			if !strings.Contains(src, want) {
				t.Errorf("settings.go lacks %q:\n%s", want, src)
			}
		}
		m := keyRe.FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("settings.go has no generated secret key:\n%s", src)
		}
		keys = append(keys, m[1])
	}
	if keys[0] == keys[1] {
		t.Errorf("two projects got the same secret key %q", keys[0])
	}
}

func TestAppWritesPackageFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blog")
	if err := App(dir, "blog", Options{}); err != nil {
		t.Fatalf("App: %v", err)
	}
	if got := listFiles(t, dir); !slices.Equal(got, appFiles) {
		t.Errorf("files:\n got %q\nwant %q", got, appFiles)
	}
	checkGoFiles(t, dir)
	for rel, want := range map[string]string{
		"app.go":                          `func (*App) Name() string { return "blog" }`,
		"urls.go":                         `urls.Include("blog/", "blog",`,
		"views.go":                        `Template:    "blog/post_list.html",`,
		"views_test.go":                   `Get("/blog/")`,
		"templates/blog/post_list.html":   `{% url 'blog:post_detail' post.id %}">{{ post }}</a>`,
		"templates/blog/post_detail.html": `{% extends "blog/base.html" %}`,
		"templates/blog/base.html":        `{% static 'blog/style.css' %}`,
	} {
		if src := readFile(t, dir, rel); !strings.Contains(src, want) {
			t.Errorf("%s lacks %q:\n%s", rel, want, src)
		}
	}
}

// TestCreatedReportsEveryFile calls Options.Created once per file written, in
// the order written.
func TestCreatedReportsEveryFile(t *testing.T) {
	dir := t.TempDir()
	var created []string
	if err := App(dir, "blog", Options{Created: func(p string) { created = append(created, p) }}); err != nil {
		t.Fatalf("App: %v", err)
	}
	want := make([]string, 0, len(appFiles))
	for _, f := range appFiles {
		want = append(want, filepath.Join(dir, filepath.FromSlash(f)))
	}
	if !slices.Equal(created, want) {
		t.Errorf("created:\n got %q\nwant %q", created, want)
	}
}

// TestExistingFileStopsGeneration reports the first file that exists and
// writes nothing, so an existing file is never replaced.
func TestExistingFileStopsGeneration(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "models.go")
	if err := os.WriteFile(existing, []byte("package blog\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := App(dir, "blog", Options{})
	var exists *ExistsError
	if !errors.As(err, &exists) || exists.Path != existing {
		t.Fatalf("App = %v, want an ExistsError for %s", err, existing)
	}
	if got := listFiles(t, dir); !slices.Equal(got, []string{"models.go"}) {
		t.Errorf("files after the failed generation: %q", got)
	}
	if got := readFile(t, dir, "models.go"); got != "package blog\n" {
		t.Errorf("models.go was replaced: %q", got)
	}
}
