package mail

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// separator ends each message that the console and file backends write.
var separator = strings.Repeat("-", 79) + "\n"

// ConsoleBackend writes the messages to a stream, standard output by
// default, as Django's console backend does.
type ConsoleBackend struct {
	mu           sync.Mutex
	stream       io.Writer
	failSilently bool
}

// NewConsoleBackend returns a console backend for the options.
func NewConsoleBackend(o Options) *ConsoleBackend {
	stream := o.Stream
	if stream == nil {
		stream = os.Stdout
	}
	return &ConsoleBackend{stream: stream, failSilently: o.FailSilently}
}

// Open does nothing.
func (b *ConsoleBackend) Open() (bool, error) { return false, nil }

// Close does nothing.
func (b *ConsoleBackend) Close() error { return nil }

// SendMessages writes each message, followed by a line of dashes.
func (b *ConsoleBackend) SendMessages(messages []*Message) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sent := 0
	for _, m := range messages {
		data, err := m.Bytes()
		if err == nil && !utf8.Valid(data) {
			err = errors.New("'utf-8' codec can't decode the message")
		}
		if err == nil {
			_, err = fmt.Fprintf(b.stream, "%s\n%s", data, separator)
		}
		if err != nil {
			if b.failSilently {
				return sent, nil
			}
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// FileBackend appends the messages to a file in a directory, as Django's
// file backend does. Each backend writes one file, named after the time
// it was first used.
type FileBackend struct {
	mu           sync.Mutex
	dir          string
	fname        string
	file         *os.File
	failSilently bool
}

// fileCounter makes the file names of backends unique within a process, as
// id(self) does in Django.
var fileCounter struct {
	sync.Mutex
	n int
}

// NewFileBackend returns a file backend for the directory of the options,
// which it creates. It fails when the directory cannot be created or
// written to.
func NewFileBackend(o Options) (*FileBackend, error) {
	if o.FilePath == "" {
		return nil, errors.New("mail: the file backend needs EMAIL_FILE_PATH or WithFilePath")
	}
	dir, err := filepath.Abs(o.FilePath)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("Path for saving email messages exists, but is not a directory: %s", dir) //nolint:revive,staticcheck // Django's message
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, fmt.Errorf("Could not create directory for saving email messages: %s (%w)", dir, err) //nolint:revive,staticcheck // Django's message
	}
	probe, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return nil, fmt.Errorf("Could not write to directory: %s", dir) //nolint:revive,staticcheck // Django's message
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return &FileBackend{dir: dir, failSilently: o.FailSilently}, nil
}

// filename returns the path of the file of this backend.
func (b *FileBackend) filename() string {
	if b.fname == "" {
		fileCounter.Lock()
		fileCounter.n++
		n := fileCounter.n
		fileCounter.Unlock()
		name := fmt.Sprintf("%s-%d%d.log", time.Now().Format("20060102-150405"), os.Getpid(), n)
		b.fname = filepath.Join(b.dir, name)
	}
	return b.fname
}

// Path returns the file this backend writes to.
func (b *FileBackend) Path() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.filename()
}

// Open opens the file for appending. It reports false when the file is
// open already.
func (b *FileBackend) Open() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.open()
}

// open opens the file; b.mu is held.
func (b *FileBackend) open() (bool, error) {
	if b.file != nil {
		return false, nil
	}
	f, err := os.OpenFile(b.filename(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return false, err
	}
	b.file = f
	return true, nil
}

// Close closes the file.
func (b *FileBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.close()
}

// close closes the file; b.mu is held.
func (b *FileBackend) close() error {
	if b.file == nil {
		return nil
	}
	err := b.file.Close()
	b.file = nil
	return err
}

// SendMessages appends each message, followed by a line of dashes.
func (b *FileBackend) SendMessages(messages []*Message) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	sent := 0
	created, err := b.open()
	if err == nil {
		for _, m := range messages {
			var data []byte
			if data, err = m.Bytes(); err != nil {
				break
			}
			if _, err = fmt.Fprintf(b.file, "%s\n%s", data, separator); err != nil {
				break
			}
			sent++
		}
		if created {
			if cerr := b.close(); err == nil {
				err = cerr
			}
		}
	}
	if err != nil && !b.failSilently {
		return sent, err
	}
	return sent, nil
}

// outbox holds the messages that locmem backends sent, as Django's
// mail.outbox.
var outbox struct {
	sync.Mutex
	messages []*Message
}

// Outbox returns the messages that locmem backends sent since the last
// ClearOutbox.
func Outbox() []*Message {
	outbox.Lock()
	defer outbox.Unlock()
	return append([]*Message(nil), outbox.messages...)
}

// ClearOutbox empties the outbox, as Django's test runner does before each
// test.
func ClearOutbox() {
	outbox.Lock()
	outbox.messages = nil
	outbox.Unlock()
}

// LocmemBackend keeps the messages in the outbox instead of sending them,
// as Django's locmem backend does for tests.
type LocmemBackend struct{}

// NewLocmemBackend returns a locmem backend.
func NewLocmemBackend(Options) LocmemBackend { return LocmemBackend{} }

// Open does nothing.
func (LocmemBackend) Open() (bool, error) { return false, nil }

// Close does nothing.
func (LocmemBackend) Close() error { return nil }

// SendMessages builds each message, which checks its headers, and appends
// a copy to the outbox.
func (LocmemBackend) SendMessages(messages []*Message) (int, error) {
	sent := 0
	for _, m := range messages {
		if _, err := m.Bytes(); err != nil {
			return sent, err
		}
		outbox.Lock()
		outbox.messages = append(outbox.messages, m.clone())
		outbox.Unlock()
		sent++
	}
	return sent, nil
}

// clone returns a deep copy of the message.
func (m *Message) clone() *Message {
	c := *m
	c.To = append([]string(nil), m.To...)
	c.Cc = append([]string(nil), m.Cc...)
	c.Bcc = append([]string(nil), m.Bcc...)
	c.ReplyTo = append([]string(nil), m.ReplyTo...)
	c.Headers = append([]Header(nil), m.Headers...)
	c.Alternatives = append([]Alternative(nil), m.Alternatives...)
	c.Attachments = make([]Attachment, len(m.Attachments))
	for i, a := range m.Attachments {
		if a.Content != nil {
			a.Content = append([]byte{}, a.Content...)
		}
		if a.Message != nil {
			a.Message = a.Message.clone()
		}
		if a.Part != nil {
			p := *a.Part
			p.Content = append([]byte(nil), p.Content...)
			p.Headers = append([]Header(nil), p.Headers...)
			a.Part = &p
		}
		c.Attachments[i] = a
	}
	if m.Attachments == nil {
		c.Attachments = nil
	}
	return &c
}
