package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/oliverhaas/djangogo/orm"
)

// UsernameMaxLength is the length limit of the username column.
const UsernameMaxLength = 150

// EmailMaxLength is the length limit of the email column.
const EmailMaxLength = 254

// SystemUsername returns the name of the operating system user running the
// program, or "" when it cannot be found (Django's get_system_username, which
// uses Python's getpass.getuser). The environment variables LOGNAME, USER,
// LNAME and USERNAME are tried first, in that order.
func SystemUsername() string {
	for _, name := range []string{"LOGNAME", "USER", "LNAME", "USERNAME"} {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

// DefaultUsername turns the operating system user name system into the
// username that createsuperuser offers as its default (Django's
// get_default_username): reduced to ASCII through NFKD decomposition, without
// spaces and in lower case. It returns "" when the result fails the username
// validators, and, when db is not nil, when a user of that name exists.
func DefaultUsername(ctx context.Context, db *orm.DB, system string) (string, error) {
	var b strings.Builder
	for _, r := range norm.NFKD.String(system) {
		if r < utf8.RuneSelf && r != ' ' {
			b.WriteRune(r)
		}
	}
	name := strings.ToLower(b.String())
	if name == "" {
		return "", nil
	}
	if len(UsernameFieldErrors(name)) > 0 {
		return "", nil
	}
	if db == nil {
		return name, nil
	}
	switch _, err := orm.Query[User](db).Get(ctx, "username", name); {
	case err == nil:
		return "", nil
	case errors.Is(err, orm.ErrDoesNotExist):
		return name, nil
	default:
		return "", fmt.Errorf("auth: look up username: %w", err)
	}
}

// UsernameFieldErrors returns the messages that cleaning a value of the
// username column gives, in Django's order (Field.clean): "This field cannot
// be blank." for an empty value, otherwise the messages of the username
// validator and of the length limit. It returns nil for a valid username. It
// does not look for an existing user of that name.
func UsernameFieldErrors(username string) []string {
	if username == "" {
		return []string{msgFieldBlank}
	}
	var msgs []string
	if err := ValidateUsername(username); err != nil {
		msgs = append(msgs, err.Error())
	}
	return appendMaxLength(msgs, username, UsernameMaxLength)
}

// EmailFieldErrors returns the messages that cleaning a value of the email
// column gives: none for an empty value, which the column allows, otherwise
// the messages of ValidateEmail and of the length limit. It returns nil for a
// valid value.
func EmailFieldErrors(email string) []string {
	if email == "" {
		return nil
	}
	var msgs []string
	if err := ValidateEmail(email); err != nil {
		msgs = append(msgs, err.Error())
	}
	return appendMaxLength(msgs, email, EmailMaxLength)
}

// msgFieldBlank is Django's message of a model field without blank=True.
const msgFieldBlank = "This field cannot be blank."

// appendMaxLength adds the message of Django's MaxLengthValidator to msgs when
// value has more than limit characters.
func appendMaxLength(msgs []string, value string, limit int) []string {
	n := utf8.RuneCountInString(value)
	if n <= limit {
		return msgs
	}
	unit := "characters"
	if limit == 1 {
		unit = "character"
	}
	return append(msgs, fmt.Sprintf("Ensure this value has at most %d %s (it has %d).", limit, unit, n))
}
