package djangogo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// defaultHandler is the placeholder root handler served by runserver until the
// urls/views layer lands (Milestone 5). It answers GET / with a liveness line.
func defaultHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "Djan-Go-Go %s is running.\n", Version)
	})
	return mux
}

// runserverCommand starts the development HTTP server with graceful shutdown.
type runserverCommand struct {
	app *Application
}

func (*runserverCommand) Name() string { return "runserver" }
func (*runserverCommand) Help() string { return "Start the development HTTP server" }

func (c *runserverCommand) Run(_ []string) error {
	addr := c.app.Settings.Host + ":" + c.app.Settings.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           c.app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		_, _ = fmt.Fprintf(c.app.Out, "Djan-Go-Go development server at http://%s/  (Ctrl-C to quit)\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
