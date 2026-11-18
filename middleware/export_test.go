package middleware

import "net/http"

// GZipWithRandBelow is GZip with randBelow as the source of the length of the
// random file name in the gzip header. The ported Django tests patch
// secrets.randbelow the same way.
func GZipWithRandBelow(randBelow func(n int) int) func(http.Handler) http.Handler {
	return newGZip(randBelow)
}
