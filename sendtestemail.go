package djangogo

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/oliverhaas/djangogo/mail"
	"github.com/oliverhaas/djangogo/manage"
)

// sendtestemailCommand sends a test email to the given addresses, the
// managers or the admins (Django's sendtestemail).
type sendtestemailCommand struct {
	app *Application

	managers, admins bool
	// now replaces time.Now in tests.
	now func() time.Time
}

// Name returns "sendtestemail".
func (*sendtestemailCommand) Name() string { return "sendtestemail" }

// Help returns Django's help text of the command.
func (*sendtestemailCommand) Help() string {
	return "Sends a test email to the email addresses specified as arguments."
}

// Usage names the positional arguments.
func (*sendtestemailCommand) Usage() string { return "[email ...]" }

// Flags declares Django's options of sendtestemail.
func (c *sendtestemailCommand) Flags(fs *flag.FlagSet) {
	fs.BoolVar(&c.managers, "managers", false, "Send a test email to the addresses specified in settings.MANAGERS.")
	fs.BoolVar(&c.admins, "admins", false, "Send a test email to the addresses specified in settings.ADMINS.")
}

// Run sends one message to the addresses in emails, then one to the managers
// and one to the admins when the flags ask for them.
func (c *sendtestemailCommand) Run(emails []string) error {
	if len(emails) == 0 && !c.managers && !c.admins {
		return &manage.UsageError{Msg: "You must specify some email recipients, or pass the --managers or --admin options."}
	}
	hostname, _ := os.Hostname()
	subject := fmt.Sprintf("Test email from %s on %s", hostname, c.timestamp())
	if _, err := mail.SendMail(subject, "If you're reading this, it was successful.", "", emails); err != nil {
		return err
	}
	if c.managers {
		if err := mail.MailManagers(subject, "This email was sent to the site managers."); err != nil {
			return err
		}
	}
	if c.admins {
		if err := mail.MailAdmins(subject, "This email was sent to the site admins."); err != nil {
			return err
		}
	}
	return nil
}

// timestamp returns the current time as Python prints Django's
// timezone.now(): aware in UTC with USE_TZ, naive local time without.
func (c *sendtestemailCommand) timestamp() string {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	t := now()
	layout := "2006-01-02 15:04:05"
	if t.Nanosecond()/1000 != 0 {
		layout += ".000000"
	}
	if c.app.Settings.UseTZ {
		return t.UTC().Format(layout + "-07:00")
	}
	return t.Local().Format(layout)
}
