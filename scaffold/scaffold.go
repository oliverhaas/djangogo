// Package scaffold generates Djan-Go-Go projects and apps, the engine behind
// the startproject and startapp commands. The files come from the template
// trees project_template and app_template, the counterparts of Django's
// conf/project_template and conf/app_template.
//
// Every file of a tree is a text/template with the delimiters [[ and ]], so
// the {{ }} of the Django templates in the trees pass through unchanged. A
// path component "app_name" becomes the name of the app, and the suffix
// "-tpl" is dropped: it keeps the Go tool away from the .go files and the
// go.mod of the trees, as ".py-tpl" does for Django.
package scaffold

import (
	"bytes"
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// trees holds the template trees.
//
//go:embed project_template app_template
var trees embed.FS

const (
	// goVersion is the Go version written into the go.mod of a project.
	goVersion = "1.26"
	// appNamePart is the path component that becomes the name of the app.
	appNamePart = "app_name"
	// templateSuffix ends the names of the files the Go tool must not see.
	templateSuffix = "-tpl"
	// secretKeyPrefix marks a generated key as unfit for production; the
	// security.W009 check of check --deploy reports it, as in Django.
	secretKeyPrefix = "django-insecure-"
	// secretKeyChars and secretKeyLength are those of Django's
	// get_random_secret_key.
	secretKeyChars  = "abcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*(-_=+)"
	secretKeyLength = 50
)

// Options configures Project and App.
type Options struct {
	// ReplacePath, when not empty, points the go.mod of a project at a local
	// checkout of Djan-Go-Go with a replace directive, so the project builds
	// against it before a release.
	ReplacePath string
	// Created, when not nil, is called with the path of every file written,
	// in order: Django's "Creating <path>" at verbosity 2.
	Created func(path string)
}

// ExistsError reports a file that a generation would replace. Nothing is
// written then.
type ExistsError struct {
	// Path is the file that exists.
	Path string
}

// Error returns the path and the reason.
func (e *ExistsError) Error() string { return "scaffold: " + e.Path + " already exists" }

// data carries the values the templates use.
type data struct {
	// Name is the name of the project or of the app.
	Name string
	// AppName is the name, package and label of the app.
	AppName string
	// Module is the module path of a project.
	Module string
	// ReplacePath is Options.ReplacePath.
	ReplacePath string
	// GoVersion is the Go version of the go.mod of a project.
	GoVersion string
	// SecretKey is the generated secret key of a project.
	SecretKey string
}

// output is one rendered file.
type output struct {
	path    string
	content []byte
}

// ProjectAppName returns the name of the sample app of a project named name.
func ProjectAppName(name string) string { return name + "app" }

// Project writes a project named name into dir: go.mod with the module path
// name, main.go running the management commands, settings.go with a new
// secret key and the environment overlay, the migrations package of the
// sample app and the sample app itself, named ProjectAppName(name). Nothing
// is written when one of the files exists.
func Project(dir, name string, opts Options) error {
	key, err := secretKey()
	if err != nil {
		return err
	}
	d := data{
		Name:        name,
		AppName:     ProjectAppName(name),
		Module:      name,
		ReplacePath: opts.ReplacePath,
		GoVersion:   goVersion,
		SecretKey:   key,
	}
	project, err := render(dir, "project_template", d)
	if err != nil {
		return err
	}
	app, err := render(filepath.Join(dir, d.AppName), "app_template", d)
	if err != nil {
		return err
	}
	return write(append(project, app...), opts.Created)
}

// App writes an app named name into dir: its configuration, a sample model
// with its admin, views, URLs and tests, and its templates and static files.
// Nothing is written when one of the files exists.
func App(dir, name string, opts Options) error {
	files, err := render(dir, "app_template", data{Name: name, AppName: name, GoVersion: goVersion})
	if err != nil {
		return err
	}
	return write(files, opts.Created)
}

// render renders the tree root for d into files under dir, in lexical order,
// and formats the Go files. It writes nothing.
func render(dir, root string, d data) ([]output, error) {
	var files []output
	err := fs.WalkDir(trees, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		src, err := trees.ReadFile(name)
		if err != nil {
			return err
		}
		t, err := template.New(name).Delims("[[", "]]").Option("missingkey=error").Parse(string(src))
		if err != nil {
			return fmt.Errorf("scaffold: parse %s: %w", name, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return fmt.Errorf("scaffold: render %s: %w", name, err)
		}
		rel := targetPath(strings.TrimPrefix(name, root+"/"), d.AppName)
		content := buf.Bytes()
		if path.Ext(rel) == ".go" {
			if content, err = format.Source(content); err != nil {
				return fmt.Errorf("scaffold: format %s: %w", name, err)
			}
		}
		files = append(files, output{path: filepath.Join(dir, filepath.FromSlash(rel)), content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scaffold: %w", err)
	}
	return files, nil
}

// targetPath returns the path a file of a tree is written to: rel with every
// component "app_name" replaced by appName and without the template suffix.
func targetPath(rel, appName string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if part == appNamePart {
			parts[i] = appName
		}
	}
	return strings.TrimSuffix(strings.Join(parts, "/"), templateSuffix)
}

// write writes files after checking that none of them exists, and calls
// created with each path written.
func write(files []output, created func(string)) error {
	for _, f := range files {
		_, err := os.Lstat(f.path)
		if err == nil {
			return &ExistsError{Path: f.path}
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("scaffold: %w", err)
		}
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return fmt.Errorf("scaffold: %w", err)
		}
		if err := os.WriteFile(f.path, f.content, 0o644); err != nil {
			return fmt.Errorf("scaffold: %w", err)
		}
		if created != nil {
			created(f.path)
		}
	}
	return nil
}

// secretKey returns a new random secret key with the insecure prefix, as
// Django's startproject writes into settings.py.
func secretKey() (string, error) {
	var b strings.Builder
	b.WriteString(secretKeyPrefix)
	limit := big.NewInt(int64(len(secretKeyChars)))
	for range secretKeyLength {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("scaffold: secret key: %w", err)
		}
		b.WriteByte(secretKeyChars[n.Int64()])
	}
	return b.String(), nil
}
