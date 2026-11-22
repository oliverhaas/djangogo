package djangogo_test

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/internal/autoreload"
	"github.com/oliverhaas/djangogo/manage"
)

// refuseSupervisor makes runserver of app fail the test when it starts the
// auto-reloader.
func refuseSupervisor(t *testing.T, app *djangogo.Application) {
	t.Helper()
	djangogo.SetRunserverSupervisor(app, func() (*autoreload.Supervisor, error) {
		t.Error("runserver started the auto-reloader")
		return nil, &autoreload.UnavailableError{Err: errors.New("refused by the test")}
	})
}

// TestRunserverUsesTheReloader runs the server in a child process of the
// auto-reloader, without the checks in this process. No Django test exists:
// Django's tests call runserver with use_reloader=False.
func TestRunserverUsesTheReloader(t *testing.T) {
	trueCmd, err := exec.LookPath("true")
	if err != nil {
		t.Skip("there is no true command")
	}
	app, out, errOut := runserverApp(t, conf.Settings{Debug: true})
	var started []string
	djangogo.SetRunserverSupervisor(app, func() (*autoreload.Supervisor, error) {
		return &autoreload.Supervisor{
			Exe:      "site",
			Reloader: &autoreload.StatReloader{Files: func() []string { return nil }},
			// The child exits with status 0, as a server does after an
			// interrupt.
			Command: func(exe string) *exec.Cmd {
				started = append(started, exe)
				return exec.Command(trueCmd)
			},
		}, nil
	})
	if err := app.Execute([]string{"runserver", "127.0.0.1:0"}); err != nil {
		t.Fatalf("runserver = %v, want nil", err)
	}
	if len(started) != 1 || started[0] != "site" {
		t.Errorf("started %q, want one child of site", started)
	}
	if out.String() != "" {
		t.Errorf("Out = %q, want nothing: the checks and the banner come from the child", out.String())
	}
	if want := "Watching for file changes with StatReloader\n"; errOut.String() != want {
		t.Errorf("Err = %q, want %q", errOut.String(), want)
	}
}

// TestRunserverWithoutReloader serves in the process with a notice when the
// program cannot be built again. No Django test exists: Python needs no
// build.
func TestRunserverWithoutReloader(t *testing.T) {
	app, out, errOut := runserverApp(t, conf.Settings{Debug: true})
	djangogo.SetRunserverSupervisor(app, func() (*autoreload.Supervisor, error) {
		return nil, &autoreload.UnavailableError{Err: errors.New("the go command is missing")}
	})
	served := false
	if err := serveRunserver(t, app, func(net.Addr) { served = true }, "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver = %v, want nil", err)
	}
	if !served {
		t.Error("runserver did not serve")
	}
	if want := "Autoreload is off: the go command is missing.\n"; errOut.String() != want {
		t.Errorf("Err = %q, want %q", errOut.String(), want)
	}
	if !strings.HasPrefix(out.String(), "Performing system checks...") {
		t.Errorf("Out = %q, want the checks of this process", out.String())
	}
}

// TestRunserverNoReload serves in the process with --noreload. No Django
// test exists: Django's tests pass use_reloader=False to call_command, which
// is what --noreload sets.
func TestRunserverNoReload(t *testing.T) {
	app, _, errOut := runserverApp(t, conf.Settings{Debug: true})
	refuseSupervisor(t, app)
	served := false
	if err := serveRunserver(t, app, func(net.Addr) { served = true }, "--noreload", "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver = %v, want nil", err)
	}
	if !served {
		t.Error("runserver did not serve")
	}
	if errOut.String() != "" {
		t.Errorf("Err = %q, want nothing", errOut.String())
	}
}

// Ported from Django: tests/utils_tests/test_autoreload.py::RunWithReloaderTests.test_calls_start_django
func TestRunWithReloaderCallsStartDjango(t *testing.T) {
	// The server process of the reloader serves instead of starting another
	// reloader.
	t.Setenv(autoreload.EnvVar, "true")
	app, _, _ := runserverApp(t, conf.Settings{Debug: true})
	refuseSupervisor(t, app)
	served := false
	if err := serveRunserver(t, app, func(net.Addr) { served = true }, "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver = %v, want nil", err)
	}
	if !served {
		t.Error("runserver did not serve")
	}
}

// TestRunserverChildAddressInUse exits with autoreload.StopCode when the
// server process of the reloader cannot listen, so that the reloader exits
// too. No Django test exists: Django's server process calls os._exit(1),
// which ends the reloader with it.
func TestRunserverChildAddressInUse(t *testing.T) {
	t.Setenv(autoreload.EnvVar, "true")
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	app, _, errOut := runserverApp(t, conf.Settings{Debug: true})
	refuseSupervisor(t, app)
	if code := app.Run([]string{"runserver", ln.Addr().String()}); code != autoreload.StopCode {
		t.Errorf("exit code = %d, want %d", code, autoreload.StopCode)
	}
	if want := "Error: That port is already in use.\n"; errOut.String() != want {
		t.Errorf("Err = %q, want %q", errOut.String(), want)
	}
	err = app.Execute([]string{"runserver", ln.Addr().String()})
	if code := manage.ExitCode(err); code != autoreload.StopCode {
		t.Errorf("ExitCode(%v) = %d, want %d", err, code, autoreload.StopCode)
	}
}
