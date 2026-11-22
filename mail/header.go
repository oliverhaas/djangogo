package mail

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The header code ports the folding of Python's email package
// (email._header_value_parser, policy.default), which Django's generated
// messages depend on byte for byte.

// Character sets of Python's header parser.
const (
	wspChars        = " \t"
	cfwsLeaderChars = " \t("
	specialsChars   = "()<>@,:;.\\\"[]"
	nlChars         = "\r\n"
)

// isWSP reports whether c is folding whitespace.
func isWSP(c byte) bool { return c == ' ' || c == '\t' }

// inSet reports whether the byte c is one of chars. Bytes of multi-byte
// characters are never in a set, as non-ASCII characters are never special.
func inSet(c byte, chars string) bool {
	return c < utf8.RuneSelf && strings.IndexByte(chars, c) >= 0
}

// atomEnds are the characters that end an atom (SPECIALS and WSP).
const atomEnds = specialsChars + wspChars

// dotAtomEnds are the characters that end a dot-atom.
const dotAtomEnds = "()<>@,:;\\\"[] \t"

// phraseEnds are the characters that end a phrase: SPECIALS without '.',
// '"' and '('.
const phraseEnds = "()<>@,:;\\[]"

// tok is a node of a parsed header value, as the token classes of Python's
// email._header_value_parser build them. A terminal holds text; a list holds
// child tokens.
type tok struct {
	// typ is the token type, such as "atom" or "address-list".
	typ string
	// terminal marks a leaf with the text val.
	terminal bool
	val      string
	// ws marks a whitespace terminal; ewWS marks whitespace between two
	// encoded words, which renders as nothing.
	ws, ewWS bool
	// parts holds the children of a list.
	parts []*tok
	// defects holds the problems the parser found in this node.
	defects []defect
	// noEW forbids encoding the token as an encoded word (Python's
	// as_ew_allowed = False).
	noEW bool
	// noBreak marks a token that is no syntactic break.
	noBreak bool
	// invalidClass marks a token built by get_invalid_mailbox, whose
	// address accessors return nothing (Python's InvalidMailbox class).
	invalidClass bool
	// params holds the decoded name and value pairs of a mime-parameters
	// token.
	params []param
}

// defect is a problem the parser found, as the defect classes of Python's
// email.errors.
type defect struct {
	msg string
	// invalid marks an InvalidHeaderDefect, which makes a mailbox invalid.
	invalid bool
}

// param is a decoded MIME parameter.
type param struct{ name, value string }

// Constructors of the token kinds.

func newList(typ string, parts ...*tok) *tok { return &tok{typ: typ, parts: parts} }

func valueTerm(val, typ string) *tok { return &tok{typ: typ, terminal: true, val: val} }

func wsTerm(val, typ string) *tok { return &tok{typ: typ, terminal: true, ws: true, val: val} }

func (t *tok) add(parts ...*tok) { t.parts = append(t.parts, parts...) }

func (t *tok) prepend(p *tok) { t.parts = append([]*tok{p}, t.parts...) }

func (t *tok) addDefect(msg string, invalid bool) {
	t.defects = append(t.defects, defect{msg, invalid})
}

// listSeparator returns the ',' between addresses, which never becomes part
// of an encoded word and is no syntactic break.
func listSeparator() *tok {
	return &tok{typ: "list-separator", terminal: true, val: ",", noEW: true, noBreak: true}
}

