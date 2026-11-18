package conf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// ReadDotEnv parses a .env file into a map of variable names to values.
//
// A line is KEY=value, with an optional "export " in front. Blank lines and
// lines that start with "#" are skipped. Spaces around the "=" are trimmed. A
// name consists of letters, digits and underscores and does not start with a
// digit. A value has one of three forms:
//
//   - unquoted: the text up to the end of the line, with the spaces at both
//     ends trimmed. A "#" after a space starts a comment.
//   - single-quoted: taken literally.
//   - double-quoted: \n, \t, \" and \\ are escapes; any other backslash is kept.
//
// After a quoted value only a comment may follow. A value does not continue on
// the next line, and "${NAME}" is not expanded. When a name occurs twice, the
// last value wins.
//
// A malformed line is an error that gives the file and the line number, as in
// "conf: .env:3: invalid variable name"; the errors of all malformed lines are
// joined. The text of a line is never quoted, because it may hold a secret.
// The error of a file that does not exist matches fs.ErrNotExist.
func ReadDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("conf: %w", err)
	}
	vars := make(map[string]string)
	var errs []error
	line := 0
	for text := range strings.SplitSeq(strings.TrimPrefix(string(data), utf8BOM), "\n") {
		line++
		name, value, ok, err := parseDotEnvLine(text)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("conf: %s:%d: %w", path, line, err))
		case ok:
			vars[name] = value
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return vars, nil
}

// LoadDotEnv reads the given files (".env" when none is given) and sets every
// variable that is not already set in the process environment, so the real
// environment wins. Of two files that set the same variable the first one
// wins: name the most specific file first. A missing file is not an error.
//
// When a file is malformed or unreadable, no variable is set. Call it before
// ApplyEnv, and before other goroutines read the environment.
func LoadDotEnv(paths ...string) error {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	var (
		files []map[string]string
		errs  []error
	)
	for _, path := range paths {
		vars, err := ReadDotEnv(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			errs = append(errs, err)
		default:
			files = append(files, vars)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	for _, vars := range files {
		for name, value := range vars {
			if _, set := os.LookupEnv(name); set {
				continue
			}
			if err := os.Setenv(name, value); err != nil {
				errs = append(errs, fmt.Errorf("conf: set %s: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// parseDotEnvLine parses one line of a .env file. ok is false for a line that
// sets no variable: a blank line or a comment.
func parseDotEnvLine(line string) (name, value string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '#' {
		return "", "", false, nil
	}
	if rest, found := strings.CutPrefix(line, "export"); found && rest != "" && isBlank(rest[0]) {
		line = strings.TrimLeft(rest, " \t")
	}
	name, rest, found := strings.Cut(line, "=")
	if !found {
		return "", "", false, errors.New("expected KEY=value")
	}
	name = strings.TrimRight(name, " \t")
	if !isEnvName(name) {
		return "", "", false, errors.New("invalid variable name")
	}
	value, err = parseDotEnvValue(rest)
	if err != nil {
		return "", "", false, err
	}
	return name, value, true, nil
}

// parseDotEnvValue parses what follows the "=" of a line.
func parseDotEnvValue(rest string) (string, error) {
	quoted := strings.TrimLeft(rest, " \t")
	switch {
	case strings.HasPrefix(quoted, "'"):
		end := strings.IndexByte(quoted[1:], '\'')
		if end < 0 {
			return "", errors.New("unterminated single-quoted value")
		}
		return quoted[1 : 1+end], checkDotEnvTrailer(quoted[end+2:])
	case strings.HasPrefix(quoted, `"`):
		return parseDoubleQuoted(quoted)
	}
	// A "#" starts a comment only after a space, so a value such as a color
	// (#fff) or a URL fragment is kept whole.
	for i := 1; i < len(rest); i++ {
		if rest[i] == '#' && isBlank(rest[i-1]) {
			rest = rest[:i]
			break
		}
	}
	return strings.TrimSpace(rest), nil
}

// parseDoubleQuoted parses a value that starts with a double quote.
func parseDoubleQuoted(s string) (string, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			return b.String(), checkDotEnvTrailer(s[i+1:])
		}
		if c == '\\' && i+1 < len(s) {
			if escaped, ok := dotEnvEscapes[s[i+1]]; ok {
				b.WriteByte(escaped)
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return "", errors.New("unterminated double-quoted value")
}

// dotEnvEscapes maps the character after a backslash in a double-quoted value
// to the character the pair stands for.
var dotEnvEscapes = map[byte]byte{'n': '\n', 't': '\t', '"': '"', '\\': '\\'}

// checkDotEnvTrailer reports text after the closing quote of a value that is
// not a comment.
func checkDotEnvTrailer(rest string) error {
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" || rest[0] == '#' {
		return nil
	}
	return errors.New("unexpected text after the closing quote")
}

// utf8BOM is the byte order mark some editors put at the start of a file.
const utf8BOM = "\xef\xbb\xbf"

// isBlank reports whether c is a space or a tab.
func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// isEnvName reports whether s is a variable name: letters, digits and
// underscores, not starting with a digit.
func isEnvName(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}
