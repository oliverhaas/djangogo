package orm

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strings"
)

// UUID is a 128-bit universally unique identifier (RFC 4122 / RFC 9562),
// represented as a 16-byte value type using only the standard library (no
// external go.mod dependency).
//
// UUID is comparable with == and usable as a map key. Its zero value is the
// nil UUID (00000000-0000-0000-0000-000000000000); use *orm.UUID for a
// nullable column that distinguishes SQL NULL from the nil UUID.
type UUID [16]byte

// uuid4Default is the sentinel value stored in Field.Default for
// orm:"default=uuid4". Each created row gets a freshly generated UUIDv4.
type uuid4Default struct{}

// DefaultUUID4 marks a KindUUID field whose default generates a fresh UUIDv4 on
// Create and BulkCreate, the counterpart of Django's default=uuid.uuid4.
var DefaultUUID4 any = uuid4Default{}

// NewUUID generates a random version-4 UUID using crypto/rand.
func NewUUID() UUID {
	var u UUID
	if _, err := rand.Read(u[:]); err != nil {
		panic(fmt.Sprintf("orm.NewUUID: %v", err))
	}
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return u
}

// ParseUUID parses a UUID string in any format Python's uuid.UUID accepts:
// canonical hyphenated, 32-char hex without hyphens, braced "{...}",
// "urn:uuid:...", or with arbitrary hyphen placement.
func ParseUUID(s string) (UUID, error) {
	cleaned := strings.TrimSpace(s)
	lower := strings.ToLower(cleaned)
	lower = strings.TrimPrefix(lower, "urn:")
	lower = strings.TrimPrefix(lower, "uuid:")
	if strings.HasPrefix(lower, "{") && strings.HasSuffix(lower, "}") {
		lower = lower[1 : len(lower)-1]
	}
	stripped := strings.ReplaceAll(lower, "-", "")
	if len(stripped) != 32 {
		return UUID{}, fmt.Errorf("orm: %q is not a valid UUID", s)
	}
	var u UUID
	if _, err := hex.Decode(u[:], []byte(stripped)); err != nil {
		return UUID{}, fmt.Errorf("orm: %q is not a valid UUID", s)
	}
	return u, nil
}

// MustUUID parses s with ParseUUID and panics on error.
func MustUUID(s string) UUID {
	u, err := ParseUUID(s)
	if err != nil {
		panic(fmt.Sprintf("orm.MustUUID(%q): %v", s, err))
	}
	return u
}

// IsZero reports whether u is the all-zero nil UUID.
func (u UUID) IsZero() bool { return u == (UUID{}) }

// Hex returns the 32 lowercase hexadecimal digits of u without hyphens, the
// format Django stores in SQLite CHAR(32) columns.
func (u UUID) Hex() string { return hex.EncodeToString(u[:]) }

// String returns the canonical lowercase hyphenated form
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
func (u UUID) String() string {
	var buf [36]byte
	hex.Encode(buf[0:8], u[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], u[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], u[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], u[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], u[10:16])
	return string(buf[:])
}

// Value implements driver.Valuer, returning the canonical hyphenated string.
// Dialects with non-native UUID storage (SQLite) reformat via ValueDialect.
func (u UUID) Value() (driver.Value, error) {
	return u.String(), nil
}

// Scan implements sql.Scanner, accepting 16 raw bytes, 32-char hex, or
// canonical 36-char strings.
func (u *UUID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = UUID{}
		return nil
	case string:
		parsed, err := ParseUUID(v)
		if err != nil {
			return err
		}
		*u = parsed
		return nil
	case []byte:
		if len(v) == 16 {
			copy(u[:], v)
			return nil
		}
		parsed, err := ParseUUID(string(v))
		if err != nil {
			return err
		}
		*u = parsed
		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into UUID", src)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (u UUID) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *UUID) UnmarshalText(text []byte) error {
	parsed, err := ParseUUID(string(text))
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}
