package orm

import (
	"fmt"
	"reflect"
	"strings"
)

// Registry holds all registered model descriptors and is safe for concurrent
// reads after Freeze is called.
type Registry struct {
	models            []*Model
	autoThroughModels []*Model
	byType            map[reflect.Type]*Model
	byLabel           map[string]*Model   // keyed by Model.Label()
	byName            map[string][]*Model // keyed by the bare Go type name
	byTable           map[string]*Model
	frozen            bool
}

// NewRegistry returns an empty, unfrozen Registry.
func NewRegistry() *Registry {
	return &Registry{
		byType:  make(map[reflect.Type]*Model),
		byLabel: make(map[string]*Model),
		byName:  make(map[string][]*Model),
		byTable: make(map[string]*Model),
	}
}

// Register inspects the struct type pointed to by model, builds a Model
// descriptor, and stores it in the registry without an app label. It is
// RegisterApp with an empty app.
//
// model must be a non-nil pointer to a struct.
// Register returns an error if the registry is frozen, if model is not a
// pointer to a struct, if the model has already been registered, if the
// struct contains tag errors, or if no unambiguous primary key can be
// resolved.
func (r *Registry) Register(model any) (*Model, error) {
	return r.RegisterApp("", model)
}

// RegisterApp is Register for a model that belongs to the app labeled app
// (Django's app_label, e.g. "blog"). The label scopes the model: its registry
// key becomes "<app>.<Name>", so two apps may each declare a model with the same
// Go type name, and its default table name becomes "<app>_<name>" (Django's
// "blog_post") instead of the bare lowercased type name. A Meta().Table override
// still wins. An empty app behaves exactly like Register.
func (r *Registry) RegisterApp(app string, model any) (*Model, error) {
	// Step 1: reject if frozen.
	if r.frozen {
		name := nameOf(model)
		return nil, fmt.Errorf("orm: registry is frozen; cannot register %s", name)
	}

	// Step 2: validate that model is a non-nil pointer to a struct.
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, fmt.Errorf("orm: Register requires a non-nil pointer to a struct, got %T", model)
	}
	rt := rv.Type()
	st := rt.Elem()
	if st.Kind() != reflect.Struct {
		return nil, fmt.Errorf("orm: Register requires a non-nil pointer to a struct, got %T", model)
	}
	name := st.Name()

	// Step 3: duplicate check. A Go type is registered at most once, and a label
	// ("<app>.<Name>", or the bare name without an app) is unique.
	label := name
	if app != "" {
		label = app + "." + name
	}
	if _, exists := r.byType[st]; exists {
		return nil, fmt.Errorf("orm: model %s already registered", label)
	}
	if _, exists := r.byLabel[label]; exists {
		return nil, fmt.Errorf("orm: model %s already registered", label)
	}

	// Step 4: collect concrete fields and many-to-many fields.
	var fields []*Field
	var m2mFields []*Field
	cacheIndex := -1
	for i := range st.NumField() {
		sf := st.Field(i)
		if sf.Type == relCacheType {
			cacheIndex = i
			continue
		}
		f, keep, err := parseStructField(sf, i)
		if err != nil {
			return nil, fmt.Errorf("orm: model %s: %w", name, err)
		}
		if keep {
			if f.Rel != nil && f.Rel.Kind == RelM2M {
				m2mFields = append(m2mFields, f)
			} else {
				fields = append(fields, f)
			}
		}
	}

	// Step 5: resolve table name. The default is Django's "<app>_<model>" for a
	// model registered under an app, and the bare lowercased type name otherwise.
	table := strings.ToLower(name)
	if app != "" {
		table = app + "_" + table
	}
	if wm, ok := model.(withMeta); ok {
		if t := wm.Meta().Table; t != "" {
			table = t
		}
	}
	if other, exists := r.byTable[table]; exists {
		return nil, fmt.Errorf("orm: model %s: table %q is already used by model %s", label, table, other.Label())
	}

	// Step 6: primary key resolution.
	var pkField *Field
	var pkCount int
	for _, f := range fields {
		if f.PrimaryKey {
			pkCount++
			pkField = f
		}
	}
	if pkCount > 1 {
		return nil, fmt.Errorf("orm: model %s has multiple primary keys", name)
	}
	if pkCount == 0 {
		// Auto-promote an integer field named exactly "ID".
		for _, f := range fields {
			if f.Name == "ID" && f.Kind == KindInt {
				pkField = f
				break
			}
		}
		if pkField == nil {
			return nil, fmt.Errorf(
				"orm: model %s has no primary key (add an integer ID field or tag a field with pk)",
				name,
			)
		}
	}
	// Promote the pk field. Relation primary keys (O2O) remain KindInt rather
	// than becoming autoincrementing KindAuto columns.
	pkField.PrimaryKey = true
	if pkField.Kind == KindInt && pkField.Rel == nil {
		pkField.Kind = KindAuto
	}
	if pkField.Kind == KindAuto && pkField.HasDefault {
		return nil, fmt.Errorf("orm: model %s: primary key %s cannot have a default", name, pkField.Name)
	}

	// Step 7: build lookup maps and check for duplicate column names.
	byName := make(map[string]*Field, len(fields))
	byAttr := make(map[string]*Field, len(fields))
	byColumn := make(map[string]*Field, len(fields))
	for _, f := range fields {
		byName[f.Name] = f
		if other, dup := byAttr[f.Attr]; dup {
			return nil, fmt.Errorf("orm: model %s: fields %s and %s share the attribute name %s", name, other.Name, f.Name, f.Attr)
		}
		byAttr[f.Attr] = f
		if _, dup := byColumn[f.Column]; dup {
			return nil, fmt.Errorf("orm: model %s has duplicate column %s", name, f.Column)
		}
		byColumn[f.Column] = f
	}
	byM2MName := make(map[string]*Field, len(m2mFields))
	byM2MAttr := make(map[string]*Field, len(m2mFields))
	for _, f := range m2mFields {
		if other, dup := byAttr[f.Attr]; dup {
			return nil, fmt.Errorf("orm: model %s: fields %s and %s share the attribute name %s", name, other.Name, f.Name, f.Attr)
		}
		if other, dup := byM2MAttr[f.Attr]; dup {
			return nil, fmt.Errorf("orm: model %s: fields %s and %s share the attribute name %s", name, other.Name, f.Name, f.Attr)
		}
		byM2MName[f.Name] = f
		byM2MAttr[f.Attr] = f
	}

	// Apply model-declared field choices (legacy Choices() hook).
	if err := applyWithChoices(name, byName, byAttr, byColumn, model); err != nil {
		return nil, err
	}

	// Step 8: construct and store the Model.
	m := &Model{
		app:        app,
		name:       name,
		table:      table,
		fields:     fields,
		pk:         pkField,
		byName:     byName,
		byAttr:     byAttr,
		byColumn:   byColumn,
		m2mFields:  m2mFields,
		byM2MName:  byM2MName,
		byM2MAttr:  byM2MAttr,
		goType:     st,
		cacheIndex: cacheIndex,
	}
	if err := applyModelMeta(m, model); err != nil {
		return nil, err
	}
	r.models = append(r.models, m)
	r.byType[st] = m
	r.byLabel[label] = m
	r.byName[name] = append(r.byName[name], m)
	r.byTable[table] = m
	return m, nil
}

