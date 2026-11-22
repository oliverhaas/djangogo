package mail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The parsers below port the ones of Python's email._header_value_parser
// that Django's messages use: unstructured text, address lists and message
// IDs. They build the token trees that refold folds.

// parseError reports text that does not match the grammar, as Python's
// HeaderParseError does. Callers use it to try another production.
type parseError struct {
	msg string
	// invalidEW marks a malformed encoded word (Python's _InvalidEwError).
	invalidEW bool
}

// Error returns the message.
func (e *parseError) Error() string { return e.msg }

// perr returns a parseError with a formatted message.
func perr(format string, args ...any) error {
	return &parseError{msg: fmt.Sprintf(format, args...)}
}

// rfc2047Matcher finds an encoded word inside a word.
var rfc2047Matcher = regexp.MustCompile(`=\?[^?]*\?[qQbB]\?.*?\?=`)

// getFWS splits off leading whitespace.
func getFWS(value string) (*tok, string) {
	rest := strings.TrimLeftFunc(value, pyIsSpace)
	return wsTerm(value[:len(value)-len(rest)], "fws"), rest
}

// splitWSP splits value before its first space or tab.
func splitWSP(value string) (string, string) {
	if i := strings.IndexAny(value, wspChars); i >= 0 {
		return value[:i], value[i:]
	}
	return value, ""
}

// validateXText records the non-printable ASCII characters of a terminal.
func validateXText(t *tok) {
	var found []string
	for i := 0; i < len(t.val); i++ {
		if c := t.val[i]; c <= 0x20 || c == 0x7f {
			found = append(found, pyRepr(string(c)))
		}
	}
	if found != nil {
		t.addDefect("the following ASCII non-printables found in header: ["+strings.Join(found, ", ")+"]", false)
	}
}

// getEncodedWord parses an RFC 2047 encoded word into its decoded text,
// whose words become terminals of type terminalType.
func getEncodedWord(value, terminalType string) (*tok, string, error) {
	if !strings.HasPrefix(value, "=?") {
		return nil, value, perr("expected encoded word but found %s", value)
	}
	word, remstr, found := strings.Cut(value[2:], "?=")
	if !found {
		return nil, value, perr("expected encoded word but found %s", value)
	}
	if len(remstr) > 1 && isHexDigit(remstr[0]) && isHexDigit(remstr[1]) && strings.Count(word, "?") < 2 {
		rest, rem, _ := strings.Cut(remstr, "?=")
		word = word + "?=" + rest
		remstr = rem
	}
	ew := newList("encoded-word")
	if len(strings.Fields(word)) > 1 {
		ew.addDefect("whitespace inside encoded word", true)
	}
	text, ok := decodeEncodedWord("=?" + word + "?=")
	if !ok {
		return nil, value, &parseError{msg: fmt.Sprintf("encoded word format invalid: '%s'", value), invalidEW: true}
	}
	for text != "" {
		if isWSP(text[0]) {
			var t *tok
			t, text = getFWS(text)
			ew.add(t)
			continue
		}
		var chars string
		chars, text = splitWSP(text)
		vtext := valueTerm(chars, terminalType)
		validateXText(vtext)
		ew.add(vtext)
	}
	if remstr != "" && !isWSP(remstr[0]) {
		ew.addDefect("missing trailing whitespace after encoded-word", true)
	}
	return ew, remstr, nil
}

// isHexDigit reports whether c is a hexadecimal digit.
func isHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// decodeEncodedWord decodes "=?charset?cte?text?=". It reports false when
// the word is malformed.
func decodeEncodedWord(ew string) (string, bool) {
	fields := strings.Split(ew, "?")
	if len(fields) != 5 {
		return "", false
	}
	charset, _, _ := strings.Cut(fields[1], "*")
	var data []byte
	switch strings.ToLower(fields[2]) {
	case "q":
		data = decodeQ(fields[3])
	case "b":
		data = decodeB(fields[3])
	default:
		return "", false
	}
	if text, err := decodeCharset(data, charset); err == nil {
		return text, true
	}
	return string(data), true
}

// decodeQ decodes the Q encoding of an encoded word.
func decodeQ(s string) []byte {
	s = strings.ReplaceAll(s, "_", " ")
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '=' && i+2 < len(s)+0 && i+2 <= len(s)-1 && isHexDigit(s[i+1]) && isHexDigit(s[i+2]) {
			v, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			out = append(out, byte(v))
			i += 2
			continue
		}
		out = append(out, s[i])
	}
	return out
}

// decodeB decodes the base64 encoding of an encoded word, tolerating
// missing padding.
func decodeB(s string) []byte {
	if data, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return data
	}
	return []byte(s)
}

// parseUnstructured parses unstructured header text, decoding encoded words,
// as Python's get_unstructured does.
func parseUnstructured(value string) *tok {
	u := newList("unstructured")
	for value != "" {
		if isWSP(value[0]) {
			var t *tok
			t, value = getFWS(value)
			u.add(t)
			continue
		}
		validEW := true
		if strings.HasPrefix(value, "=?") {
			ew, rest, err := getEncodedWord(value, "utext")
			if err != nil {
				var pe *parseError
				if errors.As(err, &pe) && pe.invalidEW {
					validEW = false
				}
			} else {
				value = rest
				haveWS := true
				if n := len(u.parts); n > 0 && u.parts[n-1].typ != "fws" {
					u.addDefect("missing whitespace before encoded word", true)
					haveWS = false
				}
				if n := len(u.parts); haveWS && n > 1 && u.parts[n-2].typ == "encoded-word" {
					ws := u.parts[n-1]
					u.parts[n-1] = &tok{typ: "fws", terminal: true, ws: true, ewWS: true, val: ws.val}
				}
				u.add(ew)
				continue
			}
		}
		word, rest := splitWSP(value)
		if validEW && rfc2047Matcher.MatchString(word) {
			before, after, _ := strings.Cut(value, "=?")
			word, rest = before, "=?"+after
			if word == "" {
				word, rest = "=?", after
			}
		}
		vtext := valueTerm(word, "utext")
		validateXText(vtext)
		u.add(vtext)
		value = rest
	}
	return u
}

