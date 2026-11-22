package mail

import (
	"encoding/json"
	"os"
	"testing"
)

// headerCase is one header of testdata/headers.json, as
// fidelity/oracle/mail.py records it from Python's email package.
type headerCase struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Error     string `json:"error"`
	Default   string `json:"default"`
	SMTP      string `json:"smtp"`
	Mailboxes []struct {
		Type        string  `json:"type"`
		DisplayName *string `json:"display_name"`
		LocalPart   *string `json:"local_part"`
		Domain      *string `json:"domain"`
		AddrSpec    *string `json:"addr_spec"`
	} `json:"mailboxes"`
	Defects []string `json:"defects"`
	TreeStr string   `json:"tree_str"`
}

// headerGolden is testdata/headers.json.
type headerGolden struct {
	Headers []headerCase `json:"headers"`
	// Names holds the error of setting a header with each name, or "".
	Names []struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	} `json:"names"`
	// Unique reports for each name whether a message refuses a second
	// header with it.
	Unique map[string]bool `json:"unique"`
}

// loadHeaderGolden reads the header goldens.
func loadHeaderGolden(t *testing.T) headerGolden {
	t.Helper()
	data, err := os.ReadFile("testdata/headers.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden headerGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

// loadHeaderCases reads the header cases of the goldens.
func loadHeaderCases(t *testing.T) []headerCase {
	t.Helper()
	return loadHeaderGolden(t).Headers
}

// optional returns the value and presence of a Python value that may be
// None.
func optional(s string, ok bool) *string {
	if !ok {
		return nil
	}
	return &s
}

// show prints an optional string for a failure message.
func show(s *string) string {
	if s == nil {
		return "None"
	}
	return pyRepr(*s)
}

// No Django test exists: the folded header lines must match those of
// Python's policy.default and policy.SMTP, which Django's messages use.
func TestFoldHeaderMatchesPython(t *testing.T) {
	for _, c := range loadHeaderCases(t) {
		tree, err := parseHeader(c.Name, c.Value)
		if c.Error != "" {
			if err == nil {
				t.Errorf("%s: %q: no error, want Python's %s", c.Name, c.Value, c.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %q: %v", c.Name, c.Value, err)
			continue
		}
		if got := foldHeader(c.Name, tree, "\n"); got != c.Default {
			t.Errorf("%s: %q:\n got %q\nwant %q", c.Name, c.Value, got, c.Default)
		}
		if got := foldHeader(c.Name, tree, "\r\n"); got != c.SMTP {
			t.Errorf("%s: %q (SMTP):\n got %q\nwant %q", c.Name, c.Value, got, c.SMTP)
		}
	}
}

// No Django test exists: the parsed mailboxes and the defects of address
// headers must match those of Python's get_address_list, which Django's SMTP
// backend checks addresses with.
func TestParseAddressListMatchesPython(t *testing.T) {
	for _, c := range loadHeaderCases(t) {
		if c.Mailboxes == nil || c.Value == "" {
			continue
		}
		tree := parseAddressList(c.Value)
		if got := tree.String(); got != c.TreeStr {
			t.Errorf("%q: str %q, want %q", c.Value, got, c.TreeStr)
		}
		var defects []string
		for _, d := range tree.allDefects() {
			defects = append(defects, d.msg)
		}
		if len(defects) != len(c.Defects) {
			t.Errorf("%q: defects %q, want %q", c.Value, defects, c.Defects)
		} else {
			for i := range defects {
				if defects[i] != c.Defects[i] {
					t.Errorf("%q: defects %q, want %q", c.Value, defects, c.Defects)
					break
				}
			}
		}
		mailboxes := tree.allMailboxes()
		if len(mailboxes) != len(c.Mailboxes) {
			t.Errorf("%q: %d mailboxes, want %d", c.Value, len(mailboxes), len(c.Mailboxes))
			continue
		}
		for i, mb := range mailboxes {
			want := c.Mailboxes[i]
			got := []*string{optional(mb.displayName()), optional(mb.localPart()), optional(mb.domain()), optional(mb.addrSpec())}
			wants := []*string{want.DisplayName, want.LocalPart, want.Domain, want.AddrSpec}
			names := []string{"display_name", "local_part", "domain", "addr_spec"}
			if mb.typ != want.Type {
				t.Errorf("%q mailbox %d: type %s, want %s", c.Value, i, mb.typ, want.Type)
			}
			for j := range got {
				if show(got[j]) != show(wants[j]) {
					t.Errorf("%q mailbox %d: %s %s, want %s", c.Value, i, names[j], show(got[j]), show(wants[j]))
				}
			}
		}
	}
}

// No Django test exists: a header name with a character outside the
// printable ASCII range or a colon must fail as in Python's
// validate_header_name, which msg[name] = value runs.
func TestValidateHeaderNameMatchesPython(t *testing.T) {
	for _, c := range loadHeaderGolden(t).Names {
		got := ""
		if err := (&entity{}).set(c.Name, "value"); err != nil {
			got = err.Error()
		}
		if got != c.Error {
			t.Errorf("%q: error %q, want %q", c.Name, got, c.Error)
		}
	}
}

// No Django test exists: a message must refuse a second header of the names
// that Python's header registry gives a max_count of 1.
func TestUniqueHeaderMatchesPython(t *testing.T) {
	for name, want := range loadHeaderGolden(t).Unique {
		if got := uniqueHeader(name); got != want {
			t.Errorf("%s: unique %v, want %v", name, got, want)
		}
	}
}
