package djangogo_test

import (
	"bytes"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
)

// adminScript runs the command line args in a new working directory, the
// test_dir of Django's AdminScriptTestCase, and returns the directory.
type adminScript struct {
	t   *testing.T
	dir string
}

// newAdminScript changes into a new temporary directory for the test.
func newAdminScript(t *testing.T) *adminScript {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	// The temporary directory may be reached through a symbolic link, as on
	// macOS; the commands print the path the working directory resolves to.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return &adminScript{t: t, dir: wd}
}

// run runs args and returns stdout and stderr, Django's run_django_admin.
func (s *adminScript) run(args ...string) (out, errOut string) {
	s.t.Helper()
	app, err := djangogo.New(conf.Settings{SecretKey: "k"})
	if err != nil {
		s.t.Fatalf("New: %v", err)
	}
	var o, e bytes.Buffer
	app.Out, app.Err = &o, &e
	app.Run(args)
	return o.String(), e.String()
}

// path returns the absolute path of rel in the working directory.
func (s *adminScript) path(rel string) string { return filepath.Join(s.dir, filepath.FromSlash(rel)) }

// exists reports whether rel exists in the working directory.
func (s *adminScript) exists(rel string) bool {
	_, err := os.Stat(s.path(rel))
	return err == nil
}

// assertNoOutput fails when out is not empty.
func assertNoOutput(t *testing.T, out string) {
	t.Helper()
	if out != "" {
		t.Errorf("unexpected output: %q", out)
	}
}

// skipWithoutGoRoot skips a test of the standard library name check when the
// Go root is unknown, as in a binary built with -trimpath.
func skipWithoutGoRoot(t *testing.T) {
	t.Helper()
	if build.Default.GOROOT == "" {
		t.Skip("the Go root is unknown")
	}
}

// TestStartProjectWrongArgs prints the usage and Django's message without a
// name.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_wrong_args
func TestStartProjectWrongArgs(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startproject")
	assertNoOutput(t, out)
	assertOutput(t, errOut, "usage:", true)
	assertOutput(t, errOut, "You must provide a project name.", true)
}

// TestStartProjectSimpleProject creates a project and refuses to create it a
// second time. Python finds the first project as a module and reports a
// conflicting module name; Go has no such lookup, so the second run reports
// the directory, the error of TemplateCommand.handle that follows the name
// check.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_simple_project
func TestStartProjectSimpleProject(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "testproject")
	assertNoOutput(t, errOut)
	if !s.exists("testproject/main.go") {
		t.Fatal("the project was not created")
	}

	out, errOut := s.run("startproject", "testproject")
	assertNoOutput(t, out)
	assertOutput(t, errOut, "CommandError: '"+s.path("testproject")+"' already exists", true)
}

// TestStartProjectInvalidProjectName refuses a name that is not an
// identifier and creates nothing.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_invalid_project_name
func TestStartProjectInvalidProjectName(t *testing.T) {
	for _, bad := range []string{"7testproject", "../testproject"} {
		t.Run(bad, func(t *testing.T) {
			s := newAdminScript(t)
			_, errOut := s.run("startproject", bad)
			assertOutput(t, errOut, "Error: '"+bad+"' is not a valid project name. Please make "+
				"sure the name is a valid identifier.", true)
			if s.exists(bad) {
				t.Errorf("%s was created", bad)
			}
		})
	}
}

// TestStartProjectImportableProjectName refuses the name of a standard
// library package, the counterpart of an existing Python module.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_importable_project_name
func TestStartProjectImportableProjectName(t *testing.T) {
	skipWithoutGoRoot(t)
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "os")
	assertOutput(t, errOut, "CommandError: 'os' conflicts with the name of an existing "+
		"Go package and cannot be used as a project name. Please try another name.", true)
	if s.exists("os") {
		t.Error("os was created")
	}
}

