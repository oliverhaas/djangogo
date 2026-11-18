package auth

import (
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

// PBKDF2Iterations is the work factor of the PBKDF2 hashers in Django 6.0.
const PBKDF2Iterations = 1_200_000

// PBKDF2Hasher hashes with PBKDF2 over HMAC-SHA256, Django's default
// (PBKDF2PasswordHasher). The encoded value is
// "pbkdf2_sha256$<iterations>$<salt>$<base64 hash>".
type PBKDF2Hasher struct {
	// Iterations is the work factor. Zero means PBKDF2Iterations.
	Iterations int
}

// Algorithm returns "pbkdf2_sha256".
func (PBKDF2Hasher) Algorithm() string { return "pbkdf2_sha256" }

// Salt returns a new random salt.
func (PBKDF2Hasher) Salt() (string, error) { return newSalt() }

// Encode hashes password with a new random salt.
func (h PBKDF2Hasher) Encode(password string) (string, error) {
	return encodeWithNewSalt(h, password)
}

// EncodeWithSalt hashes password with the given salt.
func (h PBKDF2Hasher) EncodeWithSalt(password, salt string) (string, error) {
	return h.variant().encode(password, salt, 0)
}

// Verify reports whether password matches encoded.
func (h PBKDF2Hasher) Verify(password, encoded string) bool {
	return h.variant().verify(password, encoded)
}

// MustUpdate reports whether encoded has another iteration count than the
// hasher or a salt of less than 128 bits.
func (h PBKDF2Hasher) MustUpdate(encoded string) bool { return h.variant().mustUpdate(encoded) }

// HardenRuntime runs the iterations that encoded lacks.
func (h PBKDF2Hasher) HardenRuntime(password, encoded string) {
	h.variant().hardenRuntime(password, encoded)
}

// variant returns the shared implementation set up for SHA-256.
func (h PBKDF2Hasher) variant() pbkdf2Variant {
	return pbkdf2Variant{algorithm: h.Algorithm(), digest: sha256.New, size: sha256.Size, iterations: h.Iterations}
}

// PBKDF2SHA1Hasher hashes with PBKDF2 over HMAC-SHA1, the PRF that PKCS #5
// recommends (Django's PBKDF2SHA1PasswordHasher). The encoded value is
// "pbkdf2_sha1$<iterations>$<salt>$<base64 hash>".
type PBKDF2SHA1Hasher struct {
	// Iterations is the work factor. Zero means PBKDF2Iterations.
	Iterations int
}

// Algorithm returns "pbkdf2_sha1".
func (PBKDF2SHA1Hasher) Algorithm() string { return "pbkdf2_sha1" }

// Salt returns a new random salt.
func (PBKDF2SHA1Hasher) Salt() (string, error) { return newSalt() }

// Encode hashes password with a new random salt.
func (h PBKDF2SHA1Hasher) Encode(password string) (string, error) {
	return encodeWithNewSalt(h, password)
}

// EncodeWithSalt hashes password with the given salt.
func (h PBKDF2SHA1Hasher) EncodeWithSalt(password, salt string) (string, error) {
	return h.variant().encode(password, salt, 0)
}

// Verify reports whether password matches encoded.
func (h PBKDF2SHA1Hasher) Verify(password, encoded string) bool {
	return h.variant().verify(password, encoded)
}

// MustUpdate reports whether encoded has another iteration count than the
// hasher or a salt of less than 128 bits.
func (h PBKDF2SHA1Hasher) MustUpdate(encoded string) bool { return h.variant().mustUpdate(encoded) }

// HardenRuntime runs the iterations that encoded lacks.
func (h PBKDF2SHA1Hasher) HardenRuntime(password, encoded string) {
	h.variant().hardenRuntime(password, encoded)
}

// variant returns the shared implementation set up for SHA-1.
func (h PBKDF2SHA1Hasher) variant() pbkdf2Variant {
	return pbkdf2Variant{algorithm: h.Algorithm(), digest: sha1.New, size: sha1.Size, iterations: h.Iterations}
}

// encodeWithNewSalt is Encode for a hasher that takes a salt.
func encodeWithNewSalt(h SaltedHasher, password string) (string, error) {
	salt, err := h.Salt()
	if err != nil {
		return "", err
	}
	return h.EncodeWithSalt(password, salt)
}

// pbkdf2Variant is PBKDF2 with one digest. The two exported hashers differ in
// the digest and the algorithm name only.
type pbkdf2Variant struct {
	// algorithm is the name in the encoded value.
	algorithm string
	// digest makes the hash function of the HMAC.
	digest func() hash.Hash
	// size is the length of the derived key, the output size of digest.
	size int
	// iterations is the configured work factor, zero for the default.
	iterations int
}

// count returns the configured work factor.
func (v pbkdf2Variant) count() int {
	if v.iterations > 0 {
		return v.iterations
	}
	return PBKDF2Iterations
}

// encode derives the key with the given number of iterations, or with the
// configured number when iterations is zero.
func (v pbkdf2Variant) encode(password, salt string, iterations int) (string, error) {
	if err := checkSalt(salt); err != nil {
		return "", err
	}
	if iterations <= 0 {
		iterations = v.count()
	}
	key, err := pbkdf2.Key(v.digest, password, []byte(salt), iterations, v.size)
	if err != nil {
		return "", fmt.Errorf("auth: derive %s key: %w", v.algorithm, err)
	}
	return fmt.Sprintf("%s$%d$%s$%s", v.algorithm, iterations, salt, base64.StdEncoding.EncodeToString(key)), nil
}

// decode splits encoded into the iteration count and the salt.
func (v pbkdf2Variant) decode(encoded string) (iterations int, salt string, ok bool) {
	parts := strings.SplitN(encoded, "$", 4)
	if len(parts) != 4 || parts[0] != v.algorithm {
		return 0, "", false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return 0, "", false
	}
	return iterations, parts[2], true
}

// verify hashes password with the parameters of encoded and compares.
func (v pbkdf2Variant) verify(password, encoded string) bool {
	iterations, salt, ok := v.decode(encoded)
	if !ok {
		return false
	}
	again, err := v.encode(password, salt, iterations)
	return err == nil && constantTimeEqual(encoded, again)
}

// mustUpdate compares the parameters of encoded with the configured ones.
func (v pbkdf2Variant) mustUpdate(encoded string) bool {
	iterations, salt, ok := v.decode(encoded)
	if !ok {
		return false
	}
	return iterations != v.count() || mustUpdateSalt(salt)
}

// hardenRuntime runs the iterations that separate encoded from the configured
// work factor.
func (v pbkdf2Variant) hardenRuntime(password, encoded string) {
	iterations, salt, ok := v.decode(encoded)
	if !ok {
		return
	}
	if extra := v.count() - iterations; extra > 0 {
		_, _ = v.encode(password, salt, extra)
	}
}

// MD5Hasher is the salted MD5 hasher (Django's MD5PasswordHasher). MD5 is
// broken for passwords: use it to keep tests fast, as Django's own test
// settings do, and never in production. It is not among the default hashers.
// The encoded value is "md5$<salt>$<hex digest>".
type MD5Hasher struct{}

// Algorithm returns "md5".
func (MD5Hasher) Algorithm() string { return "md5" }

// Salt returns a new random salt.
func (MD5Hasher) Salt() (string, error) { return newSalt() }

// Encode hashes password with a new random salt.
func (h MD5Hasher) Encode(password string) (string, error) { return encodeWithNewSalt(h, password) }

// EncodeWithSalt hashes password with the given salt.
func (h MD5Hasher) EncodeWithSalt(password, salt string) (string, error) {
	if err := checkSalt(salt); err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(salt + password))
	return h.Algorithm() + "$" + salt + "$" + hex.EncodeToString(sum[:]), nil
}

// Verify reports whether password matches encoded.
func (h MD5Hasher) Verify(password, encoded string) bool {
	salt, ok := h.salt(encoded)
	if !ok {
		return false
	}
	again, err := h.EncodeWithSalt(password, salt)
	return err == nil && constantTimeEqual(encoded, again)
}

// MustUpdate reports whether the salt of encoded has less than 128 bits.
func (h MD5Hasher) MustUpdate(encoded string) bool {
	salt, ok := h.salt(encoded)
	return ok && mustUpdateSalt(salt)
}

// HardenRuntime does nothing: MD5 has no work factor.
func (MD5Hasher) HardenRuntime(string, string) {}

// salt returns the salt of encoded.
func (h MD5Hasher) salt(encoded string) (string, bool) {
	parts := strings.SplitN(encoded, "$", 3)
	if len(parts) != 3 || parts[0] != h.Algorithm() {
		return "", false
	}
	return parts[1], true
}
