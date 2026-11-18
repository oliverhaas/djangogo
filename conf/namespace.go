package conf

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// ErrNamespaceNotRegistered reports the name of a settings namespace that no
// call to Register introduced. Match it with errors.Is.
var ErrNamespaceNotRegistered = errors.New("settings namespace is not registered")

// namespace is a registered settings namespace. It is not changed after it is
// stored; a second registration stores a new one.
type namespace struct {
	// typ is the struct type of the values.
	typ reflect.Type
	// defaults is the value of an application that sets nothing.
	defaults reflect.Value
	// vars lists the variables the namespace reads, without the prefix of an
	// overlay.
	vars []string
}

// registry holds the settings namespaces of the process.
var registry = struct {
	sync.RWMutex
	byName map[string]*namespace
}{byName: make(map[string]*namespace)}

// coreVars returns the variables the Settings fields are read from, without
// the prefix of an overlay.
var coreVars = sync.OnceValue(func() map[string]bool {
	vars := map[string]bool{databaseURLVar: true}
	for _, f := range envFields(reflect.TypeFor[Settings](), "", false) {
		vars[f.name] = true
	}
	return vars
})

// Register registers the settings namespace name with its default values, for
// an app that has settings of its own. T is a struct type; name consists of
// letters, digits and underscores and starts with a letter. Registering the
// same name again with the same type replaces the defaults; with another type
// it is an error. It is also an error when a variable of the namespace (see
// Get) is one that a Settings field or another namespace reads.
//
// The registry is process-wide and safe for concurrent use. An app registers
// its namespace when the program starts, in a package-level variable or an
// init function, so that the project can set values with SetNamespace.
func Register[T any](name string, defaults T) error {
	t := reflect.TypeFor[T]()
	if !isNamespaceName(name) {
		return fmt.Errorf("conf: invalid namespace name %q (want letters, digits and underscores, starting with a letter)", name)
	}
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("conf: namespace %q: %s is not a struct type", name, t)
	}
	ns := &namespace{typ: t, defaults: deepCopy(reflect.ValueOf(defaults))}
	for _, f := range envFields(t, namespacePath(name), true) {
		ns.vars = append(ns.vars, f.name)
	}

	registry.Lock()
	defer registry.Unlock()
	if old, ok := registry.byName[name]; ok && old.typ != t {
		return fmt.Errorf("conf: namespace %q is already registered with the type %s, not %s", name, old.typ, t)
	}
	for _, v := range ns.vars {
		if coreVars()[v] || strings.HasPrefix(v, databaseURLVar+"_") {
			return fmt.Errorf("conf: namespace %q: the variable %s is already read by the settings", name, v)
		}
		for other, registered := range registry.byName {
			if other != name && slices.Contains(registered.vars, v) {
				return fmt.Errorf("conf: namespace %q: the variable %s is already read by the namespace %q", name, v, other)
			}
		}
	}
	registry.byName[name] = ns
	return nil
}

// Namespaces returns the registered namespace names, sorted.
func Namespaces() []string {
	registry.RLock()
	defer registry.RUnlock()
	return slices.Sorted(maps.Keys(registry.byName))
}

// SetNamespace sets the value of a namespace in code, in place of its
// registered defaults. The namespace must be registered, and value must have
// the registered type (the struct, not a pointer to it). The value is copied.
//
// The environment still wins: Get overlays the value with the variables of
// the namespace, also when SetNamespace is called after ApplyEnv.
func (s *Settings) SetNamespace(name string, value any) error {
	ns, err := lookupNamespace(name)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(value)
	if !v.IsValid() || v.Type() != ns.typ {
		return fmt.Errorf("conf: namespace %q: the value is a %T, want a %s", name, value, ns.typ)
	}
	// The map may be shared with a copy of the settings, so it is replaced and
	// not written to.
	values := make(map[string]any, len(s.namespaces)+1)
	maps.Copy(values, s.namespaces)
	values[name] = deepCopy(v).Interface()
	s.namespaces = values
	return nil
}

