package urls

import (
	"encoding/hex"
	"errors"
	"maps"
	"strconv"
	"sync"
)

// Converter is a path converter, the counterpart of the converter classes in
// django.urls.converters. A route segment written as <name:param> (or {param},
// which uses "str") matches Regex, hands the matched text to ToGo before the
// handler sees it, and is rebuilt by ToURL when the route is reversed.
type Converter interface {
	// Regex is the regular expression the segment must match. It must not
	// contain capturing groups.
	Regex() string
	// ToGo converts the matched text to the value handlers receive (Django's
	// to_python). An error means the URL does not match the route, so the next
	// route is tried; it is Django's ValueError.
	ToGo(value string) (any, error)
	// ToURL converts a value passed to Reverse to its URL text (Django's
	// to_url). An error means the value does not fit the route, so the next
	// candidate is tried.
	ToURL(value any) (string, error)
}

// UUID is the value the "uuid" converter produces. It prints in the canonical
// lowercase 8-4-4-4-12 form.
type UUID [16]byte

// String returns the canonical lowercase text form.
func (u UUID) String() string {
	var buf [36]byte
	hex.Encode(buf[0:8], u[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], u[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], u[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], u[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], u[10:16])
	return string(buf[:])
}

// errBadUUID reports text that is not a canonical UUID.
var errBadUUID = errors.New("urls: badly formed UUID")

// ParseUUID parses the 8-4-4-4-12 hexadecimal text form of a UUID.
func ParseUUID(s string) (UUID, error) {
	var u UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, errBadUUID
	}
	compact := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.Decode(u[:], []byte(compact)); err != nil {
		return UUID{}, errBadUUID
	}
	return u, nil
}

// stringConverter is Django's StringConverter with a configurable expression;
// it also serves "slug" and "path", which differ only in what they match.
type stringConverter struct {
	regex string
}

// Regex returns the expression of the converter.
func (c stringConverter) Regex() string { return c.regex }

// ToGo returns the matched text unchanged.
func (c stringConverter) ToGo(value string) (any, error) { return value, nil }

// ToURL returns the text form of value.
func (c stringConverter) ToURL(value any) (string, error) { return pyStr(value), nil }

// intConverter is Django's IntConverter.
type intConverter struct{}

// Regex matches a run of decimal digits.
func (intConverter) Regex() string { return "[0-9]+" }

// ToGo parses the digits as an int. A number that does not fit an int is a
// non-match, where Python would produce an arbitrary-precision integer.
func (intConverter) ToGo(value string) (any, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ToURL returns the text form of value.
func (intConverter) ToURL(value any) (string, error) { return pyStr(value), nil }

// uuidConverter is Django's UUIDConverter.
type uuidConverter struct{}

// Regex matches a lowercase UUID with dashes.
func (uuidConverter) Regex() string {
	return "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
}

// ToGo parses the text into a UUID.
func (uuidConverter) ToGo(value string) (any, error) {
	u, err := ParseUUID(value)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ToURL returns the text form of value.
func (uuidConverter) ToURL(value any) (string, error) { return pyStr(value), nil }

// defaultConverters are the converters every project has, Django's
// DEFAULT_CONVERTERS.
var defaultConverters = map[string]Converter{
	"int":  intConverter{},
	"path": stringConverter{regex: ".+"},
	"slug": stringConverter{regex: "[-a-zA-Z0-9_]+"},
	"str":  stringConverter{regex: "[^/]+"},
	"uuid": uuidConverter{},
}

// converterRegistry holds the converters added with RegisterConverter.
var converterRegistry = struct {
	sync.RWMutex
	m map[string]Converter
}{m: map[string]Converter{}}

// RegisterConverter makes c available in routes under name, as in
// <name:param>. It is Django's register_converter and, like it, refuses a name
// that is already taken. Register converters before building a Router: a
// router keeps the converters it found when it was built.
func RegisterConverter(name string, c Converter) error {
	converterRegistry.Lock()
	defer converterRegistry.Unlock()
	if _, ok := defaultConverters[name]; ok {
		return configErrorf("Converter %s is already registered.", pyRepr(name))
	}
	if _, ok := converterRegistry.m[name]; ok {
		return configErrorf("Converter %s is already registered.", pyRepr(name))
	}
	converterRegistry.m[name] = c
	return nil
}

// unregisterConverter removes a converter added with RegisterConverter. Tests
// use it to leave the process-wide registry as they found it.
func unregisterConverter(name string) {
	converterRegistry.Lock()
	defer converterRegistry.Unlock()
	delete(converterRegistry.m, name)
}

// allConverters returns the default and the registered converters, Django's
// get_converters.
func allConverters() map[string]Converter {
	converterRegistry.RLock()
	defer converterRegistry.RUnlock()
	out := make(map[string]Converter, len(defaultConverters)+len(converterRegistry.m))
	maps.Copy(out, defaultConverters)
	maps.Copy(out, converterRegistry.m)
	return out
}
