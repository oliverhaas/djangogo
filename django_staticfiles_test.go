package djangogo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in this file are ports of TestInteractiveMessages and of the
// command-level tests of TestFindStatic in Django's
// tests/staticfiles_tests/test_management.py. The input of the application
// replaces Django's mock of input().
//
// Not applicable: TestConfiguration.test_local_storage_detection_helper
// (Django storage classes), TestConfiguration.test_collectstatis_check
// (STATICFILES_DIRS is a []string here, so "not a tuple or list" cannot
// occur), TestConfiguration.test_location_empty (ported at package level and
// covered by TestCollectstaticWithoutStaticRoot) and
// TestCollectionHelpSubcommand (settings files and admin scripts).

const (
	overwriteWarningMsg = "This will overwrite existing files!"
	deleteWarningMsg    = "This will DELETE ALL FILES in this location!"
	filesCopiedMsg      = "static file copied"
)

// runCollectstatic is Django's run_collectstatic: the first collection with
// --noinput.
func runCollectstatic(t *testing.T, f *staticFixture) {
	t.Helper()
	if err := f.app.Execute([]string{"collectstatic", "--noinput", "--ignore", "*.css"}); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
}

// assertOutput checks that out contains want (or not, when contains is false).
func assertOutput(t *testing.T, out, msg string, contains bool) {
	t.Helper()
	if strings.Contains(out, msg) != contains {
		t.Errorf("output %q: contains %q = %v, want %v", out, msg, !contains, contains)
	}
}

// TestWarningWhenClearingStaticdir checks the warning of --clear.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestInteractiveMessages.test_warning_when_clearing_staticdir
func TestWarningWhenClearingStaticdir(t *testing.T) {
	f := newStaticFixture(t, true, "yes\n")
	runCollectstatic(t, f)
	if err := f.app.Execute([]string{"collectstatic", "--clear"}); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, f.out.String(), overwriteWarningMsg, false)
	assertOutput(t, f.out.String(), deleteWarningMsg, true)
}

// TestWarningWhenOverwritingFilesInStaticdir checks the overwrite warning.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestInteractiveMessages.test_warning_when_overwriting_files_in_staticdir
func TestWarningWhenOverwritingFilesInStaticdir(t *testing.T) {
	f := newStaticFixture(t, true, "yes\n")
	runCollectstatic(t, f)
	if err := f.app.Execute([]string{"collectstatic"}); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, f.out.String(), overwriteWarningMsg, true)
	assertOutput(t, f.out.String(), deleteWarningMsg, false)
}

// TestNoWarningWhenStaticdirDoesNotExist checks that a missing root is no
// reason to ask.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestInteractiveMessages.test_no_warning_when_staticdir_does_not_exist
func TestNoWarningWhenStaticdirDoesNotExist(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := os.RemoveAll(f.root); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Execute([]string{"collectstatic", "--ignore", "*.css"}); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, f.out.String(), overwriteWarningMsg, false)
	assertOutput(t, f.out.String(), deleteWarningMsg, false)
	assertOutput(t, f.out.String(), filesCopiedMsg, true)
}

// TestNoWarningForEmptyStaticdir checks that an empty root is no reason to
// ask.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestInteractiveMessages.test_no_warning_for_empty_staticdir
func TestNoWarningForEmptyStaticdir(t *testing.T) {
	f := newStaticFixture(t, true, "")
	if err := f.app.Execute([]string{"collectstatic", "--ignore", "*.css"}); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, f.out.String(), overwriteWarningMsg, false)
	assertOutput(t, f.out.String(), deleteWarningMsg, false)
	assertOutput(t, f.out.String(), filesCopiedMsg, true)
}

// TestCancelled checks the error of the answer "no".
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestInteractiveMessages.test_cancelled
func TestCancelled(t *testing.T) {
	f := newStaticFixture(t, true, "no\n")
	runCollectstatic(t, f)
	err := f.app.Execute([]string{"collectstatic"})
	if err == nil || !strings.Contains(err.Error(), "Collecting static files cancelled") {
		t.Errorf("error = %v, want one containing %q", err, "Collecting static files cancelled")
	}
	if _, statErr := os.Stat(filepath.Join(f.root, "test", "file.txt")); statErr != nil {
		t.Error("the collected file is gone after the cancellation")
	}
}

// TestFindstaticMissingArgsMessage checks the usage error without a path.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestFindStatic.test_missing_args_message
func TestFindstaticMissingArgsMessage(t *testing.T) {
	f := newStaticFixture(t, true, "")
	err := f.app.Execute([]string{"findstatic"})
	if err == nil || !strings.Contains(err.Error(), "Enter at least one staticfile.") {
		t.Errorf("error = %v, want one containing %q", err, "Enter at least one staticfile.")
	}
}
