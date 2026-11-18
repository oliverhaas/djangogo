package views_test

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/templates"
)

// memEngine returns an engine over in-memory templates, keyed by template
// name (which may contain directories).
func memEngine(t *testing.T, files map[string]string) *templates.Engine {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	eng, err := templates.NewEngineFS(fsys)
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	return eng
}

// requestProcessor is a context processor that exposes the request path as
// request.path, the part of Django's request context processor the ported
// tests read.
func requestProcessor(r *http.Request) map[string]any {
	return map[string]any{"request": map[string]any{"path": r.URL.Path}}
}

// logRecords collects slog records, the counterpart of assertLogs.
type logRecords struct {
	mu      sync.Mutex
	records []slog.Record
}

func (l *logRecords) Enabled(context.Context, slog.Level) bool { return true }

func (l *logRecords) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, r)
	return nil
}

func (l *logRecords) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l *logRecords) WithGroup(string) slog.Handler { return l }

// messages returns the logged messages at the given level.
func (l *logRecords) messages(level slog.Level) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, r := range l.records {
		if r.Level == level {
			out = append(out, r.Message)
		}
	}
	return out
}

// statuses returns the "status" attribute of the records logged at the given
// level, the status_code Django attaches to its request log records.
func (l *logRecords) statuses(level slog.Level) []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []int64
	for _, r := range l.records {
		if r.Level != level {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "status" && a.Value.Kind() == slog.KindInt64 {
				out = append(out, a.Value.Int64())
			}
			return true
		})
	}
	return out
}

// errs returns the errors attached to the records logged at error level: the
// causes of the 500 responses, which Django raises out of the test client.
func (l *logRecords) errs() []error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []error
	for _, r := range l.records {
		if r.Level != slog.LevelError {
			continue
		}
		r.Attrs(func(a slog.Attr) bool {
			if err, ok := a.Value.Any().(error); ok && a.Key == "error" {
				out = append(out, err)
			}
			return true
		})
	}
	return out
}

// captureLogs routes the default slog logger into the returned collector for
// the duration of the test. Tests using it must not run in parallel.
func captureLogs(t *testing.T) *logRecords {
	t.Helper()
	previous := slog.Default()
	rec := &logRecords{}
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return rec
}