// getPtextToEndchars returns the text before the first unescaped character
// of endchars, with backslash escapes removed, and whether a quoted pair was
// found. It stops at whitespace.
func getPtextToEndchars(value, endchars string) (string, string, bool) {
	fragment, remainder := splitWSP(value)
	var vchars []byte
	escape, hadQP := false, false
	pos := 0
	stopped := false
	for pos = 0; pos < len(fragment); pos++ {
		c := fragment[pos]
		if c == '\\' {
			if escape {
				escape = false
				hadQP = true
			} else {
				escape = true
				continue
			}
		}
		if escape {
			escape = false
		} else if inSet(c, endchars) {
			stopped = true
			break
		}
		vchars = append(vchars, c)
	}
	if !stopped {
		pos = len(fragment)
	}
	return string(vchars), fragment[pos:] + remainder, hadQP
}

// getQPCtext parses comment text.
func getQPCtext(value string) (*tok, string) {
	ptext, rest, _ := getPtextToEndchars(value, "()")
	t := wsTerm(ptext, "ptext")
	validateXText(t)
	return t, rest
}

// getQContent parses quoted-string text.
func getQContent(value string) (*tok, string) {
	ptext, rest, _ := getPtextToEndchars(value, `"`)
	t := valueTerm(ptext, "ptext")
	validateXText(t)
	return t, rest
}

// getAtext parses atom text.
func getAtext(value string) (*tok, string, error) {
	n := 0
	for n < len(value) && !inSet(value[n], atomEnds) {
		n++
	}
	if n == 0 {
		return nil, value, perr("expected atext but found '%s'", value)
	}
	t := valueTerm(value[:n], "atext")
	validateXText(t)
	return t, value[n:], nil
}

// getBareQuotedString parses a quoted string without surrounding
// whitespace.
func getBareQuotedString(value string) (*tok, string, error) {
	if value == "" || value[0] != '"' {
		return nil, value, perr(`expected '"' but found '%s'`, value)
	}
	bqs := newList("bare-quoted-string")
	value = value[1:]
	if value != "" && value[0] == '"' {
		return bqs, value[1:], nil
	}
	for value != "" && value[0] != '"' {
		var t *tok
		switch {
		case isWSP(value[0]):
			t, value = getFWS(value)
		case strings.HasPrefix(value, "=?"):
			validEW := false
			ew, rest, err := getEncodedWord(value, "vtext")
			if err == nil {
				t, value = ew, rest
				bqs.addDefect("encoded word inside quoted string", true)
				validEW = true
			} else {
				t, value = getQContent(value)
			}
			if n := len(bqs.parts); validEW && n > 1 && bqs.parts[n-1].typ == "fws" && bqs.parts[n-2].typ == "encoded-word" {
				ws := bqs.parts[n-1]
				bqs.parts[n-1] = &tok{typ: "fws", terminal: true, ws: true, ewWS: true, val: ws.val}
			}
		default:
			t, value = getQContent(value)
		}
		bqs.add(t)
	}
	if value == "" {
		bqs.addDefect("end of header inside quoted string", true)
		return bqs, value, nil
	}
	return bqs, value[1:], nil
}

// getComment parses a comment, which may nest.
func getComment(value string) (*tok, string, error) {
	if value != "" && value[0] != '(' {
		return nil, value, perr("expected '(' but found '%s'", value)
	}
	c := newList("comment")
	value = value[1:]
	for value != "" && value[0] != ')' {
		var t *tok
		switch {
		case isWSP(value[0]):
			t, value = getFWS(value)
		case value[0] == '(':
			t, value, _ = getComment(value)
		default:
			t, value = getQPCtext(value)
		}
		c.add(t)
	}
	if value == "" {
		c.addDefect("end of header inside comment", true)
		return c, value, nil
	}
	return c, value[1:], nil
}

// getCFWS parses comments and folding whitespace.
func getCFWS(value string) (*tok, string) {
	cfws := newList("cfws")
	for value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		if isWSP(value[0]) {
			t, value = getFWS(value)
		} else {
			t, value, _ = getComment(value)
		}
		cfws.add(t)
	}
	return cfws, value
}

// getQuotedString parses a quoted string with surrounding comments and
// whitespace.
func getQuotedString(value string) (*tok, string, error) {
	qs := newList("quoted-string")
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		qs.add(t)
	}
	t, value, err := getBareQuotedString(value)
	if err != nil {
		return nil, value, err
	}
	qs.add(t)
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		t, value = getCFWS(value)
		qs.add(t)
	}
	return qs, value, nil
}

