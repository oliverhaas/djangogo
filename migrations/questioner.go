package migrations

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/internal/naming"
	"github.com/oliverhaas/djangogo/orm"
)

// Questioner answers what the autodetector cannot decide from the two states
// alone, the counterpart of Django's MigrationQuestioner.
type Questioner interface {
	// AskRename reports whether the field oldName of model was renamed to
	// newName. field is the new field.
	AskRename(model *ModelState, oldName, newName string, field FieldState) (bool, error)
	// AskRenameModel reports whether oldModel was renamed to newModel.
	AskRenameModel(oldModel, newModel *ModelState) (bool, error)
	// AskNotNullAddition returns a one-off default for the existing rows when
	// a NOT NULL field without a default is added to model.
	AskNotNullAddition(model *ModelState, field FieldState) (any, error)
	// AskNotNullAlteration returns a one-off default for the rows that hold
	// NULL when field becomes NOT NULL. A nil value leaves those rows to the
	// developer.
	AskNotNullAlteration(model *ModelState, field FieldState) (any, error)
}

// CommandError is an error of a management command whose message is meant for
// the user as it is. Code is the exit status Django uses for the error.
type CommandError struct {
	Message string
	Code    int
}

// Error returns the message.
func (e *CommandError) Error() string { return e.Message }

// FixedQuestioner answers every question with a fixed value. It is the
// counterpart of Django's MigrationQuestioner with a defaults dict and is
// meant for tests and scripts.
type FixedQuestioner struct {
	// Rename is the answer to every field rename question.
	Rename bool
	// RenameModel is the answer to every model rename question.
	RenameModel bool
	// NotNullAddition is the one-off default for every added NOT NULL field.
	NotNullAddition any
	// NotNullAlteration is the one-off default for every field that becomes
	// NOT NULL.
	NotNullAlteration any
}

// AskRename returns Rename.
func (q FixedQuestioner) AskRename(*ModelState, string, string, FieldState) (bool, error) {
	return q.Rename, nil
}

// AskRenameModel returns RenameModel.
func (q FixedQuestioner) AskRenameModel(*ModelState, *ModelState) (bool, error) {
	return q.RenameModel, nil
}

// AskNotNullAddition returns NotNullAddition.
func (q FixedQuestioner) AskNotNullAddition(*ModelState, FieldState) (any, error) {
	return q.NotNullAddition, nil
}

// AskNotNullAlteration returns NotNullAlteration.
func (q FixedQuestioner) AskNotNullAlteration(*ModelState, FieldState) (any, error) {
	return q.NotNullAlteration, nil
}

// PotentialRename is a removed and an added field (or model) that look like a
// rename which the questioner did not confirm. The generated migration drops
// the old column or table and creates an empty new one.
type PotentialRename struct {
	// Model is the model of a field rename, "app.Model". It is empty for a
	// model rename.
	Model string
	// From and To are the old and the new field name, or the old and the new
	// model label.
	From string
	To   string
}

// String renders a field rename as "app.Model.From -> app.Model.To" and a
// model rename as "From -> To".
func (r PotentialRename) String() string {
	if r.Model == "" {
		return r.From + " -> " + r.To
	}
	return r.Model + "." + r.From + " -> " + r.Model + "." + r.To
}

// IsModel reports whether a model, not a field, looks renamed.
func (r PotentialRename) IsModel() bool { return r.Model == "" }

// NonInteractiveQuestioner never prompts, the counterpart of Django's
// NonInteractiveMigrationQuestioner. It answers no to every rename question
// and records the pair, so the caller can warn about the data that is lost.
type NonInteractiveQuestioner struct {
	// Log, when set, receives the messages Django logs.
	Log func(msg string)
	// Declined collects the renames that were not confirmed.
	Declined []PotentialRename
}

// AskRename records the pair and answers no.
func (q *NonInteractiveQuestioner) AskRename(model *ModelState, oldName, newName string, _ FieldState) (bool, error) {
	q.Declined = append(q.Declined, PotentialRename{Model: model.Label(), From: oldName, To: newName})
	return false, nil
}

