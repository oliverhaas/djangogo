package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// BCryptRounds is the work factor of the bcrypt hashers in Django 6.0.
const BCryptRounds = 12

// BCryptSHA256Hasher hashes with bcrypt over the hex SHA-256 digest of the
// password, which lifts bcrypt's limit of 72 bytes (Django's
// BCryptSHA256PasswordHasher). The encoded value is "bcrypt_sha256$" followed
// by the bcrypt hash.
type BCryptSHA256Hasher struct {
	// Rounds is the logarithmic work factor, 4 to 31. Zero means
	// BCryptRounds.
	Rounds int
}

// Algorithm returns "bcrypt_sha256".
func (BCryptSHA256Hasher) Algorithm() string { return "bcrypt_sha256" }

// Encode hashes password with a salt drawn by the bcrypt library.
func (h BCryptSHA256Hasher) Encode(password string) (string, error) {
	return bcryptEncode(h.Algorithm(), bcryptDigest(password), h.Rounds)
}

// Verify reports whether password matches encoded.
func (h BCryptSHA256Hasher) Verify(password, encoded string) bool {
	return bcryptVerify(h.Algorithm(), bcryptDigest(password), encoded)
}

// MustUpdate reports whether encoded has another work factor than the hasher.
func (h BCryptSHA256Hasher) MustUpdate(encoded string) bool {
	return bcryptMustUpdate(h.Algorithm(), encoded, h.Rounds)
}

// HardenRuntime runs bcrypt as often as the higher work factor of the hasher
// costs more than the one of encoded.
func (h BCryptSHA256Hasher) HardenRuntime(password, encoded string) {
	bcryptHarden(h.Algorithm(), bcryptDigest(password), encoded, h.Rounds)
}

// SafeSummary returns the algorithm, the work factor and the masked salt and
// checksum of encoded.
func (h BCryptSHA256Hasher) SafeSummary(encoded string) ([]SummaryItem, bool) {
	return bcryptSummary(h.Algorithm(), encoded)
}

// BCryptHasher hashes with plain bcrypt (Django's BCryptPasswordHasher).
// bcrypt reads 72 bytes at most: Encode refuses a longer password, and Verify
// compares the first 72 bytes only. Prefer BCryptSHA256Hasher. The encoded
// value is "bcrypt$" followed by the bcrypt hash.
type BCryptHasher struct {
	// Rounds is the logarithmic work factor, 4 to 31. Zero means
	// BCryptRounds.
	Rounds int
}

// Algorithm returns "bcrypt".
func (BCryptHasher) Algorithm() string { return "bcrypt" }

// Encode hashes password with a salt drawn by the bcrypt library.
func (h BCryptHasher) Encode(password string) (string, error) {
	return bcryptEncode(h.Algorithm(), []byte(password), h.Rounds)
}

// Verify reports whether password matches encoded.
func (h BCryptHasher) Verify(password, encoded string) bool {
	return bcryptVerify(h.Algorithm(), []byte(password), encoded)
}

// MustUpdate reports whether encoded has another work factor than the hasher.
func (h BCryptHasher) MustUpdate(encoded string) bool {
	return bcryptMustUpdate(h.Algorithm(), encoded, h.Rounds)
}

// HardenRuntime runs bcrypt as often as the higher work factor of the hasher
// costs more than the one of encoded.
func (h BCryptHasher) HardenRuntime(password, encoded string) {
	bcryptHarden(h.Algorithm(), []byte(password), encoded, h.Rounds)
}

// SafeSummary returns the algorithm, the work factor and the masked salt and
// checksum of encoded.
func (h BCryptHasher) SafeSummary(encoded string) ([]SummaryItem, bool) {
	return bcryptSummary(h.Algorithm(), encoded)
}

// bcryptSummary is SafeSummary for both bcrypt hashers. The bcrypt hash is
// "$2b$<work factor>$" followed by 22 characters of salt and the checksum.
func bcryptSummary(algorithm, encoded string) ([]SummaryItem, bool) {
	parts := strings.SplitN(encoded, "$", 5)
	if len(parts) != 5 || parts[0] != algorithm || parts[1] != "" {
		return nil, false
	}
	workFactor, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, false
	}
	data := parts[4]
	salt, checksum := data, ""
	if len(data) > 22 {
		salt, checksum = data[:22], data[22:]
	}
	return []SummaryItem{
		{"algorithm", algorithm},
		{"work factor", strconv.Itoa(workFactor)},
		{"salt", maskHash(salt, 6)},
		{"checksum", maskHash(checksum, 6)},
	}, true
}

// bcryptDigest returns the hex SHA-256 digest of password, the input of
// bcrypt_sha256 (Django ticket #20138).
func bcryptDigest(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	out := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(out, sum[:])
	return out
}

// bcryptRounds returns the configured work factor.
func bcryptRounds(rounds int) int {
	if rounds > 0 {
		return rounds
	}
	return BCryptRounds
}

// bcryptEncode hashes input and puts the algorithm name in front.
func bcryptEncode(algorithm string, input []byte, rounds int) (string, error) {
	data, err := bcrypt.GenerateFromPassword(input, bcryptRounds(rounds))
	if err != nil {
		return "", fmt.Errorf("auth: %s: %w", algorithm, err)
	}
	return algorithm + "$" + string(data), nil
}

// bcryptData returns the bcrypt hash inside encoded.
func bcryptData(algorithm, encoded string) ([]byte, bool) {
	data, ok := strings.CutPrefix(encoded, algorithm+"$")
	if !ok || data == "" {
		return nil, false
	}
	return []byte(data), true
}

// bcryptVerify checks input against the bcrypt hash inside encoded. The
// library compares in constant time.
func bcryptVerify(algorithm string, input []byte, encoded string) bool {
	data, ok := bcryptData(algorithm, encoded)
	return ok && bcrypt.CompareHashAndPassword(data, input) == nil
}

// bcryptMustUpdate compares the work factor of encoded with the configured
// one.
func bcryptMustUpdate(algorithm, encoded string, rounds int) bool {
	data, ok := bcryptData(algorithm, encoded)
	if !ok {
		return false
	}
	cost, err := bcrypt.Cost(data)
	return err == nil && cost != bcryptRounds(rounds)
}

// bcryptHarden hashes input at the work factor of encoded until the total
// equals one run at the configured work factor. One more round doubles the
// load, and the failed check already ran once.
func bcryptHarden(algorithm string, input []byte, encoded string, rounds int) {
	data, ok := bcryptData(algorithm, encoded)
	if !ok {
		return
	}
	cost, err := bcrypt.Cost(data)
	if err != nil || cost >= bcryptRounds(rounds) {
		return
	}
	for range 1<<(bcryptRounds(rounds)-cost) - 1 {
		_, _ = bcrypt.GenerateFromPassword(input, cost)
	}
}
