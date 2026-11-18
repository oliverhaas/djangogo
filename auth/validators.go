package auth

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"html"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// PasswordError is one reason why a password was refused (a Django
// ValidationError with a code).
type PasswordError struct {
	// Code names the rule, such as "password_too_short".
	Code string
	// Message is the text for the user, in Django's wording.
	Message string
	// Params holds the values the message was built from, under Django's
	// names: "min_length" or "verbose_name".
	Params map[string]any
}

// Error returns the message.
func (e *PasswordError) Error() string { return e.Message }

// PasswordErrors is what ValidatePassword returns for a refused password: one
// PasswordError per validator that refused it, in the order of the validators.
type PasswordErrors []*PasswordError

// Error returns the messages, separated by spaces.
func (e PasswordErrors) Error() string { return strings.Join(e.Messages(), " ") }

// Messages returns the message of every error.
func (e PasswordErrors) Messages() []string {
	out := make([]string, len(e))
	for i, pe := range e {
		out[i] = pe.Message
	}
	return out
}

// PasswordValidator is a rule for new passwords (a Django password
// validator).
type PasswordValidator interface {
	// Validate returns nil for a password that passes the rule, and a
	// *PasswordError otherwise. user is the user the password is for; it
	// is nil when no user is known yet.
	Validate(password string, user *User) error
	// HelpText tells the user what the rule asks for.
	HelpText() string
}

// DefaultPasswordValidators returns the validators of a new Django project:
// similarity to the user's attributes, a minimum length of 8, the list of
// common passwords and the ban on passwords made of digits only.
func DefaultPasswordValidators() []PasswordValidator {
	return []PasswordValidator{
		UserAttributeSimilarityValidator{},
		MinimumLengthValidator{},
		CommonPasswordValidator{},
		NumericPasswordValidator{},
	}
}

// defaultValidators is the list used while Config.PasswordValidators is nil.
var defaultValidators = DefaultPasswordValidators()

// ConfiguredPasswordValidators returns the validators in use:
// Config.PasswordValidators, or DefaultPasswordValidators while that is nil
// (Django's get_default_password_validators).
func ConfiguredPasswordValidators() []PasswordValidator {
	if configured := settings().PasswordValidators; configured != nil {
		return configured
	}
	return defaultValidators
}

// passwordValidators returns validators, or the configured list when it is
// nil.
func passwordValidators(validators []PasswordValidator) []PasswordValidator {
	if validators != nil {
		return validators
	}
	return ConfiguredPasswordValidators()
}

