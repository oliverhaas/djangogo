package conf_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// writeFile writes a file into a new directory of the test and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Every documented form of a line gives the documented value.
func TestReadDotEnvSyntax(t *testing.T) {
	tests := []struct {
		name string
		file string
		want map[string]string
	}{
		{"plain", "KEY=value", map[string]string{"KEY": "value"}},
		{"export", "export KEY=value", map[string]string{"KEY": "value"}},
		{"export and a tab", "export\tKEY=value", map[string]string{"KEY": "value"}},
		{"a name that starts with export", "exported=value", map[string]string{"exported": "value"}},
		{"spaces around the equals sign", "KEY = value", map[string]string{"KEY": "value"}},
		{"indented", "  KEY=value  ", map[string]string{"KEY": "value"}},
		{"empty value", "KEY=", map[string]string{"KEY": ""}},
		{"equals signs in the value", "KEY=a=b=c", map[string]string{"KEY": "a=b=c"}},
		{"spaces inside the value", "KEY=two words", map[string]string{"KEY": "two words"}},
		{"lower case and digits in the name", "key_2=value", map[string]string{"key_2": "value"}},

		{"comment line", "# KEY=value", map[string]string{}},
		{"indented comment line", "   # KEY=value", map[string]string{}},
		{"blank line", "   ", map[string]string{}},
		{"trailing comment", "KEY=value # comment", map[string]string{"KEY": "value"}},
		{"trailing comment after a tab", "KEY=value\t# comment", map[string]string{"KEY": "value"}},
		{"comment in the place of the value", "KEY= # comment", map[string]string{"KEY": ""}},
		{"hash inside the value", "KEY=http://example.com/#top", map[string]string{"KEY": "http://example.com/#top"}},
		{"hash at the start of the value", "KEY=#fff", map[string]string{"KEY": "#fff"}},

		{"single quotes", "KEY='value'", map[string]string{"KEY": "value"}},
		{"single quotes are literal", `KEY='a "b" \n $C # d'`, map[string]string{"KEY": `a "b" \n $C # d`}},
		{"single quotes keep spaces", "KEY='  value  '", map[string]string{"KEY": "  value  "}},
		{"single quotes and a comment", "KEY='value' # comment", map[string]string{"KEY": "value"}},
		{"empty single quotes", "KEY=''", map[string]string{"KEY": ""}},

		{"double quotes", `KEY="value"`, map[string]string{"KEY": "value"}},
		{"double quotes keep spaces and hashes", `KEY="  a # b  "`, map[string]string{"KEY": "  a # b  "}},
		{"double quotes and a comment", `KEY="value" # comment`, map[string]string{"KEY": "value"}},
		{"empty double quotes", `KEY=""`, map[string]string{"KEY": ""}},
		{"newline and tab escapes", `KEY="one\ntwo\tthree"`, map[string]string{"KEY": "one\ntwo\tthree"}},
		{"quote and backslash escapes", `KEY="say \"hi\" \\ bye"`, map[string]string{"KEY": `say "hi" \ bye`}},
		{"an escaped backslash before n", `KEY="a\\nb"`, map[string]string{"KEY": `a\nb`}},
		{"an unknown escape is kept", `KEY="C:\dir"`, map[string]string{"KEY": `C:\dir`}},
		{"no expansion", `KEY="${OTHER} $OTHER"`, map[string]string{"KEY": "${OTHER} $OTHER"}},
		{"single quote inside double quotes", `KEY="it's"`, map[string]string{"KEY": "it's"}},

		{"the last value of a name wins", "KEY=first\nKEY=second", map[string]string{"KEY": "second"}},
		{"several lines", "A=1\n\n# comment\nB=2\n", map[string]string{"A": "1", "B": "2"}},
		{"CRLF line ends", "A=1\r\nB=\"2\"\r\n", map[string]string{"A": "1", "B": "2"}},
		{"byte order mark", "\xef\xbb\xbfA=1\n", map[string]string{"A": "1"}},
		{"empty file", "", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := conf.ReadDotEnv(writeFile(t, ".env", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, got, tt.want)
		})
	}
}

// Every malformed line is reported with the file and the line number, and the
// text of a line is not repeated, because it may hold a secret.
func TestReadDotEnvMalformedLines(t *testing.T) {
	path := writeFile(t, "broken.env", strings.Join([]string{
		"GOOD=1",
		"password hunter2",
		"1BAD=hunter2",
		"=hunter2",
		"BAD NAME=hunter2",
		"BAD-NAME=hunter2",
		"# a comment",
		"SINGLE='hunter2",
		`DOUBLE="hunter2`,
		`ESCAPED="hunter2\"`,
		`TRAILER="hunter2" hunter2`,
		"TRAILER='hunter2'hunter2",
		"export",
		"ALSO_GOOD=2",
	}, "\n"))

	vars, err := conf.ReadDotEnv(path)

	djangotest.AssertEqual(t, errorLines(err), []string{
		"conf: " + path + ":2: expected KEY=value",
		"conf: " + path + ":3: invalid variable name",
		"conf: " + path + ":4: invalid variable name",
		"conf: " + path + ":5: invalid variable name",
		"conf: " + path + ":6: invalid variable name",
		"conf: " + path + ":8: unterminated single-quoted value",
		"conf: " + path + ":9: unterminated double-quoted value",
		"conf: " + path + ":10: unterminated double-quoted value",
		"conf: " + path + ":11: unexpected text after the closing quote",
		"conf: " + path + ":12: unexpected text after the closing quote",
		"conf: " + path + ":13: expected KEY=value",
	})
	djangotest.AssertEqual(t, vars, nil)
}

