package checks_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are ports of Django's tests/check_framework/tests.py.
// A Django check receives app_configs and databases as keyword arguments; a Go
// check receives them as the Apps and Databases of its Context. Where Django
// calls the check command, a port calls what the command is made of: Run,
// TagExists, TagsAvailable and Summarize.

// dummyObj is the port of DummyObj: an object that prints as "obj".
type dummyObj struct{}

// String returns what DummyObj.__repr__ returns.
func (dummyObj) String() string { return "obj" }

// SimpleModel is the port of check_framework.models.SimpleModel.
type SimpleModel struct {
	ID    int64
	Field int64
}

// messageTexts returns the Msg of every message, in order.
func messageTexts(msgs []checks.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Msg)
	}
	return out
}

// commandChecks is the port of simple_system_check, tagged_system_check and
// deployment_system_check: a check without tags and without messages, a check
// tagged simpletag and a deployment check tagged deploymenttag.
type commandChecks struct {
	simple, tagged, deployment *recorder
}

// newCommandChecks returns fresh recorders with the messages of the Django
// functions.
func newCommandChecks() commandChecks {
	return commandChecks{
		simple:     &recorder{},
		tagged:     &recorder{messages: []checks.Message{checks.Warning("System Check")}},
		deployment: &recorder{messages: []checks.Message{checks.Warning("Deployment Check")}},
	}
}

// registry is the port of override_system_checks: a registry that holds the
// simple check when withSimple is set, the tagged check when withTagged is set
// and the deployment check when withDeployment is set.
func (c commandChecks) registry(withSimple, withTagged, withDeployment bool) *checks.Registry {
	registry := checks.NewRegistry()
	if withSimple {
		registry.Register(c.simple.run)
	}
	if withTagged {
		registry.Register(c.tagged.run, "simpletag")
	}
	if withDeployment {
		registry.RegisterDeploy(c.deployment.run, "deploymenttag")
	}
	return registry
}

// TestRegisterAndRunChecks runs checks registered without tags, with tags and
// as a deployment check. Django registers once with the decorator and once with
// the function call; the two Go forms are Register/RegisterDeploy and Add. Run
// returns the messages in registration order, so the port compares without
// sorting.
//
// Ported from Django: tests/check_framework/tests.py::SystemCheckFrameworkTests.test_register_and_run_checks
func TestRegisterAndRunChecks(t *testing.T) {
	calls := 0
	f := func(*checks.Context) []checks.Message {
		calls++
		return []checks.Message{checks.Info("1"), checks.Info("2"), checks.Info("3")}
	}
	f2 := func(*checks.Context) []checks.Message { return []checks.Message{checks.Info("4")} }
	f3 := func(*checks.Context) []checks.Message { return []checks.Message{checks.Info("5")} }

	registry := checks.NewRegistry()
	registry.Register(f)
	registry.Register(f2, "tag1", "tag2")
	registry.RegisterDeploy(f3, "tag2")

	registry2 := checks.NewRegistry()
	registry2.Add(
		checks.Check{Run: f},
		checks.Check{Run: f2, Tags: []string{"tag1", "tag2"}},
		checks.Check{Run: f3, Tags: []string{"tag2"}, Deploy: true},
	)

	errors := registry.Run(nil, checks.RunOptions{})
	errors2 := registry2.Run(nil, checks.RunOptions{})
	djangotest.AssertEqual(t, errors, errors2)
	djangotest.AssertEqual(t, messageTexts(errors), []string{"1", "2", "3", "4"})
	djangotest.AssertEqual(t, calls, 2)

	errors = registry.Run(nil, checks.RunOptions{Tags: []string{"tag1"}})
	errors2 = registry2.Run(nil, checks.RunOptions{Tags: []string{"tag1"}})
	djangotest.AssertEqual(t, errors, errors2)
	djangotest.AssertEqual(t, messageTexts(errors), []string{"4"})

	errors = registry.Run(nil, checks.RunOptions{Tags: []string{"tag1", "tag2"}, Deploy: true})
	errors2 = registry2.Run(nil, checks.RunOptions{Tags: []string{"tag1", "tag2"}, Deploy: true})
	djangotest.AssertEqual(t, errors, errors2)
	djangotest.AssertEqual(t, messageTexts(errors), []string{"4", "5"})
}