// getAtom parses an atom with surrounding comments and whitespace.
func getAtom(value string) (*tok, string, error) {
	atom := newList("atom")
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		atom.add(t)
	}
	if value != "" && inSet(value[0], atomEnds) {
		return nil, value, perr("expected atom but found '%s'", value)
	}
	var t *tok
	var err error
	if strings.HasPrefix(value, "=?") {
		var rest string
		if t, rest, err = getEncodedWord(value, "vtext"); err == nil {
			value = rest
		} else if t, value, err = getAtext(value); err != nil {
			return nil, value, err
		}
	} else if t, value, err = getAtext(value); err != nil {
		return nil, value, err
	}
	atom.add(t)
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		t, value = getCFWS(value)
		atom.add(t)
	}
	return atom, value, nil
}

// getDotAtomText parses atoms joined by dots.
func getDotAtomText(value string) (*tok, string, error) {
	dat := &tok{typ: "dot-atom-text"}
	if value == "" || inSet(value[0], atomEnds) {
		return nil, value, perr("expected atom at a start of dot-atom-text but found '%s'", value)
	}
	for value != "" && !inSet(value[0], atomEnds) {
		t, rest, err := getAtext(value)
		if err != nil {
			return nil, value, err
		}
		value = rest
		dat.add(t)
		if value != "" && value[0] == '.' {
			dat.add(valueTerm(".", "dot"))
			value = value[1:]
		}
	}
	if dat.parts[len(dat.parts)-1].typ == "dot" {
		return nil, value, perr("expected atom at end of dot-atom-text but found '%s'", "."+value)
	}
	return dat, value, nil
}

// getDotAtom parses a dot-atom with surrounding comments and whitespace.
func getDotAtom(value string) (*tok, string, error) {
	da := newList("dot-atom")
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		da.add(t)
	}
	var t *tok
	var err error
	if strings.HasPrefix(value, "=?") {
		var rest string
		if t, rest, err = getEncodedWord(value, "vtext"); err == nil {
			value = rest
		} else if t, value, err = getDotAtomText(value); err != nil {
			return nil, value, err
		}
	} else if t, value, err = getDotAtomText(value); err != nil {
		return nil, value, err
	}
	da.add(t)
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		t, value = getCFWS(value)
		da.add(t)
	}
	return da, value, nil
}

// getWord parses an atom or a quoted string.
func getWord(value string) (*tok, string, error) {
	var leader *tok
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		leader, value = getCFWS(value)
	}
	if value == "" {
		return nil, value, perr("Expected 'atom' or 'quoted-string' but found nothing.")
	}
	var t *tok
	var err error
	switch {
	case value[0] == '"':
		t, value, err = getQuotedString(value)
	case inSet(value[0], specialsChars):
		return nil, value, perr("Expected 'atom' or 'quoted-string' but found '%s'", value)
	default:
		t, value, err = getAtom(value)
	}
	if err != nil {
		return nil, value, err
	}
	if leader != nil {
		t.prepend(leader)
	}
	return t, value, nil
}

// getPhrase parses a sequence of words.
func getPhrase(value string) (*tok, string) {
	phrase := newList("phrase")
	if t, rest, err := getWord(value); err == nil {
		phrase.add(t)
		value = rest
	} else {
		phrase.addDefect("phrase does not start with word", true)
	}
	for value != "" && !inSet(value[0], phraseEnds) {
		if value[0] == '.' {
			phrase.add(valueTerm(".", "dot"))
			phrase.addDefect("period in 'phrase'", false)
			value = value[1:]
			continue
		}
		t, rest, err := getWord(value)
		if err != nil {
			if !inSet(value[0], cfwsLeaderChars) {
				// Python raises here; the callers stop at the special.
				break
			}
			t, rest = getCFWS(value)
			phrase.addDefect("comment found without atom", false)
		} else if t.parts[0].typ == "encoded-word" {
			markEWWhitespace(phrase)
		}
		value = rest
		phrase.add(t)
	}
	return phrase, value
}

// markEWWhitespace turns the whitespace after an encoded word that ends the
// last atom of phrase into whitespace that renders as nothing, because an
// encoded word follows it. Python builds that terminal from the text of the
// whole atom, which only its startswith_fws reads.
func markEWWhitespace(phrase *tok) {
	n := len(phrase.parts)
	if n == 0 {
		return
	}
	last := phrase.parts[n-1]
	m := len(last.parts)
	if last.typ != "atom" || m < 2 || last.parts[m-2].typ != "encoded-word" || last.parts[m-1].typ != "cfws" {
		return
	}
	for _, p := range last.parts[m-1].parts {
		if p.typ == "comment" {
			return
		}
	}
	last.parts[m-1] = &tok{typ: "fws", terminal: true, ws: true, ewWS: true, val: last.String()}
}

// getLocalPart parses the local part of an addr-spec.
func getLocalPart(value string) (*tok, string, error) {
	lp := &tok{typ: "local-part", noEW: true}
	var leader *tok
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		leader, value = getCFWS(value)
	}
	if value == "" {
		return nil, value, perr("expected local-part but found '%s'", value)
	}
	t, rest, err := getDotAtom(value)
	if err != nil {
		t, rest, err = getWord(value)
		if err != nil {
			if value[0] != '\\' && inSet(value[0], phraseEnds) {
				return nil, value, err
			}
			t, rest = newList(""), value
		}
	}
	value = rest
	if leader != nil {
		t.prepend(leader)
	}
	lp.add(t)
	if value != "" && (value[0] == '\\' || !inSet(value[0], phraseEnds)) {
		obs, rest, err := getObsLocalPart(lp.String() + value)
		if err != nil {
			return nil, value, err
		}
		value = rest
		if obs.typ == "invalid-obs-local-part" {
			lp.addDefect("local-part is not dot-atom, quoted-string, or obs-local-part", true)
		} else {
			lp.addDefect("local-part is not a dot-atom (contains CFWS)", false)
		}
		lp.parts[0] = obs
	}
	if !isASCII(lp.value()) {
		lp.addDefect("local-part contains non-ASCII characters)", false)
	}
	return lp, value, nil
}

