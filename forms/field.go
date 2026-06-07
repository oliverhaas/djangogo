package forms

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FieldKind identifies the logical type of a form field for parsing and validation.
type FieldKind uint8

const (
	// CharField is a single-line string with an optional MaxLength.
	CharField FieldKind = iota
	// TextField is a multi-line string.
	TextField
	// IntegerField is parsed with strconv.ParseInt into an int64.
	IntegerField
	// BoolField is a truthy checkbox value parsed into a bool.
	BoolField
	// EmailField is a string validated against a basic email pattern.
	EmailField
	// DateTimeField is parsed into a time.Time.
	DateTimeField
	// ChoiceField is a string that must be one of Choices' values.
	ChoiceField
)

// emailPattern is a basic email validation regexp (not a full RFC 5322 grammar).
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// dateTimeLayouts are the layouts DateTimeField accepts, tried in order.
var dateTimeLayouts = []string{time.RFC3339, "2006-01-02 15:04:05"}

// Field is one form field with validation.
type Field struct {
	// Name is the form field name (the HTML input name and map key).
	Name string
	// Label is the human-readable label shown in rendered output.
	Label string
	// Required indicates a non-empty value is mandatory (ignored for BoolField).
	Required bool
	// HelpText is optional descriptive text.
	HelpText string
	// Widget renders the field; when nil, a default is chosen by Kind.
	Widget Widget
	// Kind selects how the raw value is parsed and validated.
	Kind FieldKind
	// MaxLength bounds CharField values; zero means unbounded.
	MaxLength int
	// Choices lists allowed (value, label) pairs for ChoiceField and Select.
	Choices [][2]string
}

// ValidationError is a user-facing validation message. Its message is displayed
// verbatim in rendered forms, so it is capitalized and punctuated as a sentence
// (unlike conventional lowercase Go error strings).
type ValidationError struct {
	// Message is the human-readable error text shown to the user.
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string { return e.Message }

// invalid returns a *ValidationError carrying msg.
func invalid(msg string) error { return &ValidationError{Message: msg} }

// errRequired is returned by Clean when a required field is empty.
var errRequired = invalid("This field is required.")

// Clean parses and validates raw and returns the cleaned Go value or an error.
// CharField, TextField, EmailField, and ChoiceField return a string; IntegerField
// returns an int64; BoolField returns a bool; DateTimeField returns a time.Time.
func (f *Field) Clean(raw string) (any, error) {
	// BoolField is special: an unchecked box submits nothing, which means false,
	// so it is never "required" and never trims meaningfully.
	if f.Kind == BoolField {
		return isTruthy(raw), nil
	}

	value := strings.TrimSpace(raw)
	if value == "" {
		if f.Required {
			return nil, errRequired
		}
		return "", nil
	}

	switch f.Kind {
	case CharField, TextField:
		if f.MaxLength > 0 && len([]rune(value)) > f.MaxLength {
			return nil, invalid("Ensure this value has at most " +
				strconv.Itoa(f.MaxLength) + " characters.")
		}
		return value, nil
	case IntegerField:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, invalid("Enter a whole number.")
		}
		return n, nil
	case EmailField:
		if !emailPattern.MatchString(value) {
			return nil, invalid("Enter a valid email address.")
		}
		return value, nil
	case ChoiceField:
		for _, c := range f.Choices {
			if c[0] == value {
				return value, nil
			}
		}
		return nil, invalid("Select a valid choice.")
	case DateTimeField:
		for _, layout := range dateTimeLayouts {
			if t, err := time.Parse(layout, value); err == nil {
				return t, nil
			}
		}
		return nil, invalid("Enter a valid date/time.")
	case BoolField:
		// handled above
		return isTruthy(raw), nil
	default:
		return value, nil
	}
}

// defaultWidget returns the widget to use when a Field has none set, chosen by Kind.
func (f *Field) defaultWidget() Widget {
	if f.Widget != nil {
		return f.Widget
	}
	switch f.Kind {
	case TextField:
		return Textarea{}
	case IntegerField:
		return NumberInput{}
	case BoolField:
		return CheckboxInput{}
	case EmailField:
		return EmailInput{}
	case ChoiceField:
		return Select{Choices: f.Choices}
	case CharField, DateTimeField:
		return TextInput{}
	default:
		return TextInput{}
	}
}
