package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// ErrUnknownHasher is returned for an algorithm that no configured hasher
// implements (Django's "Unknown password hashing algorithm").
var ErrUnknownHasher = errors.New("auth: unknown password hashing algorithm")

// ErrInvalidSalt is returned by EncodeWithSalt for an empty salt or one that
// contains "$", the separator of the encoded value.
var ErrInvalidSalt = errors.New("auth: salt must be provided and cannot contain $")

// ErrSaltNotSupported is returned by MakePasswordWith when a salt is given for
// a hasher that only draws its own.
var ErrSaltNotSupported = errors.New("auth: hasher does not accept a salt")

// Hasher turns a password into the value of the password column and checks a
// password against such a value (Django's BasePasswordHasher). The value
// starts with the algorithm name and a "$".
type Hasher interface {
	// Algorithm is the name in front of the first "$" of the encoded value.
	Algorithm() string
	// Encode returns the value to store for password, made with a new
	// random salt.
	Encode(password string) (string, error)
	// Verify reports whether password matches encoded. A malformed value
	// matches nothing.
	Verify(password, encoded string) bool
	// MustUpdate reports whether encoded was made with other parameters
	// than this hasher uses now, so that the password is hashed again at
	// the next successful login.
	MustUpdate(encoded string) bool
	// HardenRuntime spends the time that separates the work factor of
	// encoded from the one of this hasher. It runs after a failed check of
	// an outdated hash, so that the answer takes as long as for a current
	// one. A hasher without a work factor does nothing.
	HardenRuntime(password, encoded string)
}

// SaltedHasher is a Hasher that also takes the salt from the caller (Django's
// encode(password, salt)). The bcrypt hashers do not implement it: their
// library draws the salt.
type SaltedHasher interface {
	Hasher
	// Salt returns a new random salt with at least 128 bits of entropy.
	Salt() (string, error)
	// EncodeWithSalt returns the value to store for password and salt. It
	// returns ErrInvalidSalt for an empty salt or one containing "$".
	EncodeWithSalt(password, salt string) (string, error)
}

// DefaultPasswordHashers returns Django's default PASSWORD_HASHERS: PBKDF2
// with SHA-256 first, then PBKDF2 with SHA-1, Argon2, bcrypt over SHA-256 and
// scrypt.
func DefaultPasswordHashers() []Hasher {
	return []Hasher{
		PBKDF2Hasher{},
		PBKDF2SHA1Hasher{},
		Argon2Hasher{},
		BCryptSHA256Hasher{},
		ScryptHasher{},
	}
}

// defaultHashers is the list used while Config.PasswordHashers is empty.
var defaultHashers = DefaultPasswordHashers()

// hashers returns the configured hashers, or the default list.
func hashers() []Hasher {
	if h := settings().PasswordHashers; len(h) > 0 {
		return h
	}
	return defaultHashers
}

// hasherFor returns the configured hasher of the given algorithm.
func hasherFor(algorithm string) (Hasher, error) {
	for _, h := range hashers() {
		if h.Algorithm() == algorithm {
			return h, nil
		}
	}
	return nil, fmt.Errorf("%w %q: is its hasher in Config.PasswordHashers?", ErrUnknownHasher, algorithm)
}

// GetHasher returns the configured hasher of the given algorithm (Django's
// get_hasher). "default" and the empty string name the preferred hasher, the
// first of Config.PasswordHashers. An algorithm without a configured hasher
// gives ErrUnknownHasher.
func GetHasher(algorithm string) (Hasher, error) {
	if algorithm == "" || algorithm == "default" {
		return hashers()[0], nil
	}
	return hasherFor(algorithm)
}

// IdentifyHasher returns the configured hasher that made encoded, going by the
// algorithm name in front of the first "$" (Django's identify_hasher). It
// returns ErrUnknownHasher for an unusable password and for any other value
// that no configured hasher made.
func IdentifyHasher(encoded string) (Hasher, error) {
	algorithm, _, _ := strings.Cut(encoded, "$")
	return hasherFor(algorithm)
}

// MakePassword hashes raw with the preferred hasher and a new random salt
// (Django's make_password). The empty password is hashed like any other; use
// MakeUnusablePassword for an account without a password.
func MakePassword(raw string) (string, error) {
	return MakePasswordWith(raw, "", "default")
}

// MakePasswordWith hashes raw with the hasher of the given algorithm (Django's
// make_password(password, salt, hasher)). An empty salt means a new random one.
// A given salt needs a SaltedHasher and gives ErrSaltNotSupported otherwise.
// Use a fixed salt in tests only.
func MakePasswordWith(raw, salt, algorithm string) (string, error) {
	h, err := GetHasher(algorithm)
	if err != nil {
		return "", err
	}
	if salt == "" {
		return h.Encode(raw)
	}
	salted, ok := h.(SaltedHasher)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSaltNotSupported, h.Algorithm())
	}
	return salted.EncodeWithSalt(raw, salt)
}

