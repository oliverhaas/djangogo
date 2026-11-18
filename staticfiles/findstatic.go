package staticfiles

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// FindStaticOutput renders what Django's findstatic command prints for one
// path, so that a command only has to print it. It reports whether the path
// was found: Django writes the text for a path that was found to stdout and
// the text for one that was not to stderr. The text ends with a newline, and
// it is empty when there is nothing to print, which is the case for a path
// that was not found at verbosity 0.
//
// With all set, every source that holds the path is listed; without it only
// the first (--first). The verbosity is Django's: 0 prints bare locations, one
// per line, 1 adds the "Found 'path' here:" heading and the "No matching file
// found" message, and 2 adds the locations that were searched. A location is
// the absolute path of the file with symbolic links resolved, or
// "<source name>:<path>" for a file of a source without a directory.
func FindStaticOutput(finder *Finder, path string, all bool, verbosity int) (string, bool) {
	var found, searched []string
	valid := fs.ValidPath(path)
	for _, src := range finder.sources {
		// Django lists a location once, however many entries name it.
		if location := src.searched(); !slices.Contains(searched, location) {
			searched = append(searched, location)
		}
		if !valid {
			continue
		}
		if m, _, ok := src.match(path); ok {
			found = append(found, realLocation(m))
			if !all {
				// The first match ends the search, and with it the list of
				// searched locations.
				break
			}
		}
	}
	looking := ""
	if verbosity >= 2 {
		looking = "\nLooking in the following locations:\n  " + strings.Join(searched, "\n  ")
	}
	switch {
	case len(found) > 0 && verbosity >= 1:
		return fmt.Sprintf("Found '%s' here:\n  %s%s\n", path, strings.Join(found, "\n  "), looking), true
	case len(found) > 0:
		return strings.Join(found, "\n") + "\n", true
	case verbosity >= 2:
		return fmt.Sprintf("No matching file found for '%s'.\n%s\n", path, looking), false
	case verbosity == 1:
		return fmt.Sprintf("No matching file found for '%s'.\n", path), false
	default:
		return "", false
	}
}

// realLocation returns the location of a match with symbolic links resolved,
// as findstatic applies os.path.realpath to every match. A file of a source
// without a directory keeps its location, which is not a path.
func realLocation(m Match) string {
	if m.Source.Root == "" {
		return m.Location
	}
	resolved, err := filepath.EvalSymlinks(m.Location)
	if err != nil {
		return m.Location
	}
	return resolved
}
