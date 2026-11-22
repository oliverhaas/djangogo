package orm

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// QuoteValue renders v as an SQL literal of dialect d, for a statement that
// cannot bind parameters, such as the DDL of a column default or of a check
// constraint (Django's SchemaEditor.quote_value). Strings are escaped by
// doubling the single quote, which is safe on both dialects with standard
// conforming strings. A NUL byte is rejected because neither dialect accepts
// it.
func QuoteValue(d Dialect, v any) (string, error) {
	sqlite := d.Name() == "sqlite"
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		switch {
		case !sqlite:
			return strconv.FormatBool(x), nil
		case x:
			return "1", nil
		default:
			return "0", nil
		}
	case int:
		return strconv.Itoa(x), nil
	case int8, int16, int32, int64:
		return fmt.Sprint(x), nil
	case uint, uint8, uint16, uint32, uint64:
		n, err := bindUntyped(x)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(n), nil
	case float32:
		return quoteFloat(float64(x))
	case float64:
		return quoteFloat(x)
	case Decimal:
		return x.FixedString(), nil
	case Date:
		if x.IsZero() {
			return "NULL", nil
		}
		return "'" + x.String() + "'", nil
	case Time:
		return "'" + x.String() + "'", nil
	case time.Duration:
		if sqlite {
			return strconv.FormatInt(int64(x/time.Microsecond), 10), nil
		}
		return "'" + FormatPGInterval(x) + "'", nil
	case UUID:
		if sqlite {
			return "'" + x.Hex() + "'", nil
		}
		return "'" + x.String() + "'", nil
	case json.RawMessage:
		return quoteString(string(x))
	case []byte:
		encoded := hex.EncodeToString(x)
		if sqlite {
			return "X'" + encoded + "'", nil
		}
		return "'\\x" + encoded + "'::bytea", nil
	case string:
		return quoteString(x)
	case time.Time:
		return "'" + x.UTC().Format("2006-01-02 15:04:05.999999-07:00") + "'", nil
	case netip.Addr:
		if !x.IsValid() {
			return "NULL", nil
		}
		return "'" + x.String() + "'", nil
	default:
		return "", fmt.Errorf("orm: no SQL literal for a value of type %T", v)
	}
}

// quoteFloat renders a finite float as an SQL number.
func quoteFloat(x float64) (string, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "", fmt.Errorf("orm: no SQL literal for the non-finite float %v", x)
	}
	return strconv.FormatFloat(x, 'g', -1, 64), nil
}

// quoteString renders s as an SQL string literal.
func quoteString(s string) (string, error) {
	if strings.ContainsRune(s, 0) {
		return "", errors.New("orm: an SQL literal must not contain a NUL byte")
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", nil
}
