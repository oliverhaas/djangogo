package checks

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Option sets an optional part of a Message in one of the constructors.
type Option func(*Message)

// Hint sets the hint of a message: what to do about the problem.
func Hint(hint string) Option {
	return func(m *Message) { m.Hint = hint }
}

// Obj sets the object a message is about. A message prints an object by its
// Label method when it has one, as an *orm.Model does, and with fmt otherwise.
// A message about a model field carries the text "<app>.<Model>.<attr>".
func Obj(obj any) Option {
	return func(m *Message) { m.Obj = obj }
}

// ID sets the id of a message, for example "security.W004". The id is what
// SILENCED_SYSTEM_CHECKS names.
func ID(id string) Option {
	return func(m *Message) { m.ID = id }
}

// newMessage builds a message of the given level and applies opts to it.
func newMessage(level Level, msg string, opts []Option) Message {
	m := Message{Level: level, Msg: msg}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// Debug returns a message of LevelDebug (django.core.checks.Debug).
func Debug(msg string, opts ...Option) Message {
	return newMessage(LevelDebug, msg, opts)
}

// Info returns a message of LevelInfo (django.core.checks.Info).
func Info(msg string, opts ...Option) Message {
	return newMessage(LevelInfo, msg, opts)
}

// Warning returns a message of LevelWarning (django.core.checks.Warning).
func Warning(msg string, opts ...Option) Message {
	return newMessage(LevelWarning, msg, opts)
}

// Error returns a message of LevelError (django.core.checks.Error).
func Error(msg string, opts ...Option) Message {
	return newMessage(LevelError, msg, opts)
}

// Critical returns a message of LevelCritical (django.core.checks.Critical).
func Critical(msg string, opts ...Option) Message {
	return newMessage(LevelCritical, msg, opts)
}

// labeled is an object that names itself in a message, as an *orm.Model does
// with its "<app>.<Model>" label.
type labeled interface {
	Label() string
}

// objectText returns the text a message prints for its object: "?" for no
// object, the label of an object that has one, and the fmt text otherwise. A
// nil pointer counts as no object, so a Label method is never called on one.
func objectText(obj any) string {
	if obj == nil {
		return "?"
	}
	if v := reflect.ValueOf(obj); v.Kind() == reflect.Pointer && v.IsNil() {
		return "?"
	}
	if l, ok := obj.(labeled); ok {
		return l.Label()
	}
	return fmt.Sprint(obj)
}

// String formats the message as Django's CheckMessage.__str__ does:
// "<obj>: (<id>) <msg>" and, on a second line that starts with a tab,
// "HINT: <hint>". The object is "?" when there is none, the id part is left out
// when there is no id and the hint line when there is no hint.
func (m Message) String() string {
	var b strings.Builder
	b.WriteString(objectText(m.Obj))
	b.WriteString(": ")
	if m.ID != "" {
		b.WriteString("(" + m.ID + ") ")
	}
	b.WriteString(m.Msg)
	if m.Hint != "" {
		b.WriteString("\n\tHINT: " + m.Hint)
	}
	return b.String()
}

// IsSerious reports whether the level of the message is at least level
// (Django's is_serious, whose default level is LevelError).
func (m Message) IsSerious(level Level) bool {
	return m.Level >= level
}

// IsSilenced reports whether the id of the message is in silenced, the value of
// SILENCED_SYSTEM_CHECKS (Django's is_silenced). A message without an id is
// never silenced.
func (m Message) IsSilenced(silenced []string) bool {
	return m.ID != "" && slices.Contains(silenced, m.ID)
}

// levelNames lists the named levels from the most to the least severe, the
// order in which Django's check command offers them for --fail-level.
var levelNames = []struct {
	level Level
	name  string
}{
	{LevelCritical, "CRITICAL"},
	{LevelError, "ERROR"},
	{LevelWarning, "WARNING"},
	{LevelInfo, "INFO"},
	{LevelDebug, "DEBUG"},
}

// String returns the name of the level: "DEBUG", "INFO", "WARNING", "ERROR" or
// "CRITICAL". A value that is none of the five prints as "Level(<n>)".
func (l Level) String() string {
	for _, ln := range levelNames {
		if ln.level == l {
			return ln.name
		}
	}
	return fmt.Sprintf("Level(%d)", int(l))
}

// ErrInvalidLevel is the error of ParseLevel for a name that is no level.
var ErrInvalidLevel = errors.New("invalid choice")

// ParseLevel returns the level with the given name, in any letter case. It is
// the inverse of Level.String and reads the value of check --fail-level. For
// another name the error wraps ErrInvalidLevel and reads as the argument
// parser of Django does: "invalid choice: 'x' (choose from 'CRITICAL', 'ERROR',
// 'WARNING', 'INFO', 'DEBUG')".
func ParseLevel(name string) (Level, error) {
	choices := make([]string, 0, len(levelNames))
	for _, ln := range levelNames {
		if strings.EqualFold(name, ln.name) {
			return ln.level, nil
		}
		choices = append(choices, "'"+ln.name+"'")
	}
	return 0, fmt.Errorf("%w: '%s' (choose from %s)", ErrInvalidLevel, name, strings.Join(choices, ", "))
}
