package djangogo_test

import (
	"bytes"
	"errors"
	"go/build"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/scaffold"
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

// customTemplatesDir holds the template trees of the --template tests,
// Django's tests/admin_scripts/custom_templates. The path is absolute, since
// the tests run in a temporary working directory.
var customTemplatesDir = func() string {
	dir, err := filepath.Abs(filepath.Join("testdata", "custom_templates"))
	if err != nil {
		panic(err)
	}
	return dir
}()

// customTemplate returns the path of name in the custom templates.
func customTemplate(name string) string { return filepath.Join(customTemplatesDir, name) }

// read returns the content of rel in the working directory.
func (s *adminScript) read(rel string) string {
	s.t.Helper()
	return readFile(s.t, s.path(rel))
}

// readFile returns the content of the file at path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
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

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template
func TestStartProjectCustomProjectTemplate(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "customtestproject")
	assertNoOutput(t, errOut)
	if !s.exists("customtestproject/additional_dir") {
		t.Error("customtestproject/additional_dir was not created")
	}
}

// TestStartProjectCustomProjectTemplateNonGoFilesNotFormatted copies a file
// that is not rendered as it is. Django checks that black leaves it alone;
// gofmt touches Go files only.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_non_python_files_not_formatted
func TestStartProjectCustomProjectTemplateNonGoFilesNotFormatted(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "customtestproject")
	assertNoOutput(t, errOut)
	want := readFile(t, filepath.Join(customTemplate("project_template"), "additional_dir", "requirements.in"))
	if got := s.read("customtestproject/additional_dir/requirements.in"); got != want {
		t.Errorf("requirements.in = %q, want %q", got, want)
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_template_dir_with_trailing_slash
func TestStartProjectTemplateDirWithTrailingSlash(t *testing.T) {
	s := newAdminScript(t)
	template := customTemplate("project_template") + string(filepath.Separator)
	_, errOut := s.run("startproject", "--template", template, "customtestproject")
	assertNoOutput(t, errOut)
	if !s.exists("customtestproject/additional_dir") {
		t.Error("customtestproject/additional_dir was not created")
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_from_tarball_by_path
func TestStartProjectCustomProjectTemplateFromTarballByPath(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", customTemplate("project_template.tgz"), "tarballtestproject")
	assertNoOutput(t, errOut)
	if !s.exists("tarballtestproject/run.go") {
		t.Error("tarballtestproject/run.go was not created")
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_from_tarball_to_alternative_location
func TestStartProjectCustomProjectTemplateFromTarballToAlternativeLocation(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("altlocation"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startproject", "--template", customTemplate("project_template.tgz"),
		"tarballtestproject", "altlocation")
	assertNoOutput(t, errOut)
	if !s.exists("altlocation/run.go") {
		t.Error("altlocation/run.go was not created")
	}
}

// templateServer serves the custom templates over HTTP, the live server of
// Django's StartProject tests, and records the User-Agent header of the last
// request.
type templateServer struct {
	url       string
	userAgent string
}

// newTemplateServer serves the files of the custom templates under
// /custom_templates/ and /user_agent_check/. A trailing slash after the name
// of a file is ignored.
func newTemplateServer(t *testing.T) *templateServer {
	t.Helper()
	ts := &templateServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.userAgent = r.Header.Get("User-Agent")
		rel := strings.TrimSuffix(r.URL.Path, "/")
		for _, prefix := range []string{"/custom_templates/", "/user_agent_check/"} {
			if name, ok := strings.CutPrefix(rel, prefix); ok {
				http.ServeFile(w, r, customTemplate(name))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	ts.url = srv.URL
	return ts
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_from_tarball_by_url
func TestStartProjectCustomProjectTemplateFromTarballByURL(t *testing.T) {
	ts := newTemplateServer(t)
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", ts.url+"/custom_templates/project_template.tgz", "urltestproject")
	assertNoOutput(t, errOut)
	if !s.exists("urltestproject/run.go") {
		t.Error("urltestproject/run.go was not created")
	}
}

// TestStartProjectCustomProjectTemplateFromTarballByURLUserAgent names
// Djan-Go-Go and its version as the user agent, as Django names itself.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_from_tarball_by_url_django_user_agent
func TestStartProjectCustomProjectTemplateFromTarballByURLUserAgent(t *testing.T) {
	ts := newTemplateServer(t)
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", ts.url+"/user_agent_check/project_template.tgz", "urltestproject")
	assertNoOutput(t, errOut)
	assertOutput(t, ts.userAgent, "Djan-Go-Go/"+djangogo.Version, true)
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_project_template_tarball_url
func TestStartProjectProjectTemplateTarballURL(t *testing.T) {
	ts := newTemplateServer(t)
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", ts.url+"/custom_templates/project_template.tgz/", "urltestproject")
	assertNoOutput(t, errOut)
	if !s.exists("urltestproject/run.go") {
		t.Error("urltestproject/run.go was not created")
	}
}

// TestStartProjectFileWithoutExtension renders the files -e and -n select
// besides the Go files. The Go file of the fixture holds a Go comment where
// the others hold a shell comment.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_file_without_extension
func TestStartProjectFileWithoutExtension(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "customtestproject",
		"-e", "txt", "-n", "Procfile")
	assertNoOutput(t, errOut)
	for name, comment := range map[string]string{"Procfile": "#", "additional_file.go": "//", "requirements.txt": "#"} {
		got := strings.TrimSpace(s.read("customtestproject/additional_dir/" + name))
		if want := comment + " some file for customtestproject test project"; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_context_variables
func TestStartProjectCustomProjectTemplateContextVariables(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "another_project", "project_dir")
	assertNoOutput(t, errOut)
	content := s.read("project_dir/main.go")
	assertOutput(t, content, `projectName = "another_project"`, true)
	assertOutput(t, content, `projectDirectory = "`+s.path("project_dir")+`"`, true)
}

// TestStartProjectNoEscapingOfProjectVariables renders the values without
// HTML escaping. Django passes "<&>" through the --extra option of a custom
// command; Djan-Go-Go has no way to extend startproject, so the port puts
// the characters into the directory name.
//
// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_no_escaping_of_project_variables
func TestStartProjectNoEscapingOfProjectVariables(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir<&>"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "another_project", "project_dir<&>")
	assertNoOutput(t, errOut)
	assertOutput(t, s.read("project_dir<&>/main.go"), "<&>", true)
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_destination_missing
func TestStartProjectCustomProjectDestinationMissing(t *testing.T) {
	s := newAdminScript(t)
	out, errOut := s.run("startproject", "--template", customTemplate("project_template"),
		"yet_another_project", "project_dir2")
	assertNoOutput(t, out)
	assertNoOutput(t, errOut)
	if !s.exists("project_dir2") {
		t.Error("project_dir2 was not created")
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_with_non_ascii_templates
func TestStartProjectCustomProjectTemplateWithNonASCIITemplates(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), "--extension=txt",
		"customtestproject")
	assertNoOutput(t, errOut)
	got := s.read("customtestproject/ticket-18091-non-ascii-template.txt")
	if want := "Some non-ASCII text for testing ticket #18091:\nüäö €\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_hidden_directory_default_excluded
func TestStartProjectCustomProjectTemplateHiddenDirectoryDefaultExcluded(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"),
		"custom_project_template_hidden_directories", "project_dir")
	assertNoOutput(t, errOut)
	if s.exists("project_dir/.hidden") {
		t.Error("project_dir/.hidden was created")
	}
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_hidden_directory_included
func TestStartProjectCustomProjectTemplateHiddenDirectoryIncluded(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	const projectName = "custom_project_template_hidden_directories_included"
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), projectName,
		"project_dir", "--exclude")
	assertNoOutput(t, errOut)
	assertOutput(t, s.read("project_dir/.hidden/render.go"), "// The "+projectName+" should be rendered.", true)
}

// Ported from Django: tests/admin_scripts/tests.py::StartProject.test_custom_project_template_exclude_directory
func TestStartProjectCustomProjectTemplateExcludeDirectory(t *testing.T) {
	s := newAdminScript(t)
	if err := os.Mkdir(s.path("project_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	const projectName = "custom_project_with_excluded_directories"
	_, errOut := s.run("startproject", "--template", customTemplate("project_template"), projectName,
		"project_dir", "--exclude", "additional_dir", "-x", ".hidden")
	assertNoOutput(t, errOut)
	for _, dir := range []string{".hidden", "additional_dir", ".git", "__pycache__"} {
		if s.exists("project_dir/" + dir) {
			t.Errorf("project_dir/%s was created", dir)
		}
	}
	if !s.exists("project_dir/" + projectName) {
		t.Errorf("project_dir/%s was not created", projectName)
	}
}

// No Go counterpart: StartProject.test_command_does_not_import. The name
// check looks for a directory of the standard library and imports nothing.

// No Go counterpart: StartProject.test_failure_to_format_code. The command
// formats the Go files itself and runs no formatter command.

// TestStartAppSetupEnviron names the app configuration after the app.
// Settings live in a Go value, so the settings directory of the Django test
// class has no counterpart; the startapp part is ported.
//
// Ported from Django: tests/admin_scripts/tests.py::DjangoAdminSettingsDirectory.test_setup_environ
func TestStartAppSetupEnviron(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "settings_test")
	assertNoOutput(t, errOut)
	content := s.read("settings_test/app.go")
	assertOutput(t, content, "type App struct {", true)
	assertOutput(t, content, `func (*App) Name() string { return "settings_test" }`, true)
}

// Ported from Django: tests/admin_scripts/tests.py::DjangoAdminSettingsDirectory.test_setup_environ_custom_template
func TestStartAppSetupEnvironCustomTemplate(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "--template", customTemplate("app_template"), "custom_settings_test")
	assertNoOutput(t, errOut)
	if !s.exists("custom_settings_test/api.go") {
		t.Error("custom_settings_test/api.go was not created")
	}
}

// TestStartAppUnicodeName accepts a name of letters outside ASCII, which Go
// identifiers allow as Python's do.
//
// Ported from Django: tests/admin_scripts/tests.py::DjangoAdminSettingsDirectory.test_startapp_unicode_name
func TestStartAppUnicodeName(t *testing.T) {
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "こんにちは")
	assertNoOutput(t, errOut)
	content := s.read("こんにちは/app.go")
	assertOutput(t, content, "package こんにちは\n", true)
	assertOutput(t, content, `func (*App) Name() string { return "こんにちは" }`, true)
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

// TestTemplateCommandVerbosityOfCustomTemplate prints the files to render,
// the download, the extraction and the cleanup at verbosity 2, as
// TemplateCommand.handle does, and removes the temporary files.
func TestTemplateCommandVerbosityOfCustomTemplate(t *testing.T) {
	ts := newTemplateServer(t)
	s := newAdminScript(t)
	url := ts.url + "/custom_templates/project_template.tgz"
	out, errOut := s.run("startproject", "--template", url, "urltestproject", "-v", "2",
		"-e", "txt, html", "-e", ".txt", "-n", "Procfile,settings.go")
	assertNoOutput(t, errOut)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("output:\n%s", out)
	}
	for i, want := range []string{
		"Rendering project template files with extensions: .go, .html, .txt",
		"Rendering project template files with filenames: go.mod, Procfile, settings.go",
		"Downloading " + url,
		"Extracting ",
		"Creating " + s.path("urltestproject/run.go"),
		"Cleaning up temporary files.",
	} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want %q", i+1, lines[i], want)
		}
	}
	archive := strings.TrimPrefix(lines[3], "Extracting ")
	if _, err := os.Stat(filepath.Dir(archive)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the download directory %s was not removed: %v", filepath.Dir(archive), err)
	}
}

// TestTemplateCommandTemplateErrors reports a template that is neither a
// directory nor an archive, a failed download and an archive of an unknown
// format with Django's messages.
func TestTemplateCommandTemplateErrors(t *testing.T) {
	ts := newTemplateServer(t)
	s := newAdminScript(t)

	_, errOut := s.run("startproject", "--template", "missing", "one")
	assertOutput(t, errOut, "CommandError: couldn't handle project template missing.", true)

	url := ts.url + "/custom_templates/missing.tgz"
	_, errOut = s.run("startproject", "--template", url, "two")
	assertOutput(t, errOut, "CommandError: couldn't download URL "+url+" to missing.tgz: HTTP Error 404: Not Found", true)

	_, errOut = s.run("startapp", "--template", customTemplate("project_template/additional_dir/Procfile"), "three")
	assertOutput(t, errOut, "CommandError: couldn't extract file "+customTemplate("project_template/additional_dir/Procfile")+
		" to ", true)
	assertOutput(t, errOut, ": Path not a recognized archive format: ", true)
}

// TestTemplateCommandTemplatePathForms accepts a file:// URL and a path in
// the home directory.
func TestTemplateCommandTemplatePathForms(t *testing.T) {
	home := filepath.Dir(customTemplatesDir)
	t.Setenv("HOME", home)
	s := newAdminScript(t)
	for name, template := range map[string]string{
		"fileurl": "file://" + customTemplate("app_template"),
		"home":    "~/" + filepath.Base(customTemplatesDir) + "/app_template",
	} {
		_, errOut := s.run("startapp", "--template", template, name)
		assertNoOutput(t, errOut)
		if !s.exists(name + "/api.go") {
			t.Errorf("%s/api.go was not created", name)
		}
	}
}

// TestTemplateCommandContext renders Django's context names in a custom
// tree, and refuses a name the context lacks.
func TestTemplateCommandContext(t *testing.T) {
	tree := t.TempDir()
	const context = "[[ .app_name ]] [[ .camel_case_app_name ]] [[ .app_directory ]] [[ .djangogo_version ]] " +
		"[[ .go_version ]] [[ .extensions ]] [[ .files ]] [[ .verbosity ]] [[ .exclude ]]\n"
	if err := os.WriteFile(filepath.Join(tree, "context.txt"), []byte(context), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newAdminScript(t)
	_, errOut := s.run("startapp", "--template", tree, "my_blog2go", "-n", "context.txt", "-x", "vendor")
	assertNoOutput(t, errOut)
	want := "my_blog2go MyBlog2Go " + s.path("my_blog2go") + " " + djangogo.Version + " " + scaffold.GoVersion +
		" [.go] [go.mod context.txt] 1 [vendor]\n"
	if got := s.read("my_blog2go/context.txt"); got != want {
		t.Errorf("context.txt = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(tree, "context.txt"), []byte("[[ .secret_key ]]"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut = s.run("startapp", "--template", tree, "other", "-n", "context.txt")
	assertOutput(t, errOut, `map has no entry for key "secret_key"`, true)
	_, errOut = s.run("startproject", "--template", tree, "proj", "-n", "context.txt")
	assertNoOutput(t, errOut)
	assertOutput(t, s.read("proj/context.txt"), "django-insecure-", true)
}
