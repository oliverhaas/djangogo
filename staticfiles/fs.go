package staticfiles

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// Open implements fs.FS over the merged files of the sources, so the Finder
// can be handed to http.FileServerFS, fs.WalkDir or fs.Sub.
//
// A file is the file of the first source that holds the name. A directory is
// the union of the directories that the sources hold at the name, and a name
// that several of them list is the entry of the first. The prefix of a source
// is a directory of the merged view. A directory reports no modification time,
// because it stands for several directories.
func (f *Finder) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	var parts []dirPart
	for _, src := range f.sources {
		part, ok := src.part(name)
		if !ok {
			continue
		}
		if part.child == "" {
			info, err := src.stat(part.inner)
			if err != nil {
				continue
			}
			if !info.IsDir() {
				if len(parts) > 0 {
					// An earlier source holds a directory at the name.
					continue
				}
				return openPart(src, part.inner, name)
			}
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &mergedDir{name: name, parts: parts}, nil
}

// openPart opens a file of a source and reports a failure under the name the
// caller asked for, which differs from the name inside a prefixed source.
func openPart(src Source, inner, name string) (fs.File, error) {
	file, err := src.open(inner)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return file, nil
}

// dirPart is what one source adds to a directory of the merged view.
type dirPart struct {
	src Source
	// inner is the directory inside the FS of the source.
	inner string
	// child is set in place of inner for a directory above the prefix of the
	// source: such a directory holds nothing but the next element of the
	// prefix.
	child string
}

// part maps a path of the merged view to what the source holds there. It
// reports false for a path outside the source. A prefix that is not a valid
// path, such as one with a ".." element, keeps the source out of the view.
func (s Source) part(name string) (dirPart, bool) {
	prefix := s.prefix()
	if prefix == "" {
		return dirPart{src: s, inner: name}, true
	}
	if !fs.ValidPath(prefix) {
		return dirPart{}, false
	}
	if name == prefix {
		return dirPart{src: s, inner: "."}, true
	}
	if inner, ok := strings.CutPrefix(name, prefix+"/"); ok {
		return dirPart{src: s, inner: inner}, true
	}
	below := prefix
	if name != "." {
		var ok bool
		if below, ok = strings.CutPrefix(prefix, name+"/"); !ok {
			return dirPart{}, false
		}
	}
	// A source whose directory does not exist is empty, so the directories
	// that lead to it do not exist either.
	if info, err := s.stat("."); err != nil || !info.IsDir() {
		return dirPart{}, false
	}
	child, _, _ := strings.Cut(below, "/")
	return dirPart{src: s, child: child}, true
}

// entries lists what the source adds to the directory. A symbolic link is
// listed as what it points to, as Open follows it, and a link to nothing is
// left out.
func (p dirPart) entries() ([]fs.DirEntry, error) {
	if p.child != "" {
		return []fs.DirEntry{fs.FileInfoToDirEntry(dirInfo{name: p.child})}, nil
	}
	list, err := p.src.readDir(p.inner)
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, 0, len(list))
	for _, entry := range list {
		if entry.Type()&fs.ModeSymlink != 0 {
			info, err := p.src.stat(path.Join(p.inner, entry.Name()))
			if err != nil {
				continue
			}
			entry = fs.FileInfoToDirEntry(info)
		}
		if entry.IsDir() {
			entry = fs.FileInfoToDirEntry(dirInfo{name: entry.Name()})
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// dirInfo describes a directory of the merged view. It is the same for every
// directory but the name, so that a directory entry and the directory opened
// under its name agree.
type dirInfo struct {
	name string
}

// Name returns the base name of the directory.
func (d dirInfo) Name() string { return d.name }

// Size returns 0: a directory has no content of its own.
func (d dirInfo) Size() int64 { return 0 }

// Mode returns the mode of a read-only directory.
func (d dirInfo) Mode() fs.FileMode { return fs.ModeDir | 0o555 }

// ModTime returns the zero time; see Finder.Open.
func (d dirInfo) ModTime() time.Time { return time.Time{} }

// IsDir returns true.
func (d dirInfo) IsDir() bool { return true }

// Sys returns nil: no single underlying directory exists.
func (d dirInfo) Sys() any { return nil }

// mergedDir is an open directory of the merged view: the union of the
// directories that the sources hold at one path.
type mergedDir struct {
	name  string
	parts []dirPart
	// list holds the entries once the first ReadDir call has read them, and
	// offset counts the ones already returned.
	list   []fs.DirEntry
	loaded bool
	offset int
}

// Stat describes the directory.
func (d *mergedDir) Stat() (fs.FileInfo, error) {
	return dirInfo{name: path.Base(d.name)}, nil
}

// Read fails: a directory has no bytes to read.
func (d *mergedDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}

// Close does nothing: the directory holds no open file.
func (d *mergedDir) Close() error { return nil }

// ReadDir implements fs.ReadDirFile: it returns the next n entries, sorted by
// name, or all that are left when n is not positive.
func (d *mergedDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if !d.loaded {
		list, err := d.load()
		if err != nil {
			return nil, err
		}
		d.list, d.loaded = list, true
	}
	rest := d.list[d.offset:]
	if n <= 0 {
		d.offset = len(d.list)
		return slices.Clone(rest), nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(rest))
	d.offset += n
	return slices.Clone(rest[:n]), nil
}

// load reads the directory from every source. The first source that lists a
// name provides its entry, which is the source whose file Open returns.
func (d *mergedDir) load() ([]fs.DirEntry, error) {
	var list []fs.DirEntry
	seen := map[string]bool{}
	for _, part := range d.parts {
		entries, err := part.entries()
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: d.name, Err: err}
		}
		for _, entry := range entries {
			if !seen[entry.Name()] {
				seen[entry.Name()] = true
				list = append(list, entry)
			}
		}
	}
	slices.SortFunc(list, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return list, nil
}