// getObsLocalPart parses an obsolete local part: words with dots, comments
// and misplaced backslashes.
func getObsLocalPart(value string) (*tok, string, error) {
	obs := &tok{typ: "obs-local-part", noEW: true}
	lastNonWSWasDot := false
	for value != "" && (value[0] == '\\' || !inSet(value[0], phraseEnds)) {
		if value[0] == '.' {
			if lastNonWSWasDot {
				obs.addDefect("invalid repeated '.'", true)
			}
			obs.add(valueTerm(".", "dot"))
			lastNonWSWasDot = true
			value = value[1:]
			continue
		} else if value[0] == '\\' {
			obs.add(valueTerm(value[:1], "misplaced-special"))
			value = value[1:]
			obs.addDefect("'\\' character outside of quoted-string/ccontent", true)
			lastNonWSWasDot = false
			continue
		}
		if n := len(obs.parts); n > 0 && obs.parts[n-1].typ != "dot" {
			obs.addDefect("missing '.' between words", true)
		}
		t, rest, err := getWord(value)
		if err == nil {
			lastNonWSWasDot = false
		} else {
			if !inSet(value[0], cfwsLeaderChars) {
				return nil, value, err
			}
			t, rest = getCFWS(value)
		}
		value = rest
		obs.add(t)
	}
	if len(obs.parts) == 0 {
		return nil, value, perr("expected obs-local-part but found '%s'", value)
	}
	p := obs.parts
	if p[0].typ == "dot" || p[0].typ == "cfws" && len(p) > 1 && p[1].typ == "dot" {
		obs.addDefect("Invalid leading '.' in local part", true)
	}
	if n := len(p); p[n-1].typ == "dot" || p[n-1].typ == "cfws" && n > 1 && p[n-2].typ == "dot" {
		obs.addDefect("Invalid trailing '.' in local part", true)
	}
	if len(obs.defects) > 0 {
		obs.typ = "invalid-obs-local-part"
	}
	return obs, value, nil
}

// getDtext parses domain-literal text.
func getDtext(value string) (*tok, string) {
	ptext, rest, hadQP := getPtextToEndchars(value, "[]")
	t := valueTerm(ptext, "ptext")
	if hadQP {
		t.addDefect("quoted printable found in domain-literal", false)
	}
	validateXText(t)
	return t, rest
}

// getDomainLiteral parses a domain in brackets.
func getDomainLiteral(value string) (*tok, string, error) {
	dl := &tok{typ: "domain-literal", noEW: true}
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		dl.add(t)
	}
	if value == "" {
		return nil, value, perr("expected domain-literal")
	}
	if value[0] != '[' {
		return nil, value, perr("expected '[' at start of domain-literal but found '%s'", value)
	}
	value = value[1:]
	dl.add(valueTerm("[", "domain-literal-start"))
	earlyEnd := func() bool {
		if value != "" {
			return false
		}
		dl.addDefect("end of input inside domain-literal", true)
		dl.add(valueTerm("]", "domain-literal-end"))
		return true
	}
	if earlyEnd() {
		return dl, value, nil
	}
	var t *tok
	if isWSP(value[0]) {
		t, value = getFWS(value)
		dl.add(t)
	}
	t, value = getDtext(value)
	dl.add(t)
	if earlyEnd() {
		return dl, value, nil
	}
	if isWSP(value[0]) {
		t, value = getFWS(value)
		dl.add(t)
	}
	if earlyEnd() {
		return dl, value, nil
	}
	if value[0] != ']' {
		return nil, value, perr("expected ']' at end of domain-literal but found '%s'", value)
	}
	dl.add(valueTerm("]", "domain-literal-end"))
	value = value[1:]
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		t, value = getCFWS(value)
		dl.add(t)
	}
	return dl, value, nil
}

// getDomain parses the domain of an addr-spec.
func getDomain(value string) (*tok, string, error) {
	domain := &tok{typ: "domain", noEW: true}
	var leader *tok
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		leader, value = getCFWS(value)
	}
	if value == "" {
		return nil, value, perr("expected domain but found '%s'", value)
	}
	if value[0] == '[' {
		t, rest, err := getDomainLiteral(value)
		if err != nil {
			return nil, value, err
		}
		if leader != nil {
			t.prepend(leader)
		}
		domain.add(t)
		return domain, rest, nil
	}
	t, rest, err := getDotAtom(value)
	if err != nil {
		if t, rest, err = getAtom(value); err != nil {
			return nil, value, err
		}
	}
	value = rest
	if value != "" && value[0] == '@' {
		return nil, value, perr("Invalid Domain")
	}
	if leader != nil {
		t.prepend(leader)
	}
	domain.add(t)
	if value != "" && value[0] == '.' {
		domain.addDefect("domain is not a dot-atom (contains CFWS)", false)
		if domain.parts[0].typ == "dot-atom" {
			domain.parts = append([]*tok(nil), domain.parts[0].parts...)
		}
		for value != "" && value[0] == '.' {
			domain.add(valueTerm(".", "dot"))
			t, rest, err := getAtom(value[1:])
			if err != nil {
				return nil, value, err
			}
			value = rest
			domain.add(t)
		}
	}
	return domain, value, nil
}

