// application_stub.go
// TEMPORARY: a minimal Application so the runserver command compiles and its
// tests pass for the Task 7 commit. Task 8 deletes this file and replaces it
// with the real Application in application.go. (Plan-sanctioned strict-per-task-green stub.)
package djangogo

import (
	"io"
	"net/http"

	"github.com/oliverhaas/djangogo/conf"
)

// Application stub with only the fields runserver.go reads.
type Application struct {
	Settings *conf.Settings
	Handler  http.Handler
	Out      io.Writer
}
