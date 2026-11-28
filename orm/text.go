package orm

import "errors"

// The SQL names of the text functions that resolveText binds.
const (
	fnChr      = "CHR"
	fnLeft     = "LEFT"
	fnLPad     = "LPAD"
	fnMD5      = "MD5"
	fnOrd      = "ASCII"
	fnRepeat   = "REPEAT"
	fnReplace  = "REPLACE"
	fnReverse  = "REVERSE"
	fnRight    = "RIGHT"
	fnRPad     = "RPAD"
	fnSHA1     = "SHA1"
	fnSHA224   = "SHA224"
	fnSHA256   = "SHA256"
	fnSHA384   = "SHA384"
	fnSHA512   = "SHA512"
	fnStrIndex = "INSTR"
	fnSubstr   = "SUBSTRING"
)

// Chr returns the character with the Unicode code point arg, Django's Chr.
// arg is a field key, an integer code point, or an Expr. On PostgreSQL the
// SQL function is CHR, and on SQLite CHAR. The value is a string, and nil
// when arg is NULL.
func Chr(arg any) Expr { return call(fnChr, []any{arg}) }

// Left returns the first length characters of arg, Django's Left. Each
// argument is a field key, a literal or an Expr. On PostgreSQL the SQL
// function is LEFT, and on SQLite SUBSTR(arg, 1, length). An int length
// below 1 makes the query fail with Django's message. The value is a string.
func Left(arg, length any) Expr { return call(fnLeft, []any{arg, length}) }

// LPad returns arg padded on the left with fill to length characters, or cut
// to length characters when arg is longer, Django's LPad. Pass Value(" ") for
// fill to match Django's default. A nil length yields NULL, and an int length
// below 0 makes the query fail with Django's message. On SQLite the SQL
// function is LPAD, registered by this package's SQLite backend.
func LPad(arg, length, fill any) Expr { return call(fnLPad, []any{arg, length, fill}) }

// MD5 returns the 32-character lowercase hexadecimal MD5 digest of the UTF-8
// text of arg, Django's MD5. On SQLite the SQL function is MD5, registered by
// this package's SQLite backend. The value is a string.
func MD5(arg any) Expr { return call(fnMD5, []any{arg}) }

// Ord returns the Unicode code point of the first character of arg as an
// int64, Django's Ord. On PostgreSQL the SQL function is ASCII, and on SQLite
// UNICODE.
func Ord(arg any) Expr { return call(fnOrd, []any{arg}) }

// Repeat returns the text of arg repeated number times, Django's Repeat. A
// nil number yields NULL, and an int number below 0 makes the query fail with
// Django's message. On SQLite the SQL function is REPEAT, registered by this
// package's SQLite backend.
func Repeat(arg, number any) Expr { return call(fnRepeat, []any{arg, number}) }

// Replace returns arg with every occurrence of text replaced by replacement,
// Django's Replace. Pass Value("") for replacement to match Django's default:
//
//	Annotate("clean", orm.Replace("name", orm.Value("R. R. "), orm.Value("")))
func Replace(arg, text, replacement any) Expr {
	return call(fnReplace, []any{arg, text, replacement})
}

// Reverse returns the characters of arg in reverse order, Django's Reverse.
// On SQLite the SQL function is REVERSE, registered by this package's SQLite
// backend.
func Reverse(arg any) Expr { return call(fnReverse, []any{arg}) }

// Right returns the last length characters of arg, Django's Right. On
// PostgreSQL the SQL function is RIGHT, and on SQLite
// SUBSTR(arg, (length * -1), length). An int length below 1 makes the query
// fail with Django's message.
func Right(arg, length any) Expr { return call(fnRight, []any{arg, length}) }

// RPad returns arg padded on the right with fill to length characters, or cut
// to length characters when arg is longer, Django's RPad. Pass Value(" ") for
// fill to match Django's default. A nil length yields NULL, and an int length
// below 0 makes the query fail with Django's message. On SQLite the SQL
// function is RPAD, registered by this package's SQLite backend.
func RPad(arg, length, fill any) Expr { return call(fnRPad, []any{arg, length, fill}) }

// SHA1 returns the lowercase hexadecimal SHA-1 digest of the UTF-8 text of
// arg, Django's SHA1. On SQLite it calls the SHA1 function registered by this
// package's SQLite backend. On PostgreSQL it is
// ENCODE(DIGEST(arg, 'sha1'), 'hex'), which requires the pgcrypto extension.
func SHA1(arg any) Expr { return call(fnSHA1, []any{arg}) }

// SHA224 returns the lowercase hexadecimal SHA-224 digest of the UTF-8 text
// of arg, Django's SHA224. On SQLite it calls the registered SHA224 function,
// and on PostgreSQL the built-in ENCODE(SHA224(CONVERT_TO(arg, 'UTF8')), 'hex').
func SHA224(arg any) Expr { return call(fnSHA224, []any{arg}) }

// SHA256 returns the lowercase hexadecimal SHA-256 digest of the UTF-8 text
// of arg, Django's SHA256. On SQLite it calls the registered SHA256 function,
// and on PostgreSQL the built-in ENCODE(SHA256(CONVERT_TO(arg, 'UTF8')), 'hex').
func SHA256(arg any) Expr { return call(fnSHA256, []any{arg}) }

// SHA384 returns the lowercase hexadecimal SHA-384 digest of the UTF-8 text
// of arg, Django's SHA384. On SQLite it calls the registered SHA384 function,
// and on PostgreSQL the built-in ENCODE(SHA384(CONVERT_TO(arg, 'UTF8')), 'hex').
func SHA384(arg any) Expr { return call(fnSHA384, []any{arg}) }

