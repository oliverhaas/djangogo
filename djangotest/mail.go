package djangotest

import (
	"testing"

	"github.com/oliverhaas/djangogo/mail"
)

// LocmemMail makes the mail of the rest of the test go to the outbox of the
// locmem backend and empties the outbox, as Django's test runner does for
// every test. Read the messages with mail.Outbox. The previous email
// settings come back and the outbox is emptied when the test ends.
//
// djangogo.New applies the email settings of the project, so call
// LocmemMail after New. The outbox is global to the process: a test that
// uses it must not run in parallel with another one that sends mail.
func LocmemMail(t testing.TB) {
	t.Helper()
	cfg := mail.CurrentConfig()
	cfg.Backend = "locmem"
	restore := mail.Configure(cfg)
	mail.ClearOutbox()
	t.Cleanup(func() {
		restore()
		mail.ClearOutbox()
	})
}