// TestRunChecksDatabaseExclusion runs a check tagged database only when the
// context names databases.
//
// Ported from Django: tests/check_framework/tests.py::SystemCheckFrameworkTests.test_run_checks_database_exclusion
func TestRunChecksDatabaseExclusion(t *testing.T) {
	registry := checks.NewRegistry()
	databaseErrors := []checks.Message{checks.Warning("Database Check")}
	registry.Register(func(*checks.Context) []checks.Message { return databaseErrors }, checks.TagDatabase)

	errors := registry.Run(&checks.Context{}, checks.RunOptions{})
	assertMessages(t, errors)

	errors = registry.Run(&checks.Context{Databases: []string{"default"}}, checks.RunOptions{})
	assertMessages(t, errors, databaseErrors...)
}

// TestPrinting prints the object, the message and the hint.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing
func TestPrinting(t *testing.T) {
	e := checks.Error("Message", checks.Hint("Hint"), checks.Obj(dummyObj{}))
	djangotest.AssertEqual(t, e.String(), "obj: Message\n\tHINT: Hint")
}

// TestPrintingNoHint prints no hint line for a message without a hint.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing_no_hint
func TestPrintingNoHint(t *testing.T) {
	e := checks.Error("Message", checks.Obj(dummyObj{}))
	djangotest.AssertEqual(t, e.String(), "obj: Message")
}

// TestPrintingNoObject prints "?" for a message without an object.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing_no_object
func TestPrintingNoObject(t *testing.T) {
	e := checks.Error("Message", checks.Hint("Hint"))
	djangotest.AssertEqual(t, e.String(), "?: Message\n\tHINT: Hint")
}

// TestPrintingWithGivenID prints the id in parentheses before the message.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing_with_given_id
func TestPrintingWithGivenID(t *testing.T) {
	e := checks.Error("Message", checks.Hint("Hint"), checks.Obj(dummyObj{}), checks.ID("ID"))
	djangotest.AssertEqual(t, e.String(), "obj: (ID) Message\n\tHINT: Hint")
}

// TestPrintingFieldError prints a field as "<app>.<Model>.<field>". An
// orm.Field does not know its model, so a Go message about a field carries that
// text as its object. The port prints the message the model checks build for a
// field instead of a message built around a field object.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing_field_error
func TestPrintingFieldError(t *testing.T) {
	type SimpleModel struct {
		Field int64 `orm:"pk;null"`
	}
	reg := modelRegistry(t, "check_framework", &SimpleModel{})

	msgs := modelMessages(reg)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(msgs), msgs)
	}
	expected := "check_framework.SimpleModel.field: (fields.E007) Primary keys must not have null=True." +
		"\n\tHINT: Set null=False on the field, or remove primary_key=True argument."
	djangotest.AssertEqual(t, msgs[0].String(), expected)
}

// TestPrintingModelError prints a model as its label.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_printing_model_error
func TestPrintingModelError(t *testing.T) {
	reg := modelRegistry(t, "check_framework", &SimpleModel{})
	model, _ := reg.ModelOf(&SimpleModel{})

	e := checks.Error("Error", checks.Obj(model))
	djangotest.AssertEqual(t, e.String(), "check_framework.SimpleModel: Error")
}

// TestEqualToSameConstructedCheck compares two messages built from the same
// arguments.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_equal_to_same_constructed_check
func TestEqualToSameConstructedCheck(t *testing.T) {
	reg := modelRegistry(t, "check_framework", &SimpleModel{})
	model, _ := reg.ModelOf(&SimpleModel{})

	e1 := checks.Error("Error", checks.Obj(model))
	e2 := checks.Error("Error", checks.Obj(model))
	djangotest.AssertEqual(t, e1, e2)
}

