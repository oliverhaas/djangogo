package migrations

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

// This file turns the changes the autodetector found into numbered
// migrations that continue the graph.

var (
	// leadingNumber matches the number at the start of a migration name.
	leadingNumber = regexp.MustCompile(`^\d+`)
	// squashedNumber matches the last number after "_squashed_" in the name of
	// a squashed migration, such as "0001_squashed_0004_more".
	squashedNumber = regexp.MustCompile(`.*_squashed_(\d+)`)
)

// parseNumber returns the number a migration name starts with, or 0 when it
// has none. A squashed migration counts as the last migration it replaces, as
// in Django, so numbering continues after a history squashed elsewhere.
func parseNumber(name string) int {
	digits := leadingNumber.FindString(name)
	if m := squashedNumber.FindStringSubmatch(name); m != nil {
		digits = m[1]
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}

// ArrangeForGraph gives the migrations their final names and makes the first
// one of each app depend on the leaf node of that app, so they continue the
// graph. The first migration of an app without migrations is "0001_initial".
// migrationName, when not empty, replaces the generated part of the names.
// Changes calls it; use it directly for migrations written by hand, such as
// the empty one of makemigrations --empty.
func ArrangeForGraph(changes map[string][]*Migration, graph *Graph, migrationName string) map[string][]*Migration {
	if graph == nil {
		graph = NewGraph()
	}
	leaves := graph.LeafNodes()
	renamed := map[Key]Key{}
	for app, migs := range changes {
		if len(migs) == 0 {
			continue
		}
		var leaf *Key
		for i := range leaves {
			if leaves[i].App == app {
				leaf = &leaves[i]
				break
			}
		}
		next := 1
		if leaf != nil {
			next = parseNumber(leaf.Name) + 1
		}
		for i, m := range migs {
			if i == 0 && leaf != nil {
				m.Dependencies = append(m.Dependencies, *leaf)
			}
			var suffix string
			switch {
			case migrationName != "":
				suffix = migrationName
			case i == 0 && leaf == nil:
				suffix = "initial"
			default:
				suffix = truncate(m.SuggestName(), 100)
			}
			name := fmt.Sprintf("%04d_%s", next, suffix)
			renamed[m.Key()] = Key{App: app, Name: name}
			m.Name = name
			next++
		}
	}
	for _, migs := range changes {
		for _, m := range migs {
			for i, dep := range m.Dependencies {
				if to, ok := renamed[dep]; ok {
					m.Dependencies[i] = to
				}
			}
			slices.SortFunc(m.Dependencies, compareKeys)
		}
	}
	return changes
}

// trimChanges drops the migrations of every app that is neither in apps nor
// needed by one of them.
func trimChanges(changes map[string][]*Migration, apps []string) map[string][]*Migration {
	needs := map[string][]string{}
	for app, migs := range changes {
		for _, m := range migs {
			for _, dep := range m.Dependencies {
				needs[app] = append(needs[app], dep.App)
			}
		}
	}
	required := map[string]bool{}
	queue := slices.Clone(apps)
	for len(queue) > 0 {
		app := queue[0]
		queue = queue[1:]
		if required[app] {
			continue
		}
		required[app] = true
		queue = append(queue, needs[app]...)
	}
	for app := range changes {
		if !required[app] {
			delete(changes, app)
		}
	}
	return changes
}
