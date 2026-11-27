package pyjson

import (
	"math"
	"strconv"
	"strings"
)

// FormatFloat formats f the way Python's repr(float) and json.dumps do.
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}

	sci := strconv.FormatFloat(f, 'e', -1, 64)
	eIdx := strings.IndexByte(sci, 'e')
	exp, _ := strconv.Atoi(sci[eIdx+1:])
	if exp < -4 || exp >= 16 {
		return sci
	}

	fixed := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsRune(fixed, '.') {
		return fixed + ".0"
	}
	return fixed
}