// Get returns the namespace name of the settings: the registered defaults,
// replaced as a whole by a value set with SetNamespace, then overlaid with
// every environment the settings were overlaid with.
//
// The variable of a field is the prefix of the overlay, the name in upper
// case, an underscore and the field name in upper snake case with acronyms
// kept together: with the prefix "DJANGOGO_", the field PageSize of the
// namespace "blog" is DJANGOGO_BLOG_PAGE_SIZE, APIKey is DJANGOGO_BLOG_API_KEY.
// An env tag on a field replaces the field's part of the name, and the tag "-"
// excludes the field. The fields of a nested struct are read with its name in
// front (DJANGOGO_BLOG_CACHE_TTL); those of an embedded struct without.
//
// The value syntax is that of ApplyEnv, for strings, booleans, integers,
// floats, time.Duration, types that implement encoding.TextUnmarshaler, and
// slices of these. A set variable for a field of another type is an error.
//
// It is an error when the name is not registered (ErrNamespaceNotRegistered),
// when T is not the registered type, or when a variable is malformed; in the
// last case the value is returned with the fields of the malformed variables
// unchanged. The value of a malformed variable is left out of the error when
// the name of the variable suggests a secret.
//
// The result is a copy: its slices, maps and nested structs are not shared
// with the registry or the settings. Pointers are copied as pointers. Get
// reads the environment on every call, so an app calls it once at startup and
// keeps the result. It is safe for concurrent use on the same settings as long
// as none of the callers changes them.
func Get[T any](s *Settings, name string) (T, error) {
	var zero T
	if s == nil {
		return zero, errors.New("conf: Get called with nil settings")
	}
	ns, err := lookupNamespace(name)
	if err != nil {
		return zero, err
	}
	if t := reflect.TypeFor[T](); t != ns.typ {
		return zero, fmt.Errorf("conf: namespace %q holds a %s, not a %s", name, ns.typ, t)
	}
	value, errs := s.resolveNamespace(name, ns)
	result, _ := value.Interface().(T)
	return result, errors.Join(errs...)
}

// NamespaceErrors validates every registered namespace against the settings
// and returns the errors: the malformed environment values, one error each,
// and the error of a value that implements interface{ Validate() error }. The
// system checks report them.
func (s *Settings) NamespaceErrors() []error {
	var errs []error
	for _, name := range Namespaces() {
		ns, err := lookupNamespace(name)
		if err != nil {
			continue
		}
		value, envErrs := s.resolveNamespace(name, ns)
		errs = append(errs, envErrs...)
		// The method set of the pointer holds the methods of both receivers.
		if v, ok := value.Addr().Interface().(interface{ Validate() error }); ok {
			if err := v.Validate(); err != nil {
				errs = append(errs, fmt.Errorf("conf: namespace %q: %w", name, err))
			}
		}
	}
	return errs
}

// resolveNamespace returns the value of a namespace for these settings as an
// addressable copy, and one error per malformed variable.
func (s *Settings) resolveNamespace(name string, ns *namespace) (reflect.Value, []error) {
	base := ns.defaults
	if set, ok := s.namespaces[name]; ok {
		base = reflect.ValueOf(set)
	}
	value := deepCopy(base)
	return value, s.overlay.replay(value, namespacePath(name))
}

// lookupNamespace returns the registered namespace name.
func lookupNamespace(name string) (*namespace, error) {
	registry.RLock()
	defer registry.RUnlock()
	ns, ok := registry.byName[name]
	if !ok {
		return nil, fmt.Errorf("conf: %w: %q", ErrNamespaceNotRegistered, name)
	}
	return ns, nil
}

// namespacePath returns what stands between the prefix of an overlay and the
// field part of a variable of the namespace.
func namespacePath(name string) string {
	return strings.ToUpper(name) + "_"
}

// isNamespaceName reports whether s can name a namespace: letters, digits and
// underscores, starting with a letter, so that its upper case is a part of a
// variable name.
func isNamespaceName(s string) bool {
	return isEnvName(s) && s[0] != '_'
}

// deepCopy returns an addressable copy of v that shares no slice, map or
// nested struct with it.
func deepCopy(v reflect.Value) reflect.Value {
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	unshare(c)
	return c
}

// unshare replaces every slice and map reachable from the settable value v
// through exported fields, those of embedded structs included, with a copy.
// Pointers, channels and functions stay shared: what they refer to is not
// settings data.
func unshare(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			// The exported fields of an embedded struct can be set, also when
			// the type of the struct is unexported.
			if f := v.Type().Field(i); f.IsExported() || f.Anonymous && f.Type.Kind() == reflect.Struct {
				unshare(v.Field(i))
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			unshare(v.Index(i))
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		c := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		reflect.Copy(c, v)
		for i := range c.Len() {
			unshare(c.Index(i))
		}
		v.Set(c)
	case reflect.Map:
		if v.IsNil() {
			return
		}
		c := reflect.MakeMapWithSize(v.Type(), v.Len())
		for iter := v.MapRange(); iter.Next(); {
			c.SetMapIndex(iter.Key(), deepCopy(iter.Value()))
		}
		v.Set(c)
	case reflect.Interface:
		if !v.IsNil() {
			v.Set(deepCopy(v.Elem()))
		}
	}
}
