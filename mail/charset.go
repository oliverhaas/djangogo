package mail

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// charsetAliases maps Python charset names to the names that go into MIME
// headers, as email.charset.ALIASES does. The lookup is case-sensitive.
var charsetAliases = map[string]string{
	"latin_1": "iso-8859-1", "latin-1": "iso-8859-1",
	"latin_2": "iso-8859-2", "latin-2": "iso-8859-2",
	"latin_3": "iso-8859-3", "latin-3": "iso-8859-3",
	"latin_4": "iso-8859-4", "latin-4": "iso-8859-4",
	"latin_5": "iso-8859-9", "latin-5": "iso-8859-9",
	"latin_6": "iso-8859-10", "latin-6": "iso-8859-10",
	"latin_7": "iso-8859-13", "latin-7": "iso-8859-13",
	"latin_8": "iso-8859-14", "latin-8": "iso-8859-14",
	"latin_9": "iso-8859-15", "latin-9": "iso-8859-15",
	"latin_10": "iso-8859-16", "latin-10": "iso-8859-16",
	"cp949":  "ks_c_5601-1987",
	"euc_jp": "euc-jp",
	"euc_kr": "euc-kr",
	"ascii":  "us-ascii",
}

// charsetParam returns the charset parameter value for charset.
func charsetParam(charset string) string {
	if alias, ok := charsetAliases[charset]; ok {
		return alias
	}
	return charset
}

// codecKind classifies a charset by how its text is converted.
type codecKind int

// The codec kinds.
const (
	codecUTF8 codecKind = iota
	codecASCII
	codecOther
)

// lookupCharset finds the encoding of a charset name the way Python's codec
// registry accepts it: case and the separators "-", "_" and " " do not
// matter, and Python's latin and cp names are understood.
func lookupCharset(name string) (codecKind, encoding.Encoding, error) {
	norm := strings.ToLower(strings.TrimSpace(name))
	norm = strings.NewReplacer("_", "-", " ", "-").Replace(norm)
	switch norm {
	case "utf-8", "utf8", "u8", "utf", "cp65001":
		return codecUTF8, nil, nil
	case "ascii", "us-ascii", "646", "us":
		return codecASCII, nil, nil
	case "latin", "latin1", "l1", "iso8859-1", "8859", "cp819":
		norm = "iso-8859-1"
	}
	if after, ok := strings.CutPrefix(norm, "latin-"); ok {
		norm = charsetAliases["latin-"+after]
	}
	if after, ok := strings.CutPrefix(norm, "iso8859-"); ok {
		norm = "iso-8859-" + after
	}
	if after, ok := strings.CutPrefix(norm, "cp"); ok && after != "" && strings.Trim(after, "0123456789") == "" {
		switch after {
		case "1250", "1251", "1252", "1253", "1254", "1255", "1256", "1257", "1258", "874":
			norm = "windows-" + after
		case "932":
			norm = "shift_jis"
		case "936":
			norm = "gbk"
		case "949":
			norm = "euc-kr"
		case "950":
			norm = "big5"
		default:
			norm = "ibm" + after
		}
	}
	for _, index := range []interface {
		Encoding(string) (encoding.Encoding, error)
	}{ianaindex.MIME, ianaindex.IANA} {
		if enc, err := index.Encoding(norm); err == nil && enc != nil {
			return codecOther, enc, nil
		}
		if enc, err := index.Encoding(strings.ReplaceAll(norm, "-", "_")); err == nil && enc != nil {
			return codecOther, enc, nil
		}
	}
	if enc, err := htmlindex.Get(norm); err == nil {
		return codecOther, enc, nil
	}
	return 0, nil, fmt.Errorf("unknown encoding: %s", name)
}

// encodeCharset encodes text in charset, as Python's str.encode(charset)
// does. It fails when the charset is unknown or cannot represent the text.
func encodeCharset(text, charset string) ([]byte, error) {
	kind, enc, err := lookupCharset(charset)
	if err != nil {
		return nil, err
	}
	switch kind {
	case codecUTF8:
		if !utf8.ValidString(text) {
			return nil, fmt.Errorf("'utf-8' codec can't encode the text %s", pyRepr(text))
		}
		return []byte(text), nil
	case codecASCII:
		if !isASCII(text) {
			return nil, fmt.Errorf("'ascii' codec can't encode the text %s", pyRepr(text))
		}
		return []byte(text), nil
	}
	data, err := enc.NewEncoder().Bytes([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("'%s' codec can't encode the text %s", charset, pyRepr(text))
	}
	return data, nil
}

// decodeCharset decodes data in charset. It fails when the charset is
// unknown or the data is invalid in it.
func decodeCharset(data []byte, charset string) (string, error) {
	kind, enc, err := lookupCharset(charset)
	if err != nil {
		return "", err
	}
	switch kind {
	case codecUTF8:
		if !utf8.Valid(data) {
			return "", fmt.Errorf("invalid utf-8")
		}
		return string(data), nil
	case codecASCII:
		if !isASCII(string(data)) {
			return "", fmt.Errorf("invalid ascii")
		}
		return string(data), nil
	}
	text, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(text), nil
}
