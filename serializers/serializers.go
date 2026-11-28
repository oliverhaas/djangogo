// Package serializers converts ORM model instances to and from serialized
// representations, porting Django's django.core.serializers.
package serializers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/validators"
)

// ErrSerializerDoesNotExist is matched by errors returned when a requested
// serializer format is not registered, mirroring Django's
// SerializerDoesNotExist.
var ErrSerializerDoesNotExist = errors.New("serializer does not exist")

// SerializerDoesNotExistError reports an unknown serializer format name and
// unwraps to ErrSerializerDoesNotExist.
type SerializerDoesNotExistError struct {
	// Format is the unregistered format name.
	Format string
}

// Error returns the missing format name, matching Django's KeyError message.
func (e *SerializerDoesNotExistError) Error() string { return e.Format }

// Unwrap returns ErrSerializerDoesNotExist.
func (e *SerializerDoesNotExistError) Unwrap() error { return ErrSerializerDoesNotExist }

// ErrSerialization is matched by errors raised during serialization, mirroring
// Django's SerializationError.
var ErrSerialization = errors.New("serialization error")

// SerializationError describes a failure during serialization and unwraps to
// ErrSerialization.
type SerializationError struct {
	// Message describes the failure.
	Message string
	// Err is the underlying cause, if any.
	Err error
}

// Error returns the serialization error message.
func (e *SerializationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return ""
}

// Is reports whether target is ErrSerialization.
func (e *SerializationError) Is(target error) bool {
	return target == ErrSerialization
}

// Unwrap returns the underlying cause.
func (e *SerializationError) Unwrap() error { return e.Err }

// ErrDeserialization is matched by errors raised during deserialization,
// mirroring Django's DeserializationError.
var ErrDeserialization = errors.New("deserialization error")

// DeserializationError describes a failure during deserialization and unwraps
// to ErrDeserialization.
type DeserializationError struct {
	// Message is the formatted error text.
	Message string
	// Err is the underlying cause, if any.
	Err error
}

// NewDeserializationError returns a DeserializationError with msg.
func NewDeserializationError(msg string) *DeserializationError {
	return &DeserializationError{Message: msg}
}

// DeserializationErrorWithData constructs a DeserializationError with Django's
// DeserializationError.WithData message:
// "%s: (%s:pk=%s) field_value was '%s'".
func DeserializationErrorWithData(orig error, model string, pk, fieldValue any) *DeserializationError {
	msg := fmt.Sprintf(
		"%s: (%s:pk=%s) field_value was '%s'",
		formatOriginalException(orig),
		model,
		validators.PyString(pk),
		validators.PyString(fieldValue),
	)
	return &DeserializationError{Message: msg, Err: orig}
}

// Error returns the deserialization error message.
func (e *DeserializationError) Error() string { return e.Message }

// Is reports whether target is ErrDeserialization.
func (e *DeserializationError) Is(target error) bool {
	return target == ErrDeserialization
}

// Unwrap returns the underlying cause.
func (e *DeserializationError) Unwrap() error { return e.Err }

// formatOriginalException formats orig the way Python's str(original_exc) does
// inside DeserializationError.WithData. A ValidationError is rendered as a
// Python list of its messages.
func formatOriginalException(orig error) string {
	if orig == nil {
		return "None"
	}
	var ve *orm.ValidationError
	if errors.As(orig, &ve) {
		return validators.PyString(ve.Messages())
	}
	return strings.TrimPrefix(orig.Error(), "orm: ")
}

