package sessions_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/sessions"
)

func TestSignedCookieRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := sessions.NewSignedCookieStore([]byte("super-secret"))

	s := store.New()
	s.Set("uid", float64(7)) // JSON numbers decode as float64
	s.Set("name", "ada")

	value, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := store.Decode(ctx, value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if v, ok := got.Get("uid"); !ok || v != float64(7) {
		t.Fatalf("Decode uid = %v, %v; want 7", v, ok)
	}
	if v, ok := got.Get("name"); !ok || v != "ada" {
		t.Fatalf("Decode name = %v, %v; want ada", v, ok)
	}
}

func TestSignedCookieTampered(t *testing.T) {
	ctx := context.Background()
	store := sessions.NewSignedCookieStore([]byte("super-secret"))

	s := store.New()
	s.Set("uid", float64(7))
	value, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Flip a character in the payload so the MAC no longer matches.
	tampered := "X" + value[1:]
	got, err := store.Decode(ctx, tampered)
	if err != nil {
		t.Fatalf("Decode of tampered value returned error: %v", err)
	}
	if len(got.Data()) != 0 {
		t.Fatalf("tampered cookie decoded to non-empty session: %v", got.Data())
	}

	// A value with no separator at all also yields a fresh session.
	got2, err := store.Decode(ctx, "garbage-without-dot")
	if err != nil {
		t.Fatalf("Decode of malformed value returned error: %v", err)
	}
	if len(got2.Data()) != 0 {
		t.Fatal("malformed cookie decoded to non-empty session")
	}
}

func TestSignedCookieWrongSecret(t *testing.T) {
	ctx := context.Background()
	signer := sessions.NewSignedCookieStore([]byte("secret-a"))
	verifier := sessions.NewSignedCookieStore([]byte("secret-b"))

	s := signer.New()
	s.Set("uid", float64(7))
	value, err := signer.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := verifier.Decode(ctx, value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Data()) != 0 {
		t.Fatal("cookie signed with a different secret was accepted")
	}
}

func TestSignedCookieDeleteNoOp(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	if err := store.Delete(context.Background(), store.New()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRotate(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	s := store.New()
	s.Set("uid", 7)

	// Encode through the DB store path would assign a key, but Rotate works on any
	// session directly. Capture the (empty) key first.
	before := s.Key()
	sessions.Rotate(s)
	after := s.Key()

	if before == after {
		t.Fatalf("Rotate did not change the key: %q", after)
	}
	if after == "" {
		t.Fatal("Rotate produced an empty key")
	}
	if v, ok := s.Get("uid"); !ok || v != 7 {
		t.Fatalf("Rotate dropped data: uid = %v, %v", v, ok)
	}
	if !s.Modified() {
		t.Fatal("Rotate did not mark session modified")
	}

	// A second rotation yields a different key again.
	sessions.Rotate(s)
	if s.Key() == after {
		t.Fatal("second Rotate produced the same key")
	}
}

func TestSessionKeyFormat(t *testing.T) {
	seen := make(map[string]bool)
	for range 50 {
		s := &sessions.Session{}
		s.CycleKey()
		key := s.Key()
		if len(key) != 32 {
			t.Fatalf("key %q has %d characters, want 32", key, len(key))
		}
		for _, c := range key {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
				t.Fatalf("key %q has the character %q outside [a-z0-9]", key, c)
			}
		}
		if seen[key] {
			t.Fatalf("key %q was issued twice", key)
		}
		seen[key] = true
	}
}

func TestSignedCookieMaxAge(t *testing.T) {
	ctx := context.Background()
	store := sessions.NewSignedCookieStore([]byte("secret"))
	s := store.New()
	s.Set("uid", "7")
	value, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	expired := sessions.NewSignedCookieStore([]byte("secret"))
	expired.MaxAge = time.Nanosecond
	got, err := expired.Decode(ctx, value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !got.IsEmpty() {
		t.Errorf("a value older than MaxAge was accepted: %v", got.Data())
	}

	got, err = store.Decode(ctx, value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if v, _ := got.Get("uid"); v != "7" {
		t.Errorf("uid = %v, want 7 within the default age", v)
	}
}

func TestSignedCookieFallbackSecrets(t *testing.T) {
	ctx := context.Background()
	old := sessions.NewSignedCookieStore([]byte("old secret"))
	s := old.New()
	s.Set("uid", "7")
	value, err := old.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	rotated := sessions.NewSignedCookieStore([]byte("new secret"))
	got, _ := rotated.Decode(ctx, value)
	if !got.IsEmpty() {
		t.Error("a value signed with an unknown secret was accepted")
	}

	rotated.Fallbacks = [][]byte{[]byte("old secret")}
	got, _ = rotated.Decode(ctx, value)
	if v, _ := got.Get("uid"); v != "7" {
		t.Errorf("uid = %v, want 7 from the fallback secret", v)
	}

	renewed, err := rotated.Encode(ctx, got)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if back, _ := old.Decode(ctx, renewed); !back.IsEmpty() {
		t.Error("a new value is signed with a fallback secret")
	}
}

func TestSignedCookieRejectsChangedTimestamp(t *testing.T) {
	ctx := context.Background()
	store := sessions.NewSignedCookieStore([]byte("secret"))
	s := store.New()
	s.Set("uid", "7")
	value, err := store.Encode(ctx, s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		t.Fatalf("value %q has %d parts, want 3", value, len(parts))
	}
	forged := parts[0] + ".99999999999." + parts[2]
	if got, _ := store.Decode(ctx, forged); !got.IsEmpty() {
		t.Error("a value with a changed timestamp was accepted")
	}
}
