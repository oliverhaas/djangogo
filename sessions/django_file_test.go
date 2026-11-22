package sessions_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/sessions"
)

// The tests in this file are the tests of Django's FileSessionTests. The tests
// of SessionTestsMixin that FileSessionTests runs too are in
// django_sessions_test.go, as subtests for each store.
//
// No Go counterpart: FileSessionPathLibTests runs FileSessionTests with
// SESSION_FILE_PATH as a pathlib.Path. Go has one type for paths, string.

// newFileStore returns a file store in a new temporary directory, as the setUp
// of FileSessionTests sets SESSION_FILE_PATH to one.
func newFileStore(t *testing.T) *sessions.FileStore {
	t.Helper()
	store, err := sessions.NewFileStore(t.TempDir(), "sessionid")
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return store
}

// Ported from Django: tests/sessions_tests/tests.py::FileSessionTests.test_configuration_check
func TestFileSessionConfigurationCheck(t *testing.T) {
	// Make sure the file backend checks for a good storage dir.
	dir := "/if/this/directory/exists/you/have/a/weird/computer"
	_, err := sessions.NewFileStore(dir, "sessionid")
	want := `The session storage path "` + dir + `" doesn't exist. Please set your ` +
		"SESSION_FILE_PATH setting to an existing directory in which Django can store session data."
	if err == nil || err.Error() != want {
		t.Errorf("NewFileStore error = %v, want %q", err, want)
	}
}

// Ported from Django: tests/sessions_tests/tests.py::FileSessionTests.test_invalid_key_backslash
func TestFileSessionInvalidKeyBackslash(t *testing.T) {
	// Ensure we don't allow directory-traversal.
	_, err := sessions.FileStorePath(newFileStore(t), `a\b\c`)
	djangotest.AssertErrorIs(t, err, sessions.ErrInvalidKey)
}

// Ported from Django: tests/sessions_tests/tests.py::FileSessionTests.test_invalid_key_forwardslash
func TestFileSessionInvalidKeyForwardslash(t *testing.T) {
	// Ensure we don't allow directory-traversal.
	_, err := sessions.FileStorePath(newFileStore(t), "a/b/c")
	djangotest.AssertErrorIs(t, err, sessions.ErrInvalidKey)
}

// TestFileStoreIgnoresTraversalKey loads a cookie value that names a file
// outside the directory. No Django test exists: Django's tests check the key
// check only, because load swallows its error.
func TestFileStoreIgnoresTraversalKey(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside"), []byte(`{"user":"admin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := sessions.NewFileStore(sessionDir, "")
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	s := load(t, store, "../outside")
	djangotest.AssertEqual(t, s.Data(), map[string]any{})
	djangotest.AssertEqual(t, s.Key(), "")
	djangotest.AssertEqual(t, exists(t, store, "../outside"), false)
}
