// runserver_test.go
package djangogo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultHandlerServesOK(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	defaultHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Djan-Go-Go") {
		t.Errorf("body = %q, want it to mention Djan-Go-Go", rec.Body.String())
	}
}

func TestRunserverMetadata(t *testing.T) {
	c := &runserverCommand{}
	if c.Name() != "runserver" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Help() == "" {
		t.Error("Help() should be non-empty")
	}
}
