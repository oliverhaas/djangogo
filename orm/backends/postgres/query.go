package postgres

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

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
	return orm.Features{MaxQueryParams: maxQueryParams, SelectForUpdate: true, UniqueNullsNotDistinct: true, CastJoinColumns: true}
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

// LiteralSQL casts the placeholder of an expression literal to the SQL type
// of its Go value. Without the cast the server has to infer the type of the
// parameter, and where nothing gives a hint, as in "SELECT MAX($1)", it
// assumes text and the driver then cannot send a number.
func (Dialect) LiteralSQL(placeholder string, v any) string {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return placeholder + "::bigint"
	case float32, float64:
		return placeholder + "::double precision"
	case bool:
		return placeholder + "::boolean"
	case string:
		return placeholder + "::text"
	case time.Time:
		return placeholder + "::timestamptz"
	case time.Duration:
		return placeholder + "::interval"
	case orm.Decimal:
		return placeholder + "::numeric"
	case orm.Date:
		return placeholder + "::date"
	case orm.Time:
		return placeholder + "::time"
	case orm.UUID:
		return placeholder + "::uuid"
	case json.RawMessage:
		return placeholder + "::jsonb"
	case []byte:
		return placeholder + "::bytea"
	case netip.Addr:
		return placeholder + "::inet"
	default:
		return placeholder
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