// TestNotEqualToDifferentConstructedCheck compares two messages that differ in
// their text.
//
// Ported from Django: tests/check_framework/tests.py::MessageTests.test_not_equal_to_different_constructed_check
func TestNotEqualToDifferentConstructedCheck(t *testing.T) {
	reg := modelRegistry(t, "check_framework", &SimpleModel{})
	model, _ := reg.ModelOf(&SimpleModel{})

	e1 := checks.Error("Error", checks.Obj(model))
	e2 := checks.Error("Error2", checks.Obj(model))
	djangotest.AssertNotEqual(t, e1, e2)
}

// TestSimpleCall runs every check with the context of the run.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_simple_call
func TestSimpleCall(t *testing.T) {
	c := newCommandChecks()
	registry := c.registry(true, true, false)

	registry.Run(&checks.Context{Databases: []string{"default", "other"}}, checks.RunOptions{})

	want := []*checks.Context{{Databases: []string{"default", "other"}}}
	djangotest.AssertEqual(t, c.simple.contexts, want)
	djangotest.AssertEqual(t, c.tagged.contexts, want)
}

// TestGivenApp passes the app labels of the run to every check.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_given_app
func TestGivenApp(t *testing.T) {
	c := newCommandChecks()
	registry := c.registry(true, true, false)

	registry.Run(
		&checks.Context{Apps: []string{"auth", "admin"}, Databases: []string{"default", "other"}},
		checks.RunOptions{},
	)

	want := []*checks.Context{{Apps: []string{"auth", "admin"}, Databases: []string{"default", "other"}}}
	djangotest.AssertEqual(t, c.simple.contexts, want)
	djangotest.AssertEqual(t, c.tagged.contexts, want)
}

// TestGivenTag runs only the checks that carry the tag.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_given_tag
func TestGivenTag(t *testing.T) {
	c := newCommandChecks()
	registry := c.registry(true, true, false)

	registry.Run(
		&checks.Context{Databases: []string{"default", "other"}},
		checks.RunOptions{Tags: []string{"simpletag"}},
	)

	djangotest.AssertEqual(t, len(c.simple.contexts), 0)
	djangotest.AssertEqual(t, c.tagged.contexts, []*checks.Context{{Databases: []string{"default", "other"}}})
}

// TestInvalidTag reports an unknown tag. The command turns that into its error
// 'There is no system check with the "missingtag" tag.'; the registry answers
// the question behind it.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_invalid_tag
func TestInvalidTag(t *testing.T) {
	registry := newCommandChecks().registry(true, true, false)

	djangotest.AssertEqual(t, registry.TagExists("missingtag", false), false)
	djangotest.AssertEqual(t, registry.TagExists("simpletag", false), true)
}

// TestListTagsEmpty lists no tag for checks without tags. The command prints
// the tags joined by line ends, which the port does with the result.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_list_tags_empty
func TestListTagsEmpty(t *testing.T) {
	registry := newCommandChecks().registry(true, false, false)

	djangotest.AssertEqual(t, strings.Join(registry.TagsAvailable(false), "\n")+"\n", "\n")
}

// TestListTags lists the tag of a tagged check.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_list_tags
func TestListTags(t *testing.T) {
	registry := newCommandChecks().registry(false, true, false)

	djangotest.AssertEqual(t, strings.Join(registry.TagsAvailable(false), "\n")+"\n", "simpletag\n")
}

// TestListDeploymentCheckOmitted leaves the tags of deployment checks out of
// the list of a run without deployment checks.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_list_deployment_check_omitted
func TestListDeploymentCheckOmitted(t *testing.T) {
	registry := newCommandChecks().registry(false, true, true)

	djangotest.AssertEqual(t, strings.Join(registry.TagsAvailable(false), "\n")+"\n", "simpletag\n")
}

// TestListDeploymentCheckIncluded lists the tags of deployment checks for a
// run with them, sorted with the others.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_list_deployment_check_included
func TestListDeploymentCheckIncluded(t *testing.T) {
	registry := newCommandChecks().registry(false, true, true)

	djangotest.AssertEqual(t, strings.Join(registry.TagsAvailable(true), "\n")+"\n", "deploymenttag\nsimpletag\n")
}

