// Package scaffold generates runnable Djan-Go-Go project and app skeletons from
// Go templates, the engine behind the startproject and startapp commands.
package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// data carries the values shared by every template.
type data struct {
	// Name is the project or app name supplied by the user.
	Name string
	// AppName is the generated app package name (Name + "app" for a project).
	AppName string
	// Module is the generated project module path (equal to Name for a project).
	Module string
	// ReplacePath, when non-empty, points the generated go.mod at a local
	// checkout of github.com/oliverhaas/djangogo via a replace directive.
	ReplacePath string
	// GoVersion is the Go language version written into go.mod.
	GoVersion string
}

// goVersion is the Go version written into generated go.mod files.
const goVersion = "1.26"

// file pairs an output path (relative to the target directory) with its template.
type file struct {
	path string
	tmpl string
}

// Project writes a runnable project skeleton named name into dir. When replacePath
// is non-empty, the generated go.mod adds a replace github.com/oliverhaas/djangogo
// => <replacePath> directive so it builds against a local checkout without
// publishing.
func Project(dir, name, replacePath string) error {
	d := data{
		Name:        name,
		AppName:     name + "app",
		Module:      name,
		ReplacePath: replacePath,
		GoVersion:   goVersion,
	}
	files := []file{
		{path: "go.mod", tmpl: projectGoMod},
		{path: "settings.go", tmpl: projectSettings},
		{path: "main.go", tmpl: projectMain},
		{path: filepath.Join(d.AppName, "app.go"), tmpl: appConfig},
		{path: filepath.Join(d.AppName, "models.go"), tmpl: appModels},
		{path: filepath.Join(d.AppName, "admin.go"), tmpl: appAdmin},
		{path: filepath.Join(d.AppName, "urls.go"), tmpl: appURLs},
	}
	return render(dir, d, files)
}

// App writes an app package skeleton named name into dir/<name>app. When name
// already ends in "app" the directory is dir/<name> instead, so the package name
// never doubles the suffix. replacePath is unused because App adds files to an
// existing module rather than writing a go.mod.
func App(dir, name, _ string) error {
	pkg := appPackage(name)
	d := data{Name: name, AppName: pkg, Module: pkg, GoVersion: goVersion}
	target := filepath.Join(dir, pkg)
	files := []file{
		{path: "app.go", tmpl: appConfig},
		{path: "models.go", tmpl: appModels},
		{path: "admin.go", tmpl: appAdmin},
		{path: "urls.go", tmpl: appURLs},
	}
	return render(target, d, files)
}

// appPackage returns the package directory name for an app: name with an "app"
// suffix, unless name already ends in "app".
func appPackage(name string) string {
	if len(name) >= 3 && name[len(name)-3:] == "app" {
		return name
	}
	return name + "app"
}

// render executes each template into its target path under dir, creating parent
// directories as needed.
func render(dir string, d data, files []file) error {
	for _, f := range files {
		out := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
			return fmt.Errorf("scaffold: create dir for %s: %w", f.path, err)
		}
		t, err := template.New(f.path).Parse(f.tmpl)
		if err != nil {
			return fmt.Errorf("scaffold: parse template %s: %w", f.path, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return fmt.Errorf("scaffold: render %s: %w", f.path, err)
		}
		if err := os.WriteFile(out, buf.Bytes(), 0o600); err != nil {
			return fmt.Errorf("scaffold: write %s: %w", f.path, err)
		}
	}
	return nil
}
