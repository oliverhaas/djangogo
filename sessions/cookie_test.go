package sessions_test

import (
	"context"
	"testing"

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