// Options holds configuration for serialization and deserialization calls.
type Options struct {
	// SelectedFields restricts serialization to the named field attributes when
	// non-nil.
	SelectedFields []string
	// Indent is the indentation width in spaces; a negative value (-1 by
	// default in NewOptions) means no indentation, whereas 0 indents object
	// members with newlines and no spaces while keeping objects on a single
	// comma-separated line.
	Indent int
	// UseNaturalForeignKeys serializes foreign-key and M2M references as
	// natural keys when the target model implements NaturalKeyer.
	UseNaturalForeignKeys bool
	// UseNaturalPrimaryKeys omits the primary key when the model implements
	// NaturalKeyer.
	UseNaturalPrimaryKeys bool
	// ProgressOutput receives progress-bar updates when non-nil.
	ProgressOutput io.Writer
	// ObjectCount is the total number of objects for ProgressOutput.
	ObjectCount int
	// IgnoreNonexistent skips unknown models and unknown fields during
	// deserialization.
	IgnoreNonexistent bool
	// HandleForwardReferences defers unresolvable natural-key FK and M2M
	// references onto DeserializedObject.DeferredFields.
	HandleForwardReferences bool
}

// Option configures an Options value.
type Option func(*Options)

// NewOptions builds an Options value with defaults (-1 for Indent) and applies
// opts.
func NewOptions(opts ...Option) *Options {
	o := &Options{Indent: -1}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

// Fields restricts serialization to the named field attributes.
func Fields(names ...string) Option {
	cp := make([]string, len(names))
	copy(cp, names)
	return func(o *Options) { o.SelectedFields = cp }
}

// Indent sets the indentation level in spaces.
func Indent(n int) Option {
	return func(o *Options) { o.Indent = n }
}

// NaturalForeignKeys enables natural foreign keys during serialization.
func NaturalForeignKeys() Option {
	return func(o *Options) { o.UseNaturalForeignKeys = true }
}

// NaturalPrimaryKeys omits the primary key for models that implement
// NaturalKeyer.
func NaturalPrimaryKeys() Option {
	return func(o *Options) { o.UseNaturalPrimaryKeys = true }
}

// Progress writes a progress bar for count total objects to w during
// serialization.
func Progress(w io.Writer, count int) Option {
	return func(o *Options) {
		o.ProgressOutput = w
		o.ObjectCount = count
	}
}

// IgnoreNonexistent makes deserialization skip unknown models and fields.
func IgnoreNonexistent() Option {
	return func(o *Options) { o.IgnoreNonexistent = true }
}

// HandleForwardReferences makes deserialization record missing natural-key
// targets in DeserializedObject.DeferredFields.
func HandleForwardReferences() Option {
	return func(o *Options) { o.HandleForwardReferences = true }
}

// ProgressWidth is the character width of a ProgressBar, matching Django's
// ProgressBar.progress_width.
const ProgressWidth = 75

// ProgressBar renders a text progress bar to an io.Writer, porting Django's
// django.core.serializers.base.ProgressBar.
type ProgressBar struct {
	output     io.Writer
	totalCount int
	prevDone   int
}

// NewProgressBar returns a ProgressBar writing to output for totalCount
// objects.
func NewProgressBar(output io.Writer, totalCount int) *ProgressBar {
	return &ProgressBar{output: output, totalCount: totalCount}
}

// Update advances the progress bar to count completed objects.
func (p *ProgressBar) Update(count int) {
	if p == nil || p.output == nil || p.totalCount <= 0 {
		return
	}
	perc := count * 100 / p.totalCount
	done := perc * ProgressWidth / 100
	if p.prevDone >= done {
		return
	}
	p.prevDone = done
	cr := "\r"
	if p.totalCount == 1 {
		cr = ""
	}
	_, _ = io.WriteString(p.output, cr+"["+strings.Repeat(".", done)+strings.Repeat(" ", ProgressWidth-done)+"]")
	if done == ProgressWidth {
		_, _ = io.WriteString(p.output, "\n")
	}
}

// FieldValue is a single (Name, Value) entry in an Object's field list.
type FieldValue struct {
	// Name is the field's attribute name.
	Name string
	// Value is the Python-layer serialized value of the field.
	Value any
}

// Object is Django's python-serializer dictionary representation of one model
// instance, keeping fields in declaration order.
type Object struct {
	// Model is the lowercased model label ("app.model").
	Model string
	// PK is the serialized primary key value, or nil.
	PK any
	// HasPK reports whether the "pk" key is present on the serialized object.
	HasPK bool
	// Fields holds the serialized field values in declaration order.
	Fields []FieldValue
}

// DeserializedObject holds an unsaved deserialized model instance together with
// its many-to-many primary keys and any forward-referenced deferred fields,
// porting Django's DeserializedObject.
type DeserializedObject struct {
	// Object is a pointer to the instantiated model struct.
	Object any
	// Model is the ORM descriptor for Object.
	Model *orm.Model
	// M2MData maps M2M field attribute names to slices of target primary keys.
	M2MData map[string][]any
	// DeferredFields maps deferred FK and M2M fields to their raw serialized
	// values when HandleForwardReferences is enabled.
	DeferredFields map[*orm.Field]any
}

// Save writes d.Object to db with orm.SaveRaw and then sets each M2M relation
// in d.M2MData with orm.SetManyPKs. Subsequent calls do not re-save M2M data
// unless SaveDeferredFields repopulates it.
func (d *DeserializedObject) Save(ctx context.Context, db *orm.DB) error {
	if err := orm.SaveRaw(ctx, db, d.Object); err != nil {
		return err
	}
	if len(d.M2MData) > 0 {
		m := d.Model
		if m == nil && db != nil {
			m, _ = db.Registry().ModelOf(d.Object)
		}
		if m == nil {
			return errors.New("serializers: cannot save M2M data without a registered model")
		}
		saved := make(map[string]bool, len(d.M2MData))
		for _, f := range m.ManyToMany() {
			pks, ok := d.M2MData[f.Attr]
			if !ok {
				continue
			}
			saved[f.Attr] = true
			if err := orm.SetManyPKs(ctx, db, d.Object, f, pks); err != nil {
				return err
			}
		}
		for attr, pks := range d.M2MData {
			if saved[attr] {
				continue
			}
			f, ok := m.ManyToManyField(attr)
			if !ok {
				return fmt.Errorf("serializers: model %s has no many-to-many field %q", m.Name(), attr)
			}
			if err := orm.SetManyPKs(ctx, db, d.Object, f, pks); err != nil {
				return err
			}
		}
	}
	d.M2MData = nil
	return nil
}

// SaveDeferredFields resolves the forward-referenced fields in
// d.DeferredFields against db and saves the updated instance and M2M links.
func (d *DeserializedObject) SaveDeferredFields(ctx context.Context, db *orm.DB) error {
	m := d.Model
	if m == nil && db != nil {
		m, _ = db.Registry().ModelOf(d.Object)
	}
	if m == nil {
		return errors.New("serializers: cannot save deferred fields without a registered model")
	}
	d.M2MData = make(map[string][]any)
	sv := reflect.Indirect(reflect.ValueOf(d.Object))
	pkVal := instancePKForDisplay(m, sv)
	label := m.LabelLower()

	for field, fieldValue := range d.DeferredFields {
		if field == nil || field.Rel == nil {
			continue
		}
		switch field.Rel.Kind {
		case orm.RelM2M:
			values, _, badVal, err := deserializeM2MValues(ctx, db, field, fieldValue, false)
			if err != nil {
				return DeserializationErrorWithData(err, label, pkVal, badVal)
			}
			d.M2MData[field.Attr] = values
		case orm.RelFK, orm.RelO2O:
			value, _, err := deserializeFKValue(ctx, db, field, fieldValue, false)
			if err != nil {
				return DeserializationErrorWithData(err, label, pkVal, fieldValue)
			}
			if err := setModelField(field, sv.Field(field.Index), value); err != nil {
				return DeserializationErrorWithData(err, label, pkVal, fieldValue)
			}
		}
	}
	return d.Save(ctx, db)
}

// String formats d as "<DeserializedObject: app.Model(pk=1)>", matching
// Django's DeserializedObject.__repr__.
func (d *DeserializedObject) String() string {
	if d == nil || d.Object == nil {
		return "<DeserializedObject: None(pk=None)>"
	}
	sv := reflect.Indirect(reflect.ValueOf(d.Object))
	if d.Model != nil {
		pk := instancePKForDisplay(d.Model, sv)
		return fmt.Sprintf("<DeserializedObject: %s(pk=%s)>", d.Model.Label(), validators.PyString(pk))
	}
	typeName := sv.Type().Name()
	var pk any
	if sv.Kind() == reflect.Struct {
		if f := sv.FieldByName("ID"); f.IsValid() && !f.IsZero() {
			pk = f.Interface()
		}
	}
	return fmt.Sprintf("<DeserializedObject: %s(pk=%s)>", typeName, validators.PyString(pk))
}

// Format is implemented by every wire format serializer (such as "json").
type Format interface {
	// Serialize encodes the stream of Objects to w using o.
	Serialize(w io.Writer, objs iter.Seq2[*Object, error], o *Options) error
	// Deserialize decodes a stream of Objects from r.
	Deserialize(r io.Reader) iter.Seq2[*Object, error]
}

// internalFormat is an optional interface implemented by formats that are for
// internal use only (such as "python") and omitted from PublicFormats.
type internalFormat interface {
	InternalUseOnly() bool
}

// registryEntry stores one registered Format and its name.
type registryEntry struct {
	name   string
	format Format
}

var (
	registryMu sync.RWMutex
	registry   = []registryEntry{
		{name: "python", format: pythonFormat{}},
		{name: "json", format: JSONFormat{}},
		{name: "jsonl", format: JSONLFormat{}},
	}
)

// Register registers format under name, replacing any existing entry with the
// same name.
func Register(name string, format Format) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for i, e := range registry {
		if e.name == name {
			registry[i].format = format
			return
		}
	}
	registry = append(registry, registryEntry{name: name, format: format})
}

