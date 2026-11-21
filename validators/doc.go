// Package validators provides Django-compatible validation errors and value
// validators shared by the ORM and forms packages.
//
// All error messages and codes mirror django.core.validators verbatim. The
// package depends only on the Go standard library so both orm and forms can
// import it without a cycle.
package validators
