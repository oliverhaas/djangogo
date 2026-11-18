package checks

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/orm"
)

// lookupSep separates the parts of a field lookup in a filter
// ("author__name"), so no field or model name may contain it.
const lookupSep = "__"

// checkModels is the check tagged models: it reads the metadata of the model
// registry and reports what Django's Model.check, Field.check and
// check_all_models report, with Django's ids and texts.
//
// Context.Apps limits the check to the models of those app labels; empty
// selects every model. A message about a field carries the text
// "<app>.<Model>.<attr>" as its object, one about a model the *orm.Model, and
// one about a table the table name.
//
// The registry refuses several of the states at registration (two models with
// one table, two fields with one column, a max_length that is not positive,
// on_delete=set_null on a field that is not nullable). Their checks read the
// exported metadata all the same, which holds whatever its owner set.
func checkModels(ctx *Context) []Message {
	if ctx.Models == nil {
		return nil
	}
	var messages []Message
	// tables lists the tables in the order of their first model, which keeps
	// the order of the messages stable.
	var tables []string
	users := make(map[string][]string)
	for _, m := range ctx.Models.Models() {
		if len(ctx.Apps) > 0 && !slices.Contains(ctx.Apps, m.App()) {
			continue
		}
		table := m.Table()
		if _, seen := users[table]; !seen {
			tables = append(tables, table)
		}
		users[table] = append(users[table], m.Label())
		messages = append(messages, checkModel(ctx.Models, m)...)
	}
	for _, table := range tables {
		if labels := users[table]; len(labels) > 1 {
			messages = append(messages, Error(
				fmt.Sprintf("db_table '%s' is used by multiple models: %s.", table, strings.Join(labels, ", ")),
				Obj(table),
				ID("models.E028"),
			))
		}
	}
	return messages
}

// checkModel returns the messages of one model in the order of Django's
// Model.check: the fields, the model name, the columns. As in Django, a clash
// of names hides the clashes of columns that follow from it.
func checkModel(reg *orm.Registry, m *orm.Model) []Message {
	var messages []Message
	for _, f := range m.Fields() {
		messages = append(messages, checkField(reg, m, f)...)
	}
	clashes := checkModelName(m)
	messages = append(messages, clashes...)
	if len(clashes) == 0 {
		messages = append(messages, checkColumnClashes(m)...)
	}
	return messages
}

// checkField returns the messages of one field in the order of Django's
// Field.check, followed by the checks of its field class: CharField for a
// string field with a length, ForeignKey for a relation.
func checkField(reg *orm.Registry, m *orm.Model, f *orm.Field) []Message {
	obj := Obj(m.Label() + "." + f.Attr)
	messages := checkFieldName(f, obj)
	messages = append(messages, checkChoices(f, obj)...)
	if f.PrimaryKey && f.Null {
		messages = append(messages, Error(
			"Primary keys must not have null=True.",
			Hint("Set null=False on the field, or remove primary_key=True argument."),
			obj,
			ID("fields.E007"),
		))
	}
	if f.Kind == orm.KindChar {
		messages = append(messages, checkMaxLength(f, obj)...)
	}
	if f.Rel != nil {
		messages = append(messages, checkRelation(reg, m, f, obj)...)
	}
	return messages
}

// checkFieldName reports an attribute name that a filter cannot address: one
// that ends with an underscore (fields.E001), contains the lookup separator
// (fields.E002) or is "pk", the alias of the primary key (fields.E003).
func checkFieldName(f *orm.Field, obj Option) []Message {
	switch {
	case strings.HasSuffix(f.Attr, "_"):
		return []Message{Error("Field names must not end with an underscore.", obj, ID("fields.E001"))}
	case strings.Contains(f.Attr, lookupSep):
		return []Message{Error(fmt.Sprintf("Field names must not contain %q.", lookupSep), obj, ID("fields.E002"))}
	case f.Attr == "pk":
		return []Message{Error("'pk' is a reserved word that cannot be used as a field name.", obj, ID("fields.E003"))}
	}
	return nil
}

// checkChoices reports a choice value that does not fit max_length
// (fields.E009). A field without a length (MaxLength 0, Django's None) holds
// any value.
func checkChoices(f *orm.Field, obj Option) []Message {
	if len(f.Choices) == 0 || f.MaxLength == 0 {
		return nil
	}
	longest := 0
	for _, choice := range f.Choices {
		longest = max(longest, utf8.RuneCountInString(choice.Value))
	}
	if longest <= f.MaxLength {
		return nil
	}
	return []Message{Error(
		fmt.Sprintf("'max_length' is too small to fit the longest value in 'choices' (%d characters).", longest),
		obj,
		ID("fields.E009"),
	)}
}