// AskRenameModel records the pair and answers no.
func (q *NonInteractiveQuestioner) AskRenameModel(oldModel, newModel *ModelState) (bool, error) {
	q.Declined = append(q.Declined, PotentialRename{From: oldModel.Label(), To: newModel.Label()})
	return false, nil
}

// AskNotNullAddition fails: without a prompt nobody can supply the default.
// The error carries Django's message and exit status 3.
func (q *NonInteractiveQuestioner) AskNotNullAddition(model *ModelState, field FieldState) (any, error) {
	return nil, &CommandError{
		Message: fmt.Sprintf("Field '%s' on model '%s' not migrated: it is impossible to add a "+
			"non-nullable field without specifying a default.", field.Attr(), lowerName(model.Name)),
		Code: 3,
	}
}

// AskNotNullAlteration logs Django's message and supplies no default.
func (q *NonInteractiveQuestioner) AskNotNullAlteration(model *ModelState, field FieldState) (any, error) {
	if q.Log != nil {
		q.Log(fmt.Sprintf("Field '%s' on model '%s' given a default of NOT PROVIDED and must be corrected.",
			field.Attr(), lowerName(model.Name)))
	}
	return nil, nil
}

// InteractiveQuestioner asks on a terminal, the counterpart of Django's
// InteractiveMigrationQuestioner. The question texts are Django's. End of
// input counts as the default answer of a yes/no question and cancels a
// question that has no default.
type InteractiveQuestioner struct {
	in     *bufio.Reader
	out    io.Writer
	dryRun bool
}

// NewInteractiveQuestioner returns a questioner that prompts on out and reads
// the answers from in. With dryRun it does not ask for defaults, as Django.
func NewInteractiveQuestioner(in io.Reader, out io.Writer, dryRun bool) *InteractiveQuestioner {
	return &InteractiveQuestioner{in: bufio.NewReader(in), out: out, dryRun: dryRun}
}

// errCancelled is returned when the input ends in the middle of a question.
var errCancelled = &CommandError{Message: "Cancelled.", Code: 1}

// readLine reads one line without its line ending. ok is false at end of input.
func (q *InteractiveQuestioner) readLine() (line string, ok bool, err error) {
	line, err = q.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", false, nil
	}
	return strings.TrimRight(line, "\r\n"), true, nil
}

// boolInput asks a yes/no question. An empty answer selects def.
func (q *InteractiveQuestioner) boolInput(question string, def bool) (bool, error) {
	_, _ = fmt.Fprint(q.out, question+" ")
	for {
		line, ok, err := q.readLine()
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(line)
		if !ok || line == "" {
			return def, nil
		}
		switch line[0] {
		case 'y', 'Y':
			return true, nil
		case 'n', 'N':
			return false, nil
		}
		_, _ = fmt.Fprint(q.out, "Please answer yes or no: ")
	}
}

// choiceInput prints the question with numbered choices and returns the
// number of the selected one, starting at 1.
func (q *InteractiveQuestioner) choiceInput(question string, choices []string) (int, error) {
	_, _ = fmt.Fprintln(q.out, question)
	for i, choice := range choices {
		_, _ = fmt.Fprintf(q.out, " %d) %s\n", i+1, choice)
	}
	_, _ = fmt.Fprint(q.out, "Select an option: ")
	for {
		line, ok, err := q.readLine()
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, errCancelled
		}
		if n, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && n > 0 && n <= len(choices) {
			return n, nil
		}
		_, _ = fmt.Fprint(q.out, "Please select a valid option: ")
	}
}

// askDefault prompts for a one-off default until the input parses as a value
// of the field's kind. Django evaluates a Python expression here; a Go
// literal takes its place.
func (q *InteractiveQuestioner) askDefault(field FieldState) (any, error) {
	_, _ = fmt.Fprintln(q.out, "Please enter the default value as a Go literal.")
	if field.Kind == orm.KindDateTime {
		_, _ = fmt.Fprintln(q.out, "Enter now to use the time at which the migration runs.")
	}
	_, _ = fmt.Fprintln(q.out, "Type 'exit' to exit this prompt")
	for {
		_, _ = fmt.Fprint(q.out, ">>> ")
		line, ok, err := q.readLine()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errCancelled
		}
		line = strings.TrimSpace(line)
		switch line {
		case "":
			_, _ = fmt.Fprintln(q.out, "Please enter a value, or 'exit' (without quotes) to exit.")
			continue
		case "exit":
			return nil, errCancelled
		}
		value, err := ParseDefault(field, line)
		if err != nil {
			_, _ = fmt.Fprintln(q.out, err.Error())
			continue
		}
		return value, nil
	}
}