// String returns the text of the token as Python's str() does.
func (t *tok) String() string {
	if t.terminal {
		if t.ewWS {
			return ""
		}
		return t.val
	}
	switch t.typ {
	case "bare-quoted-string":
		return quoteString(t.joinStr())
	case "comment":
		var b strings.Builder
		b.WriteByte('(')
		for _, p := range t.parts {
			if p.typ == "comment" {
				b.WriteString(p.String())
				continue
			}
			s := strings.ReplaceAll(p.String(), `\`, `\\`)
			s = strings.ReplaceAll(s, "(", `\(`)
			b.WriteString(strings.ReplaceAll(s, ")", `\)`))
		}
		b.WriteByte(')')
		return b.String()
	case "mime-parameters":
		var items []string
		for _, p := range t.params {
			if p.value != "" {
				items = append(items, p.name+"="+quoteString(p.value))
			} else {
				items = append(items, p.name)
			}
		}
		if len(items) == 0 {
			return ""
		}
		return " " + strings.Join(items, "; ")
	}
	return t.joinStr()
}

// joinStr joins the text of the children.
func (t *tok) joinStr() string {
	var b strings.Builder
	for _, p := range t.parts {
		b.WriteString(p.String())
	}
	return b.String()
}

// value returns the semantic value of the token, as Python's value property
// does: whitespace and comments become one space.
func (t *tok) value() string {
	if t.terminal {
		switch {
		case t.ewWS:
			return ""
		case t.ws:
			return " "
		}
		return t.val
	}
	switch t.typ {
	case "cfws", "comment":
		return " "
	case "bare-quoted-string":
		return t.joinStr()
	case "local-part":
		if t.parts[0].typ == "quoted-string" {
			return t.parts[0].quotedValue()
		}
		return t.parts[0].value()
	case "addr-spec":
		if len(t.parts) < 3 {
			return t.parts[0].value()
		}
		return strings.TrimRightFunc(t.parts[0].value(), pyIsSpace) + t.parts[1].value() +
			strings.TrimLeftFunc(t.parts[2].value(), pyIsSpace)
	case "display-name":
		return t.displayNameValue()
	}
	var b strings.Builder
	for _, p := range t.parts {
		b.WriteString(p.value())
	}
	return b.String()
}

// quotedValue returns a quoted-string with its quotes, as Python's
// QuotedString.quoted_value does.
func (t *tok) quotedValue() string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.typ == "bare-quoted-string" {
			b.WriteString(p.String())
		} else {
			b.WriteString(p.value())
		}
	}
	return b.String()
}

// allDefects returns the defects of the token and its children.
func (t *tok) allDefects() []defect {
	out := append([]defect(nil), t.defects...)
	for _, p := range t.parts {
		out = append(out, p.allDefects()...)
	}
	return out
}

// asEWAllowed reports whether the token may become an encoded word.
func (t *tok) asEWAllowed() bool {
	if t.noEW {
		return false
	}
	for _, p := range t.parts {
		if !p.asEWAllowed() {
			return false
		}
	}
	return true
}

// startsWithFWS reports whether the token starts with folding whitespace.
func (t *tok) startsWithFWS() bool {
	if t.terminal {
		return t.ws && t.val != "" && isWSP(t.val[0])
	}
	if len(t.parts) == 0 {
		return false
	}
	return t.parts[0].startsWithFWS()
}

// quotedPairs escapes the backslashes and double quotes of value for a
// quoted string, as Python's make_quoted_pairs does.
func quotedPairs(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

// quoteString quotes value, as Python's quote_string does.
func quoteString(value string) string {
	return `"` + quotedPairs(value) + `"`
}

// isASCII reports whether s is ASCII only.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// pyIsSpace reports whether r is whitespace for Python's str methods.
func pyIsSpace(r rune) bool {
	switch {
	case r == ' ' || (r >= '\t' && r <= '\r') || (r >= 0x1c && r <= 0x1f):
		return true
	case r < 0x85:
		return false
	}
	switch r {
	case 0x85, 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// endEWBlocked marks the end of the children of a token that must not be
// encoded, as Python's end_ew_not_allowed sentinel does.
var endEWBlocked = &tok{typ: "wrap_as_ew_blocked", terminal: true}

// foldHeader returns the folded header line name: tree with the line ending
// linesep, as Python's BaseHeader.fold does. A nil or empty tree folds to the
// name alone.
func foldHeader(name string, tree *tok, linesep string) string {
	header := newList("header",
		&tok{typ: "header-label", noEW: true, parts: []*tok{valueTerm(name, "header-name"), valueTerm(":", "header-sep")}})
	if tree != nil && (tree.terminal || len(tree.parts) > 0) {
		header.add(newList("cfws", wsTerm(" ", "fws")), tree)
	}
	return refold(header, maxLineLength, linesep)
}

// maxLineLength is the line length policy.default and policy.SMTP fold to.
const maxLineLength = 78

// refold returns the folded text of tree, each line ending in linesep, as
// Python's _refold_parse_tree does. Lengths count characters, not bytes.
func refold(tree *tok, maxlen int, linesep string) string {
	f := folder{lines: []string{""}, maxlen: maxlen}
	lastWordIsEW := false
	lastEW := -1
	lastCharset := ""
	blocked := 0
	wantEncoding := false
	parts := append([]*tok(nil), tree.parts...)
	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		if part == endEWBlocked {
			blocked--
			continue
		}
		tstr := part.String()
		if !wantEncoding {
			if part.typ == "ptext" || part.typ == "vtext" {
				wantEncoding = strings.ContainsAny(tstr, specialsChars+nlChars)
			} else {
				wantEncoding = strings.ContainsAny(tstr, nlChars)
			}
		}
		charset := "us-ascii"
		if !isASCII(tstr) {
			charset = "utf-8"
			wantEncoding = true
		}
		if part.typ == "mime-parameters" {
			f.foldMIMEParameters(part)
			lastWordIsEW = false
			continue
		}
		if wantEncoding && blocked == 0 {
			if !part.asEWAllowed() {
				wantEncoding = false
				lastEW = -1
				if !part.noBreak {
					encoded := part.fold(linesep)
					encoded = encoded[:len(encoded)-len(linesep)]
					if !strings.Contains(encoded, linesep) {
						if runeLen(encoded) > maxlen-f.lastLen() {
							newline := f.stealTrailingWSP()
							f.lines = append(f.lines, newline)
						}
						f.lines[len(f.lines)-1] += encoded
						lastWordIsEW = false
						continue
					}
				}
			}
			if !part.terminal {
				parts = append(append([]*tok(nil), part.parts...), parts...)
				wantEncoding = false
				continue
			} else if part.asEWAllowed() {
				if lastEW >= 0 && charset != lastCharset && lastCharset == "utf-8" && charset != "us-ascii" {
					lastEW = -1
				}
				lastEW = f.foldAsEW(tstr, lastEW, true, charset, lastWordIsEW)
				lastWordIsEW = true
				lastCharset = charset
				wantEncoding = false
				continue
			}
			lastEW = -1
			wantEncoding = false
		}
		if runeLen(tstr) <= maxlen-f.lastLen() {
			f.lines[len(f.lines)-1] += tstr
			lastWordIsEW = lastWordIsEW && strings.Trim(tstr, wspChars) == ""
			continue
		}
		if !part.noBreak && runeLen(tstr)+1 <= maxlen {
			newline := f.stealTrailingWSP()
			if newline != "" || part.startsWithFWS() {
				f.lines = append(f.lines, newline+tstr)
				lastWordIsEW = lastWordIsEW && strings.Trim(f.lines[len(f.lines)-1], wspChars) == ""
				lastEW = -1
				continue
			}
		}
		if !part.terminal {
			newparts := append([]*tok(nil), part.parts...)
			if part.typ == "bare-quoted-string" {
				// Fold the quotes and the quoted pairs of the value as text.
				newparts = make([]*tok, 0, len(part.parts)+2)
				newparts = append(newparts, valueTerm(`"`, "ptext"))
				for _, p := range part.parts {
					newparts = append(newparts, valueTerm(quotedPairs(p.String()), "ptext"))
				}
				newparts = append(newparts, valueTerm(`"`, "ptext"))
			}
			if part.typ == "comment" {
				newparts = []*tok{valueTerm("(", "ptext")}
				for _, p := range part.parts {
					if p.typ == "ptext" {
						s := strings.ReplaceAll(p.String(), `\`, `\\`)
						s = strings.ReplaceAll(s, "(", `\(`)
						newparts = append(newparts, valueTerm(strings.ReplaceAll(s, ")", `\)`), "ptext"))
					} else {
						newparts = append(newparts, p)
					}
				}
				newparts = append(newparts, valueTerm(")", "ptext"))
			}
			if !part.asEWAllowed() {
				blocked++
				newparts = append(newparts, endEWBlocked)
			}
			parts = append(newparts, parts...)
			continue
		}
		if part.asEWAllowed() && blocked == 0 {
			parts = append([]*tok{part}, parts...)
			wantEncoding = true
			continue
		}
		newline := f.stealTrailingWSP()
		if newline != "" || part.startsWithFWS() {
			f.lines = append(f.lines, newline+tstr)
		} else {
			f.lines[len(f.lines)-1] += tstr
		}
		lastWordIsEW = lastWordIsEW && strings.Trim(tstr, wspChars) == ""
	}
	return strings.Join(f.lines, linesep) + linesep
}

// fold returns the folded text of a token on its own, as Python's
// TokenList.fold does. A msg-id is never folded.
func (t *tok) fold(linesep string) string {
	switch t.typ {
	case "msg-id", "message-id", "invalid-message-id":
		return t.String() + linesep
	}
	return refold(t, maxLineLength, linesep)
}

// folder holds the lines of a header while it is folded.
type folder struct {
	lines  []string
	maxlen int
}

// lastLen returns the length of the last line in characters.
func (f *folder) lastLen() int { return runeLen(f.lines[len(f.lines)-1]) }

// stealTrailingWSP removes a trailing whitespace character from the last
// line and returns it, or "". A line it leaves empty is dropped, so the
// caller appends a line right after.
func (f *folder) stealTrailingWSP() string {
	if len(f.lines) == 0 {
		return ""
	}
	last := f.lines[len(f.lines)-1]
	if last == "" || !isWSP(last[len(last)-1]) {
		return ""
	}
	if len(last) == 1 {
		f.lines = f.lines[:len(f.lines)-1]
	} else {
		f.lines[len(f.lines)-1] = last[:len(last)-1]
	}
	return last[len(last)-1:]
}

// foldAsEW adds text to the lines as encoded words, as Python's _fold_as_ew
// does. lastEW is the character offset of an encoded word at the end of the
// last line that the text joins, or -1. lastWordIsEW reports that the last
// word on the lines is an encoded word, which drops the whitespace before
// the next one when decoded. It returns the new offset.
func (f *folder) foldAsEW(text string, lastEW int, combine bool, charset string, lastWordIsEW bool) int {
	toEncode := []rune(text)
	switch {
	case lastEW >= 0 && combine:
		last := []rune(f.lines[len(f.lines)-1])
		// Clamp the offset as a Python slice does.
		cut := min(lastEW, len(last))
		toEncode = []rune(parseUnstructured(string(last[cut:]) + text).String())
		f.lines[len(f.lines)-1] = string(last[:cut])
	case lastWordIsEW:
		// Move the whitespace into the encoded word, where it survives
		// decoding.
		last := f.lines[len(f.lines)-1]
		trimmed := strings.TrimRight(last, wspChars)
		leading := last[len(trimmed):]
		if leading != "" {
			trimmed += " "
		}
		f.lines[len(f.lines)-1] = trimmed
		toEncode = []rune(leading + text)
	case len(toEncode) > 0 && (toEncode[0] == ' ' || toEncode[0] == '\t'):
		leading := string(toEncode[0])
		toEncode = toEncode[1:]
		if f.lastLen() == f.maxlen {
			newline := f.stealTrailingWSP()
			f.lines = append(f.lines, newline)
		}
		f.lines[len(f.lines)-1] += leading
	}
	trailing := ""
	if n := len(toEncode); n > 0 && (toEncode[n-1] == ' ' || toEncode[n-1] == '\t') {
		trailing = string(toEncode[n-1])
		toEncode = toEncode[:n-1]
	}
	newLastEW := lastEW
	if lastEW < 0 {
		newLastEW = f.lastLen()
	}
	encodeAs := charset
	if charset == "us-ascii" {
		encodeAs = "utf-8"
	}
	chromeLen := len(encodeAs) + 7
	for len(toEncode) > 0 {
		remaining := f.maxlen - f.lastLen()
		textSpace := remaining - chromeLen
		if textSpace <= 0 {
			newline := f.stealTrailingWSP()
			if newline == "" {
				newline = " "
			}
			f.lines = append(f.lines, newline)
			newLastEW = f.lastLen()
			continue
		}
		word := fitWord(toEncode[:min(textSpace, len(toEncode))], encodeAs, remaining)
		f.lines[len(f.lines)-1] += encodeWord(string(word), encodeAs)
		toEncode = toEncode[len(word):]
		if len(toEncode) > 0 {
			f.lines = append(f.lines, " ")
			newLastEW = f.lastLen()
		}
	}
	f.lines[len(f.lines)-1] += trailing
	if combine {
		return newLastEW
	}
	return -1
}

// foldMIMEParameters adds the parameters of part to the lines, as Python's
// _fold_mime_parameters does: a value that is not ASCII or does not fit on
// a line uses the encoding of RFC 2231.
func (f *folder) foldMIMEParameters(part *tok) {
	maxlen := f.maxlen
	for _, p := range part.params {
		last := f.lines[len(f.lines)-1]
		if !strings.HasSuffix(strings.TrimRightFunc(last, pyIsSpace), ";") {
			f.lines[len(f.lines)-1] += ";"
		}
		charset := "us-ascii"
		encodingRequired := !isASCII(p.value)
		var tstr string
		if encodingRequired {
			charset = "utf-8"
			tstr = p.name + "*=" + charset + "''" + urlQuote(p.value)
		} else {
			tstr = p.name + "=" + quoteString(p.value)
		}
		if f.lastLen()+runeLen(tstr)+1 < maxlen {
			f.lines[len(f.lines)-1] += " " + tstr
			continue
		} else if runeLen(tstr)+2 <= maxlen {
			f.lines = append(f.lines, " "+tstr)
			continue
		}
		section := 0
		extraChrome := charset + "''"
		value := []rune(p.value)
		for len(value) > 0 {
			chromeLen := len(p.name) + len(strconv.Itoa(section)) + 3 + len(extraChrome)
			if maxlen <= chromeLen+3 {
				maxlen = 78
			}
			maxchars := maxlen - chromeLen - 2
			splitpoint := maxchars
			var encoded string
			for {
				encoded = urlQuote(string(value[:min(splitpoint, len(value))]))
				if len(encoded) <= maxchars {
					break
				}
				splitpoint--
			}
			f.lines = append(f.lines, fmt.Sprintf(" %s*%d*=%s%s", p.name, section, extraChrome, encoded))
			extraChrome = ""
			section++
			value = value[min(splitpoint, len(value)):]
			if len(value) > 0 {
				f.lines[len(f.lines)-1] += ";"
			}
		}
	}
}

// runeLen returns the number of characters of s.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// urlQuote percent-encodes the UTF-8 bytes of s except letters, digits and
// "_.-~", as Python's urllib.parse.quote(s, safe="") does.
func urlQuote(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("_.-~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// qSafe reports whether the Q encoding of an encoded word keeps c as it is.
func qSafe(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("-!*+/", c) >= 0
}

// qLen returns the length of byte c in the Q encoding.
func qLen(c byte) int {
	if qSafe(c) || c == ' ' {
		return 1
	}
	return 3
}

// useBase64 reports whether an encoded word of data uses base64, given the
// length of its Q encoding: the Q encoding loses when it is at least 5
// characters longer.
func useBase64(qlen, size int) (bool, int) {
	blen := (size + 2) / 3 * 4
	return qlen-blen >= 5, blen
}

// fitWord returns the longest prefix of word whose encoded word is at most
// space characters long, or the empty prefix when none is. Python's
// _fold_as_ew finds it by dropping one character at a time and encoding
// again; computing the length of every prefix in one pass gives the same
// prefix in linear time.
func fitWord(word []rune, charset string, space int) []rune {
	chrome := len(charset) + 7
	fits := 0
	qlen, size := 0, 0
	var buf [utf8.UTFMax]byte
	for i, r := range word {
		n := utf8.EncodeRune(buf[:], r)
		for _, c := range buf[:n] {
			qlen += qLen(c)
		}
		size += n
		length := qlen
		if b64, blen := useBase64(qlen, size); b64 {
			length = blen
		}
		if chrome+length <= space {
			fits = i + 1
		}
	}
	return word[:fits]
}

// encodeWord returns s as an RFC 2047 encoded word, choosing the Q encoding
// unless it is at least 5 characters longer than base64, as Python's
// email._encoded_words.encode does.
func encodeWord(s, charset string) string {
	data := []byte(s)
	qlen := 0
	for _, c := range data {
		qlen += qLen(c)
	}
	if b64, _ := useBase64(qlen, len(data)); b64 {
		return "=?" + charset + "?b?" + base64.StdEncoding.EncodeToString(data) + "?="
	}
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.WriteString("=?" + charset + "?q?")
	for _, c := range data {
		switch {
		case c == ' ':
			b.WriteByte('_')
		case qSafe(c):
			b.WriteByte(c)
		default:
			b.WriteByte('=')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	b.WriteString("?=")
	return b.String()
}
