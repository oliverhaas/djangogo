package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Record is the ORM model backing DBStore (table "sessions").
type Record struct {
	Key       string    `orm:"pk;max_length=64"`
	Data      string    `orm:"type=text"`
	ExpiresAt time.Time `orm:"null"`
}

// DBStore is a database-backed session store. The cookie holds only the opaque
// session key; the session data lives server-side in the sessions table.
type DBStore struct {
	db *orm.DB
}

// NewDBStore returns a DBStore that persists sessions in db. The Record model must be
// registered and migrated in db's registry before use.
func NewDBStore(db *orm.DB) *DBStore {
	return &DBStore{db: db}
}

// New returns a fresh, empty session with no key assigned yet.
func (s *DBStore) New() *Session {
	return &Session{data: make(map[string]any)}
}

// Encode persists s to the database and returns its key as the cookie value. A
// keyless session is assigned a new random key first. An existing row for the key is
// updated in place; otherwise a new row is inserted.
func (s *DBStore) Encode(ctx context.Context, sess *Session) (string, error) {
	if sess.key == "" {
		Rotate(sess)
	}
	raw, err := json.Marshal(sess.Data())
	if err != nil {
		return "", err
	}
	data := string(raw)

	_, err = orm.Query[Record](s.db).Get(ctx, "key", sess.key)
	switch {
	case err == nil:
		if _, uerr := orm.Query[Record](s.db).
			Filter("key", sess.key).
			Update(ctx, "data", data); uerr != nil {
			return "", uerr
		}
	case errors.Is(err, orm.ErrDoesNotExist):
		rec := Record{Key: sess.key, Data: data}
		if cerr := orm.Query[Record](s.db).Create(ctx, &rec); cerr != nil {
			return "", cerr
		}
	default:
		return "", err
	}
	return sess.key, nil
}

// Decode loads the session whose key equals cookieValue. An unknown key yields a
// fresh empty session and a nil error.
func (s *DBStore) Decode(ctx context.Context, cookieValue string) (*Session, error) {
	rec, err := orm.Query[Record](s.db).Get(ctx, "key", cookieValue)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return s.New(), nil
	}
	if err != nil {
		return nil, err
	}
	data := make(map[string]any)
	if rec.Data != "" {
		if jerr := json.Unmarshal([]byte(rec.Data), &data); jerr != nil {
			return nil, jerr
		}
	}
	return &Session{key: cookieValue, data: data}, nil
}

// Delete removes the database row backing s, if any.
func (s *DBStore) Delete(ctx context.Context, sess *Session) error {
	if sess.key == "" {
		return nil
	}
	_, err := orm.Query[Record](s.db).Filter("key", sess.key).Delete(ctx)
	return err
}
