package staticfiles

import (
	"io/fs"
	"slices"
)

// Finder looks static files up in an ordered list of sources (first match
// wins). It stands for all of Django's finders at once: the sources are the
// locations that FileSystemFinder and AppDirectoriesFinder search.
//
// A Finder also is an fs.FS over the merged files of its sources; see Open.
// It holds no state besides the sources, so it is safe for concurrent use as
// far as the filesystems of the sources are.
type Finder struct {
	sources []Source
}

// NewFinder returns a Finder that searches the sources in the given order.
// Like Django's FileSystemFinder it keeps the first of several directory
// sources with the same Root and Prefix, so a directory listed twice is
// searched once.
func NewFinder(sources ...Source) *Finder {
	f := &Finder{sources: make([]Source, 0, len(sources))}
	seen := map[[2]string]bool{}
	for _, src := range sources {
		if src.Root != "" {
			key := [2]string{src.prefix(), src.Root}
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		f.sources = append(f.sources, src)
	}
	return f
}

// Sources returns the sources in search order. They are the locations a
// lookup searches (Django's finders.searched_locations).
func (f *Finder) Sources() []Source {
	return slices.Clone(f.sources)
}

// Match is one location of a static file.
type Match struct {
	// Path is the path under the static URL, slash-separated.
	Path string
	// Source is the source that holds the file.
	Source Source
	// Location is what Django prints for the file: the absolute path on disk
	// when the source has a Root, else "<Name>:<path inside the FS>".
	Location string
}

// name returns the name of the file inside the filesystem of its source.
func (m Match) name() string {
	name, _ := m.Source.inner(m.Path)
	return name
}

// open opens the file in its source.
func (m Match) open() (fs.File, error) {
	return m.Source.open(m.name())
}

// Find returns the first source holding path (Django's finders.find(path)).
// The path is slash-separated and relative to the static URL; a path that
// fs.ValidPath rejects, such as one with a ".." element, is never found. As in
// Django a directory is found like a file.
func (f *Finder) Find(path string) (Match, bool) {
	m, _, ok := f.find(path)
	return m, ok
}

// find is Find that also describes what is at the path, for a caller that
// has to tell a file from a directory.
func (f *Finder) find(path string) (Match, fs.FileInfo, bool) {
	if !fs.ValidPath(path) {
		return Match{}, nil, false
	}
	for _, src := range f.sources {
		if m, info, ok := src.match(path); ok {
			return m, info, true
		}
	}
	return Match{}, nil, false
}

// FindAll returns every source holding path, in order (Django's
// find(path, find_all=True)). It returns nil when no source holds it.
func (f *Finder) FindAll(path string) []Match {
	if !fs.ValidPath(path) {
		return nil
	}
	var matches []Match
	for _, src := range f.sources {
		if m, _, ok := src.match(path); ok {
			matches = append(matches, m)
		}
	}
	return matches
}

// List returns every static file once, the first source winning, skipping
// names that match ignore patterns. The files come in source order, and those
// of one source in the order of fs.WalkDir: sorted by name, with the files of
// a directory in the place of the directory. A symbolic link is followed.
//
// The patterns are shell patterns with the rules of Python's fnmatch, as
// Django's ignore_patterns are: "*" matches any run of characters, slashes
// included, "?" one character and "[seq]" or "[!seq]" a set. A pattern is
// tried on the base name of every file and directory and on the path of a file
// inside its source, so "*.tmp" skips those files everywhere, "CVS" skips a
// directory of that name with all it holds, and "*/vendor/*.js" skips by path.
// List applies only the patterns it is given; pass DefaultIgnorePatterns for
// Django's defaults.
func (f *Finder) List(ignorePatterns ...string) ([]Match, error) {
	var matches []Match
	seen := map[string]bool{}
	err := f.walk(ignorePatterns, func(m Match) error {
		if !seen[m.Path] {
			seen[m.Path] = true
			matches = append(matches, m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// walk calls fn for every file of every source, in the order that List
// documents. A path that several sources hold is reported once per source,
// which is how Collect learns of a conflict.
func (f *Finder) walk(ignore []string, fn func(Match) error) error {
	for _, src := range f.sources {
		if err := src.walk(".", ignore, fn); err != nil {
			return err
		}
	}
	return nil
}