// TestTagsDeploymentCheckOmitted does not know the tag of a deployment check
// in a run without deployment checks, so the command refuses it.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_tags_deployment_check_omitted
func TestTagsDeploymentCheckOmitted(t *testing.T) {
	registry := newCommandChecks().registry(false, true, true)

	djangotest.AssertEqual(t, registry.TagExists("deploymenttag", false), false)
}

// TestTagsDeploymentCheckIncluded runs a deployment check selected by its tag
// and shows its message.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_tags_deployment_check_included
func TestTagsDeploymentCheckIncluded(t *testing.T) {
	registry := newCommandChecks().registry(false, true, true)

	djangotest.AssertEqual(t, registry.TagExists("deploymenttag", true), true)
	msgs := registry.Run(&checks.Context{}, checks.RunOptions{Tags: []string{"deploymenttag"}, Deploy: true})
	report := checks.Summarize(msgs, nil, checks.LevelError, true)

	if !strings.Contains(report.Text, "Deployment Check") {
		t.Errorf("report %q does not contain the deployment check message", report.Text)
	}
	djangotest.AssertEqual(t, report.Issues, 1)
}

// TestFailLevel fails a run with a warning when the fail level is WARNING.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_fail_level
func TestFailLevel(t *testing.T) {
	registry := newCommandChecks().registry(false, true, false)
	level, err := checks.ParseLevel("WARNING")
	if err != nil {
		t.Fatalf("ParseLevel: %v", err)
	}

	msgs := registry.Run(&checks.Context{}, checks.RunOptions{})

	djangotest.AssertEqual(t, checks.Summarize(msgs, nil, level, true).Failed, true)
	djangotest.AssertEqual(t, checks.Summarize(msgs, nil, checks.LevelError, true).Failed, false)
}

// TestDatabaseSystemChecks runs a check tagged database only for a run that
// names databases, and with those names.
//
// Ported from Django: tests/check_framework/tests.py::CheckCommandTests.test_database_system_checks
func TestDatabaseSystemChecks(t *testing.T) {
	databaseCheck := &recorder{}
	registry := checks.NewRegistry()
	registry.Register(databaseCheck.run, checks.TagDatabase)

	registry.Run(&checks.Context{}, checks.RunOptions{})
	djangotest.AssertEqual(t, len(databaseCheck.contexts), 0)

	registry.Run(&checks.Context{Databases: []string{"default"}}, checks.RunOptions{})
	djangotest.AssertEqual(t, databaseCheck.contexts, []*checks.Context{{Databases: []string{"default"}}})
}

// TestSilencedError reports a silenced error as no issue and does not fail.
// The command writes that line to stdout and nothing to stderr; the report
// says the same with its text and with zero issues.
//
// Ported from Django: tests/check_framework/tests.py::SilencingCheckTests.test_silenced_error
func TestSilencedError(t *testing.T) {
	registry := checks.NewRegistry()
	registry.Register(func(*checks.Context) []checks.Message {
		return []checks.Message{checks.Error("Error", checks.ID("myerrorcheck.E001"))}
	})

	msgs := registry.Run(&checks.Context{}, checks.RunOptions{})
	report := checks.Summarize(msgs, []string{"myerrorcheck.E001"}, checks.LevelError, true)

	djangotest.AssertEqual(t, report.Text, "System check identified no issues (1 silenced).\n")
	djangotest.AssertEqual(t, report.Issues, 0)
	djangotest.AssertEqual(t, report.Failed, false)
}

// TestSilencedWarning reports a silenced warning as no issue.
//
// Ported from Django: tests/check_framework/tests.py::SilencingCheckTests.test_silenced_warning
func TestSilencedWarning(t *testing.T) {
	registry := checks.NewRegistry()
	registry.Register(func(*checks.Context) []checks.Message {
		return []checks.Message{checks.Warning("Warning", checks.ID("mywarningcheck.E001"))}
	})

	msgs := registry.Run(&checks.Context{}, checks.RunOptions{})
	report := checks.Summarize(msgs, []string{"mywarningcheck.E001"}, checks.LevelError, true)

	djangotest.AssertEqual(t, report.Text, "System check identified no issues (1 silenced).\n")
	djangotest.AssertEqual(t, report.Issues, 0)
	djangotest.AssertEqual(t, report.Failed, false)
}
