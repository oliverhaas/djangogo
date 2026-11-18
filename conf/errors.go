package conf

// ImproperlyConfigured reports a project that is configured in a way the
// framework cannot run with (django.core.exceptions.ImproperlyConfigured). The
// message is the whole error text, so Django's wording reaches the user
// unchanged. Match it with errors.As.
type ImproperlyConfigured struct {
	// Msg is the error text.
	Msg string
}

// Error returns the message.
func (e *ImproperlyConfigured) Error() string { return e.Msg }