// Unregister removes the serializer registered under name, or returns an error
// matching ErrSerializerDoesNotExist when name is not registered.
func Unregister(name string) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	for i, e := range registry {
		if e.name == name {
			registry = slices.Delete(registry, i, i+1)
			return nil
		}
	}
	return &SerializerDoesNotExistError{Format: name}
}

// Formats returns all registered serializer format names in registration order.
func Formats() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, len(registry))
	for i, e := range registry {
		out[i] = e.name
	}
	return out
}

// PublicFormats returns all registered serializer format names that are not
// marked internal-use-only.
func PublicFormats() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for _, e := range registry {
		if intFmt, ok := e.format.(internalFormat); ok && intFmt.InternalUseOnly() {
			continue
		}
		out = append(out, e.name)
	}
	return out
}

// Get returns the Format registered under name, or an error matching
// ErrSerializerDoesNotExist.
func Get(name string) (Format, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, e := range registry {
		if e.name == name {
			return e.format, nil
		}
	}
	return nil, &SerializerDoesNotExistError{Format: name}
}

// Serialize serializes objs (a slice of model values or pointers) in the named
// format and returns the resulting string.
func Serialize(ctx context.Context, db *orm.DB, format string, objs any, opts ...Option) (string, error) {
	var b strings.Builder
	if err := SerializeTo(ctx, db, &b, format, objs, opts...); err != nil {
		return "", err
	}
	return b.String(), nil
}

// SerializeTo serializes objs (a slice of model values or pointers) in the
// named format and writes the output to w.
func SerializeTo(ctx context.Context, db *orm.DB, w io.Writer, format string, objs any, opts ...Option) error {
	f, err := Get(format)
	if err != nil {
		return err
	}
	o := NewOptions(opts...)
	seq := iterObjects(ctx, db, objs, o)
	return f.Serialize(w, seq, o)
}

// Deserialize deserializes a stream from r in the named format and yields
// unsaved DeserializedObject values.
func Deserialize(ctx context.Context, db *orm.DB, format string, r io.Reader, opts ...Option) iter.Seq2[*DeserializedObject, error] {
	f, err := Get(format)
	if err != nil {
		return func(yield func(*DeserializedObject, error) bool) {
			yield(nil, err)
		}
	}
	return FromObjects(ctx, db, f.Deserialize(r), opts...)
}
