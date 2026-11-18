package postgres

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oliverhaas/djangogo/orm"
)

// The SQLSTATE codes of class 23, integrity constraint violation.
const (
	stateIntegrityClass = "23"
	stateNotNull        = "23502"
	stateForeignKey     = "23503"
	stateUnique         = "23505"
	stateCheck          = "23514"
)

// maxQueryParams is the bound-parameter cap of the PostgreSQL wire protocol,
// whose parameter count is a 16-bit integer.
const maxQueryParams = 65535

// Features reports the SQL capabilities of PostgreSQL: row locking with
// SELECT ... FOR UPDATE and the protocol's parameter cap.
func (Dialect) Features() orm.Features {
	return orm.Features{MaxQueryParams: maxQueryParams, SelectForUpdate: true}
}

// LookupSQL returns the SQL of the text matching lookups. The column is cast
// to text so that the lookups also work on a non-text column, and the
// case-insensitive ones compare the upper-cased sides, because LIKE is
// case-sensitive on PostgreSQL. The regex lookups use the POSIX operators.
func (Dialect) LookupSQL(lookup, lhs, rhs string) (string, bool) {
	switch lookup {
	case "contains", "startswith", "endswith":
		return lhs + `::text LIKE ` + rhs + ` ESCAPE '\'`, true
	case "iexact", "icontains", "istartswith", "iendswith":
		return `UPPER(` + lhs + `::text) LIKE UPPER(` + rhs + `) ESCAPE '\'`, true
	case "regex":
		return lhs + "::text ~ " + rhs, true
	case "iregex":
		return lhs + "::text ~* " + rhs, true
	default:
		return "", false
	}
}

// ClassifyError maps a constraint violation reported by the server to its
// kind, by its SQLSTATE.
func (Dialect) ClassifyError(err error) orm.IntegrityKind {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return orm.IntegrityNone
	}
	switch pe.Code {
	case stateUnique:
		return orm.IntegrityUnique
	case stateForeignKey:
		return orm.IntegrityForeignKey
	case stateNotNull:
		return orm.IntegrityNotNull
	case stateCheck:
		return orm.IntegrityCheck
	}
	if strings.HasPrefix(pe.Code, stateIntegrityClass) {
		return orm.IntegrityOther
	}
	return orm.IntegrityNone
}
