package djangogo

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
)

// shellReprSize is the number of rows a query shows, Django's
// REPR_OUTPUT_SIZE for the repr of a QuerySet.
const shellReprSize = 20

// shellHelp lists the statements of the shell.
const shellHelp = `Statements:
  models                         list the models
  fields <Model>                 list the fields of a model
  <Model>.count()                count the rows
  <Model>.all()                  show the rows
  <Model>.filter(attr=value, ...) show the rows that match every condition
  sql <statement>                run raw SQL and show the rows it returns
  help                           show this help
  exit, quit                     leave the shell
A model is named by its name (Post) or its label (blog.Post). A value is a
quoted string, a number, True, False or None.
`

// queryRe matches a QuerySet-style statement: Model.method(arguments).
var queryRe = regexp.MustCompile(`^([A-Za-z_][\w.]*)\.(count|all|filter)\((.*)\)$`)

// shellCommand is Djan-Go-Go's shell. Go has no REPL to run code in, so
// instead of Django's Python shell it is a query shell over the model
// registry: it lists models and fields, counts and shows rows with
// QuerySet-style statements, and runs raw SQL.
type shellCommand struct {
	app     *Application
	command string
}

// Name returns the command name.
func (*shellCommand) Name() string { return "shell" }

// Help returns the help text.
func (*shellCommand) Help() string {
	return "Runs a query shell over the models of the project. Type 'help' in the shell for its statements."
}

// SkipSystemChecks reports that the shell runs no checks first
// (requires_system_checks = []).
func (*shellCommand) SkipSystemChecks() bool { return true }

// Flags declares the options of shell.
func (c *shellCommand) Flags(fs *flag.FlagSet) {
	const help = "Instead of opening an interactive shell, run a command and exit."
	fs.StringVar(&c.command, "command", "", help)
	fs.StringVar(&c.command, "c", "", help)
}

// Run runs the statements of --command, or reads statements from the input
// until exit or the end of the input.
func (c *shellCommand) Run(_ []string) error {
	ctx := context.Background()
	if c.command != "" {
		for line := range strings.SplitSeq(c.command, "\n") {
			if done := c.exec(ctx, line); done {
				break
			}
		}
		return nil
	}

	out := c.app.out()
	_, _ = fmt.Fprintf(out, "Djan-Go-Go %s query shell. Type 'help' for the statements, 'exit' to leave.\n", Version)
	scanner := bufio.NewScanner(c.app.in())
	for {
		_, _ = io.WriteString(out, ">>> ")
		if !scanner.Scan() {
			_, _ = io.WriteString(out, "\n")
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("djangogo: read statement: %w", err)
			}
			return nil
		}
		if done := c.exec(ctx, scanner.Text()); done {
			return nil
		}
	}
}

// exec runs one statement and reports whether it ends the shell. An error is
// printed and the shell goes on, as in an interactive interpreter.
func (c *shellCommand) exec(ctx context.Context, line string) (done bool) {
	line = strings.TrimSpace(line)
	var err error
	switch word, rest, _ := strings.Cut(line, " "); {
	case line == "":
	case line == "exit" || line == "quit" || line == "exit()" || line == "quit()":
		return true
	case line == "help":
		_, _ = io.WriteString(c.app.out(), shellHelp)
	case line == "models":
		c.listModels()
	case word == "fields":
		err = c.listFields(strings.TrimSpace(rest))
	case word == "sql":
		err = c.rawSQL(ctx, strings.TrimSpace(rest))
	default:
		err = c.query(ctx, line)
	}
	if err != nil {
		_, _ = fmt.Fprintf(c.app.errOut(), "Error: %v\n", err)
	}
	return false
}

// listModels prints every model with its table, by label.
func (c *shellCommand) listModels() {
	models := c.app.Models.Models()
	slices.SortFunc(models, func(a, b *orm.Model) int { return strings.Compare(a.Label(), b.Label()) })
	w := tabwriter.NewWriter(c.app.out(), 0, 0, 2, ' ', 0)
	for _, m := range models {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", m.Label(), m.Table())
	}
	_ = w.Flush()
}

