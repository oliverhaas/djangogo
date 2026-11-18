package migrations_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The model and the field the questions are asked about.
var (
	questionModel = &migrations.ModelState{App: "migrations", Name: "ModelName", Table: "migrations_modelname"}
	questionField = migrations.FieldState{Name: "FieldName", Column: "field_name", Kind: orm.KindInt}
)

// newQuestioner returns an interactive questioner that reads input and the
// buffer its prompts go to. The input takes the place of Django's mocked
// input(); its end stands for Ctrl-C.
func newQuestioner(input string) (*migrations.InteractiveQuestioner, *bytes.Buffer) {
	prompt := &bytes.Buffer{}
	return migrations.NewInteractiveQuestioner(strings.NewReader(input), prompt, false), prompt
}

// assertExit checks that err is a command error with the message and the exit
// status. It takes the place of assertRaises(SystemExit): the Go questioner
// returns the error and the command exits with its status.
func assertExit(t *testing.T, err error, message string, code int) {
	t.Helper()
	var cerr *migrations.CommandError
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %v (%T), want a CommandError", err, err)
	}
	djangotest.AssertEqual(t, cerr.Message, message)
	djangotest.AssertEqual(t, cerr.Code, code)
}

// assertPrompted checks that the prompt output contains text.
func assertPrompted(t *testing.T, prompt *bytes.Buffer, text string) {
	t.Helper()
	if !strings.Contains(prompt.String(), text) {
		t.Errorf("prompt output %q does not contain %q", prompt.String(), text)
	}
}

// Ported from Django: tests/migrations/test_questioner.py::QuestionerTests.test_ask_not_null_alteration
func TestAskNotNullAlteration(t *testing.T) {
	value, err := migrations.FixedQuestioner{}.AskNotNullAlteration(questionModel, questionField)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, nil)
}

// Django returns NOT_PROVIDED for "Ignore for now". The Go questioner returns
// no default, which leaves the rows that hold NULL to the developer.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerTests.test_ask_not_null_alteration_not_provided
func TestAskNotNullAlterationNotProvided(t *testing.T) {
	questioner, prompt := newQuestioner("2\n")
	value, err := questioner.AskNotNullAlteration(questionModel, questionField)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, nil)
	assertPrompted(t, prompt, "It is impossible to change a nullable field 'field_name' on modelname to "+
		"non-nullable without providing a default. This is because the database needs something to populate "+
		"existing rows.\nPlease select a fix:\n 1) Provide a one-off default now")
}

// Django evaluates the input as a Python expression, here a timedelta. The Go
// prompt takes a literal of the kind of the field, and "now" for a date-time
// field.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_default_timedelta
func TestQuestionerDefaultTimedelta(t *testing.T) {
	tests := []struct {
		kind  orm.Kind
		input string
		want  any
	}{
		{orm.KindInt, "86400", int64(86400)},
		{orm.KindBool, "true", true},
		{orm.KindChar, `"1 day"`, "1 day"},
		{orm.KindText, "1 day", "1 day"},
		{orm.KindDateTime, "now", migrations.Now},
	}
	for _, tt := range tests {
		questioner, prompt := newQuestioner(tt.input + "\n")
		field := questionField
		field.Kind = tt.kind
		value, err := questioner.AskDefault(field)
		if err != nil {
			t.Fatalf("AskDefault(%q): %v", tt.input, err)
		}
		djangotest.AssertEqual(t, value, tt.want)
		assertPrompted(t, prompt, "Please enter the default value as a Go literal.\n")
		assertPrompted(t, prompt, "Type 'exit' to exit this prompt\n>>> ")
	}
}

// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_no_user_entry
func TestQuestionerNoDefaultNoUserEntry(t *testing.T) {
	questioner, prompt := newQuestioner("\nexit\n")
	_, err := questioner.AskDefault(questionField)
	assertExit(t, err, "Cancelled.", 1)
	assertPrompted(t, prompt, "Please enter a value, or 'exit' (without quotes) to exit.")
}

// A literal that does not parse takes the place of Python's SyntaxError.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_syntax_error
func TestQuestionerNoDefaultSyntaxError(t *testing.T) {
	questioner, prompt := newQuestioner("bad code\nexit\n")
	_, err := questioner.AskDefault(questionField)
	assertExit(t, err, "Cancelled.", 1)
	assertPrompted(t, prompt, `migrations: "bad code" is not an integer`)
}

// The Go prompt resolves no names. The counterpart of Python's NameError is a
// word that is not a literal of a boolean field.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_name_error
func TestQuestionerNoDefaultNameError(t *testing.T) {
	questioner, prompt := newQuestioner("datetim\nexit\n")
	field := questionField
	field.Kind = orm.KindBool
	_, err := questioner.AskDefault(field)
	assertExit(t, err, "Cancelled.", 1)
	assertPrompted(t, prompt, `migrations: "datetim" is not a boolean (true or false)`)
}

// The counterpart of Python's AttributeError is an expression for a date-time
// field, which accepts only "now".
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_attribute_error
func TestQuestionerNoDefaultAttributeError(t *testing.T) {
	questioner, prompt := newQuestioner("datetime.dat\nexit\n")
	field := questionField
	field.Kind = orm.KindDateTime
	_, err := questioner.AskDefault(field)
	assertExit(t, err, "Cancelled.", 1)
	assertPrompted(t, prompt, "Enter now to use the time at which the migration runs.\n")
	assertPrompted(t, prompt, "migrations: the only default for a date-time field is now")
}

// Django writes "Cancelled." to the prompt and exits. The Go questioner
// returns it as the message of the command error.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_keyboard_interrupt
func TestQuestionerNoDefaultKeyboardInterrupt(t *testing.T) {
	questioner, _ := newQuestioner("")
	_, err := questioner.AskDefault(questionField)
	assertExit(t, err, "Cancelled.", 1)
}

// Django's _boolean_input without a default asks again after an empty answer.
// Every yes/no question of the Go questioner has a default, so the empty
// answer selects it and the second line is not read.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_default_no_user_entry_boolean
func TestQuestionerNoDefaultNoUserEntryBoolean(t *testing.T) {
	questioner, prompt := newQuestioner("\nn\n")
	value, err := questioner.BoolInput("Proceed?", false)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, false)
	djangotest.AssertEqual(t, prompt.String(), "Proceed? ")
}

// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_default_no_user_entry_boolean
func TestQuestionerDefaultNoUserEntryBoolean(t *testing.T) {
	questioner, _ := newQuestioner("\n")
	value, err := questioner.BoolInput("Proceed?", true)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, true)
}

// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_bad_user_choice
func TestQuestionerBadUserChoice(t *testing.T) {
	questioner, prompt := newQuestioner("10\ngarbage\n1\n")
	question := "Make a choice:"
	value, err := questioner.ChoiceInput(question, []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	assertPrompted(t, prompt, question+"\n 1) a\n 2) b\n 3) c\n")
	djangotest.AssertEqual(t, value, 1)
}

// Django writes "Cancelled." to the prompt and exits. The Go questioner
// returns it as the message of the command error.
//
// Ported from Django: tests/migrations/test_questioner.py::QuestionerHelperMethodsTests.test_questioner_no_choice_keyboard_interrupt
func TestQuestionerNoChoiceKeyboardInterrupt(t *testing.T) {
	questioner, prompt := newQuestioner("")
	question := "Make a choice:"
	_, err := questioner.ChoiceInput(question, []string{"a", "b", "c"})
	assertExit(t, err, "Cancelled.", 1)
	djangotest.AssertEqual(t, prompt.String(), question+"\n 1) a\n 2) b\n 3) c\nSelect an option: ")
}
