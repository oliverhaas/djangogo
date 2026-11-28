package sqlite

import (
	"database/sql/driver"
	"errors"
	"math"
	"math/rand/v2"

	moderncsqlite "modernc.org/sqlite"
)

// cotFunc implements COT(x), Django's _sqlite_cot: the cotangent of x, NULL
// for NULL. Python's division by zero fails, and so does a zero tangent.
func cotFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	x, ok := number(args[0])
	if !ok {
		return nil, nil
	}
	tan := math.Tan(x)
	if tan == 0 {
		return nil, errors.New("sqlite: COT: float division by zero")
	}
	return 1 / tan, nil
}

// randFunc implements RAND(), Django's random.random: a float in [0, 1).
// SQLite's RANDOM returns an integer.
func randFunc(_ *moderncsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
	return rand.Float64(), nil //nolint:gosec // Django's random.random is not cryptographic either.
}