// TestStartProjectSimpleProjectDifferentDirectory creates a project in a
// given directory and refuses to overlay it a second time.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_simple_project_different_directory
func TestStartProjectSimpleProjectDifferentDirectory(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("othertestproject"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startproject", "testproject", "othertestproject")
	assertNoOutput(t, errOut)
	if !s.exists("othertestproject/main.go") {
		t.Fatal("the project was not created in othertestproject")
	}

	out, errOut := s.run("startproject", "testproject", "othertestproject")
	assertNoOutput(t, out)
	assertOutput(t, errOut, "already exists. Overlaying a project into an existing directory "+
		"won't replace conflicting files.", true)
}

// TestStartProjectCustomProjectDestinationMissing creates a missing
// destination directory. Djan-Go-Go has no --template option, so the port
// uses the built-in template.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_destination_missing
func TestStartProjectCustomProjectDestinationMissing(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startproject", "yet_another_project", "project_dir2")
	assertNoOutput(t, out)
	assertNoOutput(t, errOut)
	if !s.exists("project_dir2") {
		t.Error("project_dir2 was not created")
	}
}

// TestStartAppInvalidName refuses an app name that is not an identifier.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_invalid_name
func TestStartAppInvalidName(t *testing.T) {
	for _, bad := range []string{"7testproject", "../testproject"} {
		t.Run(bad, func(t *testing.T) {
			s := newAdminScript(t)
			_, errOut := s.run("startapp", bad)
			assertOutput(t, errOut, "CommandError: '"+bad+"' is not a valid app name. Please make "+
				"sure the name is a valid identifier.", true)
			if s.exists(bad) {
				t.Errorf("%s was created", bad)
			}
		})
	}
}

// TestStartAppImportableName refuses the name of a standard library package.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_importable_name
func TestStartAppImportableName(t *testing.T) {
	skipWithoutGoRoot(t)
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "os")
	assertOutput(t, errOut, "CommandError: 'os' conflicts with the name of an existing "+
		"Go package and cannot be used as an app name. Please try another name.", true)
	if s.exists("os") {
		t.Error("os was created")
	}
}

// TestStartAppInvalidTargetName refuses a target directory whose name is not
// an identifier.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_invalid_target_name
func TestStartAppInvalidTargetName(t *testing.T) {
	for _, bad := range []string{"invalid.dir_name", "7invalid_dir_name", ".invalid_dir_name"} {
		t.Run(bad, func(t *testing.T) {
			s := newAdminScript(t)
			_, errOut := s.run("startapp", "app", bad)
			assertOutput(t, errOut, "CommandError: '"+bad+"' is not a valid app directory. Please "+
				"make sure the directory is a valid identifier.", true)
		})
	}
}

// TestStartAppImportableTargetName refuses a target directory named like a
// standard library package.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_importable_target_name
func TestStartAppImportableTargetName(t *testing.T) {
	skipWithoutGoRoot(t)
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "app", "os")
	assertOutput(t, errOut, "CommandError: 'os' conflicts with the name of an existing Go "+
		"package and cannot be used as an app directory. Please try another directory.", true)
}

