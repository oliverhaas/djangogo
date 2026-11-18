package orm

import "errors"

// ErrDoesNotExist is returned by Get when no row matches.
var ErrDoesNotExist = errors.New("orm: object does not exist")

// ErrMultipleObjectsReturned is returned by Get when more than one row matches.
var ErrMultipleObjectsReturned = errors.New("orm: multiple objects returned")

// ErrIntegrity is matched by every database constraint violation, like
// Django's IntegrityError. The more specific errors below tell the violated
// constraint apart:
//
//	if errors.Is(err, orm.ErrUniqueViolation) { ... }
var ErrIntegrity = errors.New("orm: integrity error")

// ErrUniqueViolation is matched by a violation of a UNIQUE or PRIMARY KEY
// constraint.
var ErrUniqueViolation = errors.New("orm: unique constraint violation")

// ErrForeignKeyViolation is matched by a violation of a FOREIGN KEY
// constraint.
var ErrForeignKeyViolation = errors.New("orm: foreign key constraint violation")

// ErrNotNullViolation is matched by a NULL written to a NOT NULL column.
var ErrNotNullViolation = errors.New("orm: not null constraint violation")

// ErrCheckViolation is matched by a violation of a CHECK constraint.
var ErrCheckViolation = errors.New("orm: check constraint violation")

// IntegrityKind identifies the constraint a statement violated.
type IntegrityKind uint8

const (
	// IntegrityNone means the error is not a constraint violation.
	IntegrityNone IntegrityKind = iota
	// IntegrityOther is a constraint violation of a kind not listed here.
	IntegrityOther
	// IntegrityUnique is a UNIQUE or PRIMARY KEY violation.
	IntegrityUnique
	// IntegrityForeignKey is a FOREIGN KEY violation.
	IntegrityForeignKey
	// IntegrityNotNull is a NOT NULL violation.
	IntegrityNotNull
	// IntegrityCheck is a CHECK violation.
	IntegrityCheck
)

// IntegrityError is a constraint violation reported by the database. It wraps
// the driver's error, whose message it keeps, and matches ErrIntegrity and
// the sentinel of its Kind with errors.Is.
type IntegrityError struct {
	// Kind is the violated constraint.
	Kind IntegrityKind
	// Err is the driver's error.
	Err error
}

// Error returns the driver's message.
func (e *IntegrityError) Error() string { return e.Err.Error() }

// Unwrap returns the driver's error.
func (e *IntegrityError) Unwrap() error { return e.Err }

// Is reports whether target is ErrIntegrity or the sentinel of e.Kind.
func (e *IntegrityError) Is(target error) bool {
	switch target {
	case ErrIntegrity:
		return true
	case ErrUniqueViolation:
		return e.Kind == IntegrityUnique
	case ErrForeignKeyViolation:
		return e.Kind == IntegrityForeignKey
	case ErrNotNullViolation:
		return e.Kind == IntegrityNotNull
	case ErrCheckViolation:
		return e.Kind == IntegrityCheck
	default:
		return false
	}
}

// ErrorClassifier is an optional capability of a Dialect: it recognizes the
// constraint violations among its driver's errors. Without it no error is
// reported as an IntegrityError.
type ErrorClassifier interface {
	// ClassifyError returns the constraint err violated, or IntegrityNone.
	ClassifyError(err error) IntegrityKind
}

// classify wraps a driver error in an *IntegrityError when the dialect
// recognizes it as a constraint violation, and returns it unchanged otherwise.
func (db *DB) classify(err error) error {
	if err == nil {
		return nil
	}
	var known *IntegrityError
	if errors.As(err, &known) {
		return err
	}
	ec, ok := db.dialect.(ErrorClassifier)
	if !ok {
		return err
	}
	kind := ec.ClassifyError(err)
	if kind == IntegrityNone {
		return err
	}
	return &IntegrityError{Kind: kind, Err: err}
}
