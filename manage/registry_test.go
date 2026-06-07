// manage/registry_test.go
package manage

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	if err := r.Execute([]string{"nope"}); err == nil {
		t.Error("unknown command should error")
	}
}

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