// Models returns the registered models in registration order. The returned slice is
// a copy; the *Model values are read-only.
func (r *Registry) Models() []*Model {
	cp := make([]*Model, len(r.models))
	copy(cp, r.models)
	return cp
}

// Resolve links every relation field to its target model. Call it after all
// models are registered and before Freeze. It returns an error if a relation
// references a model that was never registered. Resolve is idempotent.
func (r *Registry) Resolve() error {
	for _, m := range r.models {
		for _, f := range m.fields {
			if f.Rel == nil {
				continue
			}
			tm, ok := r.byType[f.Rel.targetType]
			if !ok {
				return fmt.Errorf(
					"orm: model %s field %s references unregistered model %s",
					m.name, f.Name, f.Rel.targetType,
				)
			}
			f.Rel.Target = tm
		}
	}
	if err := r.resolveM2MRelations(); err != nil {
		return err
	}
	r.resolveReverseRelations()
	return nil
}

// Freeze marks the registry as read-only. After Freeze, Register will return
// an error.
func (r *Registry) Freeze() { r.frozen = true }

// Get returns the Model registered under name, which is either a label
// ("blog.Post") or a bare Go type name ("Post"). A bare name resolves only when
// exactly one registered model carries it; when two apps declare the same type
// name it is ambiguous and Get reports false, so use the label.
func (r *Registry) Get(name string) (*Model, bool) {
	if m, ok := r.byLabel[name]; ok {
		return m, true
	}
	if ms := r.byName[name]; len(ms) == 1 {
		return ms[0], true
	}
	return nil, false
}

// AppModels returns the models registered under the app labeled app, in
// registration order.
func (r *Registry) AppModels(app string) []*Model {
	var out []*Model
	for _, m := range r.models {
		if m.app == app {
			out = append(out, m)
		}
	}
	return out
}

// ModelOf returns the Model for the concrete type of v.
// If v is a pointer its element type is used.
// It returns false if no model has been registered for that type.
func (r *Registry) ModelOf(v any) (*Model, bool) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, false
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	m, ok := r.byType[t]
	return m, ok
}

// nameOf returns a best-effort string representation of v for error messages.
func nameOf(v any) string {
	if v == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		if t.Elem().Kind() == reflect.Struct {
			return t.Elem().Name()
		}
	}
	return fmt.Sprintf("%T", v)
}
