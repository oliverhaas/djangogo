package sqlite

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql/driver"
	"encoding/hex"
	"slices"
	"strings"

	moderncsqlite "modernc.org/sqlite"
)

// lpadFunc implements LPAD(text, length, fill_text), Django's _sqlite_lpad:
// text padded on the left with fill_text to length characters, or cut to
// length characters when text is longer, and NULL when any argument is NULL.
func lpadFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	n, ok := args[1].(int64)
	if !ok {
		return nil, nil
	}
	length := int(n)
	if length <= 0 {
		return "", nil
	}
	text := []rune(textOf(args[0]))
	delta := length - len(text)
	if delta <= 0 {
		return string(text[:length]), nil
	}
	pad := []rune(strings.Repeat(textOf(args[2]), length))
	if len(pad) > delta {
		pad = pad[:delta]
	}
	return string(pad) + string(text), nil
}

// rpadFunc implements RPAD(text, length, fill_text), Django's _sqlite_rpad:
// text padded on the right with fill_text to length characters, or cut to
// length characters when text is longer, and NULL when any argument is NULL.
func rpadFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	n, ok := args[1].(int64)
	if !ok {
		return nil, nil
	}
	length := int(n)
	if length <= 0 {
		return "", nil
	}
	padded := []rune(textOf(args[0]) + strings.Repeat(textOf(args[2]), length))
	if len(padded) > length {
		padded = padded[:length]
	}
	return string(padded), nil
}

// repeatFunc implements REPEAT(text, count), Django's _sqlite_repeat: text
// repeated count times, "" when count is not positive, and NULL when either
// argument is NULL.
func repeatFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	count, ok := args[1].(int64)
	if !ok {
		return nil, nil
	}
	if count <= 0 {
		return "", nil
	}
	return strings.Repeat(textOf(args[0]), int(count)), nil
}

// reverseFunc implements REVERSE(text), Django's _sqlite_reverse: the runes
// of text in reverse order, and NULL for NULL.
func reverseFunc(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	r := []rune(textOf(args[0]))
	slices.Reverse(r)
	return string(r), nil
}

// md5Func implements MD5(text), Django's _sqlite_md5: the lowercase hex MD5
// digest of the UTF-8 bytes of text, and NULL for NULL.
func md5Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := md5.Sum([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

// sha1Func implements SHA1(text), Django's _sqlite_sha1: the lowercase hex
// SHA-1 digest of the UTF-8 bytes of text, and NULL for NULL.
func sha1Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := sha1.Sum([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

// sha224Func implements SHA224(text), Django's _sqlite_sha224: the lowercase
// hex SHA-224 digest of the UTF-8 bytes of text, and NULL for NULL.
func sha224Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := sha256.Sum224([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

// sha256Func implements SHA256(text), Django's _sqlite_sha256: the lowercase
// hex SHA-256 digest of the UTF-8 bytes of text, and NULL for NULL.
func sha256Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

// sha384Func implements SHA384(text), Django's _sqlite_sha384: the lowercase
// hex SHA-384 digest of the UTF-8 bytes of text, and NULL for NULL.
func sha384Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := sha512.Sum384([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

// sha512Func implements SHA512(text), Django's _sqlite_sha512: the lowercase
// hex SHA-512 digest of the UTF-8 bytes of text, and NULL for NULL.
func sha512Func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	sum := sha512.Sum512([]byte(textOf(args[0])))
	return hex.EncodeToString(sum[:]), nil
}
