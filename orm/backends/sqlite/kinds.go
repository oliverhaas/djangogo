package sqlite

import (
	"fmt"
	"math"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// HasNativeUUID reports false: SQLite stores UUIDs in CHAR(32) without hyphens.
func (Dialect) HasNativeUUID() bool { return false }

// BindValue adapts typed Go values to SQLite's storage conventions:
//   - KindUUID -> 32 lowercase hex characters without hyphens
//   - KindDuration -> signed int64 microseconds (stored in BIGINT)
//   - Unsigned integers -> int64 (within signed 64-bit bounds)
func (Dialect) BindValue(f *orm.Field, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if f != nil {
		switch f.Kind {
		case orm.KindUUID:
			if u, ok := v.(orm.UUID); ok {
				return u.Hex(), nil
			}
		case orm.KindDuration:
			if d, ok := v.(time.Duration); ok {
				return int64(d / time.Microsecond), nil
			}
		}
	}
	switch x := v.(type) {
	case uint:
		if uint64(x) > math.MaxInt64 {
			return nil, fmt.Errorf("sqlite: unsigned value %d exceeds int64 range", x)
		}
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("sqlite: unsigned value %d exceeds int64 range", x)
		}
		return int64(x), nil
	}
	return v, nil
}
