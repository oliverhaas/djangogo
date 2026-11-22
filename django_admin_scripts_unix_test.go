//go:build unix

package djangogo_test

import (
	"os"
	"syscall"
	"testing"
)

// TestStartProjectHonorUmask creates the files and directories with the
// permissions the umask leaves. Django's manage.py is executable in its
// template; the main.go of the built-in project is not, so the port checks
// the executable case with the main.go of the custom template.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_honor_umask
func TestStartProjectHonorUmask(t *testing.T) {
	s := newAdminScript(t)
	old := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(old) })

	_, errOut := s.run("startproject", "testproject")
	assertNoOutput(t, errOut)
	_, errOut = s.run("startproject", "--template", customTemplate("project_template"), "customproject")
	assertNoOutput(t, errOut)
	for rel, want := range map[string]os.FileMode{
		"testproject":                             0o700,
		"testproject/main.go":                     0o600,
		"testproject/testprojectapp":              0o700,
		"testproject/settings.go":                 0o600,
		"customproject/main.go":                   0o700,
		"customproject/customproject":             0o700,
		"customproject/customproject/settings.go": 0o600,
	} {
		info, err := os.Stat(s.path(rel))
		if err != nil {
			t.Error(err)
			continue
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s: mode %o, want %o", rel, got, want)
		}
	}
}
