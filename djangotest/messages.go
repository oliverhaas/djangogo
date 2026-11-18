package djangotest

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/messages"
)

// messageCapture wraps the handler of a client and records the message
// storage of every request it serves, keyed by the request that Response
// carries.
type messageCapture struct {
	next      http.Handler
	mu        sync.Mutex
	recorders map[*http.Request]*messages.Recorder
}

// ServeHTTP puts a fresh recorder into the request context and serves the
// request.
func (h *messageCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &messages.Recorder{}
	h.mu.Lock()
	h.recorders[r] = rec
	h.mu.Unlock()
	h.next.ServeHTTP(w, r.WithContext(messages.WithRecorder(r.Context(), rec)))
}

// recorder returns the recorder of the request r.
func (h *messageCapture) recorder(r *http.Request) (*messages.Recorder, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, ok := h.recorders[r]
	return rec, ok
}

// WithMessages makes the client record the message storage of every request,
// which AssertMessages reads (Django's response.wsgi_request._messages).
func WithMessages() ClientOption {
	return func(c *Client) {
		c.handler = &messageCapture{next: c.handler, recorders: make(map[*http.Request]*messages.Recorder)}
	}
}

// MessagesOption configures AssertMessages.
type MessagesOption func(*messagesConfig)

// messagesConfig collects the options of AssertMessages.
type messagesConfig struct {
	unordered bool
}

// UnorderedMessages makes AssertMessages ignore the order of the messages
// (Django's ordered=False).
func UnorderedMessages() MessagesOption {
	return func(c *messagesConfig) { c.unordered = true }
}

// AssertMessages asserts that the request of resp ended with the expected
// messages (Django's assertMessages): the ones it brought from the session
// and the ones it added, compared by level and text and in order unless
// UnorderedMessages is given. The client must be built with WithMessages; a
// Response built by hand works when its request carries a storage
// (messages.NewContext). The failure messages are those of Python's
// assertEqual and assertCountEqual, except that the line diff has no "?"
// hint lines.
func AssertMessages(t TB, resp *Response, expected []messages.Message, opts ...MessagesOption) {
	t.Helper()
	var cfg messagesConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	got, ok := requestMessages(resp)
	if !ok {
		t.Fatalf("djangotest: AssertMessages needs a client built with WithMessages()")
		return
	}
	if cfg.unordered {
		if msg := countDiff(got, expected); msg != "" {
			t.Errorf("%s", msg)
		}
		return
	}
	if msg := listDiff(got, expected); msg != "" {
		t.Errorf("%s", msg)
	}
}

// requestMessages returns the messages of the request of resp, and false when
// the messages of the request were not recorded. A request that did not pass
// through the message middleware has none, as in Django.
func requestMessages(resp *Response) ([]messages.Message, bool) {
	if resp.Request == nil {
		return nil, false
	}
	ctx := resp.Request.Context()
	if s, ok := messages.FromContext(ctx); ok {
		return s.Messages(), true
	}
	rec, ok := messages.RecorderFromContext(ctx)
	if !ok && resp.client != nil {
		if h, capturing := resp.client.handler.(*messageCapture); capturing {
			rec, ok = h.recorder(resp.Request)
		}
	}
	if !ok {
		return nil, false
	}
	if s := rec.Storage(); s != nil {
		return s.Messages(), true
	}
	return nil, true
}

// reprs returns Python's repr of each message.
func reprs(ms []messages.Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Repr()
	}
	return out
}

// reprList returns Python's repr of a list of messages.
func reprList(ms []messages.Message) string {
	return "[" + strings.Join(reprs(ms), ", ") + "]"
}

// pformatList returns Python's pprint.pformat of a list of messages: the
// repr on one line when it fits in 80 columns, else one element per line.
func pformatList(ms []messages.Message) string {
	if s := reprList(ms); utf8.RuneCountInString(s) <= reprMaxLength {
		return s
	}
	return "[" + strings.Join(reprs(ms), ",\n ") + "]"
}

