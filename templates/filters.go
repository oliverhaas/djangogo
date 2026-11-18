package templates

import (
	"fmt"

	"github.com/flosch/pongo2/v6"
)

// registerFilters installs the Django filters of this package: the ones pongo2
// lacks, and replacements for the ones it renders differently from Django.
func registerFilters() {
	for name, fn := range map[string]pongo2.FilterFunction{
		"escape":       filterEscape,
		"e":            filterEscape,
		"force_escape": filterForceEscape,
	} {
		mustFilter(name, fn)
	}
}

// mustFilter registers a filter, replacing the pongo2 filter of the same name
// if there is one. pongo2 rejects neither case for a valid name, so a failure
// is a programming error.
func mustFilter(name string, fn pongo2.FilterFunction) {
	var err error
	if pongo2.FilterExists(name) {
		err = pongo2.ReplaceFilter(name, fn)
	} else {
		err = pongo2.RegisterFilter(name, fn)
	}
	if err != nil {
		panic(fmt.Sprintf("templates: register filter %q: %v", name, err))
	}
}
