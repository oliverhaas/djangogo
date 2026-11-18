package checks_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// TestCloneIsIndependent adds a check to a registry and to its clone and finds
// each one only where it was added.
func TestCloneIsIndependent(t *testing.T) {
	message := func(text string) checks.Func {
		return func(*checks.Context) []checks.Message { return []checks.Message{checks.Info(text)} }
	}
	original := checks.NewRegistry()
	original.Register(message("shared"))

	clone := original.Clone()
	clone.Register(message("clone"))
	original.Register(message("original"))

	djangotest.AssertEqual(t, messageTexts(original.Run(nil, checks.RunOptions{})), []string{"shared", "original"})
	djangotest.AssertEqual(t, messageTexts(clone.Run(nil, checks.RunOptions{})), []string{"shared", "clone"})
}

// TestAddPanicsWithoutFunction refuses a check that has nothing to run.
func TestAddPanicsWithoutFunction(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Add did not panic for a check without a function")
		}
	}()

	checks.NewRegistry().Add(checks.Check{Tags: []string{"tag"}})
}

// TestAddKeepsItsOwnTags keeps the tags of a registered check when the caller
// changes the slice it passed.
func TestAddKeepsItsOwnTags(t *testing.T) {
	tags := []string{"first"}
	registry := checks.NewRegistry()
	registry.Add(checks.Check{Run: func(*checks.Context) []checks.Message { return nil }, Tags: tags})

	tags[0] = "second"

	djangotest.AssertEqual(t, registry.TagsAvailable(false), []string{"first"})
}

// TestTagsAvailableListsEachTagOnce lists a tag that several checks carry
// once.
func TestTagsAvailableListsEachTagOnce(t *testing.T) {
	none := func(*checks.Context) []checks.Message { return nil }
	registry := checks.NewRegistry()
	registry.Register(none, "b", "a")
	registry.Register(none, "a")

	djangotest.AssertEqual(t, registry.TagsAvailable(false), []string{"a", "b"})
}

// TestRegisterOnDefault adds a check and a deployment check to the Default
// registry through the package functions.
func TestRegisterOnDefault(t *testing.T) {
	none := func(*checks.Context) []checks.Message { return nil }

	checks.Register(none, "checks_test_plain")
	checks.RegisterDeploy(none, "checks_test_deploy")

	djangotest.AssertEqual(t, checks.Default.TagExists("checks_test_plain", false), true)
	djangotest.AssertEqual(t, checks.Default.TagExists("checks_test_deploy", false), false)
	djangotest.AssertEqual(t, checks.Default.TagExists("checks_test_deploy", true), true)
}

// TestBuiltinTags finds the tags of the built-in checks in the Default
// registry, the tag security only for a run with the deployment checks.
func TestBuiltinTags(t *testing.T) {
	builtin := []string{
		checks.TagCompatibility, checks.TagModels, checks.TagSettings, checks.TagStaticfiles, checks.TagURLs,
	}
	for _, tag := range builtin {
		if !checks.Default.TagExists(tag, false) {
			t.Errorf("no built-in check carries the tag %q", tag)
		}
	}
	djangotest.AssertEqual(t, checks.Default.TagExists(checks.TagSecurity, false), false)
	djangotest.AssertEqual(t, slices.Contains(checks.Default.TagsAvailable(true), checks.TagSecurity), true)
}

// TestSecurityChecksNeedDeploy runs the security checks only for a run with
// the deployment checks.
func TestSecurityChecksNeedDeploy(t *testing.T) {
	ctx := &checks.Context{Settings: &conf.Settings{}}
	securityIDs := func(opts checks.RunOptions) int {
		n := 0
		for _, m := range checks.Default.Run(ctx, opts) {
			if strings.HasPrefix(m.ID, "security.") {
				n++
			}
		}
		return n
	}

	djangotest.AssertEqual(t, securityIDs(checks.RunOptions{}), 0)
	if securityIDs(checks.RunOptions{Deploy: true}) == 0 {
		t.Error("a deployment run of settings without a secret key reported no security message")
	}
}

// TestBuiltinChecksWithoutSettingsAndModels reports nothing for a run that has
// neither settings nor a model registry to check.
func TestBuiltinChecksWithoutSettingsAndModels(t *testing.T) {
	assertMessages(t, checks.Default.Run(nil, checks.RunOptions{Deploy: true}))
}