// listDiff returns the failure message of Python's assertEqual for two lists
// of messages, or "" when they are equal.
func listDiff(first, second []messages.Message) string {
	var b strings.Builder
	a1, a2 := shortenRepr(reprList(first), reprList(second))
	fmt.Fprintf(&b, "Lists differ: %s != %s\n", a1, a2)
	differ := false
	for i := range min(len(first), len(second)) {
		if !first[i].Equal(second[i]) {
			e1, e2 := shortenRepr(first[i].Repr(), second[i].Repr())
			fmt.Fprintf(&b, "\nFirst differing element %d:\n%s\n%s\n", i, e1, e2)
			differ = true
			break
		}
	}
	switch {
	case len(first) > len(second):
		fmt.Fprintf(&b, "\nFirst list contains %d additional elements.\n", len(first)-len(second))
		fmt.Fprintf(&b, "First extra element %d:\n%s\n", len(second), first[len(second)].Repr())
	case len(first) < len(second):
		fmt.Fprintf(&b, "\nSecond list contains %d additional elements.\n", len(second)-len(first))
		fmt.Fprintf(&b, "First extra element %d:\n%s\n", len(first), second[len(first)].Repr())
	case !differ:
		return ""
	}
	b.WriteString("\n" + lineDiff(pformatList(first), pformatList(second)))
	return b.String()
}

// countDiff returns the failure message of Python's assertCountEqual for two
// lists of messages, or "" when they hold the same messages the same number of
// times (unittest's _count_diff_all_purpose, as messages are not hashable).
func countDiff(first, second []messages.Message) string {
	s, t := messageRefs(first), messageRefs(second)
	var lines []string
	for i, elem := range s {
		if elem == nil {
			continue
		}
		cntS, cntT := 0, 0
		for j := i; j < len(s); j++ {
			if s[j] != nil && s[j].Equal(*elem) {
				cntS++
				s[j] = nil
			}
		}
		for j, other := range t {
			if other != nil && other.Equal(*elem) {
				cntT++
				t[j] = nil
			}
		}
		if cntS != cntT {
			lines = append(lines, fmt.Sprintf("First has %d, Second has %d:  %s", cntS, cntT, elem.Repr()))
		}
	}
	for i, elem := range t {
		if elem == nil {
			continue
		}
		cntT := 0
		for j := i; j < len(t); j++ {
			if t[j] != nil && t[j].Equal(*elem) {
				cntT++
				t[j] = nil
			}
		}
		lines = append(lines, fmt.Sprintf("First has 0, Second has %d:  %s", cntT, elem.Repr()))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Element counts were not equal:\n" + strings.Join(lines, "\n")
}

// messageRefs returns pointers to copies of the messages; countDiff clears
// an entry once it is counted.
func messageRefs(ms []messages.Message) []*messages.Message {
	refs := make([]*messages.Message, len(ms))
	for i := range ms {
		m := ms[i]
		refs[i] = &m
	}
	return refs
}

// The limits of unittest's _common_shorten_repr.
const (
	reprMaxLength   = 80
	reprPlaceholder = 12
	reprMinBegin    = 5
	reprMinEnd      = 5
	reprMinCommon   = 5
	reprMinDiff     = reprMaxLength - (reprMinBegin + reprPlaceholder + reprMinEnd)
)

// shortenRepr shortens two reprs whose longer one exceeds 80 characters by
// eliding the middle of their common prefix and of their tails (unittest's
// _common_shorten_repr).
func shortenRepr(a, b string) (string, string) {
	ra, rb := []rune(a), []rune(b)
	maxLen := max(len(ra), len(rb))
	if maxLen <= reprMaxLength {
		return a, b
	}
	prefixLen := 0
	for prefixLen < min(len(ra), len(rb)) && ra[prefixLen] == rb[prefixLen] {
		prefixLen++
	}
	prefix := ra[:prefixLen]
	commonLen := reprMaxLength - (maxLen - prefixLen + reprMinBegin + reprPlaceholder)
	if commonLen > reprMinCommon {
		p := shortenRunes(prefix, reprMinBegin, commonLen)
		return p + string(ra[prefixLen:]), p + string(rb[prefixLen:])
	}
	p := shortenRunes(prefix, reprMinBegin, reprMinCommon)
	return p + shortenRunes(ra[prefixLen:], reprMinDiff, reprMinEnd),
		p + shortenRunes(rb[prefixLen:], reprMinDiff, reprMinEnd)
}

// shortenRunes replaces the middle of s with "[N chars]" when more than 12
// characters lie between its first prefixLen and last suffixLen characters
// (unittest's _shorten).
func shortenRunes(s []rune, prefixLen, suffixLen int) string {
	skip := len(s) - prefixLen - suffixLen
	if skip > reprPlaceholder {
		return fmt.Sprintf("%s[%d chars]%s", string(s[:prefixLen]), skip, string(s[len(s)-suffixLen:]))
	}
	return string(s)
}