// getAddrSpec parses local-part "@" domain.
func getAddrSpec(value string) (*tok, string, error) {
	as := &tok{typ: "addr-spec", noEW: true}
	t, value, err := getLocalPart(value)
	if err != nil {
		return nil, value, err
	}
	as.add(t)
	if value == "" || value[0] != '@' {
		as.addDefect("addr-spec local part with no domain", true)
		return as, value, nil
	}
	as.add(valueTerm("@", "address-at-symbol"))
	t, value, err = getDomain(value[1:])
	if err != nil {
		return nil, value, err
	}
	as.add(t)
	return as, value, nil
}

// getObsRoute parses an obsolete source route such as "@a,@b:".
func getObsRoute(value string) (*tok, string, error) {
	route := newList("obs-route")
	for value != "" && (value[0] == ',' || inSet(value[0], cfwsLeaderChars)) {
		if value != "" && inSet(value[0], cfwsLeaderChars) {
			var t *tok
			t, value = getCFWS(value)
			route.add(t)
		} else {
			route.add(listSeparator())
			value = value[1:]
		}
	}
	if value == "" || value[0] != '@' {
		return nil, value, perr("expected obs-route domain but found '%s'", value)
	}
	route.add(valueTerm("@", "route-component-marker"))
	t, value, err := getDomain(value[1:])
	if err != nil {
		return nil, value, err
	}
	route.add(t)
	for value != "" && value[0] == ',' {
		route.add(listSeparator())
		value = value[1:]
		if value == "" {
			break
		}
		if value != "" && inSet(value[0], cfwsLeaderChars) {
			t, value = getCFWS(value)
			route.add(t)
		}
		if value != "" && value[0] == '@' {
			route.add(valueTerm("@", "route-component-marker"))
			if t, value, err = getDomain(value[1:]); err != nil {
				return nil, value, err
			}
			route.add(t)
		}
	}
	if value == "" {
		return nil, value, perr("end of header while parsing obs-route")
	}
	if value[0] != ':' {
		return nil, value, perr("expected ':' marking end of obs-route but found '%s'", value)
	}
	route.add(valueTerm(":", "end-of-obs-route-marker"))
	return route, value[1:], nil
}

// getAngleAddr parses "<" addr-spec ">".
func getAngleAddr(value string) (*tok, string, error) {
	aa := newList("angle-addr")
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		aa.add(t)
	}
	if value == "" || value[0] != '<' {
		return nil, value, perr("expected angle-addr but found '%s'", value)
	}
	aa.add(valueTerm("<", "angle-addr-start"))
	value = value[1:]
	if value != "" && value[0] == '>' {
		aa.add(valueTerm(">", "angle-addr-end"))
		aa.addDefect("null addr-spec in angle-addr", true)
		return aa, value[1:], nil
	}
	if value == "" {
		return nil, value, perr("expected addr-spec or obs-route but found '%s'", value)
	}
	t, rest, err := getAddrSpec(value)
	if err != nil {
		route, rest2, err2 := getObsRoute(value)
		if err2 != nil {
			return nil, value, perr("expected addr-spec or obs-route but found '%s'", value)
		}
		aa.addDefect("obsolete route specification in angle-addr", false)
		aa.add(route)
		if t, rest, err = getAddrSpec(rest2); err != nil {
			return nil, value, err
		}
	}
	value = rest
	aa.add(t)
	if value != "" && value[0] == '>' {
		value = value[1:]
	} else {
		aa.addDefect("missing trailing '>' on angle-addr", true)
	}
	aa.add(valueTerm(">", "angle-addr-end"))
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		t, value = getCFWS(value)
		aa.add(t)
	}
	return aa, value, nil
}

// getDisplayName parses the phrase before an angle-addr.
func getDisplayName(value string) (*tok, string) {
	phrase, value := getPhrase(value)
	dn := &tok{typ: "display-name", parts: phrase.parts, defects: phrase.defects}
	return dn, value
}

// getNameAddr parses [display-name] angle-addr.
func getNameAddr(value string) (*tok, string, error) {
	na := newList("name-addr")
	var leader *tok
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		leader, value = getCFWS(value)
		if value == "" {
			return nil, value, perr("expected name-addr but found '%s'", leader.String())
		}
	}
	if value == "" {
		return nil, value, perr("expected name-addr but found '%s'", value)
	}
	if value[0] != '<' {
		if inSet(value[0], phraseEnds) {
			return nil, value, perr("expected name-addr but found '%s'", value)
		}
		dn, rest := getDisplayName(value)
		value = rest
		if value == "" {
			return nil, value, perr("expected name-addr but found '%s'", dn.String())
		}
		if leader != nil {
			if len(dn.parts) > 0 && !dn.parts[0].terminal {
				dn.parts[0].prepend(leader)
			} else {
				dn.prepend(leader)
			}
			leader = nil
		}
		na.add(dn)
	}
	t, value, err := getAngleAddr(value)
	if err != nil {
		return nil, value, err
	}
	if leader != nil {
		t.prepend(leader)
	}
	na.add(t)
	return na, value, nil
}

// getMailbox parses a name-addr or an addr-spec.
func getMailbox(value string) (*tok, string, error) {
	mb := newList("mailbox")
	t, rest, err := getNameAddr(value)
	if err != nil {
		if t, rest, err = getAddrSpec(value); err != nil {
			return nil, value, perr("expected mailbox but found '%s'", value)
		}
	}
	for _, d := range t.allDefects() {
		if d.invalid {
			mb.typ = "invalid-mailbox"
			break
		}
	}
	mb.add(t)
	return mb, rest, nil
}

