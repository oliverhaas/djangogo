package conf_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
)

// mapEnv returns an environment lookup that reads the given variables.
func mapEnv(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := vars[name]
		return value, ok
	}
}

// activate makes the settings the active ones for the test and restores the
// former state afterwards. A test that uses it must not run in parallel.
func activate(t *testing.T, s conf.Settings) *conf.Settings {
	t.Helper()
	previous := conf.SwapActive(nil)
	t.Cleanup(func() { conf.SwapActive(previous) })
	return conf.Configure(s)
}

// unsetEnv removes a variable from the process environment for the test and
// restores it afterwards, also when the test sets it in between.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	// Setenv registers the cleanup that restores the former state.
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// errorLines returns the messages of a joined error, one per line; nil for a
// nil error.
func errorLines(err error) []string {
	if err == nil {
		return nil
	}
	return strings.Split(err.Error(), "\n")
}

// taggedField is a field of Settings that the environment can set.
type taggedField struct {
	// tag is the env tag: the variable name without a prefix.
	tag string
	// value is the field, addressable in the settings it was listed from.
	value reflect.Value
}

// taggedFields lists the fields of s that have an env tag, those of the nested
// structs included, in the order of their declaration.
func taggedFields(s *conf.Settings) []taggedField {
	var fields []taggedField
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if tag := f.Tag.Get("env"); tag != "" {
				fields = append(fields, taggedField{tag: tag, value: v.Field(i)})
				continue
			}
			if f.Type.Kind() == reflect.Struct {
				walk(v.Field(i))
			}
		}
	}
	walk(reflect.ValueOf(s).Elem())
	return fields
}
