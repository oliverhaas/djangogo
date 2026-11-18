package staticfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Source is one place static files are looked up.
type Source struct {
	// Name identifies the source in output: the app label, or the directory
	// path.
	Name string
	// FS holds the files, rooted so that "css/site.css" is served as
	// STATIC_URL + "css/site.css".
	FS fs.FS
	// Prefix is the optional namespace of a STATICFILES_DIRS entry (Django's
	// ("prefix", "/path") tuple form): the files appear under Prefix + "/".
	// A slash at either end of it is ignored.
	Prefix string
	// Root is the directory on disk behind FS when there is one ("" for an
	// embedded filesystem). It is used to print absolute paths as Django does.
	Root string
}

// DirSource returns a Source for a directory on disk (os.DirFS), with Name and
// Root set to dir. A directory that does not exist is an empty source.
func DirSource(dir string) Source {
	return Source{Name: dir, FS: os.DirFS(dir), Root: dir}
}

// prefix returns Prefix without the slashes around it, so "downloads/" and
// "downloads" name the same namespace.
func (s Source) prefix() string {
	return strings.Trim(s.Prefix, "/")
}

// inner maps a path under the static URL to the name of the same file inside
// FS. It reports false for a path outside the prefix of the source.
func (s Source) inner(urlPath string) (string, bool) {
	prefix := s.prefix()
	if prefix == "" {
		return urlPath, true
	}
	return strings.CutPrefix(urlPath, prefix+"/")
}

// outer is the reverse of inner: the path under the static URL of a file
// inside FS.
func (s Source) outer(name string) string {
	if prefix := s.prefix(); prefix != "" {
		return prefix + "/" + name
	}
	return name
}

// location returns what Django prints for a file of the source: its absolute
// path when the source is a directory on disk. A file of an embedded
// filesystem has no path, so it is named by the source and the name inside it.
func (s Source) location(name string) string {
	if s.Root == "" {
		return s.Name + ":" + name
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		root = s.Root
	}
	return filepath.Join(root, filepath.FromSlash(name))
}

// searched returns what names the source in a list of searched locations: the
// directory as it was configured, or the name of an embedded source.
func (s Source) searched() string {
	if s.Root != "" {
		return s.Root
	}
	return s.Name
}

// stat describes a name inside FS, following a symbolic link. A source
// without a filesystem holds no files.
func (s Source) stat(name string) (fs.FileInfo, error) {
	if s.FS == nil {
		return nil, fs.ErrNotExist
	}
	return fs.Stat(s.FS, name)
}

// open opens a name inside FS.
func (s Source) open(name string) (fs.File, error) {
	if s.FS == nil {
		return nil, fs.ErrNotExist
	}
	return s.FS.Open(name)
}

// readDir lists a directory inside FS, sorted by name.
func (s Source) readDir(name string) ([]fs.DirEntry, error) {
	if s.FS == nil {
		return nil, fs.ErrNotExist
	}
	return fs.ReadDir(s.FS, name)
}

// newMatch returns the Match for a name inside FS.
func (s Source) newMatch(name string) Match {
	return Match{Path: s.outer(name), Source: s, Location: s.location(name)}
}

// match looks a path under the static URL up in the source, as Django's
// find_location and find_in_app do. Like them it matches whatever exists at
// the path, a file or a directory, and it returns what it found there.
func (s Source) match(urlPath string) (Match, fs.FileInfo, bool) {
	name, ok := s.inner(urlPath)
	if !ok {
		return Match{}, nil, false
	}
	info, err := s.stat(name)
	if err != nil {
		return Match{}, nil, false
	}
	return s.newMatch(name), info, true
}

// entryType resolves what a directory entry is. A symbolic link counts as
// what it points to, because Django's listing follows links; a link to nothing
// is neither a file nor a directory.
func (s Source) entryType(name string, entry fs.DirEntry) fs.FileMode {
	mode := entry.Type()
	if mode&fs.ModeSymlink == 0 {
		return mode
	}
	info, err := s.stat(name)
	if err != nil {
		return fs.ModeIrregular
	}
	return info.Mode().Type()
}

// walk calls fn for every regular file below dir, the entries of a directory
// sorted by name and a subdirectory descended in its place, leaving out what
// the ignore patterns match. It is Django's utils.get_files: a
// pattern is tried on the base name of a file or directory and on the path of
// a file inside the source, never on the prefix.
func (s Source) walk(dir string, ignore []string, fn func(Match) error) error {
	entries, err := s.readDir(dir)
	if err != nil {
		// Django skips a STATICFILES_DIRS entry and an app directory that do
		// not exist.
		if dir == "." && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("staticfiles: list %s: %w", s.location(dir), err)
	}
	for _, entry := range entries {
		base := entry.Name()
		if matchesPatterns(base, ignore) {
			continue
		}
		name := path.Join(dir, base)
		mode := s.entryType(name, entry)
		switch {
		case mode.IsDir():
			if err := s.walk(name, ignore, fn); err != nil {
				return err
			}
		case mode.IsRegular():
			if dir != "." && matchesPatterns(name, ignore) {
				continue
			}
			if err := fn(s.newMatch(name)); err != nil {
				return err
			}
		}
	}
	return nil
}