// A caller can tell a missing file from a malformed one.
func TestReadDotEnvMissingFile(t *testing.T) {
	vars, err := conf.ReadDotEnv(filepath.Join(t.TempDir(), "missing.env"))

	djangotest.AssertErrorIs(t, err, fs.ErrNotExist)
	djangotest.AssertEqual(t, vars, nil)
	if !strings.HasPrefix(err.Error(), "conf: ") {
		t.Errorf("the error lacks the package prefix: %v", err)
	}
}

// A file read with ReadDotEnv can stand in for the process environment.
func TestReadDotEnvFeedsApplyEnvFunc(t *testing.T) {
	vars, err := conf.ReadDotEnv(writeFile(t, ".env", "APP_DEBUG=true\nAPP_ALLOWED_HOSTS='a.example, b.example'\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := conf.Defaults()
	if err := s.ApplyEnvFunc("APP_", mapEnv(vars)); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, s.Debug, true)
	djangotest.AssertEqual(t, s.AllowedHosts, []string{"a.example", "b.example"})
}

// lookupEnv returns the value of a process variable, or "<unset>".
func lookupEnv(name string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return "<unset>"
}

// The file fills in the variables that the process environment lacks.
func TestLoadDotEnv(t *testing.T) {
	unsetEnv(t, "CONFTEST_NEW")
	unsetEnv(t, "CONFTEST_EMPTY")
	t.Setenv("CONFTEST_REAL", "from the real environment")
	t.Setenv("CONFTEST_REAL_EMPTY", "")
	path := writeFile(t, ".env", strings.Join([]string{
		"CONFTEST_NEW=from the file",
		"CONFTEST_EMPTY=",
		"CONFTEST_REAL=from the file",
		"CONFTEST_REAL_EMPTY=from the file",
	}, "\n"))

	if err := conf.LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, lookupEnv("CONFTEST_NEW"), "from the file")
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_EMPTY"), "")
	// A variable of the real environment wins, also a set but empty one.
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_REAL"), "from the real environment")
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_REAL_EMPTY"), "")
}

// Without a path, the file is .env in the working directory.
func TestLoadDotEnvReadsDotEnvOfTheWorkingDirectory(t *testing.T) {
	unsetEnv(t, "CONFTEST_NEW")
	t.Chdir(filepath.Dir(writeFile(t, ".env", "CONFTEST_NEW=from .env\n")))

	if err := conf.LoadDotEnv(); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, lookupEnv("CONFTEST_NEW"), "from .env")
}

// A project without a .env file loads nothing and gets no error.
func TestLoadDotEnvWithoutAFile(t *testing.T) {
	unsetEnv(t, "CONFTEST_NEW")
	dir := t.TempDir()
	t.Chdir(dir)

	djangotest.AssertEqual(t, conf.LoadDotEnv(), error(nil))
	djangotest.AssertEqual(t, conf.LoadDotEnv(filepath.Join(dir, "missing.env")), error(nil))
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_NEW"), "<unset>")
}

// Of several files the first one that sets a variable wins, and a missing
// file among them is skipped.
func TestLoadDotEnvFirstFileWins(t *testing.T) {
	unsetEnv(t, "CONFTEST_BOTH")
	unsetEnv(t, "CONFTEST_BASE")
	local := writeFile(t, ".env.local", "CONFTEST_BOTH=local\n")
	missing := filepath.Join(t.TempDir(), ".env.missing")
	base := writeFile(t, ".env", "CONFTEST_BOTH=base\nCONFTEST_BASE=base\n")

	if err := conf.LoadDotEnv(local, missing, base); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, lookupEnv("CONFTEST_BOTH"), "local")
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_BASE"), "base")
}

// When one file is malformed, no file changes the environment.
func TestLoadDotEnvMalformedSetsNothing(t *testing.T) {
	unsetEnv(t, "CONFTEST_NEW")
	unsetEnv(t, "CONFTEST_OTHER")
	good := writeFile(t, ".env", "CONFTEST_NEW=1\n")
	broken := writeFile(t, ".env", "CONFTEST_OTHER=2\nnot a variable\n")

	err := conf.LoadDotEnv(good, broken)

	djangotest.AssertEqual(t, errorLines(err), []string{"conf: " + broken + ":2: expected KEY=value"})
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_NEW"), "<unset>")
	djangotest.AssertEqual(t, lookupEnv("CONFTEST_OTHER"), "<unset>")
}

// A path that cannot be read as a file is an error, unlike a missing file.
func TestLoadDotEnvUnreadableFile(t *testing.T) {
	err := conf.LoadDotEnv(t.TempDir())
	if err == nil {
		t.Fatal("LoadDotEnv of a directory returned no error")
	}
}
