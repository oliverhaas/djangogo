package djangogo_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
)

// get requests path from the server at addr and returns the status and body.
func get(t *testing.T, addr net.Addr, path string) (status int, body string) {
	t.Helper()
	resp, err := http.Get("http://" + addr.String() + path)
	if err != nil {
		t.Errorf("GET %s: %v", path, err)
		return 0, ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("read body of %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

// TestRunserverInvalidAddrport checks the errors for a malformed addrport and
// for an IPv4 address with --ipv6.
func TestRunserverInvalidAddrport(t *testing.T) {
	app := plainRunserverApp(t)
	for _, tc := range []struct {
		addrport string
		ipv6     bool
		want     string
	}{
		{"abc:xyz", false, `"abc:xyz" is not a valid port number or address:port pair.`},
		{"1.2.3.4:", false, `"1.2.3.4:" is not a valid port number or address:port pair.`},
		{"1.2.3.4:8000", true, `"1.2.3.4" is not a valid IPv6 address.`},
	} {
		_, _, err := djangogo.RunserverAddr(app, tc.addrport, tc.ipv6)
		if err == nil || err.Error() != tc.want {
			t.Errorf("RunserverAddr(%q, %v) error = %v, want %q", tc.addrport, tc.ipv6, err, tc.want)
		}
	}
}

// TestRunserverTooManyArguments checks that two addrport arguments are a
// usage error.
func TestRunserverTooManyArguments(t *testing.T) {
	app, _, _ := runserverApp(t, conf.Settings{Debug: true})
	if code := app.Run([]string{"runserver", "1", "2"}); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	var usage *manage.UsageError
	if err := app.Execute([]string{"runserver", "1", "2"}); !errors.As(err, &usage) {
		t.Errorf("error = %v, want a *manage.UsageError", err)
	}
}

// requestLogRe is the format of the request log line of Django's server.
var requestLogRe = regexp.MustCompile(`^\[\d{2}/\w{3}/\d{4} \d{2}:\d{2}:\d{2}\] "GET / HTTP/1.1" 200 \d+\n$`)

// TestRunserverAnswersAndLogsRequest checks that a running server answers a
// request and writes Django's request log line to Err.
func TestRunserverAnswersAndLogsRequest(t *testing.T) {
	app, _, errOut := runserverApp(t, conf.Settings{AllowedHosts: []string{"127.0.0.1"}})
	var status int
	var body string
	err := serveRunserver(t, app, func(addr net.Addr) { status, body = get(t, addr, "/") }, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("runserver: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(body, "is running") {
		t.Errorf("GET / = %d %q, want 200 and \"is running\"", status, body)
	}
	if !requestLogRe.MatchString(errOut.String()) {
		t.Errorf("Err = %q, want a request log line", errOut.String())
	}
}

// staticApp returns an application that serves a file css/app.css from a
// static directory, and the content of the file.
func staticApp(t *testing.T, s conf.Settings) (*djangogo.Application, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	const content = "body { color: red; }\n"
	if err := os.WriteFile(filepath.Join(dir, "css", "app.css"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s.StaticFilesDirs = []string{dir}
	app, _, _ := runserverApp(t, s)
	return app, content
}

// TestRunserverServesStaticFiles checks that static files are served under
// StaticURL with Debug on, not with Debug off unless --insecure is given, and
// not with --nostatic. The project without routes answers every other path
// with its liveness line, so the check is on the file content.
func TestRunserverServesStaticFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		debug  bool
		args   []string
		served bool
	}{
		{"debug", true, nil, true},
		{"production", false, nil, false},
		{"insecure", false, []string{"--insecure"}, true},
		{"nostatic", true, []string{"--nostatic"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, content := staticApp(t, conf.Settings{Debug: tc.debug, AllowedHosts: []string{"127.0.0.1"}})
			var status int
			var body string
			args := append(tc.args, "127.0.0.1:0")
			err := serveRunserver(t, app, func(addr net.Addr) { status, body = get(t, addr, "/static/css/app.css") }, args...)
			if err != nil {
				t.Fatalf("runserver: %v", err)
			}
			if tc.served && (status != http.StatusOK || body != content) {
				t.Errorf("GET = %d %q, want 200 %q", status, body, content)
			}
			// The default handler answers every GET path, so "not served"
			// means the file content does not come back, not a 404.
			if !tc.served && body == content {
				t.Errorf("GET = %d %q, want the file not to be served", status, body)
			}
		})
	}
}

// TestRunserverAddressInUse checks the error for a port that is taken.
func TestRunserverAddressInUse(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	app, out, errOut := runserverApp(t, conf.Settings{Debug: true})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	djangogo.SetRunserverHooks(ctx, app, func(net.Addr) { t.Error("the server listened on a taken port") }, nil)
	if code := app.Run([]string{"runserver", ln.Addr().String()}); code != 1 {
		t.Errorf("exit code = %d, want 1 (Out %q)", code, out.String())
	}
	if want := "Error: That port is already in use.\n"; errOut.String() != want {
		t.Errorf("Err = %q, want %q", errOut.String(), want)
	}
}

// TestRunserverGracefulShutdown checks that cancelling the context stops the
// server and Execute returns nil.
func TestRunserverGracefulShutdown(t *testing.T) {
	app, _, _ := runserverApp(t, conf.Settings{Debug: true})
	if err := serveRunserver(t, app, nil, "127.0.0.1:0"); err != nil {
		t.Errorf("runserver error = %v, want nil", err)
	}
}

// TestRunserverBannerTime checks that the first line after the check output
// is the time.
func TestRunserverBannerTime(t *testing.T) {
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	if err := serveRunserver(t, app, nil, "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver: %v", err)
	}
	_, banner, ok := strings.Cut(out.String(), "Performing system checks...\n\n")
	if !ok {
		t.Fatalf("Out = %q, want the check output", out.String())
	}
	first, _, _ := strings.Cut(banner, "\n")
	// The checks print their summary before the banner; skip to the time.
	if first != "November 18, 2026 - 09:05:07" && !strings.Contains(banner, "\nNovember 18, 2026 - 09:05:07\n") {
		t.Errorf("banner = %q, want the line %q", banner, "November 18, 2026 - 09:05:07")
	}
}