// TestStartAppTrailingSlashInTargetAppDirectoryName accepts a target
// directory with a trailing slash.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_trailing_slash_in_target_app_directory_name
func TestStartAppTrailingSlashInTargetAppDirectoryName(t *testing.T) {
	s := newAdminScript(t)
	if err := os.MkdirAll(s.path("apps/app1"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startapp", "app", filepath.Join("apps", "app1")+string(filepath.Separator))
	assertNoOutput(t, errOut)
	if !s.exists("apps/app1/app.go") {
		t.Error("apps/app1/app.go was not created")
	}
}

// TestStartAppOverlayingApp refuses to write an app over the files of
// another one.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_overlaying_app
func TestStartAppOverlayingApp(t *testing.T) {
	s := newAdminScript(t)
	if err := os.MkdirAll(s.path("apps/app1"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.run("startapp", "app1", "apps/app1")
	_, errOut := s.run("startapp", "app2", "apps/app1")
	assertOutput(t, errOut, "already exists. Overlaying an app into an existing directory "+
		"won't replace conflicting files.", true)
}

// TestStartAppTemplate names the app configuration after the app. Django
// writes class NewAppConfig(AppConfig) with name = 'new_app' into apps.py;
// the Go counterpart is the App type in app.go.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_template
func TestStartAppTemplate(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "new_app")
	assertNoOutput(t, errOut)
	content, err := os.ReadFile(s.path("new_app/app.go"))
	if err != nil {
		t.Fatalf("new_app/app.go: %v", err)
	}
	assertOutput(t, string(content), "package new_app\n", true)
	assertOutput(t, string(content), "type App struct {", true)
	assertOutput(t, string(content), `func (*App) Name() string { return "new_app" }`, true)
}

// TestStartAppCreatesDirectoryWhenCustomAppDestinationMissing creates a
// missing target directory.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_creates_directory_when_custom_app_destination_missing
func TestStartAppCreatesDirectoryWhenCustomAppDestinationMissing(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startapp", "my_app", "my_app")
	assertNoOutput(t, out)
	assertNoOutput(t, errOut)
	if !s.exists("my_app") {
		t.Error("my_app was not created")
	}
}

// TestStartAppCustomAppDestinationMissingWithNestedSubdirectory creates the
// missing parents of the target directory too.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_custom_app_destination_missing_with_nested_subdirectory
func TestStartAppCustomAppDestinationMissingWithNestedSubdirectory(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startapp", "my_app", "apps/my_app")
	assertNoOutput(t, out)
	assertNoOutput(t, errOut)
	if !s.exists("apps/my_app") {
		t.Error("apps/my_app was not created")
	}
}

// TestStartAppCustomNameWithAppWithinOtherApp creates an app inside another.
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_custom_name_with_app_within_other_app
func TestStartAppCustomNameWithAppWithinOtherApp(t *testing.T) {
	s := newAdminScript(t)
	s.run("startapp", "parent")
	if !s.exists("parent") {
		t.Fatal("parent was not created")
	}
	out, errOut := s.run("startapp", "child", "parent/child")
	assertNoOutput(t, out)
	assertNoOutput(t, errOut)
	if !s.exists("parent/child") {
		t.Error("parent/child was not created")
	}
}

// TestStartAppCustomAppDirectoryCreationErrorHandling reports the error of
// creating the target directory. Go spells the error "permission denied".
//
// Ported from Django: tests/admin_scripts/tests.py::StartApp.test_custom_app_directory_creation_error_handling
func TestStartAppCustomAppDirectoryCreationErrorHandling(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir"), 0o200); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.path("project_dir"), 0o700) })
	out, errOut := s.run("startapp", "my_app", "project_dir/my_app")
	assertNoOutput(t, out)
	assertOutput(t, errOut, "permission denied", true)
	if s.exists("project_dir/my_app") {
		t.Error("project_dir/my_app was created")
	}
}

// TestTemplateCommandVerbosity prints every file it creates at verbosity 2
// and nothing at the default verbosity, as TemplateCommand.handle does.
func TestTemplateCommandVerbosity(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startapp", "blog", "-v", "2")
	assertNoOutput(t, errOut)
	assertOutput(t, out, "Creating "+s.path("blog/app.go")+"\n", true)
	assertOutput(t, out, "Creating "+s.path("blog/templates/blog/post_list.html")+"\n", true)
	if n := strings.Count(out, "Creating "); n != strings.Count(out, "\n") {
		t.Errorf("unexpected lines in the output:\n%s", out)
	}
}

// TestTemplateCommandReservedNames refuses the names Go reserves for
// packages, which Python accepts as identifiers.
func TestTemplateCommandReservedNames(t *testing.T) {
	for _, name := range []string{"main", "init", "_"} {
		t.Run(name, func(t *testing.T) {
			s := newAdminScript(t)
			_, errOut := s.run("startapp", name)
			assertOutput(t, errOut, "CommandError: '"+name+"' is reserved by Go and cannot be used "+
				"as an app name. Please try another name.", true)
		})
	}
}

// TestTemplateCommandTooManyArguments refuses a third argument.
func TestTemplateCommandTooManyArguments(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "a", "b", "c")
	assertOutput(t, errOut, "error: unrecognized arguments: c", true)
	_, errOut = s.run("startapp")
	assertOutput(t, errOut, "You must provide an application name.", true)
}
