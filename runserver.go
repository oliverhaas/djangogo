package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oliverhaas/djangogo/internal/autoreload"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/middleware"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// Timeouts of the development server. They bound a slow or stalled client
// without cutting off a request that is merely large or slow to handle.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = time.Minute
	writeTimeout      = 2 * time.Minute
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 5 * time.Second
)

// naiveIPRe is Django's naiveip_re: an optional address (an IPv4 address, a
// bracketed IPv6 address or a host name) followed by a colon, and a port.
var naiveIPRe = regexp.MustCompile(`^(?:(?P<addr>(?P<ipv4>\d{1,3}(?:\.\d{1,3}){3})|` +
	`(?P<ipv6>\[[a-fA-F0-9:]+\])|(?P<fqdn>[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)*)):)?(?P<port>\d+)$`)

// defaultHandler is the root handler of a project without routes. It answers
// GET / with a liveness line, so a new project shows it is running, and any
// other path with 404.
func defaultHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "Djan-Go-Go %s is running.\n", Version)
	})
	return mux
}

// runserverCommand is Django's runserver: it runs the system checks, warns
// about unapplied migrations and serves the project over HTTP until SIGINT
// or SIGTERM, with the request log of Django's development server and, with
// Debug on, the static files under StaticURL.
//
// Unless --noreload is given, the server runs under the auto-reloader of
// package autoreload: this process starts the server in a child process,
// builds the program again when a source file changes and restarts the child
// with the new binary. When the program cannot be built again, for example
// without the go command, a notice says so and the server runs in this
// process. --nothreading is accepted and does nothing, as a Go server does
// not run on one thread. Without an addrport argument the server listens on
// Settings.Host and Settings.Port, where Django takes 127.0.0.1:8000.
type runserverCommand struct {
	app *Application

	useIPv6, noThreading, noReload, noStatic, insecure, skipChecks bool

	// ctx, when set, stops the server when it is done, as a signal does.
	ctx context.Context
	// ready, when set, is called with the address the server listens on
	// once it serves. The server stops only after ready returns.
	ready func(addr net.Addr)
	// now, when set, gives the time of the banner.
	now func() time.Time
	// newSupervisor, when set, replaces autoreload.NewSupervisor, so that a
	// test binary can run the reloader.
	newSupervisor func() (*autoreload.Supervisor, error)
}

// Name returns the command name.
func (*runserverCommand) Name() string { return "runserver" }

// Help returns the help text.
func (*runserverCommand) Help() string { return "Starts a lightweight web server for development." }

// Usage names the positional argument.
func (*runserverCommand) Usage() string { return "[addrport]" }

// SkipSystemChecks reports that the registry runs no checks before
// runserver: the command runs them itself and shows the result, as Django's
// runserver does after "Performing system checks...".
func (*runserverCommand) SkipSystemChecks() bool { return true }

// Flags declares Django's options of runserver, those of the staticfiles
// variant and --skip-checks.
func (c *runserverCommand) Flags(fs *flag.FlagSet) {
	const ipv6Help = "Tells Djan-Go-Go to use an IPv6 address."
	fs.BoolVar(&c.useIPv6, "ipv6", false, ipv6Help)
	fs.BoolVar(&c.useIPv6, "6", false, ipv6Help)
	fs.BoolVar(&c.noThreading, "nothreading", false, "Accepted for compatibility; does nothing.")
	fs.BoolVar(&c.noReload, "noreload", false, "Tells Djan-Go-Go to NOT use the auto-reloader.")
	fs.BoolVar(&c.noStatic, "nostatic", false,
		"Tells Djan-Go-Go to NOT automatically serve static files at STATIC_URL.")
	fs.BoolVar(&c.insecure, "insecure", false, "Allows serving static files even if DEBUG is False.")
	fs.BoolVar(&c.skipChecks, "skip-checks", false, "Skip system checks.")
}

