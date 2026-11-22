package sqlite

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"

	moderncsqlite "modernc.org/sqlite"
)

// statAggregate is STDDEV_POP, STDDEV_SAMP, VAR_POP or VAR_SAMP, which SQLite
// lacks and Django registers from Python's statistics module. NULL values are
// skipped, as PostgreSQL skips them. The value is NULL without a value, and
// for a sample with one value, where PostgreSQL gives NULL and Python's
// statistics raises an error.
type statAggregate struct {
	values []float64
	// sample divides by n-1 instead of n, and root takes the square root of
	// the variance.
	sample, root bool
}

// Step adds the value of one row.
func (a *statAggregate) Step(_ *moderncsqlite.FunctionContext, args []driver.Value) error {
	switch x := args[0].(type) {
	case nil:
	case int64:
		a.values = append(a.values, float64(x))
	case float64:
		a.values = append(a.values, x)
	default:
		f, err := strconv.ParseFloat(textOf(x), 64)
		if err != nil {
			return fmt.Errorf("sqlite: %q is not a number", textOf(x))
		}
		a.values = append(a.values, f)
	}
	return nil
}

// WindowInverse removes the value of the oldest row of the window.
func (a *statAggregate) WindowInverse(_ *moderncsqlite.FunctionContext, args []driver.Value) error {
	if args[0] != nil && len(a.values) > 0 {
		a.values = a.values[1:]
	}
	return nil
}

// WindowValue returns the deviation or the variance of the values.
func (a *statAggregate) WindowValue(*moderncsqlite.FunctionContext) (driver.Value, error) {
	n := len(a.values)
	if n == 0 || (a.sample && n < 2) {
		return nil, nil
	}
	var mean float64
	for _, v := range a.values {
		mean += v
	}
	mean /= float64(n)
	var squares float64
	for _, v := range a.values {
		squares += (v - mean) * (v - mean)
	}
	if a.sample {
		n--
	}
	variance := squares / float64(n)
	if a.root {
		return math.Sqrt(variance), nil
	}
	return variance, nil
}

// Final has nothing to release.
func (*statAggregate) Final(*moderncsqlite.FunctionContext) {}

// statFunction returns the registration of a statAggregate.
func statFunction(sample, root bool) *moderncsqlite.FunctionImpl {
	return &moderncsqlite.FunctionImpl{
		NArgs:         1,
		Deterministic: true,
		MakeAggregate: func(moderncsqlite.FunctionContext) (moderncsqlite.AggregateFunction, error) {
			return &statAggregate{sample: sample, root: root}, nil
		},
	}
}

// anyValueAggregate is ANY_VALUE, which SQLite lacks and Django registers: the
// first value that is not NULL, as PostgreSQL gives one that is not NULL.
type anyValueAggregate struct {
	value driver.Value
}

// Step keeps the value of the row when no value is kept yet.
func (a *anyValueAggregate) Step(_ *moderncsqlite.FunctionContext, args []driver.Value) error {
	if a.value != nil {
		return nil
	}
	if b, ok := args[0].([]byte); ok {
		a.value = append([]byte(nil), b...)
	} else {
		a.value = args[0]
	}
	return nil
}

// WindowInverse keeps the value: any value of the group will do.
func (*anyValueAggregate) WindowInverse(*moderncsqlite.FunctionContext, []driver.Value) error {
	return nil
}

// WindowValue returns the kept value.
func (a *anyValueAggregate) WindowValue(*moderncsqlite.FunctionContext) (driver.Value, error) {
	return a.value, nil
}

// Final has nothing to release.
func (*anyValueAggregate) Final(*moderncsqlite.FunctionContext) {}

// anyValueFunction is the registration of ANY_VALUE.
var anyValueFunction = &moderncsqlite.FunctionImpl{
	NArgs: 1,
	MakeAggregate: func(moderncsqlite.FunctionContext) (moderncsqlite.AggregateFunction, error) {
		return &anyValueAggregate{}, nil
	},
}
