package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Record is one row of Django's django_session table.
type Record struct {
	// SessionKey is the key the session cookie carries.
	SessionKey string `orm:"pk;max_length=40"`
	// SessionData holds the values of the session as a JSON object.
	SessionData string `orm:"type=text"`
	// ExpireDate is the time after which the session is no longer loaded.
	ExpireDate time.Time `orm:"index"`
}

// Meta gives the table Django's name.
func (Record) Meta() orm.Meta { return orm.Meta{Table: "django_session"} }

// String returns the session key, as Django's Session.__str__ does.
func (r Record) String() string { return r.SessionKey }

// GetDecoded returns the values stored in the row (Django's get_decoded).
func (r Record) GetDecoded() (map[string]any, error) {
	data := make(map[string]any)
	if r.SessionData == "" {
		return data, nil
	}
	if err := json.Unmarshal([]byte(r.SessionData), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// App is the sessions app: install it to get the django_session table that
// DBStore needs.
type App struct{}

// Name returns the app label, "sessions".
func (App) Name() string { return "sessions" }

// Models returns the model of the app.
func (App) Models() []any { return []any{&Record{}} }

// DBStore keeps sessions in the django_session table (Django's db backend).
// The cookie carries the session key only.
type DBStore struct {
	// db is the database that holds the table.
	db *orm.DB
}

// NewDBStore returns a DBStore on db. The Record model must be registered and
// migrated there.
func NewDBStore(db *orm.DB) *DBStore {
	return &DBStore{db: db}
}

// New returns an empty session. It gets its key at the first save.
func (s *DBStore) New() *Session {
	return &Session{data: make(map[string]any)}
}

// Encode saves sess and returns its key. A session without a row is inserted
// under a new key. A session that was loaded from a row updates that row; when
// the row is gone the result is ErrUpdate and nothing is saved. Rows of keys
// retired by CycleKey are deleted.
func (s *DBStore) Encode(ctx context.Context, sess *Session) (string, error) {
	raw, err := json.Marshal(sess.data)
	if err != nil {
		return "", err
	}
	if err := s.dropStale(ctx, sess); err != nil {
		return "", err
	}
	expires := storedTime(sess.expiryDate(time.Now()))

	if sess.key != "" && sess.persisted {
		n, err := orm.Query[Record](s.db).Filter("session_key", sess.key).
			Update(ctx, "session_data", string(raw), "expire_date", expires)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", ErrUpdate
		}
		return sess.key, nil
	}

	if sess.key == "" {
		sess.key = newKey()
	}
	rec := Record{SessionKey: sess.key, SessionData: string(raw), ExpireDate: expires}
	if err := orm.Query[Record](s.db).Create(ctx, &rec); err != nil {
		return "", err
	}
	sess.persisted = true
	return sess.key, nil
}

// Decode loads the session with the key cookieValue. An unknown or expired key
// yields an empty session without a key: a key chosen by the client is never
// taken over, which would allow session fixation.
func (s *DBStore) Decode(ctx context.Context, cookieValue string) (*Session, error) {
	rec, err := orm.Query[Record](s.db).
		Filter("session_key", cookieValue, "expire_date__gt", storedTime(time.Now())).
		Get(ctx)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return s.New(), nil
	}
	if err != nil {
		return nil, err
	}
	data, err := rec.GetDecoded()
	if err != nil {
		// Corrupted data ends the session, as in Django.
		return s.New(), nil //nolint:nilerr
	}
	return &Session{key: rec.SessionKey, data: data, persisted: true}, nil
}

// Delete removes the row of sess and the rows of its retired keys. A later
// save of sess fails with ErrUpdate.
func (s *DBStore) Delete(ctx context.Context, sess *Session) error {
	if err := s.dropStale(ctx, sess); err != nil {
		return err
	}
	return s.deleteKey(ctx, sess.key)
}

// Exists reports whether a row with the key is stored (Django's exists).
func (s *DBStore) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	return orm.Query[Record](s.db).Filter("session_key", key).Exists(ctx)
}

// ClearExpired deletes the rows whose expiry date has passed (Django's
// clear_expired). Expired rows are never loaded, so this only frees space.
func (s *DBStore) ClearExpired(ctx context.Context) error {
	_, err := orm.Query[Record](s.db).Filter("expire_date__lt", storedTime(time.Now())).Delete(ctx)
	return err
}

// dropStale deletes the rows of the keys sess had before CycleKey and Flush.
func (s *DBStore) dropStale(ctx context.Context, sess *Session) error {
	for _, key := range sess.stale {
		if err := s.deleteKey(ctx, key); err != nil {
			return err
		}
	}
	sess.stale = nil
	return nil
}

// deleteKey deletes the row with the key, if there is one.
func (s *DBStore) deleteKey(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := orm.Query[Record](s.db).Filter("session_key", key).Delete(ctx)
	return err
}

// storedTime returns t in the form expiry dates are stored and compared in:
// UTC, whole seconds. SQLite compares the text of a timestamp, which only
// orders correctly when every value has the same layout.
func storedTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}
