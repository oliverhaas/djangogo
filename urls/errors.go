package urls

import (
	"errors"
	"fmt"
)

// ErrNoReverseMatch is the sentinel every *NoReverseMatch matches with
// errors.Is.
var ErrNoReverseMatch = errors.New("urls: no reverse match")

// NoReverseMatch reports that Reverse found no route for a name and its
// arguments. Its message is the one Django's NoReverseMatch carries.
type NoReverseMatch struct {
	// Msg is Django's message, unchanged.
	Msg string
}

// Error returns Django's message.
func (e *NoReverseMatch) Error() string { return e.Msg }

// Is makes errors.Is(err, ErrNoReverseMatch) hold for every NoReverseMatch.
func (e *NoReverseMatch) Is(target error) bool { return target == ErrNoReverseMatch }

// noReverseMatchf builds a NoReverseMatch from a format string.
func noReverseMatchf(format string, args ...any) *NoReverseMatch {
	return &NoReverseMatch{Msg: fmt.Sprintf(format, args...)}
}

// ErrResolver404 is the sentinel every *Resolver404 matches with errors.Is.
var ErrResolver404 = errors.New("urls: no route matches the path")

// Resolver404 reports that no route matches a path, Django's Resolver404.
type Resolver404 struct {
	// Path is the path that was resolved.
	Path string
	// Tried lists the patterns that were tried, one chain of include prefixes
	// and route per entry, in the order they were tried.
	Tried [][]TriedPattern
}

// Error names the path that did not resolve.
func (e *Resolver404) Error() string {
	return fmt.Sprintf("urls: no route matches %q", e.Path)
}

// Is makes errors.Is(err, ErrResolver404) hold for every Resolver404.
func (e *Resolver404) Is(target error) bool { return target == ErrResolver404 }

// ErrImproperlyConfigured is the sentinel every *ImproperlyConfigured matches
// with errors.Is.
var ErrImproperlyConfigured = errors.New("urls: improperly configured")

// ImproperlyConfigured reports a mistake in a route declaration: an unknown
// converter, a parameter name that is not an identifier, an expression that
// does not compile. NewRouterChecked returns it and NewRouter panics with it.
// The message is Django's where Django has one.
type ImproperlyConfigured struct {
	// Msg describes the mistake.
	Msg string
}

// Error returns the message.
func (e *ImproperlyConfigured) Error() string { return e.Msg }

// Is makes errors.Is(err, ErrImproperlyConfigured) hold for every
// ImproperlyConfigured.
func (e *ImproperlyConfigured) Is(target error) bool { return target == ErrImproperlyConfigured }

// configErrorf builds an ImproperlyConfigured from a format string.
func configErrorf(format string, args ...any) *ImproperlyConfigured {
	return &ImproperlyConfigured{Msg: fmt.Sprintf(format, args...)}
}

// usageError is an error whose text is a Django message kept verbatim, capital
// letter and punctuation included.
type usageError string

// Error returns the message.
func (e usageError) Error() string { return string(e) }

// ErrMixedArgs is returned by Reverse when it is given both positional and
// keyword arguments. Django raises a ValueError with the same text.
var ErrMixedArgs error = usageError("Don't mix *args and **kwargs in call to reverse()!")