// getInvalidMailbox collects text up to one of endchars as an invalid
// mailbox.
func getInvalidMailbox(value, endchars string) (*tok, string) {
	im := &tok{typ: "invalid-mailbox", invalidClass: true}
	for value != "" && !inSet(value[0], endchars) {
		if inSet(value[0], phraseEnds) {
			im.add(valueTerm(value[:1], "misplaced-special"))
			value = value[1:]
			continue
		}
		var t *tok
		before := value
		t, value = getPhrase(value)
		im.add(t)
		if value == before {
			im.add(valueTerm(value[:1], "misplaced-special"))
			value = value[1:]
		}
	}
	return im, value
}

// getMailboxList parses the mailboxes of a group.
func getMailboxList(value string) (*tok, string) {
	ml := newList("mailbox-list")
	for value != "" && value[0] != ';' {
		t, rest, err := getMailbox(value)
		if err == nil {
			ml.add(t)
			value = rest
		} else {
			var leader *tok
			switch {
			case value != "" && inSet(value[0], cfwsLeaderChars):
				leader, value = getCFWS(value)
				if value == "" || inSet(value[0], ",;") {
					ml.add(leader)
					ml.addDefect("empty element in mailbox-list", false)
				} else {
					t, value = getInvalidMailbox(value, ",;")
					t.prepend(leader)
					ml.add(t)
					ml.addDefect("invalid mailbox in mailbox-list", true)
				}
			case value[0] == ',':
				ml.addDefect("empty element in mailbox-list", false)
			default:
				t, value = getInvalidMailbox(value, ",;")
				ml.add(t)
				ml.addDefect("invalid mailbox in mailbox-list", true)
			}
		}
		if value != "" && !inSet(value[0], ",;") {
			mb := ml.parts[len(ml.parts)-1]
			mb.typ = "invalid-mailbox"
			var t *tok
			t, value = getInvalidMailbox(value, ",;")
			mb.add(t.parts...)
			ml.addDefect("invalid mailbox in mailbox-list", true)
		}
		if value != "" && value[0] == ',' {
			ml.add(listSeparator())
			value = value[1:]
		}
	}
	return ml, value
}

// getGroupList parses the content of a group.
func getGroupList(value string) (*tok, string) {
	gl := newList("group-list")
	if value == "" {
		gl.addDefect("end of header before group-list", true)
		return gl, value
	}
	var leader *tok
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		leader, value = getCFWS(value)
		if value == "" {
			gl.addDefect("end of header in group-list", true)
			gl.add(leader)
			return gl, value
		}
		if value[0] == ';' {
			gl.add(leader)
			return gl, value
		}
	}
	ml, value := getMailboxList(value)
	if len(ml.allMailboxes()) == 0 {
		if leader != nil {
			gl.add(leader)
		}
		gl.add(ml.parts...)
		gl.addDefect("group-list with empty entries", false)
		return gl, value
	}
	if leader != nil {
		ml.prepend(leader)
	}
	gl.add(ml)
	return gl, value
}

// getGroup parses display-name ":" [group-list] ";".
func getGroup(value string) (*tok, string, error) {
	g := newList("group")
	dn, value := getDisplayName(value)
	if value == "" || value[0] != ':' {
		return nil, value, perr("expected ':' at end of group display name but found '%s'", value)
	}
	g.add(dn, valueTerm(":", "group-display-name-terminator"))
	value = value[1:]
	if value != "" && value[0] == ';' {
		g.add(valueTerm(";", "group-terminator"))
		return g, value[1:], nil
	}
	gl, value := getGroupList(value)
	g.add(gl)
	if value == "" {
		g.addDefect("end of header in group", true)
	} else if value[0] != ';' {
		return nil, value, perr("expected ';' at end of group but found %s", value)
	}
	g.add(valueTerm(";", "group-terminator"))
	if value != "" {
		value = value[1:]
	}
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		g.add(t)
	}
	return g, value, nil
}

// getAddress parses a group or a mailbox.
func getAddress(value string) (*tok, string, error) {
	addr := newList("address")
	t, rest, err := getGroup(value)
	if err != nil {
		if t, rest, err = getMailbox(value); err != nil {
			return nil, value, perr("expected address but found '%s'", value)
		}
	}
	addr.add(t)
	return addr, rest, nil
}

// parseAddressList parses an address header, as Python's get_address_list
// does. It never fails: text it cannot parse becomes invalid mailboxes with
// defects.
func parseAddressList(value string) *tok {
	al := newList("address-list")
	for value != "" {
		t, rest, err := getAddress(value)
		if err == nil {
			al.add(t)
			value = rest
		} else {
			var leader *tok
			switch {
			case value != "" && inSet(value[0], cfwsLeaderChars):
				leader, value = getCFWS(value)
				if value == "" || value[0] == ',' {
					al.add(leader)
					al.addDefect("address-list entry with no content", false)
				} else {
					t, value = getInvalidMailbox(value, ",")
					t.prepend(leader)
					al.add(newList("address", t))
					al.addDefect("invalid address in address-list", true)
				}
			case value[0] == ',':
				al.addDefect("empty element in address-list", false)
			default:
				t, value = getInvalidMailbox(value, ",")
				al.add(newList("address", t))
				al.addDefect("invalid address in address-list", true)
			}
		}
		if value != "" && value[0] != ',' {
			var t *tok
			t, value = getInvalidMailbox(value, ",")
			al.add(newList("address", t))
			al.addDefect("invalid address in address-list", true)
		}
		if value != "" {
			al.add(listSeparator())
			value = value[1:]
		}
	}
	return al
}