// Run validates the address, starts the auto-reloader or runs the checks, and
// serves until stopped.
func (c *runserverCommand) Run(args []string) error {
	a := c.app
	if len(args) > 1 {
		return &manage.UsageError{Msg: "unrecognized arguments: " + strings.Join(args[1:], " ")}
	}
	if !a.Settings.Debug && len(a.Settings.AllowedHosts) == 0 {
		return manage.Errorf("You must set settings.ALLOWED_HOSTS if DEBUG is False.")
	}
	addrport := ""
	if len(args) == 1 {
		addrport = args[0]
	}
	host, port, err := c.listenAddr(addrport)
	if err != nil {
		return err
	}

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !c.noReload && !autoreload.IsChild() {
		if reloaded, err := c.reload(ctx); reloaded {
			return err
		}
	}

	if !c.skipChecks {
		_, _ = fmt.Fprint(a.out(), "Performing system checks...\n\n")
		if err := a.runChecks(checkOptions{displayNumErrors: true}); err != nil {
			return err
		}
	}
	c.checkMigrations(ctx)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return listenError(err)
	}
	srv := &http.Server{
		Handler:           c.handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	c.printBanner(host, ln.Addr())
	return serve(ctx, srv, ln, func() {
		if c.ready != nil {
			c.ready(ln.Addr())
		}
	})
}

// reload runs the server under the auto-reloader until ctx is done and
// reports whether it did (Django's run_with_reloader). It returns false after
// a notice when the program cannot be built again here; the caller then
// serves in this process. A test binary serves in its process without the
// notice.
func (c *runserverCommand) reload(ctx context.Context) (bool, error) {
	newSupervisor := autoreload.NewSupervisor
	if c.newSupervisor != nil {
		newSupervisor = c.newSupervisor
	}
	var unavailable *autoreload.UnavailableError
	s, err := newSupervisor()
	if err == nil {
		s.Out = c.app.errOut()
		err = s.Run(ctx)
	}
	switch {
	case errors.Is(err, autoreload.ErrTestBinary):
		return false, nil
	case errors.As(err, &unavailable):
		_, _ = fmt.Fprintf(c.app.errOut(), "Autoreload is off: %v.\n", err)
		return false, nil
	}
	return true, err
}

// listenAddr resolves the addrport argument as Django's runserver does and
// returns the host and the port to listen on.
func (c *runserverCommand) listenAddr(addrport string) (host, port string, err error) {
	s := c.app.Settings
	useIPv6 := c.useIPv6
	port = s.Port
	if addrport != "" {
		m := naiveIPRe.FindStringSubmatch(addrport)
		if m == nil {
			return "", "", manage.Errorf("\"%s\" is not a valid port number or address:port pair.", addrport)
		}
		addr, ipv6, fqdn := m[naiveIPRe.SubexpIndex("addr")], m[naiveIPRe.SubexpIndex("ipv6")], m[naiveIPRe.SubexpIndex("fqdn")]
		host, port = addr, m[naiveIPRe.SubexpIndex("port")]
		switch {
		case ipv6 != "":
			host = strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
		case addr != "" && useIPv6 && fqdn == "":
			return "", "", manage.Errorf("\"%s\" is not a valid IPv6 address.", addr)
		}
	}
	switch {
	case host == "" && useIPv6:
		host = "::1"
	case host == "":
		host = s.Host
	case host == "0":
		// Python reads the address "0" as 0.0.0.0; Go would look it up as a
		// host name.
		host = "0.0.0.0"
	}
	return host, port, nil
}

// handler returns the handler runserver serves: the project's, with the
// static files in front of it when Debug is on (or with --insecure) and
// StaticURL is a path, as Django's staticfiles runserver does, and the
// request log around everything.
func (c *runserverCommand) handler() http.Handler {
	a := c.app
	h := a.Handler
	serveStatic := !c.noStatic && (a.Settings.Debug || c.insecure)
	if serveStatic && strings.HasPrefix(a.Settings.StaticURL, "/") && a.Static != nil {
		h = staticfiles.Middleware(a.Settings.StaticURL, a.Static)(h)
	}
	return middleware.RequestLog(middleware.RequestLogOptions{Out: a.errOut()})(h)
}

