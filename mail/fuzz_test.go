package mail

import "testing"

// No Django test exists: the address parser returns defects for any input
// and never panics.
func FuzzParseAddressList(f *testing.F) {
	for _, seed := range []string{
		"to@example.com", "to@", "@", "to@ ", "<to@example.com>", `"a b" <c@d>`,
		"a@[1.2.3.4]", "a@[", "(comment) a@b", "group: a@b, c@d;", "a@b.", "=?utf-8?q?x?= <a@b>",
		"\"evil@example.com\r\nto\"@example.com", "a@b (c", "<", "a <b@c", ":;", "a@b@c", "\\", "\"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		parsed := parseAddressList(s)
		parsed.allDefects()
		for _, mb := range parsed.allMailboxes() {
			mb.addrSpec()
			mb.domain()
			mb.localPart()
		}
		PrepAddress(s, true)
		PrepAddress(s, false)
	})
}

// No Django test exists: building a message returns an error for bad input
// and never panics.
func FuzzMessageBytes(f *testing.F) {
	f.Add("Subject", "to@example.com", "Body\n", "utf-8")
	f.Add("Gżegżółka "+string(make([]byte, 80)), `"Sürname, Firstname" <to@éxample.com>`, "Firstname Sürname\n", "iso-8859-1")
	f.Add("Subject\nInjection", "to@", ".\r\n..", "")
	f.Fuzz(func(_ *testing.T, subject, to, body, encoding string) {
		if encoding != "" && encoding != "utf-8" && encoding != "iso-8859-1" && encoding != "us-ascii" {
			encoding = ""
		}
		m := &Message{
			Subject: subject, Body: body, From: to, To: []string{to}, Cc: []string{to}, ReplyTo: []string{to}, Encoding: encoding,
			Headers: []Header{{"X-Custom", subject}},
		}
		m.AttachAlternative(body, "text/html")
		m.Attach(subject, []byte(body), "")
		m.Bytes()
		m.SMTPBytes()
	})
}

// No Django test exists: fitWord picks the prefix that the shrinking loop
// of Python's _fold_as_ew picks.
func FuzzFitWord(f *testing.F) {
	f.Add("hello world", "utf-8", 30)
	f.Add("ééééé ok", "utf-8", 25)
	f.Add("\x89\x89\x89", "utf-8", 20)
	f.Add("a=b_c?d", "iso-8859-1", 22)
	f.Fuzz(func(t *testing.T, s, charset string, space int) {
		if len(s) > 200 || space < 0 || space > 300 {
			return
		}
		word := []rune(s)
		want := word
		for len(encodeWord(string(want), charset)) > space && len(want) > 0 {
			want = want[:len(want)-1]
		}
		if got := fitWord(word, charset, space); string(got) != string(want) {
			t.Errorf("fitWord(%q, %q, %d) = %q, want %q", s, charset, space, string(got), string(want))
		}
	})
}
