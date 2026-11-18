package djangogo_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
)

// checkApp is a minimal app config for the check tests.
type checkApp struct{ name string }

// Name returns the name of the app.
func (a checkApp) Name() string { return a.name }

// checkRecorder is a check function that records its contexts and returns
// fixed messages.
type checkRecorder struct {
	contexts []*checks.Context
	messages []checks.Message
}

// run is the check function of the recorder.
func (r *checkRecorder) run(ctx *checks.Context) []checks.Message {
	r.contexts = append(r.contexts, ctx)
	return r.messages
}

// checkRan is a command that records that it ran.
type checkRan struct {
	ran  int
	skip bool
}

// Name returns the command name.
func (*checkRan) Name() string { return "ranner" }

// Help returns the help text.
func (*checkRan) Help() string { return "Records a run." }

// Run records the run.
func (c *checkRan) Run([]string) error { c.ran++; return nil }

// SkipSystemChecks reports whether the command opts out of the checks.
func (c *checkRan) SkipSystemChecks() bool { return c.skip }

// checkHarness is an application with a fresh registry and buffers.
type checkHarness struct {
	app      *djangogo.Application
	out, err *bytes.Buffer
}

// newCheckHarness builds an application whose checks are the given registry.
func newCheckHarness(t *testing.T, s conf.Settings, reg *checks.Registry, cfgs ...apps.Config) *checkHarness {
	t.Helper()
	app, err := djangogo.New(s, cfgs...)
	if err != nil {
		t.Fatal(err)
	}
	h := &checkHarness{app: app, out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	app.Checks = reg
	app.Out = h.out
	app.Err = h.err
	return h
}

// checkSettings returns settings that pass validation.
func checkSettings() conf.Settings { return conf.Settings{SecretKey: "k"} }

// checkRegistry returns a registry with the check, tagged as given.
func checkRegistry(r *checkRecorder, tags ...string) *checks.Registry {
	reg := checks.NewRegistry()
	reg.Register(r.run, tags...)
	return reg
}

// TestCheckCommandNoIssues mirrors CheckCommandTests.test_simple_call: a check
// with no messages reports no issues on Out.
func TestCheckCommandNoIssues(t *testing.T) {
	r := &checkRecorder{}
	h := newCheckHarness(t, checkSettings(), checkRegistry(r))
	if err := h.app.Execute([]string{"check"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.out.String(), "System check identified no issues (0 silenced).\n")
	djangotest.AssertEqual(t, h.err.String(), "")
	djangotest.AssertEqual(t, len(r.contexts), 1)
}

// TestCheckCommandAppLabels passes the app labels to the check as Context.Apps
// and rejects an unknown label.
func TestCheckCommandAppLabels(t *testing.T) {
	r := &checkRecorder{}
	h := newCheckHarness(t, checkSettings(), checkRegistry(r), checkApp{name: "blog"})
	if err := h.app.Execute([]string{"check", "blog"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, r.contexts[0].Apps, []string{"blog"})

	r.contexts = nil
	err := h.app.Execute([]string{"check", "nosuchapp"})
	if err == nil {
		t.Fatal("an unknown label must be an error")
	}
	var cmdErr *manage.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("want a *manage.CommandError, got %T", err)
	}
	djangotest.AssertEqual(t, err.Error(), "No installed app with label 'nosuchapp'.")
	djangotest.AssertEqual(t, len(r.contexts), 0)
}

// TestCheckCommandTags mirrors CheckCommandTests.test_given_tag and
// test_invalid_tag: --tag and -t run only the tagged check, and an unknown tag
// is an error.
func TestCheckCommandTags(t *testing.T) {
	for _, flagName := range []string{"--tag", "-t"} {
		t.Run(flagName, func(t *testing.T) {
			simple := &checkRecorder{}
			tagged := &checkRecorder{messages: []checks.Message{checks.Warning("System Check")}}
			reg := checks.NewRegistry()
			reg.Register(simple.run)
			reg.Register(tagged.run, "simpletag")
			h := newCheckHarness(t, checkSettings(), reg)
			if err := h.app.Execute([]string{"check", flagName, "simpletag"}); err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, len(simple.contexts), 0)
			djangotest.AssertEqual(t, len(tagged.contexts), 1)
			checkContains(t, h.err.String(), "System Check")
		})
	}

	h := newCheckHarness(t, checkSettings(), checkRegistry(&checkRecorder{}, "simpletag"))
	err := h.app.Execute([]string{"check", "--tag", "missingtag"})
	if err == nil {
		t.Fatal("an unknown tag must be an error")
	}
	djangotest.AssertEqual(t, err.Error(), `There is no system check with the "missingtag" tag.`)
}

// TestCheckCommandListTags mirrors CheckCommandTests.test_list_tags_empty and
// test_list_deployment_check_included: --list-tags prints the sorted tags, with
// the deployment tags only given --deploy.
func TestCheckCommandListTags(t *testing.T) {
	reg := checks.NewRegistry()
	reg.Register((&checkRecorder{}).run)
	h := newCheckHarness(t, checkSettings(), reg)
	if err := h.app.Execute([]string{"check", "--list-tags"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.out.String(), "\n")

	reg = checks.NewRegistry()
	reg.Register((&checkRecorder{}).run, "simpletag")
	reg.RegisterDeploy((&checkRecorder{}).run, "deploymenttag")
	h = newCheckHarness(t, checkSettings(), reg)
	if err := h.app.Execute([]string{"check", "--list-tags"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.out.String(), "simpletag\n")

	h = newCheckHarness(t, checkSettings(), reg)
	if err := h.app.Execute([]string{"check", "--list-tags", "--deploy"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.out.String(), "deploymenttag\nsimpletag\n")
}

// TestCheckCommandDeployTag mirrors CheckCommandTests.test_tags_deployment_check_omitted
// and test_tags_deployment_check_included: a deployment tag is unknown without --deploy and runs with it.
func TestCheckCommandDeployTag(t *testing.T) {
	dep := &checkRecorder{messages: []checks.Message{checks.Warning("Deployment Check")}}
	reg := checks.NewRegistry()
	reg.RegisterDeploy(dep.run, "deploymenttag")
	h := newCheckHarness(t, checkSettings(), reg)

	err := h.app.Execute([]string{"check", "--tag", "deploymenttag"})
	if err == nil {
		t.Fatal("a deployment tag without --deploy must be an error")
	}
	djangotest.AssertEqual(t, err.Error(), `There is no system check with the "deploymenttag" tag.`)

	if err := h.app.Execute([]string{"check", "--tag", "deploymenttag", "--deploy"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(dep.contexts), 1)
	checkContains(t, h.err.String(), "Deployment Check")
}

// TestCheckCommandFailLevel mirrors CheckCommandTests.test_fail_level: a
// warning fails the run at --fail-level WARNING, and an unknown level is a
// usage error.
func TestCheckCommandFailLevel(t *testing.T) {
	r := &checkRecorder{messages: []checks.Message{checks.Warning("System Check")}}
	h := newCheckHarness(t, checkSettings(), checkRegistry(r))

	err := h.app.Execute([]string{"check", "--fail-level", "WARNING"})
	var sce *checks.SystemCheckError
	if !errors.As(err, &sce) {
		t.Fatalf("want a *checks.SystemCheckError, got %v", err)
	}

	code := h.app.Run([]string{"check", "--fail-level", "WARNING"})
	djangotest.AssertEqual(t, code, 1)
	if !strings.HasPrefix(h.err.String(), "SystemCheckError: System check identified some issues:") {
		t.Errorf("unexpected Err: %q", h.err.String())
	}

	// Without the option a warning does not fail.
	h = newCheckHarness(t, checkSettings(), checkRegistry(r))
	djangotest.AssertEqual(t, h.app.Run([]string{"check"}), 0)
	checkContains(t, h.err.String(), "System check identified some issues:")

	h = newCheckHarness(t, checkSettings(), checkRegistry(r))
	djangotest.AssertEqual(t, h.app.Run([]string{"check", "--fail-level", "NOPE"}), 2)
}

// TestCheckCommandDatabase mirrors CheckCommandTests.test_database_system_checks:
// --database
// passes the aliases to the check and an alias that is not configured is a
// usage error.
func TestCheckCommandDatabase(t *testing.T) {
	r := &checkRecorder{}
	h := newCheckHarness(t, checkSettings(), checkRegistry(r))
	if err := h.app.Execute([]string{"check"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(r.contexts[0].Databases), 0)

	if err := h.app.Execute([]string{"check", "--database", "default"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, r.contexts[1].Databases, []string{"default"})

	err := h.app.Execute([]string{"check", "--database", "other"})
	var usage *manage.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("want a *manage.UsageError, got %v", err)
	}
	checkContains(t, err.Error(), "argument --database: invalid choice: 'other' (choose from 'default')")
	djangotest.AssertEqual(t, h.app.Run([]string{"check", "--database", "other"}), 2)
}

// TestCheckCommandSilenced mirrors SilencingCheckTests.test_silenced_error: an
// error whose id is in SilencedSystemChecks counts as silenced.
func TestCheckCommandSilenced(t *testing.T) {
	r := &checkRecorder{messages: []checks.Message{checks.Error("Error", checks.ID("myerrorcheck.E001"))}}
	s := conf.Settings{SecretKey: "k", SilencedSystemChecks: []string{"myerrorcheck.E001"}}
	h := newCheckHarness(t, s, checkRegistry(r))
	djangotest.AssertEqual(t, h.app.Run([]string{"check"}), 0)
	djangotest.AssertEqual(t, h.out.String(), "System check identified no issues (1 silenced).\n")
	djangotest.AssertEqual(t, h.err.String(), "")
}

// TestSystemChecksBeforeCommand runs the checks before a command: a warning
// reaches Err and the command still runs, an error stops it.
func TestSystemChecksBeforeCommand(t *testing.T) {
	cmd := &checkRan{}
	warn := &checkRecorder{messages: []checks.Message{checks.Warning("A warning")}}
	h := newCheckHarness(t, checkSettings(), checkRegistry(warn))
	if err := h.app.Commands.Register(cmd); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.app.Run([]string{"ranner"}), 0)
	djangotest.AssertEqual(t, cmd.ran, 1)
	checkContains(t, h.err.String(), "A warning")

	cmd = &checkRan{}
	bad := &checkRecorder{messages: []checks.Message{checks.Error("An error")}}
	h = newCheckHarness(t, checkSettings(), checkRegistry(bad))
	if err := h.app.Commands.Register(cmd); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.app.Run([]string{"ranner"}), 1)
	djangotest.AssertEqual(t, cmd.ran, 0)
	if !strings.HasPrefix(h.err.String(), "SystemCheckError:") {
		t.Errorf("unexpected Err: %q", h.err.String())
	}
}

// TestSystemChecksSkipped runs no checks for a command that implements
// SkipSystemChecks and for a run with --skip-checks.
func TestSystemChecksSkipped(t *testing.T) {
	bad := &checkRecorder{messages: []checks.Message{checks.Error("An error")}}

	cmd := &checkRan{skip: true}
	h := newCheckHarness(t, checkSettings(), checkRegistry(bad))
	if err := h.app.Commands.Register(cmd); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.app.Run([]string{"ranner"}), 0)
	djangotest.AssertEqual(t, cmd.ran, 1)
	djangotest.AssertEqual(t, len(bad.contexts), 0)

	cmd = &checkRan{}
	h = newCheckHarness(t, checkSettings(), checkRegistry(bad))
	if err := h.app.Commands.Register(cmd); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, h.app.Run([]string{"ranner", "--skip-checks"}), 0)
	djangotest.AssertEqual(t, cmd.ran, 1)
	djangotest.AssertEqual(t, len(bad.contexts), 0)
}

// TestSystemChecksBuiltinsExempt runs no checks before version, and check runs
// its own checks only once.
func TestSystemChecksBuiltinsExempt(t *testing.T) {
	bad := &checkRecorder{messages: []checks.Message{checks.Error("An error")}}
	h := newCheckHarness(t, checkSettings(), checkRegistry(bad))
	djangotest.AssertEqual(t, h.app.Run([]string{"version"}), 0)
	djangotest.AssertEqual(t, len(bad.contexts), 0)
	djangotest.AssertEqual(t, h.out.String(), djangogo.Version+"\n")

	good := &checkRecorder{}
	h = newCheckHarness(t, checkSettings(), checkRegistry(good))
	djangotest.AssertEqual(t, h.app.Run([]string{"check"}), 0)
	djangotest.AssertEqual(t, len(good.contexts), 1)
}

// checkContains fails the test when text does not contain want.
func checkContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Errorf("%q does not contain %q", text, want)
	}
}
