package mail

// SendMail sends one message to the recipients, who all see each other in
// the To header, and returns the number of messages sent, as Django's
// send_mail does. An empty from means the DefaultFromEmail setting.
// WithConnection sends through an open backend; otherwise the options build
// a new one. WithHTML adds an HTML alternative.
func SendMail(subject, message, from string, to []string, opts ...Option) (int, error) {
	conn, o, err := connectionFor(opts)
	if err != nil {
		return 0, err
	}
	m := &Message{Subject: subject, Body: message, From: from, To: to, Connection: conn}
	if o.html != "" {
		if err := m.AttachAlternative(o.html, "text/html"); err != nil {
			return 0, err
		}
	}
	return m.Send()
}

// MassMessage is one message of SendMassMail.
type MassMessage struct {
	Subject string
	Body    string
	// From is the sender. Empty means the DefaultFromEmail setting.
	From string
	To   []string
}

// SendMassMail sends each message over one connection and returns the
// number of messages sent, as Django's send_mass_mail does.
func SendMassMail(messages []MassMessage, opts ...Option) (int, error) {
	conn, _, err := connectionFor(opts)
	if err != nil {
		return 0, err
	}
	out := make([]*Message, len(messages))
	for i, mm := range messages {
		out[i] = &Message{Subject: mm.Subject, Body: mm.Body, From: mm.From, To: mm.To, Connection: conn}
	}
	return conn.SendMessages(out)
}

// MailAdmins sends a message to the addresses of the Admins setting, as
// Django's mail_admins does. The subject starts with the SubjectPrefix
// setting and the sender is the ServerEmail setting. An empty Admins
// setting sends nothing.
func MailAdmins(subject, message string, opts ...Option) error { //nolint:revive // Django's name
	return sendServerMessage(CurrentConfig().Admins, subject, message, opts)
}

// MailManagers sends a message to the addresses of the Managers setting, as
// MailAdmins does for Admins.
func MailManagers(subject, message string, opts ...Option) error { //nolint:revive // Django's name
	return sendServerMessage(CurrentConfig().Managers, subject, message, opts)
}

// sendServerMessage sends the message of MailAdmins and MailManagers.
func sendServerMessage(recipients []string, subject, message string, opts []Option) error {
	if len(recipients) == 0 {
		return nil
	}
	cfg := CurrentConfig()
	o := resolve(opts)
	m := &Message{
		Subject:    cfg.SubjectPrefix + subject,
		Body:       message,
		From:       cfg.ServerEmail,
		To:         append([]string(nil), recipients...),
		Connection: o.connection,
	}
	if o.html != "" {
		if err := m.AttachAlternative(o.html, "text/html"); err != nil {
			return err
		}
	}
	_, err := m.Send(opts...)
	return err
}
