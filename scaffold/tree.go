package scaffold

import (
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// TreeOptions configures Tree.
type TreeOptions struct {
	// Placeholder is the text of the paths in the tree that becomes Name,
	// "project_name" or "app_name".
	Placeholder string
	// Name is the name of the project or the app.
	Name string
	// Context holds the values of the templates by name.
	Context map[string]any
	// Extensions lists the extensions of the files to render, with the dot.
	Extensions []string
	// Files lists the names of further files to render.
	Files []string
	// SkipDir, when not nil, reports whether to leave out a directory of the
	// tree with the given name, and everything in it.
	SkipDir func(name string) bool
	// Created is Options.Created.
	Created func(path string)
}

// Tree writes the template tree in the directory src into dir, as Django's
// TemplateCommand does with --template. Placeholder becomes Name wherever it
// occurs in a path, and the suffix "-tpl" is dropped. A file is rendered with
// the delimiters [[ and ]] and the values of Context when its path ends with
// one of Extensions or its name, before or after the renaming, is one of
// Files; any other file is copied as it is. A Go file is formatted when it
// parses. A file keeps the permission bits of its source, plus write
// permission for the owner, and the umask applies. Nothing is written when
// one of the files exists.
func Tree(dir, src string, opts TreeOptions) error {
	var files []output
	err := filepath.WalkDir(src, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if opts.Placeholder != "" {
			rel = strings.ReplaceAll(rel, opts.Placeholder, opts.Name)
		}
		target := filepath.Join(dir, rel)
		if entry.IsDir() {
			if opts.SkipDir != nil && opts.SkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			files = append(files, output{path: target, dir: true})
			return nil
		}
		// Stat follows a symbolic link. A link to a directory is not walked,
		// as Python's os.walk leaves it.
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			return err
		}
		f, err := treeFile(p, strings.TrimSuffix(target, templateSuffix), opts)
		if err != nil {
			return err
		}
		f.mode = info.Mode().Perm() | 0o200
		files = append(files, f)
		return nil
	})
	if err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}
	return write(files, opts.Created)
}

// treeFile reads the file src of a tree and renders it for target when
// opts selects it.
func treeFile(src, target string, opts TreeOptions) (output, error) {
	content, err := os.ReadFile(src)
	if err != nil {
		return output{}, err
	}
	if opts.renders(filepath.Base(src), target) {
		if content, err = execute(src, content, opts.Context); err != nil {
			return output{}, err
		}
	}
	if filepath.Ext(target) == ".go" {
		if formatted, err := format.Source(content); err == nil {
			content = formatted
		}
	}
	return output{path: target, content: content}, nil
}

// renders reports whether the file name of the tree is rendered into target.
func (o TreeOptions) renders(name, target string) bool {
	for _, ext := range o.Extensions {
		if strings.HasSuffix(target, ext) {
			return true
		}
	}
	return slices.Contains(o.Files, name) || slices.Contains(o.Files, filepath.Base(target))
}
