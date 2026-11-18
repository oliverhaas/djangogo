package manage

import (
	"bytes"
	"strings"
	"testing"
)

// fakeCmd records its runs; it implements none of the optional interfaces,
// so its arguments reach it unparsed.
type fakeCmd struct {
	name string
	ran  *[]string
}

func (c fakeCmd) Name() string { return c.name }
func (c fakeCmd) Help() string { return "help for " + c.name }
func (c fakeCmd) Run(args []string) error {
	*c.ran = append(*c.ran, c.name+":"+strings.Join(args, ","))
	return nil
}

// TestExecuteDispatches: a command without Flags gets its arguments as they
// are on the command line.
func TestExecuteDispatches(t *testing.T) {
	var ran []string
	r := NewRegistry()
	if err := r.Register(fakeCmd{"runserver", &ran}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := r.Execute([]string{"runserver", "--port", "9000"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(ran) != 1 || ran[0] != "runserver:--port,9000" {
		t.Errorf("ran = %v", ran)
	}
}

// TestExecuteUnknown: a name that is not registered is an error.
func TestExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	if err := r.Execute([]string{"nope"}); err == nil {
		t.Error("unknown command should error")
	}
}

// TestExecuteNoArgsPrintsUsage: without arguments Execute prints the main
// help, which names the commands, and returns nil.
func TestExecuteNoArgsPrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	r := NewRegistry()
	r.Out = &buf
	var ran []string
	_ = r.Register(fakeCmd{"runserver", &ran})

	if err := r.Execute(nil); err != nil {
		t.Fatalf("Execute(nil): %v", err)
	}
	if !strings.Contains(buf.String(), "runserver") {
		t.Errorf("usage output missing command name: %q", buf.String())
	}
}
