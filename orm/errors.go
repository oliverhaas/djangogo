package orm

import "errors"

// ErrDoesNotExist is returned by Get when no row matches.
var ErrDoesNotExist = errors.New("orm: object does not exist")

// ErrMultipleObjectsReturned is returned by Get when more than one row matches.
var ErrMultipleObjectsReturned = errors.New("orm: multiple objects returned")