// listFields prints the fields of a model: attribute, kind, column and flags.
func (c *shellCommand) listFields(name string) error {
	m, err := c.model(name)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(c.app.out(), 0, 0, 2, ' ', 0)
	for _, f := range m.Fields() {
		var flags []string
		if f.PrimaryKey {
			flags = append(flags, "primary key")
		}
		if f.Null {
			flags = append(flags, "null")
		}
		if f.Unique {
			flags = append(flags, "unique")
		}
		if f.Rel != nil && f.Rel.Target != nil {
			flags = append(flags, "-> "+f.Rel.Target.Label())
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Attr, f.Kind, f.Column, strings.Join(flags, ", "))
	}
	return w.Flush()
}

// model finds a model by label, by name, or by name ignoring case when that
// is unambiguous.
func (c *shellCommand) model(name string) (*orm.Model, error) {
	var folded []*orm.Model
	for _, m := range c.app.Models.Models() {
		if m.Label() == name || m.Name() == name {
			return m, nil
		}
		if strings.EqualFold(m.Name(), name) {
			folded = append(folded, m)
		}
	}
	if len(folded) == 1 {
		return folded[0], nil
	}
	return nil, fmt.Errorf("no model named %q; 'models' lists them", name)
}

// query runs a QuerySet-style statement. The SQL is built from the model
// metadata only: identifiers are quoted by the dialect, and every value is
// a bind parameter.
func (c *shellCommand) query(ctx context.Context, line string) error {
	match := queryRe.FindStringSubmatch(line)
	if match == nil {
		return fmt.Errorf("unknown statement %q; 'help' lists the statements", line)
	}
	m, err := c.model(match[1])
	if err != nil {
		return err
	}
	method, arguments := match[2], strings.TrimSpace(match[3])
	if method != "filter" && arguments != "" {
		return fmt.Errorf("%s() takes no arguments", method)
	}
	db := c.app.DB
	if db == nil {
		return errors.New("no database is configured")
	}
	d := db.Dialect()
	where, args, err := filterClause(m, d, arguments)
	if err != nil {
		return err
	}

	from := " FROM " + d.Quote(m.Table()) + where
	if method == "count" {
		var n int64
		if err := db.SQL().QueryRowContext(ctx, "SELECT COUNT(*)"+from, args...).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", m.Label(), err)
		}
		_, _ = fmt.Fprintln(c.app.out(), n)
		return nil
	}

	quoted := make([]string, 0, len(m.Fields()))
	kinds := make([]orm.Kind, 0, len(m.Fields()))
	for _, f := range m.Fields() {
		quoted = append(quoted, d.Quote(f.Column))
		kinds = append(kinds, f.Kind)
	}
	q := "SELECT " + strings.Join(quoted, ", ") + from +
		" ORDER BY " + d.Quote(m.PrimaryKey().Column) + " LIMIT " + strconv.Itoa(shellReprSize+1)
	rows, err := db.SQL().QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("query %s: %w", m.Label(), err)
	}
	return c.printRows(rows, kinds, shellReprSize)
}

// filterClause builds the WHERE clause of filter(attr=value, ...): every
// key must be the attribute or the column of a field of m.
func filterClause(m *orm.Model, d orm.Dialect, arguments string) (string, []any, error) {
	if arguments == "" {
		return "", nil, nil
	}
	pairs, err := parseShellArguments(arguments)
	if err != nil {
		return "", nil, err
	}
	conditions := make([]string, 0, len(pairs))
	args := make([]any, 0, len(pairs))
	for _, p := range pairs {
		f, ok := m.FieldByAttr(p.key)
		if !ok {
			f, ok = m.FieldByColumn(p.key)
		}
		if !ok {
			return "", nil, fmt.Errorf("cannot resolve keyword %q into field of %s", p.key, m.Label())
		}
		if p.value == nil {
			conditions = append(conditions, d.Quote(f.Column)+" IS NULL")
			continue
		}
		v, err := shellValue(f, *p.value)
		if err != nil {
			return "", nil, err
		}
		args = append(args, v)
		conditions = append(conditions, d.Quote(f.Column)+" = "+d.Placeholder(len(args)))
	}
	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}