// productionWarning is the line runserver prints after its banner, as
// Django's does, unless DJANGOGO_RUNSERVER_HIDE_WARNING is "true".
const productionWarning = "WARNING: This is a development server. Do not use it in a production setting. " +
	"Serve Application.Handler from your own http.Server instead.\n"

// printBanner prints Django's startup lines (on_bind): the time, the
// version, the address and how to quit, then the production warning.
func (c *runserverCommand) printBanner(host string, addr net.Addr) {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	shown := host
	if strings.Contains(host, ":") {
		shown = "[" + host + "]"
	}
	port := ""
	if tcp, ok := addr.(*net.TCPAddr); ok {
		port = strconv.Itoa(tcp.Port)
	}
	quit := "CONTROL-C"
	if runtime.GOOS == "windows" {
		quit = "CTRL-BREAK"
	}
	out := c.app.out()
	_, _ = fmt.Fprintf(out, "%s\nDjan-Go-Go version %s\nStarting development server at http://%s:%s/\n"+
		"Quit the server with %s.\n",
		now().Format("January 02, 2006 - 15:04:05"), Version, shown, port, quit)
	if os.Getenv("DJANGOGO_RUNSERVER_HIDE_WARNING") != "true" {
		_, _ = io.WriteString(out, productionWarning)
	}
}

// checkMigrations prints Django's notice about unapplied migrations. It
// prints nothing without a database or when the database cannot be reached;
// a reachable database without the table of applied migrations has none
// applied.
func (c *runserverCommand) checkMigrations(ctx context.Context) {
	a := c.app
	if a.DB == nil || len(a.Migrations.All()) == 0 {
		return
	}
	// Django's check_migrations: the plan from the applied state to the leaf
	// node of every app.
	executor, err := migrations.NewExecutor(ctx, a.DB, a.Migrations, a.Apps.Names())
	if err != nil {
		return
	}
	plan, err := executor.MigrationPlan(executor.Loader.Graph.LeafNodes(), false)
	if err != nil || len(plan) == 0 {
		return
	}
	pending := len(plan)
	var waiting []string
	for _, step := range plan {
		if !slices.Contains(waiting, step.Migration.App) {
			waiting = append(waiting, step.Migration.App)
		}
	}
	slices.Sort(waiting)
	_, _ = fmt.Fprintf(a.out(), "\nYou have %d unapplied migration(s). Your project may not work properly "+
		"until you apply the migrations for app(s): %s.\nRun '%s migrate' to apply them.\n",
		pending, strings.Join(waiting, ", "), a.prog())
}

// serve serves on ln until ctx is done, then shuts the server down
// gracefully: open requests get shutdownTimeout to finish. It calls started
// once the server runs.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, started func()) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	started()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("djangogo: shut down server: %w", err)
		}
		return nil
	}
}

// serverError is an error of the server socket, printed as Django's
// runserver prints an OSError: "Error: <text>".
type serverError struct {
	text string
	err  error
}

// Error returns the text.
func (e *serverError) Error() string { return e.text }

// Unwrap returns the error of the socket.
func (e *serverError) Unwrap() error { return e.err }

// Formatted returns the line runserver prints.
func (e *serverError) Formatted() string { return "Error: " + e.text + "\n" }

// ExitCode returns 1, as Django's runserver exits after a socket error. A
// server process of the auto-reloader returns autoreload.StopCode, which
// tells the reloader to exit with 1 as well instead of waiting for a change.
func (e *serverError) ExitCode() int {
	if autoreload.IsChild() {
		return autoreload.StopCode
	}
	return 1
}

// listenError words a failure to listen with Django's texts for the common
// causes.
func listenError(err error) error {
	text := err.Error()
	switch {
	case errors.Is(err, syscall.EACCES):
		text = "You don't have permission to access that port."
	case errors.Is(err, syscall.EADDRINUSE):
		text = "That port is already in use."
	case errors.Is(err, syscall.EADDRNOTAVAIL):
		text = "That IP address can't be assigned to."
	}
	return &serverError{text: text, err: err}
}