// checkMaxLength reports a string field without max_length (fields.E120) or
// with a negative one (fields.E121). MaxLength 0 is Django's None.
func checkMaxLength(f *orm.Field, obj Option) []Message {
	switch {
	case f.MaxLength == 0:
		return []Message{Error("CharFields must define a 'max_length' attribute.", obj, ID("fields.E120"))}
	case f.MaxLength < 0:
		return []Message{Error("'max_length' must be a positive integer.", obj, ID("fields.E121"))}
	}
	return nil
}

// checkRelation reports a relation to a model that is not registered
// (fields.E300) and on_delete=set_null on a field that is not nullable
// (fields.E320).
func checkRelation(reg *orm.Registry, m *orm.Model, f *orm.Field, obj Option) []Message {
	var messages []Message
	if name, missing := missingTarget(reg, m, f); missing {
		messages = append(messages, Error(
			fmt.Sprintf("Field defines a relation with model '%s', which is either not installed, or is abstract.", name),
			obj,
			ID("fields.E300"),
		))
	}
	if f.Rel.OnDelete == orm.OnDeleteSetNull && !f.Null {
		messages = append(messages, Error(
			"Field specifies on_delete=SET_NULL, but cannot be null.",
			Hint("Set null=True argument on the field, or change the on_delete rule."),
			obj,
			ID("fields.E320"),
		))
	}
	return messages
}

// missingTarget reports whether the model a relation field points to is not in
// the registry, and returns the name of that model.
//
// Registry.Resolve sets Rel.Target, but it stops at the first missing model, so
// a nil Target does not prove that the target is missing: the Go type of the
// field decides. A field whose target type cannot be read is not reported.
func missingTarget(reg *orm.Registry, m *orm.Model, f *orm.Field) (name string, missing bool) {
	if f.Rel.Target != nil {
		return "", false
	}
	target, ok := relationTarget(m, f)
	if !ok {
		return "", false
	}
	if _, registered := reg.ModelOf(reflect.New(target).Interface()); registered {
		return "", false
	}
	return target.Name(), true
}

// relationTarget returns the Go type a relation field points to: the T of its
// orm.FK[T]. The metadata keeps that type unexported, so it is read from the
// result of the Object method of the struct field, which returns a *T.
func relationTarget(m *orm.Model, f *orm.Field) (reflect.Type, bool) {
	t := m.GoType()
	if t.Kind() != reflect.Struct || f.Index < 0 || f.Index >= t.NumField() {
		return nil, false
	}
	object, ok := t.Field(f.Index).Type.MethodByName("Object")
	if !ok || object.Type.NumOut() == 0 || object.Type.Out(0).Kind() != reflect.Pointer {
		return nil, false
	}
	return object.Type.Out(0).Elem(), true
}

// checkModelName reports a model name that collides with the lookup syntax:
// one that starts or ends with an underscore (models.E023) or contains the
// lookup separator (models.E024).
func checkModelName(m *orm.Model) []Message {
	name := m.Name()
	switch {
	case strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_"):
		return []Message{Error(
			fmt.Sprintf("The model name '%s' cannot start or end with an underscore as it collides with the query lookup syntax.", name),
			Obj(m),
			ID("models.E023"),
		)}
	case strings.Contains(name, lookupSep):
		return []Message{Error(
			fmt.Sprintf("The model name '%s' cannot contain double underscores as it collides with the query lookup syntax.", name),
			Obj(m),
			ID("models.E024"),
		)}
	}
	return nil
}

// checkColumnClashes reports every field whose column an earlier field of the
// model already uses (models.E007). A field without a column clashes with
// nothing.
func checkColumnClashes(m *orm.Model) []Message {
	var messages []Message
	used := make(map[string]bool)
	for _, f := range m.Fields() {
		if f.Column == "" || !used[f.Column] {
			used[f.Column] = true
			continue
		}
		messages = append(messages, Error(
			fmt.Sprintf("Field '%s' has column name '%s' that is used by another field.", f.Attr, f.Column),
			Hint("Specify a 'db_column' for the field."),
			Obj(m),
			ID("models.E007"),
		))
	}
	return messages
}