// VerifyPassword reports whether raw matches encoded and whether encoded
// should be replaced by a hash made with the preferred hasher and its current
// parameters (Django's verify_password).
//
// For an unusable password and for a value that no configured hasher made it
// runs the preferred hasher once, so that the answer takes about as long as
// for a real hash and does not tell which accounts have a password.
func VerifyPassword(raw, encoded string) (isCorrect, mustUpdate bool) {
	preferred := hashers()[0]
	hasher, err := IdentifyHasher(encoded)
	if err != nil || !IsPasswordUsable(encoded) {
		if dummy, rerr := randomString(unusablePasswordSuffixLength); rerr == nil {
			_, _ = preferred.Encode(dummy)
		}
		return false, false
	}

	hasherChanged := hasher.Algorithm() != preferred.Algorithm()
	mustUpdate = hasherChanged || preferred.MustUpdate(encoded)
	isCorrect = hasher.Verify(raw, encoded)

	// When the hasher is the preferred one and the hash is outdated, close
	// the gap between the work factor of the stored hash and the current
	// one. A changed hasher gets no such protection, as in Django.
	if !isCorrect && !hasherChanged && mustUpdate {
		hasher.HardenRuntime(raw, encoded)
	}
	return isCorrect, mustUpdate
}

// CheckPassword reports whether raw matches the encoded password (Django's
// check_password). An unusable or malformed value matches nothing.
func CheckPassword(raw, encoded string) bool {
	isCorrect, _ := VerifyPassword(raw, encoded)
	return isCorrect
}

// CheckPasswordAndUpgrade is CheckPassword with Django's setter argument: when
// raw is correct and encoded is outdated (another hasher than the preferred
// one, or other parameters), it calls setter with raw so that the caller
// stores a new hash. An error of setter is returned next to the result of the
// check.
func CheckPasswordAndUpgrade(raw, encoded string, setter func(raw string) error) (bool, error) {
	isCorrect, mustUpdate := VerifyPassword(raw, encoded)
	if setter != nil && isCorrect && mustUpdate {
		if err := setter(raw); err != nil {
			return true, err
		}
	}
	return isCorrect, nil
}

// unusablePasswordPrefix marks a password that no input matches. It never
// occurs at the start of an encoded hash (Django's UNUSABLE_PASSWORD_PREFIX).
const unusablePasswordPrefix = "!"

// unusablePasswordSuffixLength is the number of random characters after the
// prefix. They keep a password reset token of one user from working for
// another user without a password.
const unusablePasswordSuffixLength = 40

// MakeUnusablePassword returns a value for the password column that
// CheckPassword never accepts: "!" followed by random characters (Django's
// make_password(None)).
func MakeUnusablePassword() (string, error) {
	suffix, err := randomString(unusablePasswordSuffixLength)
	if err != nil {
		return "", err
	}
	return unusablePasswordPrefix + suffix, nil
}

// IsPasswordUsable reports whether encoded was not made by
// MakeUnusablePassword (Django's is_password_usable). As in Django, the empty
// string and a value that is no valid hash count as usable; CheckPassword
// still rejects every input for them.
func IsPasswordUsable(encoded string) bool {
	return !strings.HasPrefix(encoded, unusablePasswordPrefix)
}

// randomChars is the alphabet of generated salts and tokens (Django's
// RANDOM_STRING_CHARS).
const randomChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString returns n characters drawn uniformly from randomChars with the
// system's secure random source (Django's get_random_string).
func randomString(n int) (string, error) {
	out := make([]byte, n)
	limit := big.NewInt(int64(len(randomChars)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("auth: read random data: %w", err)
		}
		out[i] = randomChars[idx.Int64()]
	}
	return string(out), nil
}

// saltEntropy is the number of bits of entropy a salt has to carry (Django's
// BasePasswordHasher.salt_entropy).
const saltEntropy = 128

// bitsPerSaltChar is the entropy of one character drawn from randomChars.
var bitsPerSaltChar = math.Log2(float64(len(randomChars)))

// newSalt returns a random salt of at least saltEntropy bits, which is 22
// characters (Django's BasePasswordHasher.salt).
func newSalt() (string, error) {
	return randomString(int(math.Ceil(saltEntropy / bitsPerSaltChar)))
}

// mustUpdateSalt reports whether salt is shorter than a salt newSalt makes
// (Django's must_update_salt).
func mustUpdateSalt(salt string) bool {
	return float64(len(salt))*bitsPerSaltChar < saltEntropy
}

// checkSalt returns ErrInvalidSalt for a salt that EncodeWithSalt must not
// take (Django's _check_encode_args).
func checkSalt(salt string) error {
	if salt == "" || strings.Contains(salt, "$") {
		return ErrInvalidSalt
	}
	return nil
}

// constantTimeEqual compares two encoded values without leaking the position
// of the first difference (Django's constant_time_compare).
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
