package manage

import (
	"math"
	"testing"
)

// django60Commands are the command names of a new Django 6.0 project, the
// candidates the expected values of the tests below were computed against.
var django60Commands = []string{
	"changepassword", "check", "clearsessions", "collectstatic", "compilemessages",
	"createcachetable", "createsuperuser", "dbshell", "diffsettings", "dumpdata",
	"findstatic", "flush", "inspectdb", "loaddata", "makemessages", "makemigrations",
	"migrate", "optimizemigration", "remove_stale_contenttypes", "runserver",
	"sendtestemail", "shell", "showmigrations", "sqlflush", "sqlmigrate",
	"sqlsequencereset", "squashmigrations", "startapp", "startproject", "test", "testserver",
}

// TestRatioMatchesDifflib compares ratio with values printed by Python 3.12:
// difflib.SequenceMatcher(None, a, b).ratio().
func TestRatioMatchesDifflib(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"runserver", "rnserver", 0.9411764705882353},
		{"abcd", "bcde", 0.75},
		{"", "", 1.0},
		{"a", "", 0.0},
		{"migrate", "migrat", 0.9230769230769231},
		{"sqlmigrate", "migrat", 0.75},
		{"testserver", "tset", 0.42857142857142855},
		{"startproject", "startapp", 0.6},
		{"xxabcxx", "yyabcyyabcd", 0.3333333333333333},
		{"private Thread currentThread;", "private volatile Thread currentThread;", 0.8656716417910447},
		{"abxcd", "abcd", 0.8888888888888888},
		{"qabxcd", "abycdf", 0.6666666666666666},
		{"aXbXcXd", "abcd", 0.7272727272727273},
		{"café", "cafe", 0.75},
	}
	for _, tt := range tests {
		if got := ratio(tt.a, tt.b); math.Abs(got-tt.want) > 1e-15 {
			t.Errorf("ratio(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// TestCloseMatchMatchesDifflib compares closeMatch with the first element of
// difflib.get_close_matches(word, commands), as printed by Python 3.12; ""
// stands for an empty list.
func TestCloseMatchMatchesDifflib(t *testing.T) {
	tests := []struct{ word, want string }{
		{"rnserver", "runserver"},
		{"abcdef", ""},
		{"migrat", "migrate"},
		{"makemigration", "makemigrations"},
		{"tset", "test"},
		{"sqlmigrat", "sqlmigrate"},
		{"start", "startapp"},
		{"shel", "shell"},
		{"chek", "check"},
		{"runsrver", "runserver"},
		{"test", "test"},
		{"tests", "test"},
		{"sql", ""},
		{"x", ""},
		{"", ""},
		{"startap", "startapp"},
		{"collectstatc", "collectstatic"},
		{"migrations", "showmigrations"},
		{"MIGRATE", ""},
		{"dumpdta", "dumpdata"},
		{"showmigration", "showmigrations"},
		{"testsever", "testserver"},
		{"create", "migrate"},
		{"createsuperusr", "createsuperuser"},
		{"flushh", "flush"},
		{"sqlflash", "sqlflush"},
	}
	for _, tt := range tests {
		got, found := closeMatch(tt.word, django60Commands)
		if got != tt.want || found != (tt.want != "") {
			t.Errorf("closeMatch(%q) = %q, %v; want %q", tt.word, got, found, tt.want)
		}
	}
}

// TestCloseMatchTie: of two names with the same score Python returns the
// greater one, because it orders (score, name) pairs.
func TestCloseMatchTie(t *testing.T) {
	// get_close_matches("ab", ["abc", "abd"]) is ['abd', 'abc'].
	if got, _ := closeMatch("ab", []string{"abc", "abd"}); got != "abd" {
		t.Errorf("closeMatch = %q, want %q", got, "abd")
	}
	if got, _ := closeMatch("ab", []string{"abd", "abc"}); got != "abd" {
		t.Errorf("closeMatch = %q, want %q", got, "abd")
	}
}

// TestPyRepr compares pyRepr with Python's repr.
func TestPyRepr(t *testing.T) {
	tests := []struct{ in, want string }{
		{"rnserver", `'rnserver'`},
		{"", `''`},
		{"it's", `"it's"`},
		{`a"b`, `'a"b'`},
		{`it's "x"`, `'it\'s "x"'`},
		{`a\b`, `'a\\b'`},
		{"a\nb\tc\rd", `'a\nb\tc\rd'`},
		{"a\x00b", `'a\x00b'`},
		{"café", `'café'`},
		{"a\u200bb", `'a\u200bb'`},
		{"a\U000e0001b", `'a\U000e0001b'`},
		{"a\U0001f600b", "'a\U0001f600b'"},
	}
	for _, tt := range tests {
		if got := pyRepr(tt.in); got != tt.want {
			t.Errorf("pyRepr(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
