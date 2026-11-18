package checks_test

import (
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// assertMessages asserts that got holds exactly want, in order. No message
// matches an empty want whether got is nil or an empty slice.
func assertMessages(t *testing.T, got []checks.Message, want ...checks.Message) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	djangotest.AssertEqual(t, got, want)
}

// withID returns the messages that carry one of ids, in their order. A Django
// test calls one check function; a port runs every built-in check through the
// registry and keeps the ids of that one function.
func withID(msgs []checks.Message, ids ...string) []checks.Message {
	var out []checks.Message
	for _, m := range msgs {
		if slices.Contains(ids, m.ID) {
			out = append(out, m)
		}
	}
	return out
}

// deployMessages runs the built-in deployment checks tagged security on s and
// returns the messages that carry one of ids.
func deployMessages(s conf.Settings, ids ...string) []checks.Message {
	ctx := &checks.Context{Settings: &s}
	msgs := checks.Default.Run(ctx, checks.RunOptions{Tags: []string{checks.TagSecurity}, Deploy: true})
	return withID(msgs, ids...)
}

// modelRegistry registers models under the app label app in a private
// registry. It resolves the relations and ignores a failure to do so: a test
// of a relation to a missing model needs exactly that state.
func modelRegistry(t *testing.T, app string, models ...any) *orm.Registry {
	t.Helper()
	reg := orm.NewRegistry()
	for _, m := range models {
		if _, err := reg.RegisterApp(app, m); err != nil {
			t.Fatalf("RegisterApp(%q, %T): %v", app, m, err)
		}
	}
	_ = reg.Resolve()
	return reg
}

// modelMessages runs the built-in checks tagged models on reg.
func modelMessages(reg *orm.Registry, apps ...string) []checks.Message {
	ctx := &checks.Context{Models: reg, Apps: apps}
	return checks.Default.Run(ctx, checks.RunOptions{Tags: []string{checks.TagModels}})
}

// field returns the field of model with the Go name name.
func field(t *testing.T, reg *orm.Registry, model any, name string) *orm.Field {
	t.Helper()
	m, ok := reg.ModelOf(model)
	if !ok {
		t.Fatalf("model %T is not registered", model)
	}
	f, ok := m.FieldByName(name)
	if !ok {
		t.Fatalf("model %s has no field %s", m.Label(), name)
	}
	return f
}

// recorder is a check function that records the contexts it ran with and
// returns fixed messages (the port of the Django test checks that store their
// kwargs on the function).
type recorder struct {
	contexts []*checks.Context
	messages []checks.Message
}

// run is the check function of the recorder.
func (r *recorder) run(ctx *checks.Context) []checks.Message {
	r.contexts = append(r.contexts, ctx)
	return r.messages
}