// ParseDefault parses text as a default value for field: an integer for an
// integer or foreign-key field, true or false for a boolean field, a string
// (quoted as in Go, or taken as typed) for a text field, and "now" for a
// date-time field.
func ParseDefault(field FieldState, text string) (any, error) {
	switch field.Kind {
	case orm.KindBool:
		v, err := strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("migrations: %q is not a boolean (true or false)", text)
		}
		return v, nil
	case orm.KindChar, orm.KindText:
		if strings.HasPrefix(text, `"`) || strings.HasPrefix(text, "`") {
			v, err := strconv.Unquote(text)
			if err != nil {
				return nil, fmt.Errorf("migrations: %s is not a valid Go string literal", text)
			}
			return v, nil
		}
		return text, nil
	case orm.KindDateTime:
		if text != "now" {
			return nil, errors.New("migrations: the only default for a date-time field is now")
		}
		return Now, nil
	default:
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrations: %q is not an integer", text)
		}
		return v, nil
	}
}

// errQuit is returned when the developer chooses to quit and fix the model.
var errQuit = &CommandError{
	Message: "No migration was made. Define a default value for the field in the model and run makemigrations again.",
	Code:    3,
}

// AskRename asks Django's question about a renamed field.
func (q *InteractiveQuestioner) AskRename(model *ModelState, oldName, newName string, field FieldState) (bool, error) {
	name := lowerName(model.Name)
	return q.boolInput(fmt.Sprintf("Was %s.%s renamed to %s.%s (a %s)? [y/N]",
		name, naming.SnakeCase(oldName), name, naming.SnakeCase(newName), fieldClassName(field)), false)
}

// AskRenameModel asks Django's question about a renamed model.
func (q *InteractiveQuestioner) AskRenameModel(oldModel, newModel *ModelState) (bool, error) {
	return q.boolInput(fmt.Sprintf("Was the model %s.%s renamed to %s? [y/N]",
		oldModel.App, oldModel.Name, newModel.Name), false)
}

// AskNotNullAddition asks for a one-off default or lets the developer quit.
func (q *InteractiveQuestioner) AskNotNullAddition(model *ModelState, field FieldState) (any, error) {
	if q.dryRun {
		return nil, nil
	}
	choice, err := q.choiceInput(
		fmt.Sprintf("It is impossible to add a non-nullable field '%s' to %s without specifying a default. "+
			"This is because the database needs something to populate existing rows.\n"+
			"Please select a fix:", field.Attr(), lowerName(model.Name)),
		[]string{
			"Provide a one-off default now (will be set on all existing rows with a null value for this column)",
			"Quit and manually define a default value in the model.",
		})
	if err != nil {
		return nil, err
	}
	if choice == 2 {
		return nil, errQuit
	}
	return q.askDefault(field)
}

// AskNotNullAlteration asks for a one-off default, lets the developer handle
// the NULL rows by hand, or quit.
func (q *InteractiveQuestioner) AskNotNullAlteration(model *ModelState, field FieldState) (any, error) {
	if q.dryRun {
		return nil, nil
	}
	choice, err := q.choiceInput(
		fmt.Sprintf("It is impossible to change a nullable field '%s' on %s to non-nullable without providing a "+
			"default. This is because the database needs something to populate existing rows.\n"+
			"Please select a fix:", field.Attr(), lowerName(model.Name)),
		[]string{
			"Provide a one-off default now (will be set on all existing rows with a null value for this column)",
			"Ignore for now. Existing rows that contain NULL values will have to be handled manually, " +
				"for example with a RunGo or RunSQL operation.",
			"Quit and manually define a default value in the model.",
		})
	if err != nil {
		return nil, err
	}
	switch choice {
	case 2:
		return nil, nil
	case 3:
		return nil, errQuit
	}
	return q.askDefault(field)
}