// parseMessageID parses a Message-ID header. Text that is no msg-id is kept
// as unstructured text.
func parseMessageID(value string) *tok {
	mid := &tok{typ: "message-id", noEW: true}
	t, rest, err := getMsgID(value)
	if err != nil {
		u := parseUnstructured(value)
		return &tok{
			typ: "invalid-message-id", noEW: true, parts: u.parts,
			defects: []defect{{msg: invalidMsgID(err), invalid: true}},
		}
	}
	mid.add(t)
	if rest != "" {
		mid.addDefect("Unexpected "+pyRepr(rest), true)
	}
	return mid
}

// parseMessageIDs parses an In-Reply-To or References header, as Python's
// parse_message_ids does. A comma between msg-ids becomes a space, and the
// text from the first invalid msg-id on is kept as unstructured text.
func parseMessageIDs(value string) *tok {
	list := newList("message-id-list")
	for value != "" {
		if value[0] == ',' {
			list.addDefect("comma in msg-id list", true)
			list.add(wsTerm(" ", "invalid-comma-replacement"))
			value = value[1:]
			continue
		}
		t, rest, err := getMsgID(value)
		if err != nil {
			u := parseUnstructured(value)
			list.add(&tok{typ: "invalid-message-id", noEW: true, parts: u.parts})
			list.addDefect(invalidMsgID(err), true)
			break
		}
		list.add(t)
		value = rest
	}
	return list
}

// invalidMsgID returns the defect message for a msg-id that err rejects.
func invalidMsgID(err error) string {
	return "Invalid msg-id: HeaderParseError(" + pyRepr(err.Error()) + ")"
}

// getMsgID parses "<" id-left "@" id-right ">".
func getMsgID(value string) (*tok, string, error) {
	mid := &tok{typ: "msg-id", noEW: true}
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		mid.add(t)
	}
	if value == "" || value[0] != '<' {
		return nil, value, perr("expected msg-id but found '%s'", value)
	}
	mid.add(valueTerm("<", "msg-id-start"))
	value = value[1:]
	t, rest, err := getDotAtomText(value)
	if err != nil {
		if t, rest, err = getObsLocalPart(value); err != nil {
			return nil, value, perr("expected dot-atom-text or obs-id-left but found '%s'", value)
		}
		mid.addDefect("obsolete id-left in msg-id", false)
	}
	value = rest
	mid.add(t)
	if value == "" || value[0] != '@' {
		mid.addDefect("msg-id with no id-right", true)
		if value != "" && value[0] == '>' {
			mid.add(valueTerm(">", "msg-id-end"))
			value = value[1:]
		}
		return mid, value, nil
	}
	mid.add(valueTerm("@", "address-at-symbol"))
	value = value[1:]
	if t, rest, err = getDotAtomText(value); err != nil {
		if t, rest, err = getNoFoldLiteral(value); err != nil {
			if value == "" {
				return nil, value, perr("expected dot-atom-text, no-fold-literal or obs-id-right but found '%s'", value)
			}
			if t, rest, err = getDomain(value); err != nil {
				return nil, value, perr("expected dot-atom-text, no-fold-literal or obs-id-right but found '%s'", value)
			}
			mid.addDefect("obsolete id-right in msg-id", false)
		}
	}
	value = rest
	mid.add(t)
	if value != "" && value[0] == '>' {
		value = value[1:]
	} else {
		mid.addDefect("missing trailing '>' on msg-id", true)
	}
	mid.add(valueTerm(">", "msg-id-end"))
	if value != "" && inSet(value[0], cfwsLeaderChars) {
		var t *tok
		t, value = getCFWS(value)
		mid.add(t)
	}
	return mid, value, nil
}

// getNoFoldLiteral parses "[" dtext "]".
func getNoFoldLiteral(value string) (*tok, string, error) {
	nfl := &tok{typ: "no-fold-literal", noEW: true}
	if value == "" || value[0] != '[' {
		return nil, value, perr("expected '[' at the start of no-fold-literal but found '%s'", value)
	}
	nfl.add(valueTerm("[", "no-fold-literal-start"))
	t, value := getDtext(value[1:])
	nfl.add(t)
	if value == "" || value[0] != ']' {
		return nil, value, perr("expected ']' at the end of no-fold-literal but found '%s'", value)
	}
	nfl.add(valueTerm("]", "no-fold-literal-end"))
	return nfl, value[1:], nil
}

// The accessors below read the parts of a parsed mailbox, as the properties
// of Python's Mailbox, NameAddr, AngleAddr and AddrSpec classes do. ok is
// false where Python returns None.

// allMailboxes returns the mailboxes and invalid mailboxes of an address
// list, a group or a mailbox list.
func (t *tok) allMailboxes() []*tok {
	var out []*tok
	switch t.typ {
	case "address-list":
		for _, p := range t.parts {
			if p.typ == "address" {
				out = append(out, p.allMailboxes()...)
			}
		}
	case "address":
		first := t.parts[0]
		if first.typ == "mailbox" || first.typ == "invalid-mailbox" {
			return []*tok{first}
		}
		return first.allMailboxes()
	case "group":
		if len(t.parts) > 2 && t.parts[2].typ == "group-list" {
			return t.parts[2].allMailboxes()
		}
	case "group-list":
		if len(t.parts) > 0 && t.parts[0].typ == "mailbox-list" {
			return t.parts[0].allMailboxes()
		}
	case "mailbox-list":
		for _, p := range t.parts {
			if p.typ == "mailbox" || p.typ == "invalid-mailbox" {
				out = append(out, p)
			}
		}
	}
	return out
}

