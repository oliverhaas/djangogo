package migrations

import (
	"fmt"
	"slices"
	"strings"
)

// MergeBranch is one of the branches that makemigrations --merge joins.
type MergeBranch struct {
	// Leaf is the last migration of the branch.
	Leaf *Migration
	// Operations are the operations of the migrations of the branch after
	// the last migration it shares with the other branches.
	Operations []Operation
}

// MergeBranches returns the branches of app that end in the leaf migrations
// leaves, in that order, as Django's makemigrations --merge works them out:
// each branch holds the migrations of app after the last ancestor the
// branches have in common. Migrations of other apps are left out.
func (l *Loader) MergeBranches(app string, leaves []string) ([]MergeBranch, error) {
	ancestries := make([][]Key, len(leaves))
	for i, name := range leaves {
		plan, err := l.Graph.ForwardsPlan(Key{App: app, Name: name})
		if err != nil {
			return nil, err
		}
		for _, key := range plan {
			if key.App == app {
				ancestries[i] = append(ancestries[i], key)
			}
		}
	}
	common := 0
	for !slices.ContainsFunc(ancestries, func(a []Key) bool {
		return len(a) <= common || a[common] != ancestries[0][common]
	}) {
		common++
	}
	if common == 0 {
		return nil, &CommandError{Message: "Could not find common ancestor of " + pythonList(leaves), Code: 1}
	}
	branches := make([]MergeBranch, len(leaves))
	for i, ancestry := range ancestries {
		branches[i].Leaf, _ = l.Migration(ancestry[len(ancestry)-1])
		for _, key := range ancestry[common:] {
			m, _ := l.Migration(key)
			branches[i].Operations = append(branches[i].Operations, m.Operations...)
		}
	}
	return branches, nil
}

// NewMergeMigration returns the migration of app that depends on the leaf
// migrations leaves and has no operations. Its number follows the highest
// number of the leaves. A non-empty name replaces "merge" and the leaf names
// in its name; leaf names longer than 47 characters give way to the current
// time.
func NewMergeMigration(app string, leaves []string, name string) Migration {
	biggest, found := 0, false
	for _, leaf := range leaves {
		if leadingNumber.MatchString(leaf) || squashedNumber.MatchString(leaf) {
			biggest = max(biggest, parseNumber(leaf))
			found = true
		}
	}
	if !found {
		biggest = 1
	}
	parts := []string{fmt.Sprintf("%04d", biggest+1)}
	if name != "" {
		parts = append(parts, name)
	} else {
		joined := strings.Join(slices.Sorted(slices.Values(leaves)), "_")
		if len(joined) > 47 {
			joined = nameTimestamp()
		}
		parts = append(parts, "merge", joined)
	}
	m := Migration{App: app, Name: strings.Join(parts, "_")}
	for _, leaf := range leaves {
		m.Dependencies = append(m.Dependencies, Key{App: app, Name: leaf})
	}
	return m
}

// pythonList formats names as Python prints a list of strings, as Django's
// messages show them.
func pythonList(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + name + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
