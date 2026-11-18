package middleware

import (
	"net/http"
	"slices"
)

// Chain composes middleware into one, the counterpart of Django's MIDDLEWARE
// setting. The first argument is the outermost: it sees the request first and
// the response last. A nil entry is skipped, so a caller can leave out an
// optional middleware without building the list by hand. Without arguments the
// result returns its handler unchanged.
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	// Copy the list: the caller may reuse the slice it spread into the call.
	mws = slices.Clone(mws)
	return func(next http.Handler) http.Handler {
		for _, mw := range slices.Backward(mws) {
			if mw != nil {
				next = mw(next)
			}
		}
		return next
	}
}
