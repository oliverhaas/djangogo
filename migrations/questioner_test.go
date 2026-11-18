package migrations_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
)

// An answer that is neither yes nor no is asked for again.
func TestBoolInputAsksAgain(t *testing.T) {
	questioner, prompt := newQuestioner("maybe\nYes\n")
	value, err := questioner.BoolInput("Proceed?", false)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, true)
	djangotest.AssertEqual(t, prompt.String(), "Proceed? Please answer yes or no: ")
}

// The rename questions carry Django's texts and default to no.
func TestInteractiveRenameQuestions(t *testing.T) {
	questioner, prompt := newQuestioner("y\n\n")
	renamed, err := questioner.AskRename(questionModel, "FieldName", "NewName", questionField)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, renamed, true)
	djangotest.AssertEqual(t, prompt.String(),
		"Was modelname.field_name renamed to modelname.new_name (a IntegerField)? [y/N] ")

	prompt.Reset()
	newModel := &migrations.ModelState{App: "migrations", Name: "Renamed", Table: "migrations_renamed"}
	renamed, err = questioner.AskRenameModel(questionModel, newModel)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, renamed, false)
	djangotest.AssertEqual(t, prompt.String(), "Was the model migrations.ModelName renamed to Renamed? [y/N] ")
}

// A one-off default is asked for after the first choice, and the last choice
// quits with exit status 3.
func TestInteractiveNotNullQuestions(t *testing.T) {
	questioner, prompt := newQuestioner("1\n42\n")
	value, err := questioner.AskNotNullAddition(questionModel, questionField)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, any(int64(42)))
	assertPrompted(t, prompt, "It is impossible to add a non-nullable field 'field_name' to modelname without "+
		"specifying a default. This is because the database needs something to populate existing rows.\n"+
		"Please select a fix:\n"+
		" 1) Provide a one-off default now (will be set on all existing rows with a null value for this column)\n"+
		" 2) Quit and manually define a default value in the model.\n"+
		"Select an option: ")

	const quit = "No migration was made. Define a default value for the field in the model and run makemigrations again."
	questioner, _ = newQuestioner("2\n")
	_, err = questioner.AskNotNullAddition(questionModel, questionField)
	assertExit(t, err, quit, 3)
	questioner, _ = newQuestioner("3\n")
	_, err = questioner.AskNotNullAlteration(questionModel, questionField)
	assertExit(t, err, quit, 3)
}

// A dry run writes no migration, so it asks for no default.
func TestInteractiveDryRunAsksNoDefault(t *testing.T) {
	prompt := &bytes.Buffer{}
	dry := migrations.NewInteractiveQuestioner(strings.NewReader(""), prompt, true)
	for _, ask := range []func(*migrations.ModelState, migrations.FieldState) (any, error){
		dry.AskNotNullAddition, dry.AskNotNullAlteration,
	} {
		value, err := ask(questionModel, questionField)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, value, nil)
	}
	djangotest.AssertEqual(t, prompt.String(), "")

	// Without the dry run the same question needs an answer.
	questioner, _ := newQuestioner("")
	_, err := questioner.AskNotNullAddition(questionModel, questionField)
	assertExit(t, err, "Cancelled.", 1)
}

// The non-interactive questioner declines every rename and remembers it,
// refuses a NOT NULL addition and logs a NOT NULL alteration.
func TestNonInteractiveQuestioner(t *testing.T) {
	var logged []string
	questioner := &migrations.NonInteractiveQuestioner{Log: func(msg string) { logged = append(logged, msg) }}

	renamed, err := questioner.AskRename(questionModel, "FieldName", "NewName", questionField)
	if err != nil || renamed {
		t.Errorf("AskRename = %v, %v, want false", renamed, err)
	}
	newModel := &migrations.ModelState{App: "migrations", Name: "Renamed", Table: "migrations_renamed"}
	renamed, err = questioner.AskRenameModel(questionModel, newModel)
	if err != nil || renamed {
		t.Errorf("AskRenameModel = %v, %v, want false", renamed, err)
	}
	djangotest.AssertEqual(t, len(questioner.Declined), 2)
	djangotest.AssertEqual(t, questioner.Declined[0].IsModel(), false)
	djangotest.AssertEqual(t, questioner.Declined[0].String(),
		"migrations.ModelName.FieldName -> migrations.ModelName.NewName")
	djangotest.AssertEqual(t, questioner.Declined[1].IsModel(), true)
	djangotest.AssertEqual(t, questioner.Declined[1].String(), "migrations.ModelName -> migrations.Renamed")

	_, err = questioner.AskNotNullAddition(questionModel, questionField)
	assertExit(t, err, "Field 'field_name' on model 'modelname' not migrated: it is impossible to add a "+
		"non-nullable field without specifying a default.", 3)

	value, err := questioner.AskNotNullAlteration(questionModel, questionField)
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, value, nil)
	djangotest.AssertSequenceEqual(t, logged, []string{
		"Field 'field_name' on model 'modelname' given a default of NOT PROVIDED and must be corrected.",
	})
}

// ParseDefault refuses a value that is not a literal of the kind of the field.
func TestParseDefault(t *testing.T) {
	for _, tt := range []struct {
		field migrations.FieldState
		input string
		want  any
	}{
		{questionField, "-3", int64(-3)},
		{fkField("Pony", "pony_id", "zoo.Pony"), "7", int64(7)},
		{yellowField, "`a \"raw\" string`", `a "raw" string`},
		{yellowField, "it's plain", "it's plain"},
	} {
		value, err := migrations.ParseDefault(tt.field, tt.input)
		if err != nil {
			t.Errorf("ParseDefault(%q): %v", tt.input, err)
		}
		djangotest.AssertEqual(t, value, tt.want)
	}
	for _, tt := range []struct {
		field migrations.FieldState
		input string
	}{
		{questionField, "1.5"},
		{yellowField, `"unterminated`},
	} {
		if value, err := migrations.ParseDefault(tt.field, tt.input); err == nil {
			t.Errorf("ParseDefault(%q) = %v, want an error", tt.input, value)
		}
	}
}
