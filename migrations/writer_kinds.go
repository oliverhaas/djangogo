package migrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// defaultsEqual compares two FieldState.Default values without panicking on
// slice types ([]byte and json.RawMessage).
func defaultsEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch x := a.(type) {
	case []byte:
		y, ok := b.([]byte)
		return ok && bytes.Equal(x, y)
	case json.RawMessage:
		y, ok := b.(json.RawMessage)
		return ok && bytes.Equal(x, y)
	default:
		return a == b
	}
}

// defaultLiteral renders a field default value as Go source and marks any
// extra imports the generated migration file needs.
func (w *writer) defaultLiteral(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("float64(%s)", strconv.FormatFloat(x, 'g', -1, 64))
	case orm.Decimal:
		w.usesORM = true
		return fmt.Sprintf("orm.MustDecimal(%q)", x.String())
	case orm.Date:
		w.usesORM = true
		return fmt.Sprintf("orm.MustDate(%q)", x.String())
	case orm.Time:
		w.usesORM = true
		return fmt.Sprintf("orm.MustTime(%q)", x.String())
	case time.Duration:
		w.usesTime = true
		return fmt.Sprintf("time.Duration(%d)", int64(x))
	case orm.UUID:
		w.usesORM = true
		return fmt.Sprintf("orm.MustUUID(%q)", x.String())
	case json.RawMessage:
		w.usesJSON = true
		return fmt.Sprintf("json.RawMessage(%q)", string(x))
	case []byte:
		return fmt.Sprintf("%#v", x)
	default:
		if v == orm.DefaultUUID4 {
			w.usesORM = true
			return "orm.DefaultUUID4"
		}
		return literal(v)
	}
}

// oneOffKindDefault renders a one-off default of one of the extended field
// kind Go types.
func (w *writer) oneOffKindDefault(v any) (string, bool) {
	switch v.(type) {
	case float64, orm.Decimal, orm.Date, orm.Time, time.Duration, orm.UUID, json.RawMessage, []byte:
		return "Default: " + w.defaultLiteral(v) + ",\n", true
	}
	if v == orm.DefaultUUID4 {
		w.usesORM = true
		return "Default: orm.DefaultUUID4,\n", true
	}
	return "", false
}
