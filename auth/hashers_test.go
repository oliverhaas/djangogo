package auth_test

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
)

func TestMakePasswordFormat(t *testing.T) {
	encoded, err := auth.MakePassword("s3cret")
	if err != nil {
		t.Fatalf("MakePassword: %v", err)
	}
	prefix := fmt.Sprintf("pbkdf2_sha256$%d$", auth.Iterations)
	if !strings.HasPrefix(encoded, prefix) {
		t.Fatalf("encoded = %q, want prefix %q", encoded, prefix)
	}
	if n := strings.Count(encoded, "$"); n != 3 {
		t.Fatalf("encoded = %q, want 3 %q separators, got %d", encoded, "$", n)
	}
}

func TestCheckPasswordRoundTrip(t *testing.T) {
	encoded, err := auth.MakePassword("correct horse")
	if err != nil {
		t.Fatalf("MakePassword: %v", err)
	}
	if !auth.CheckPassword("correct horse", encoded) {
		t.Fatal("CheckPassword returned false for the correct password")
	}
	if auth.CheckPassword("wrong password", encoded) {
		t.Fatal("CheckPassword returned true for the wrong password")
	}
}

func TestCheckPasswordMalformed(t *testing.T) {
	cases := []string{
		"",
		"plaintext",
		"pbkdf2_sha256$100000$salt", // only 3 parts
		"bcrypt$100000$salt$hash",   // wrong algorithm
		"pbkdf2_sha256$notanint$salt$hash",
		"pbkdf2_sha256$100000$salt$!!!not-base64!!!",
		"$$$",
	}
	for _, encoded := range cases {
		if auth.CheckPassword("anything", encoded) {
			t.Errorf("CheckPassword(%q) = true, want false", encoded)
		}
	}
}

func TestCheckPasswordKnownDjangoFormat(t *testing.T) {
	// Build a known Django-format string by hand with a small iteration count and
	// verify CheckPassword recomputes and matches it.
	const raw = "pa$$word"
	const iter = 1000
	salt := "abcdefghijklmnop"
	dk, err := pbkdf2.Key(sha256.New, raw, []byte(salt), iter, 32)
	if err != nil {
		t.Fatalf("pbkdf2.Key: %v", err)
	}
	encoded := fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iter, salt, base64.StdEncoding.EncodeToString(dk))
	if !auth.CheckPassword(raw, encoded) {
		t.Fatalf("CheckPassword failed for known-good encoded %q", encoded)
	}
	if auth.CheckPassword("nope", encoded) {
		t.Fatal("CheckPassword matched the wrong raw for a known encoded value")
	}
}