// SHA512 returns the lowercase hexadecimal SHA-512 digest of the UTF-8 text
// of arg, Django's SHA512. On SQLite it calls the registered SHA512 function,
// and on PostgreSQL the built-in ENCODE(SHA512(CONVERT_TO(arg, 'UTF8')), 'hex').
func SHA512(arg any) Expr { return call(fnSHA512, []any{arg}) }

// StrIndex returns the 1-based position of the first occurrence of substring
// in arg, or 0 when substring is absent, as an int64, Django's StrIndex. On
// PostgreSQL the SQL function is STRPOS, and on SQLite INSTR.
func StrIndex(arg, substring any) Expr { return call(fnStrIndex, []any{arg, substring}) }

// Substr returns length characters of arg starting at the 1-based position
// pos, or the rest of arg when length is nil, Django's Substr. On PostgreSQL
// the SQL function is SUBSTRING, and on SQLite SUBSTR. An int pos below 1
// makes the query fail with Django's message.
func Substr(arg, pos, length any) Expr { return call(fnSubstr, []any{arg, pos, length}) }

// checkTextArgs validates the literal integer arguments of Left, Right, LPad,
// RPad, Repeat and Substr with Django's error messages.
func checkTextArgs(e Expr) error {
	if len(e.args) < 2 {
		return nil
	}
	var n int64
	switch v := e.args[1].value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	default:
		return nil
	}
	switch e.fn {
	case fnLeft, fnRight:
		if n < 1 {
			return errors.New("orm: 'length' must be greater than 0.") //nolint:revive,staticcheck // Django's message
		}
	case fnLPad, fnRPad:
		if n < 0 {
			return errors.New("orm: 'length' must be greater or equal to 0.") //nolint:revive,staticcheck // Django's message
		}
	case fnRepeat:
		if n < 0 {
			return errors.New("orm: 'number' must be greater or equal to 0.") //nolint:revive,staticcheck // Django's message
		}
	case fnSubstr:
		if n < 1 {
			return errors.New("orm: 'pos' must be greater than 0") //nolint:revive,staticcheck // Django's message
		}
	}
	return nil
}

// isTextIntArg reports whether argument i of function fn is an integer count,
// length, position or code point, which PostgreSQL's built-in text functions
// take as an INTEGER rather than the BIGINT that LiteralSQL binds.
func isTextIntArg(fn string, i int) bool {
	switch fn {
	case fnChr:
		return i == 0
	case fnLeft, fnRight, fnLPad, fnRPad, fnRepeat:
		return i == 1
	case fnSubstr:
		return i == 1 || i == 2
	default:
		return false
	}
}

// resolveText binds Chr, Left, LPad, MD5, Ord, Repeat, Replace, Reverse,
// Right, RPad, SHA1, SHA224, SHA256, SHA384, SHA512, StrIndex or Substr.
func (c *compiler) resolveText(e Expr) (sqlNode, error) {
	if err := checkTextArgs(e); err != nil {
		return nil, err
	}
	sqlite := c.d.Name() == "sqlite"
	args := make([]sqlNode, 0, len(e.args))
	for i, a := range e.args {
		if e.fn == fnSubstr && i == 2 && a.kind == exprValue && a.value == nil {
			continue
		}
		n, err := c.resolveExpr(a)
		if err != nil {
			return nil, err
		}
		if !sqlite && isTextIntArg(e.fn, i) {
			n = castNode{node: n, typ: "INTEGER"}
		}
		args = append(args, n)
	}
	switch e.fn {
	case fnChr:
		fn := "CHR"
		if sqlite {
			fn = "CHAR"
		}
		return callNode{fn: fn, args: args, typ: typeString}, nil
	case fnOrd:
		fn := "ASCII"
		if sqlite {
			fn = "UNICODE"
		}
		return callNode{fn: fn, args: args, typ: typeInt}, nil
	case fnStrIndex:
		fn := "STRPOS"
		if sqlite {
			fn = "INSTR"
		}
		return callNode{fn: fn, args: args, typ: typeInt}, nil
	case fnSubstr:
		fn := "SUBSTRING"
		if sqlite {
			fn = "SUBSTR"
		}
		return callNode{fn: fn, args: args, typ: typeString}, nil
	case fnLeft:
		if sqlite {
			return callNode{
				fn:   "SUBSTR",
				args: []sqlNode{args[0], rawNode{sql: "1", typ: typeInt}, args[1]},
				typ:  typeString,
			}, nil
		}
	case fnRight:
		if sqlite {
			neg := binaryNode{op: "*", left: args[1], right: rawNode{sql: "-1", typ: typeInt}}
			return callNode{
				fn:   "SUBSTR",
				args: []sqlNode{args[0], neg, args[1]},
				typ:  typeString,
			}, nil
		}
	case fnSHA1:
		if !sqlite {
			digest := callNode{
				fn:   "DIGEST",
				args: []sqlNode{args[0], rawNode{sql: "'sha1'", typ: typeString}},
				typ:  typeBinary,
			}
			return callNode{
				fn:   "ENCODE",
				args: []sqlNode{digest, rawNode{sql: "'hex'", typ: typeString}},
				typ:  typeString,
			}, nil
		}
	case fnSHA224, fnSHA256, fnSHA384, fnSHA512:
		if !sqlite {
			utf8 := callNode{
				fn:   "CONVERT_TO",
				args: []sqlNode{args[0], rawNode{sql: "'UTF8'", typ: typeString}},
				typ:  typeBinary,
			}
			hash := callNode{fn: e.fn, args: []sqlNode{utf8}, typ: typeBinary}
			return callNode{
				fn:   "ENCODE",
				args: []sqlNode{hash, rawNode{sql: "'hex'", typ: typeString}},
				typ:  typeString,
			}, nil
		}
	}
	return callNode{fn: e.fn, args: args, typ: typeString}, nil
}
