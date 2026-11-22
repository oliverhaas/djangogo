package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// errInvalidKey is Django's InvalidSessionKey: a key with characters that
// could leave the storage directory.
var errInvalidKey = errors.New("sessions: invalid characters in session key")

// configError is the error of a wrong setup (Django's ImproperlyConfigured).
// Its text is Django's.
type configError string

// Error returns the text of the error.
func (e configError) Error() string { return string(e) }

// FileStore keeps each session in a file of a directory (Django's file
// backend). The cookie carries the session key; the file named by the prefix
// and the key holds the values as JSON.
//
// A file is written to a temporary file first and renamed into place, so that
// a concurrent reader sees either the old or the new values, never a part.
type FileStore struct {
	// MaxAge is the cookie age of the middleware. A session without a custom
	// expiry expires MaxAge after its file was last written. Zero means
	// DefaultCookieAge.
	MaxAge time.Duration

	// dir is the directory that holds the session files.
	dir string
	// prefix starts every file name, so that the store can tell its files
	// from others in a shared directory.
	prefix string
}

// NewFileStore returns a FileStore that keeps the sessions in dir, in files
// named prefix plus the session key. Django uses the session cookie name as
// the prefix. An empty dir means os.TempDir(). It is an error when dir is not
// an existing directory.
func NewFileStore(dir, prefix string) (*FileStore, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, configError(fmt.Sprintf("The session storage path %q doesn't exist. Please set your "+
			"SESSION_FILE_PATH setting to an existing directory in which "+
			"Django can store session data.", dir))
	}
	return &FileStore{dir: dir, prefix: prefix}, nil
}

// New returns an empty session. It gets its key at the first save.
func (s *FileStore) New() *Session {
	return &Session{data: make(map[string]any)}
}

// Encode saves sess and returns its key. A session without a file gets a new
// file under a key no other file has. A session that was loaded from a file
// replaces that file; when the file is gone the result is ErrUpdate and
// nothing is saved. Files of keys retired by CycleKey are deleted.
func (s *FileStore) Encode(_ context.Context, sess *Session) (string, error) {
	if err := sess.load(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(sess.data)
	if err != nil {
		return "", err
	}
	s.dropStale(sess)

	if sess.key != "" && sess.persisted {
		name, err := s.path(sess.key)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(name); errors.Is(err, fs.ErrNotExist) {
			return "", ErrUpdate
		}
		if err := s.write(name, raw); err != nil {
			return "", err
		}
		return sess.key, nil
	}

	if sess.key == "" {
		sess.key = newKey()
	}
	for {
		name, err := s.path(sess.key)
		if err != nil {
			return "", err
		}
		// The empty file reserves the key: O_EXCL fails when another
		// session holds it.
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			sess.key = newKey()
			continue
		}
		if err != nil {
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		if err := s.write(name, raw); err != nil {
			return "", err
		}
		sess.persisted = true
		return sess.key, nil
	}
}

// Decode loads the session with the key cookieValue. An unknown, invalid or
// expired key yields an empty session without a key: a key chosen by the
// client is never taken over, which would allow session fixation. The file of
// an expired session is deleted.
func (s *FileStore) Decode(_ context.Context, cookieValue string) (*Session, error) {
	if sess, ok := s.load(cookieValue, time.Now()); ok {
		return sess, nil
	}
	return s.New(), nil
}

// Delete removes the file of sess and the files of its retired keys. A later
// save of sess fails with ErrUpdate.
func (s *FileStore) Delete(_ context.Context, sess *Session) error {
	if err := sess.load(); err != nil {
		return err
	}
	s.dropStale(sess)
	s.remove(sess.key)
	return nil
}

// Exists reports whether a file holds the session with the key (Django's
// exists).
func (s *FileStore) Exists(_ context.Context, key string) (bool, error) {
	name, err := s.path(key)
	if err != nil {
		return false, nil //nolint:nilerr // An invalid key names no file.
	}
	_, err = os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// ClearExpired deletes the files of the sessions whose expiry date has passed
// (Django's clear_expired). Expired sessions are never loaded, so this only
// frees space.
func (s *FileStore) ClearExpired(_ context.Context) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, e := range entries {
		if key, ok := strings.CutPrefix(e.Name(), s.prefix); ok && e.Type().IsRegular() {
			s.load(key, now)
		}
	}
	return nil
}

// load reads the session with the key and reports whether it is there, holds
// JSON and is not expired. It deletes the file of an expired session. A file
// that does not hold JSON stays until it expires by the time it was written.
func (s *FileStore) load(key string, now time.Time) (*Session, bool) {
	name, err := s.path(key)
	if err != nil {
		return nil, false
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, false
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	// An empty file is the placeholder of a session whose first save is not
	// done yet.
	data := make(map[string]any)
	corrupt := len(raw) > 0 && json.Unmarshal(raw, &data) != nil
	if corrupt {
		data = make(map[string]any)
	}
	sess := &Session{key: key, data: data, persisted: true, cookieAge: s.MaxAge}
	if !sess.expiryDate(info.ModTime()).After(now) {
		s.remove(key)
		return nil, false
	}
	// Corrupted data ends the session, as in the database store.
	return sess, !corrupt
}

// write replaces the content of the file name with raw. It writes a temporary
// file in the same directory and renames it, which is atomic on POSIX
// systems.
func (s *FileStore) write(name string, raw []byte) error {
	tmp, err := os.CreateTemp(s.dir, filepath.Base(name)+"_out_")
	if err != nil {
		return err
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return err
	}
	renamed = true
	return nil
}

// dropStale deletes the files of the keys sess had before CycleKey and Flush.
func (s *FileStore) dropStale(sess *Session) {
	for _, key := range sess.stale {
		s.remove(key)
	}
	sess.stale = nil
}

// remove deletes the file of the key, if there is one.
func (s *FileStore) remove(key string) {
	if name, err := s.path(key); err == nil {
		_ = os.Remove(name)
	}
}

// path returns the name of the file of the key (Django's _key_to_file). A key
// with characters other than those of generated keys, such as a slash, is
// rejected, so that no key reaches outside the directory.
func (s *FileStore) path(key string) (string, error) {
	if key == "" || strings.Trim(key, keyAlphabet) != "" {
		return "", errInvalidKey
	}
	return filepath.Join(s.dir, s.prefix+key), nil
}
