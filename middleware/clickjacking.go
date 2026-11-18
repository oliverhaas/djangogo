package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
)

// xframeExemptKey is the context key under which XFrameOptions stores the
// exemption flag of a request.
type xframeExemptKey struct{}

// XFrameOptions is Django's XFrameOptionsMiddleware, a protection against
// clickjacking: it adds the X-Frame-Options header to every response, which
// tells the browser whether the page may be shown in a frame. value is Django's
// X_FRAME_OPTIONS setting, "DENY" or "SAMEORIGIN". It is upper-cased, and the
// empty string means "DENY".
//
// A response that already has the header keeps it, and so does a response of a
// handler that called ExemptXFrameOptions.
//
// Django's middleware is extended by overriding get_xframe_options_value. To
// choose the value per request here, set the header in a middleware inside this
// one or in the handler.
func XFrameOptions(value string) func(http.Handler) http.Handler {
	value = strings.ToUpper(value)
	if value == "" {
		value = "DENY"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A pointer in the context lets the handler report back: the
			// request it gets is a copy, so a plain value would be lost.
			exempt := new(atomic.Bool)
			r = r.WithContext(context.WithValue(r.Context(), xframeExemptKey{}, exempt))

			rw := newResponseWriter(w, func() {
				if !exempt.Load() {
					setDefaultHeader(w.Header(), "X-Frame-Options", value)
				}
			})
			// A handler that returns or panics without writing still gets the
			// header: its response is sent later, by net/http or by Recover.
			defer rw.runBeforeHeader()
			next.ServeHTTP(rw, r)
		})
	}
}

// ExemptXFrameOptions is Django's xframe_options_exempt decorator: it keeps
// XFrameOptions from adding the header to the response of this request, for a
// page that is meant to be shown in a frame on other sites. Call it in the
// handler before the response is started. It does nothing when the request did
// not pass through XFrameOptions.
func ExemptXFrameOptions(r *http.Request) {
	if exempt, ok := r.Context().Value(xframeExemptKey{}).(*atomic.Bool); ok {
		exempt.Store(true)
	}
}