// shellValue converts a literal to the Go type of the field's kind, so the
// database driver gets a value of the column's type.
func shellValue(f *orm.Field, literal string) (any, error) {
	switch f.Kind {
	case orm.KindAuto, orm.KindInt:
		n, err := strconv.ParseInt(literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("field %q expected a number but got %q", f.Attr, literal)
		}
		return n, nil
	case orm.KindBool:
		switch literal {
		case "True", "true", "1":
			return true, nil
		case "False", "false", "0":
			return false, nil
		}
		return nil, fmt.Errorf("field %q expected True or False but got %q", f.Attr, literal)
	default:
		return literal, nil
	}
}

// rawSQL runs a statement as it is and shows the rows it returns.
func (c *shellCommand) rawSQL(ctx context.Context, statement string) error {
	if statement == "" {
		return errors.New("sql needs a statement")
	}
	if c.app.DB == nil {
		return errors.New("no database is configured")
	}
	rows, err := c.app.DB.SQL().QueryContext(ctx, statement)
	if err != nil {
		return err
	}
	return c.printRows(rows, nil, -1)
}

// printRows prints rows as a table with a header of column names, at most
// limit rows (no limit when negative) and Django's truncation line after
// them. kinds, when given, are the kinds of the columns. It closes rows.
func (c *shellCommand) printRows(rows *sql.Rows, kinds []orm.Kind, limit int) error {
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		_, _ = fmt.Fprintln(c.app.out(), "OK")
		return rows.Err()
	}
	w := tabwriter.NewWriter(c.app.out(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(cols, "\t"))
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	shown, truncated := 0, false
	for rows.Next() {
		if limit >= 0 && shown == limit {
			truncated = true
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		cells := make([]string, len(values))
		for i, v := range values {
			if i < len(kinds) && kinds[i] == orm.KindBool {
				v = shellBool(v)
			}
			cells[i] = shellCell(v)
		}
		_, _ = fmt.Fprintln(w, strings.Join(cells, "\t"))
		shown++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if truncated {
		_, _ = fmt.Fprintln(w, "...(remaining elements truncated)...")
	}
	return w.Flush()
}

// shellCell formats one value of a row as Python shows it: NULL as None and
// a boolean as True or False.
func shellCell(v any) string {
	switch v := v.(type) {
	case nil:
		return "None"
	case bool:
		if v {
			return "True"
		}
		return "False"
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// shellBool returns the boolean a database stores as an integer (SQLite has
// no boolean type) and any other value unchanged.
func shellBool(v any) any {
	if n, ok := v.(int64); ok {
		return n != 0
	}
	return v
}

// shellArgument is one key=value argument of filter; a nil value is None.
type shellArgument struct {
	key   string
	value *string
}

// parseShellArguments parses "key=value, key='a, b', key=None". A value is
// a single- or double-quoted string (a backslash escapes the next
// character), None, or a bare word up to the next comma.
func parseShellArguments(s string) ([]shellArgument, error) {
	var out []shellArgument
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		key, rest, found := strings.Cut(s, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("expected key=value, got %q", s)
		}
		rest = strings.TrimSpace(rest)
		var value string
		quoted := rest != "" && (rest[0] == '\'' || rest[0] == '"')
		if quoted {
			var err error
			value, rest, err = quotedLiteral(rest)
			if err != nil {
				return nil, err
			}
		} else {
			value, rest, _ = strings.Cut(rest, ",")
			value = strings.TrimSpace(value)
			rest = "," + rest
		}
		arg := shellArgument{key: key, value: &value}
		if !quoted && (value == "None" || value == "null") {
			arg.value = nil
		}
		out = append(out, arg)
		rest = strings.TrimSpace(rest)
		if rest != "" && rest[0] != ',' {
			return nil, fmt.Errorf("expected a comma before %q", rest)
		}
		s = strings.TrimPrefix(rest, ",")
	}
	return out, nil
}

// quotedLiteral reads a quoted string from the start of s and returns its
// value and the rest of s.
func quotedLiteral(s string) (value, rest string, err error) {
	quote := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case ch == quote:
			return b.String(), s[i+1:], nil
		default:
			b.WriteByte(ch)
		}
	}
	return "", "", fmt.Errorf("unterminated string %s", s)
}

// Assert that shellCommand takes flags.
var _ manage.Flagger = (*shellCommand)(nil)