// ValidatePassword checks password against validators and returns nil when
// all of them pass, and PasswordErrors with every complaint otherwise
// (Django's validate_password). user may be nil. A nil validators means
// Config.PasswordValidators; an empty, non-nil list accepts every password.
func ValidatePassword(password string, user *User, validators []PasswordValidator) error {
	var errs PasswordErrors
	for _, v := range passwordValidators(validators) {
		err := v.Validate(password, user)
		if err == nil {
			continue
		}
		pe, ok := err.(*PasswordError) //nolint:errorlint // a validator returns the error itself, not a wrapped one
		if !ok {
			pe = &PasswordError{Code: "invalid", Message: err.Error()}
		}
		errs = append(errs, pe)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// PasswordValidatorsHelpTexts returns the help text of every validator
// (Django's password_validators_help_texts). A nil validators means
// Config.PasswordValidators.
func PasswordValidatorsHelpTexts(validators []PasswordValidator) []string {
	list := passwordValidators(validators)
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = v.HelpText()
	}
	return out
}

// PasswordValidatorsHelpTextHTML returns the help texts as an HTML list with
// escaped items, or "" without validators (Django's
// password_validators_help_text_html).
func PasswordValidatorsHelpTextHTML(validators []PasswordValidator) string {
	texts := PasswordValidatorsHelpTexts(validators)
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, text := range texts {
		b.WriteString("<li>" + html.EscapeString(text) + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// MinimumLengthValidator refuses passwords shorter than MinLength characters.
type MinimumLengthValidator struct {
	// MinLength is the shortest accepted length. Zero means 8.
	MinLength int
}

// minLength returns MinLength or its default.
func (v MinimumLengthValidator) minLength() int {
	if v.MinLength > 0 {
		return v.MinLength
	}
	return 8
}

// Validate implements PasswordValidator.
func (v MinimumLengthValidator) Validate(password string, _ *User) error {
	n := v.minLength()
	if utf8.RuneCountInString(password) >= n {
		return nil
	}
	return &PasswordError{
		Code:    "password_too_short",
		Message: fmt.Sprintf("This password is too short. It must contain at least %d %s.", n, plural(n, "character")),
		Params:  map[string]any{"min_length": n},
	}
}

// HelpText implements PasswordValidator.
func (v MinimumLengthValidator) HelpText() string {
	n := v.minLength()
	return fmt.Sprintf("Your password must contain at least %d %s.", n, plural(n, "character"))
}

// plural returns word, with an "s" unless n is 1.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// UserAttributeSimilarityValidator refuses passwords that resemble the name
// or the email address of the user. It compares with the whole value and with
// its parts, such as the two sides of an email address.
type UserAttributeSimilarityValidator struct {
	// UserAttributes are the columns to compare with. Nil means username,
	// first_name, last_name and email. Other names are ignored.
	UserAttributes []string
	// MaxSimilarity is the similarity from which a password is refused, on
	// a scale from 0 to 1. Zero means 0.7. Django refuses values below
	// 0.1, which would reject nearly every password; here they count as
	// 0.1.
	MaxSimilarity float64
}

// userAttributes are the default columns of UserAttributeSimilarityValidator.
var userAttributes = []string{"username", "first_name", "last_name", "email"}

// attribute returns the value and Django's verbose name of a user column.
func attribute(u *User, name string) (value, verboseName string) {
	switch name {
	case "username":
		return u.Username, "username"
	case "first_name":
		return u.FirstName, "first name"
	case "last_name":
		return u.LastName, "last name"
	case "email":
		return u.Email, "email address"
	default:
		return "", ""
	}
}

// Validate implements PasswordValidator.
func (v UserAttributeSimilarityValidator) Validate(password string, user *User) error {
	if user == nil {
		return nil
	}
	maxSimilarity := v.MaxSimilarity
	switch {
	case maxSimilarity == 0:
		maxSimilarity = 0.7
	case maxSimilarity < 0.1:
		maxSimilarity = 0.1
	}
	attrs := v.UserAttributes
	if attrs == nil {
		attrs = userAttributes
	}
	password = strings.ToLower(password)
	for _, name := range attrs {
		value, verboseName := attribute(user, name)
		if value == "" {
			continue
		}
		value = strings.ToLower(value)
		for _, part := range append(splitNonWord(value), value) {
			if exceedsMaximumLengthRatio(password, maxSimilarity, part) {
				continue
			}
			if quickRatio(password, part) >= maxSimilarity {
				return &PasswordError{
					Code:    "password_too_similar",
					Message: "The password is too similar to the " + verboseName + ".",
					Params:  map[string]any{"verbose_name": verboseName},
				}
			}
		}
	}
	return nil
}

// HelpText implements PasswordValidator.
func (UserAttributeSimilarityValidator) HelpText() string {
	return "Your password can’t be too similar to your other personal information."
}

// isWord reports whether r is a word character of Python's \w.
func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// splitNonWord is Python's re.split(r"\W+", s): the runs of word characters
// of s, with an empty string where s starts or ends with other characters.
func splitNonWord(s string) []string {
	var parts []string
	start, inGap := 0, false
	for i, r := range s {
		switch {
		case !isWord(r) && !inGap:
			parts = append(parts, s[start:i])
			inGap = true
		case isWord(r) && inGap:
			start, inGap = i, false
		}
	}
	if inGap {
		return append(parts, "")
	}
	return append(parts, s[start:])
}

// exceedsMaximumLengthRatio reports that value is too short next to password
// to reach the similarity limit, which saves the comparison (Django's
// exceeds_maximum_length_ratio).
func exceedsMaximumLengthRatio(password string, maxSimilarity float64, value string) bool {
	passwordLen, valueLen := utf8.RuneCountInString(password), utf8.RuneCountInString(value)
	return passwordLen >= 10*valueLen && float64(valueLen) < maxSimilarity/2*float64(passwordLen)
}

// quickRatio is difflib.SequenceMatcher(a=a, b=b).quick_ratio(): twice the
// number of characters the two strings share, counted without regard to
// order, over their total length. It is an upper bound of their similarity.
func quickRatio(a, b string) float64 {
	total := utf8.RuneCountInString(a) + utf8.RuneCountInString(b)
	if total == 0 {
		return 1
	}
	avail := make(map[rune]int)
	for _, r := range b {
		avail[r]++
	}
	matches := 0
	for _, r := range a {
		if avail[r] > 0 {
			matches++
		}
		avail[r]--
	}
	return 2 * float64(matches) / float64(total)
}

// commonPasswordsGz is the list of 20,000 common passwords that Django ships
// (django/contrib/auth/common-passwords.txt.gz), compiled by Royce Williams:
// one lowercase password per line.
//
//go:embed common-passwords.txt.gz
var commonPasswordsGz []byte

// commonPasswords unpacks the embedded list on first use.
var commonPasswords = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{}, 20000)
	zr, err := gzip.NewReader(bytes.NewReader(commonPasswordsGz))
	if err != nil {
		return set
	}
	scanner := bufio.NewScanner(zr)
	for scanner.Scan() {
		set[strings.TrimSpace(scanner.Text())] = struct{}{}
	}
	return set
})

// CommonPasswordValidator refuses the passwords of a list of common
// passwords, compared in lower case. The zero value uses the 20,000 passwords
// Django ships.
type CommonPasswordValidator struct {
	// passwords is a custom list. Nil means the embedded one.
	passwords map[string]struct{}
}

// NewCommonPasswordValidator returns a validator for a custom list (Django's
// password_list_path option). The passwords must be in lower case.
func NewCommonPasswordValidator(passwords []string) CommonPasswordValidator {
	set := make(map[string]struct{}, len(passwords))
	for _, p := range passwords {
		set[strings.TrimSpace(p)] = struct{}{}
	}
	return CommonPasswordValidator{passwords: set}
}

// Validate implements PasswordValidator.
func (v CommonPasswordValidator) Validate(password string, _ *User) error {
	list := v.passwords
	if list == nil {
		list = commonPasswords()
	}
	if _, common := list[strings.TrimSpace(strings.ToLower(password))]; common {
		return &PasswordError{Code: "password_too_common", Message: "This password is too common."}
	}
	return nil
}

// HelpText implements PasswordValidator.
func (CommonPasswordValidator) HelpText() string {
	return "Your password can’t be a commonly used password."
}

// NumericPasswordValidator refuses passwords that consist of digits only.
type NumericPasswordValidator struct{}

// Validate implements PasswordValidator.
func (NumericPasswordValidator) Validate(password string, _ *User) error {
	if password == "" || strings.ContainsFunc(password, func(r rune) bool { return !unicode.IsDigit(r) }) {
		return nil
	}
	return &PasswordError{Code: "password_entirely_numeric", Message: "This password is entirely numeric."}
}

// HelpText implements PasswordValidator.
func (NumericPasswordValidator) HelpText() string {
	return "Your password can’t be entirely numeric."
}

// Messages of the username validators, in Django's wording.
const (
	// unicodeUsernameMessage is the message of ValidateUsername.
	unicodeUsernameMessage = "Enter a valid username. This value may contain only letters, " +
		"numbers, and @/./+/-/_ characters."
	// asciiUsernameMessage is the message of ValidateASCIIUsername.
	asciiUsernameMessage = "Enter a valid username. This value may contain only unaccented lowercase a-z " +
		"and uppercase A-Z letters, numbers, and @/./+/-/_ characters."
)

// UsernameError is returned for a username with characters that are not
// allowed. Its text is Django's message for the user.
type UsernameError struct {
	// Message is the text for the user.
	Message string
}

// Error returns the message.
func (e *UsernameError) Error() string { return e.Message }

// ValidateUsername returns nil for a username made of letters, digits and
// the characters "@.+-_", and a *UsernameError otherwise (Django's
// UnicodeUsernameValidator, the validator of the username column).
func ValidateUsername(username string) error {
	if validUsername(username, isWord) {
		return nil
	}
	return &UsernameError{Message: unicodeUsernameMessage}
}

// ValidateASCIIUsername is ValidateUsername for ASCII letters and digits only
// (Django's ASCIIUsernameValidator).
func ValidateASCIIUsername(username string) error {
	ascii := func(r rune) bool { return r < utf8.RuneSelf && isWord(r) }
	if validUsername(username, ascii) {
		return nil
	}
	return &UsernameError{Message: asciiUsernameMessage}
}

// validUsername reports whether username is not empty and consists of word
// characters and "@.+-".
func validUsername(username string, word func(rune) bool) bool {
	if username == "" {
		return false
	}
	for _, r := range username {
		if !word(r) && !strings.ContainsRune(".@+-", r) {
			return false
		}
	}
	return true
}
