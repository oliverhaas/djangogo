package sqlite

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/oliverhaas/djangogo/orm"

	moderncsqlite "modernc.org/sqlite"
)

// The extended result codes of the SQLITE_CONSTRAINT family, see
// https://www.sqlite.org/rescode.html. The low byte of each is
// SQLITE_CONSTRAINT.
const (
	codeConstraint           = 19
	codeConstraintCheck      = 275
	codeConstraintForeignKey = 787
	codeConstraintNotNull    = 1299
	codeConstraintPrimaryKey = 1555
	codeConstraintUnique     = 2067
	codeConstraintRowID      = 2579
)

// maxQueryParams is the bound-parameter cap this backend plans for. Current
// SQLite allows 32766, but 999 is the limit of every build before 3.32 and of
// builds that keep the old default, so a statement that stays below it runs
// everywhere.
const maxQueryParams = 999

// Features reports the SQL capabilities of SQLite: OFFSET needs a LIMIT, a
// statement takes a limited number of parameters, and there is no row locking.
func (Dialect) Features() orm.Features {
	return orm.Features{NoLimitValue: "-1", MaxQueryParams: maxQueryParams}
}

// LookupSQL returns the SQL of the regex lookups, which run on the regexp
// function this package registers. The LIKE based lookups use the portable
// form: SQLite's LIKE ignores the case of ASCII letters only, so the
// case-sensitive lookups (contains, startswith, endswith) match ASCII letters
// of either case, as they do in Django on SQLite.
func (Dialect) LookupSQL(lookup, lhs, rhs string) (string, bool) {
	switch lookup {
	case "regex":
		return lhs + " REGEXP " + rhs, true
	case "iregex":
		return lhs + " REGEXP '(?i)' || " + rhs, true
	default:
		return "", false
	}
}

// ClassifyError maps a constraint failure reported by the driver to its kind.
func (Dialect) ClassifyError(err error) orm.IntegrityKind {
	var se *moderncsqlite.Error
	if !errors.As(err, &se) {
		return orm.IntegrityNone
	}
	code := se.Code()
	switch code {
	case codeConstraintUnique, codeConstraintPrimaryKey, codeConstraintRowID:
		return orm.IntegrityUnique
	case codeConstraintForeignKey:
		return orm.IntegrityForeignKey
	case codeConstraintNotNull:
		return orm.IntegrityNotNull
	case codeConstraintCheck:
		return orm.IntegrityCheck
	}
	if code&0xff == codeConstraint {
		return orm.IntegrityOther
	}
	return orm.IntegrityNone
}

// regexCacheSize bounds the compiled patterns kept between calls.
const regexCacheSize = 256

// regexCache holds the compiled patterns of the regexp SQL function, which
// SQLite calls once per row with the same pattern.
var regexCache = struct {
	sync.Mutex
	compiled map[string]*regexp.Regexp
}{compiled: make(map[string]*regexp.Regexp)}

// compileRegex returns the compiled form of pattern, from the cache when it
// has been seen before.
func compileRegex(pattern string) (*regexp.Regexp, error) {
	regexCache.Lock()
	defer regexCache.Unlock()
	if re, ok := regexCache.compiled[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	if len(regexCache.compiled) >= regexCacheSize {
		clear(regexCache.compiled)
	}
	regexCache.compiled[pattern] = re
	return re, nil
}

// regexpFunc implements the SQL function behind "value REGEXP pattern", which
// SQLite calls as regexp(pattern, value). A NULL on either side yields NULL.
// The pattern uses Go's regexp syntax (RE2).
func regexpFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	re, err := compileRegex(textOf(args[0]))
	if err != nil {
		return nil, fmt.Errorf("sqlite: regexp: %w", err)
	}
	return re.MatchString(textOf(args[1])), nil
}

// textOf returns the text form of a value SQLite passes to a function.
func textOf(v driver.Value) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

// regexpRegistration is the outcome of installing the regexp function for
// every connection the driver opens; Open reports a failure. SQLite ships the
// REGEXP operator without an implementation.
var regexpRegistration = moderncsqlite.RegisterDeterministicScalarFunction("regexp", 2, regexpFunc)
