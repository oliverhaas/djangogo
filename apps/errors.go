package apps

import (
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/conf"
)

// errReentrant reports a call of Ready from inside the ready pass (Django:
// "populate() isn't reentrant").
var errReentrant = errors.New("apps: Ready isn't reentrant")

// LookupError reports a lookup of an app that is not installed, where Django
// raises LookupError. Match it with errors.As.
type LookupError struct {
	// Label is the label that was looked up.
	Label string
	// Suggestion is the label of the installed app that was probably meant;
	// empty when there is none.
	Suggestion string
}

// Error returns Django's message: "No installed app with label 'x'.",
// followed by " Did you mean 'y'?" when there is a suggestion.
func (e *LookupError) Error() string {
	msg := fmt.Sprintf("No installed app with label '%s'.", e.Label)
	if e.Suggestion != "" {
		msg += fmt.Sprintf(" Did you mean '%s'?", e.Suggestion)
	}
	return msg
}

// improperlyConfigured returns a *conf.ImproperlyConfigured with the formatted
// message, so Django's wording reaches the user without a prefix.
func improperlyConfigured(format string, args ...any) error {
	return &conf.ImproperlyConfigured{Msg: fmt.Sprintf(format, args...)}
}