// addrSpecOf returns the addr-spec token below a mailbox, name-addr or
// angle-addr.
func (t *tok) addrSpecOf() *tok {
	if t.invalidClass || len(t.parts) == 0 {
		return nil
	}
	switch t.typ {
	case "addr-spec":
		return t
	case "mailbox", "invalid-mailbox":
		return t.parts[0].addrSpecOf()
	case "name-addr":
		return t.parts[len(t.parts)-1].addrSpecOf()
	case "angle-addr":
		for _, p := range t.parts {
			if p.typ == "addr-spec" {
				return p
			}
		}
	}
	return nil
}

// localPart returns the local part of a mailbox with comments removed.
func (t *tok) localPart() (string, bool) {
	as := t.addrSpecOf()
	if as == nil {
		return "", false
	}
	lp := as.parts[0]
	if lp.typ != "local-part" {
		return "", false
	}
	inner := lp.parts[0]
	dot := valueTerm(".", "dot")
	res := []*tok{dot}
	last := dot
	lastIsList := false
	for _, x := range append(append([]*tok(nil), inner.parts...), dot) {
		if x.typ == "cfws" {
			continue
		}
		if lastIsList && x.typ == "dot" && len(last.parts) > 0 && last.parts[len(last.parts)-1].typ == "cfws" {
			res[len(res)-1] = newList("", last.parts[:len(last.parts)-1]...)
		}
		isList := !x.terminal
		if isList && last.typ == "dot" && len(x.parts) > 0 && x.parts[0].typ == "cfws" {
			res = append(res, newList("", x.parts[1:]...))
		} else {
			res = append(res, x)
		}
		last = res[len(res)-1]
		lastIsList = isList
	}
	return newList("", res[1:len(res)-1]...).value(), true
}

// domain returns the domain of a mailbox without whitespace.
func (t *tok) domain() (string, bool) {
	as := t.addrSpecOf()
	if as == nil || len(as.parts) < 3 {
		return "", false
	}
	return strings.Join(strings.FieldsFunc(as.parts[len(as.parts)-1].value(), pyIsSpace), ""), true
}

// addrSpec returns the addr-spec of a mailbox, quoting the local part when
// needed.
func (t *tok) addrSpec() (string, bool) {
	if t.invalidClass {
		return "", false
	}
	as := t.addrSpecOf()
	if as == nil {
		if t.typ == "mailbox" || t.typ == "invalid-mailbox" {
			if len(t.parts) > 0 && t.parts[0].typ == "name-addr" {
				return "<>", true
			}
		}
		return "", false
	}
	lp, _ := as.localPart()
	quoted := lp
	if strings.ContainsAny(lp, dotAtomEnds) {
		quoted = quoteString(lp)
	}
	d, hasDomain := as.domain()
	if t.typ != "addr-spec" && lp == "" {
		quoted = quoteString(lp)
	}
	if hasDomain {
		return quoted + "@" + d, true
	}
	return quoted, true
}

// displayName returns the display name of a mailbox, or "" and false.
func (t *tok) displayName() (string, bool) {
	if t.invalidClass || len(t.parts) == 0 || t.parts[0].typ != "name-addr" {
		return "", false
	}
	na := t.parts[0]
	if len(na.parts) == 1 {
		return "", false
	}
	return na.parts[0].displayNameText(), true
}

// displayNameText returns the text of a display-name without the comments
// and whitespace at its ends.
func (t *tok) displayNameText() string {
	res := append([]*tok(nil), t.parts...)
	if len(res) == 0 {
		return ""
	}
	if res[0].typ == "cfws" {
		res = res[1:]
	} else if !res[0].terminal && len(res[0].parts) > 0 && res[0].parts[0].typ == "cfws" {
		res[0] = newList("", res[0].parts[1:]...)
	}
	if n := len(res); n > 0 {
		if res[n-1].typ == "cfws" {
			res = res[:n-1]
		} else if last := res[n-1]; !last.terminal && len(last.parts) > 0 && last.parts[len(last.parts)-1].typ == "cfws" {
			res[n-1] = newList("", last.parts[:len(last.parts)-1]...)
		}
	}
	return newList("", res...).value()
}

// displayNameValue returns the value of a display-name token, quoting it
// when it holds a quoted string or has defects.
func (t *tok) displayNameValue() string {
	quote := len(t.defects) > 0
	for _, p := range t.parts {
		if p.typ == "quoted-string" {
			quote = true
		}
	}
	if len(t.parts) == 0 || !quote {
		var b strings.Builder
		for _, p := range t.parts {
			b.WriteString(p.value())
		}
		return b.String()
	}
	startsCFWS := func(p *tok) bool {
		return p.typ == "cfws" || !p.terminal && len(p.parts) > 0 && p.parts[0].typ == "cfws"
	}
	endsCFWS := func(p *tok) bool {
		return p.typ == "cfws" || !p.terminal && len(p.parts) > 0 && p.parts[len(p.parts)-1].typ == "cfws"
	}
	pre, post := "", ""
	if startsCFWS(t.parts[0]) {
		pre = " "
	}
	if endsCFWS(t.parts[len(t.parts)-1]) {
		post = " "
	}
	return pre + quoteString(t.displayNameText()) + post
}

// pyRepr returns s quoted as Python's repr() quotes a str.
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.IndexByte(s, '\'') >= 0 && strings.IndexByte(s, '"') < 0 {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r == utf8.RuneError:
			b.WriteString(`�`)
		case r >= 0x80 && r < 0xa0:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}
