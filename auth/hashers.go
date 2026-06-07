// Package auth provides user, group, and permission models, Django-format
// password hashing, session-based login, and HTTP access guards. It builds on
// the orm and sessions packages and does not import either of djangogo's app or
// project layers, so it carries no import cycle.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Iterations is the pbkdf2 work factor (kept modest for the PoC; Django uses far more).
var Iterations = 100_000

// keyLen is the derived-key length in bytes (256-bit, matching Django's pbkdf2_sha256).
const keyLen = 32

// algoPBKDF2SHA256 is the algorithm identifier in the encoded hash's first field.
const algoPBKDF2SHA256 = "pbkdf2_sha256"

// MakePassword hashes raw into the Django format "pbkdf2_sha256$<iter>$<salt>$<b64hash>".
// It draws a fresh 16-byte random salt and derives the key with Iterations rounds
// of pbkdf2-sha256.
func MakePassword(raw string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("auth: MakePassword: read random salt: %w", err)
	}
	salt := base64.StdEncoding.EncodeToString(saltBytes)
	dk, err := pbkdf2.Key(sha256.New, raw, []byte(salt), Iterations, keyLen)
	if err != nil {
		return "", fmt.Errorf("auth: MakePassword: derive key: %w", err)
	}
	return fmt.Sprintf("%s$%d$%s$%s", algoPBKDF2SHA256, Iterations, salt, base64.StdEncoding.EncodeToString(dk)), nil
}

// CheckPassword reports whether raw matches the encoded "pbkdf2_sha256$..." hash.
// Any malformed input (wrong algorithm, non-numeric iteration count, invalid
// base64, or the wrong number of fields) yields false rather than an error.
func CheckPassword(raw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return false
	}
	algo, iterStr, salt, want := parts[0], parts[1], parts[2], parts[3]
	if algo != algoPBKDF2SHA256 {
		return false
	}
	iter, err := strconv.Atoi(iterStr)
	if err != nil || iter <= 0 {
		return false
	}
	wantBytes, err := base64.StdEncoding.DecodeString(want)
	if err != nil {
		return false
	}
	dk, err := pbkdf2.Key(sha256.New, raw, []byte(salt), iter, len(wantBytes))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(dk, wantBytes) == 1
}
