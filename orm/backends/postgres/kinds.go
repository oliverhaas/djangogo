package postgres

import (
	"fmt"
	"math"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// HasNativeUUID reports true: PostgreSQL stores UUIDs in native UUID columns
// whose text cast includes hyphens.
func (Dialect) HasNativeUUID() bool { return true }

// BindValue adapts typed Go values to PostgreSQL's wire expectations:
//   - KindUUID -> canonical lowercase hyphenated string
//   - KindDuration -> exact PostgreSQL INTERVAL text ("N microseconds")
//   - Unsigned integers -> int64 (within signed 64-bit bounds)
func (Dialect) BindValue(f *orm.Field, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if f != nil {
		switch f.Kind {
		case orm.KindUUID:
			if u, ok := v.(orm.UUID); ok {
				return u.String(), nil
			}
		case orm.KindDuration:
			if d, ok := v.(time.Duration); ok {
				return orm.FormatPGInterval(d), nil
			}
		}
	}
	switch x := v.(type) {
	case uint:
		if uint64(x) > math.MaxInt64 {
			return nil, fmt.Errorf("postgres: unsigned value %d exceeds int64 range", x)
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
			return nil, fmt.Errorf("postgres: unsigned value %d exceeds int64 range", x)
		}
		return int64(x), nil
	}
	return v, nil
}
