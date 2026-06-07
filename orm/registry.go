package orm

import (
	"fmt"
	"reflect"
	"strings"
)

// Registry holds all registered model descriptors and is safe for concurrent
// reads after Freeze is called.
type Registry struct {
	models []*Model
	byType map[reflect.Type]*Model
	byName map[string]*Model
	frozen bool
}

// NewRegistry returns an empty, unfrozen Registry.
func NewRegistry() *Registry {
	return &Registry{
		byType: make(map[reflect.Type]*Model),
		byName: make(map[string]*Model),
	}
}

// Register inspects the struct type pointed to by model, builds a Model
// descriptor, and stores it in the registry.
//
// model must be a non-nil pointer to a struct.
// Register returns an error if the registry is frozen, if model is not a
// pointer to a struct, if the model has already been registered, if the
// struct contains tag errors, or if no unambiguous primary key can be
// resolved.
func (r *Registry) Register(model any) (*Model, error) {
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

	// Step 3: duplicate check.
	if _, exists := r.byName[name]; exists {
		return nil, fmt.Errorf("orm: model %s already registered", name)
	}

	// Step 4: collect fields.
	var fields []*Field
	for i := range st.NumField() {
		sf := st.Field(i)
		f, keep, err := parseStructField(sf, i)
		if err != nil {
			return nil, fmt.Errorf("orm: model %s: %w", name, err)
		}
		if keep {
			fields = append(fields, f)
		}
	}

	// Step 5: resolve table name.
	table := strings.ToLower(name)
	if wm, ok := model.(withMeta); ok {
		if t := wm.Meta().Table; t != "" {
			table = t
		}
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
	// Promote the pk field.
	pkField.PrimaryKey = true
	if pkField.Kind == KindInt {
		pkField.Kind = KindAuto
	}

	// Step 7: build lookup maps and check for duplicate column names.
	byName := make(map[string]*Field, len(fields))
	byColumn := make(map[string]*Field, len(fields))
	for _, f := range fields {
		byName[f.Name] = f
		if existing, dup := byColumn[f.Column]; dup && existing != f {
			return nil, fmt.Errorf("orm: model %s has duplicate column %s", name, f.Column)
		}
		byColumn[f.Column] = f
	}

	// Step 8: construct and store the Model.
	m := &Model{
		name:     name,
		table:    table,
		fields:   fields,
		pk:       pkField,
		byName:   byName,
		byColumn: byColumn,
		goType:   st,
	}
	r.models = append(r.models, m)
	r.byType[st] = m
	r.byName[name] = m
	return m, nil
}

// Freeze marks the registry as read-only. After Freeze, Register will return
// an error.
func (r *Registry) Freeze() { r.frozen = true }

// Get returns the Model registered under the given Go type name.
func (r *Registry) Get(name string) (*Model, bool) {
	m, ok := r.byName[name]
	return m, ok
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
