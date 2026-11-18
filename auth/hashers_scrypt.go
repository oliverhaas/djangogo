package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"
)

// Parameters of the scrypt hasher in Django 6.0.
const (
	// ScryptWorkFactor is the CPU and memory cost N.
	ScryptWorkFactor = 1 << 14
	// ScryptBlockSize is the block size r.
	ScryptBlockSize = 8
	// ScryptParallelism is the parallelization p.
	ScryptParallelism = 5
)

// scryptKeyLen is the length of the derived key in bytes.
const scryptKeyLen = 64

// ScryptHasher hashes with scrypt (Django's ScryptPasswordHasher). The encoded
// value is "scrypt$<work factor>$<salt>$<block size>$<parallelism>$<base64
// hash>".
type ScryptHasher struct {
	// WorkFactor is N, a power of two. Zero means ScryptWorkFactor.
	WorkFactor int
	// BlockSize is r. Zero means ScryptBlockSize.
	BlockSize int
	// Parallelism is p. Zero means ScryptParallelism.
	Parallelism int
}

// Algorithm returns "scrypt".
func (ScryptHasher) Algorithm() string { return "scrypt" }

// Salt returns a new random salt.
func (ScryptHasher) Salt() (string, error) { return newSalt() }

// Encode hashes password with a new random salt.
func (h ScryptHasher) Encode(password string) (string, error) { return encodeWithNewSalt(h, password) }

// EncodeWithSalt hashes password with the given salt.
func (h ScryptHasher) EncodeWithSalt(password, salt string) (string, error) {
	n, r, p := h.params()
	return h.encode(password, salt, n, r, p)
}

// Verify reports whether password matches encoded.
func (h ScryptHasher) Verify(password, encoded string) bool {
	n, salt, r, p, ok := h.decode(encoded)
	if !ok {
		return false
	}
	again, err := h.encode(password, salt, n, r, p)
	return err == nil && constantTimeEqual(encoded, again)
}

// MustUpdate reports whether encoded was made with other parameters than the
// hasher has.
func (h ScryptHasher) MustUpdate(encoded string) bool {
	n, _, r, p, ok := h.decode(encoded)
	if !ok {
		return false
	}
	wantN, wantR, wantP := h.params()
	return n != wantN || r != wantR || p != wantP
}

// HardenRuntime does nothing: the runtime of scrypt does not grow in a way
// that a number of extra runs could bridge.
func (ScryptHasher) HardenRuntime(string, string) {}

// params returns the configured parameters.
func (h ScryptHasher) params() (n, r, p int) {
	n, r, p = h.WorkFactor, h.BlockSize, h.Parallelism
	if n <= 0 {
		n = ScryptWorkFactor
	}
	if r <= 0 {
		r = ScryptBlockSize
	}
	if p <= 0 {
		p = ScryptParallelism
	}
	return n, r, p
}

// encode derives the key with the given parameters.
func (h ScryptHasher) encode(password, salt string, n, r, p int) (string, error) {
	if err := checkSalt(salt); err != nil {
		return "", err
	}
	key, err := scrypt.Key([]byte(password), []byte(salt), n, r, p, scryptKeyLen)
	if err != nil {
		return "", fmt.Errorf("auth: derive scrypt key: %w", err)
	}
	return fmt.Sprintf("%s$%d$%s$%d$%d$%s", h.Algorithm(), n, salt, r, p, base64.StdEncoding.EncodeToString(key)), nil
}

// decode splits encoded into its parameters and the salt.
func (h ScryptHasher) decode(encoded string) (n int, salt string, r, p int, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != h.Algorithm() {
		return 0, "", 0, 0, false
	}
	n, errN := strconv.Atoi(parts[1])
	r, errR := strconv.Atoi(parts[3])
	p, errP := strconv.Atoi(parts[4])
	if errN != nil || errR != nil || errP != nil {
		return 0, "", 0, 0, false
	}
	return n, parts[2], r, p, true
}

// Parameters of the Argon2 hasher in Django 6.0.
const (
	// Argon2TimeCost is the number of passes over the memory.
	Argon2TimeCost = 2
	// Argon2MemoryCost is the memory in KiB.
	Argon2MemoryCost = 102400
	// Argon2Parallelism is the number of lanes.
	Argon2Parallelism = 8
)

// argon2KeyLen is the length of the derived key in bytes, the default of the
// argon2-cffi library that Django uses.
const argon2KeyLen = 32

// Names of the Argon2 variants in the encoded value.
const (
	argon2id = "argon2id"
	argon2i  = "argon2i"
)

// Argon2Hasher hashes with Argon2id (Django's Argon2PasswordHasher). The
// encoded value is "argon2$argon2id$v=19$m=<memory>,t=<time>,p=<parallelism>$
// <base64 salt>$<base64 hash>", the PHC string of the reference library behind
// the algorithm name.
//
// It verifies Argon2id and Argon2i hashes of version 1.3 (v=19). Hashes of
// version 1.0 and of Argon2d, which Django reads through the C library, match
// nothing here.
type Argon2Hasher struct {
	// TimeCost is the number of passes. Zero means Argon2TimeCost.
	TimeCost int
	// MemoryCost is the memory in KiB. Zero means Argon2MemoryCost.
	MemoryCost int
	// Parallelism is the number of lanes, 255 at most. Zero means
	// Argon2Parallelism.
	Parallelism int
}

// Algorithm returns "argon2".
func (Argon2Hasher) Algorithm() string { return "argon2" }

// Salt returns a new random salt.
func (Argon2Hasher) Salt() (string, error) { return newSalt() }

// Encode hashes password with a new random salt.
func (h Argon2Hasher) Encode(password string) (string, error) { return encodeWithNewSalt(h, password) }

// EncodeWithSalt hashes password with the given salt.
func (h Argon2Hasher) EncodeWithSalt(password, salt string) (string, error) {
	if err := checkSalt(salt); err != nil {
		return "", err
	}
	p := h.params()
	p.salt = []byte(salt)
	if !p.valid() {
		return "", fmt.Errorf("auth: argon2: parameters out of range")
	}
	key := p.key(password, argon2KeyLen)
	return fmt.Sprintf("%s$%s$v=%d$m=%d,t=%d,p=%d$%s$%s", h.Algorithm(), p.variety, argon2.Version,
		p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(p.salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify reports whether password matches encoded.
func (h Argon2Hasher) Verify(password, encoded string) bool {
	p, ok := h.decode(encoded)
	if !ok || p.version != argon2.Version || !p.valid() || len(p.hash) == 0 {
		return false
	}
	key := p.key(password, len(p.hash))
	return subtle.ConstantTimeCompare(key, p.hash) == 1
}

// MustUpdate reports whether encoded was made with another variant, version
// or parameters than the hasher has, or with a salt of less than 128 bits.
func (h Argon2Hasher) MustUpdate(encoded string) bool {
	got, ok := h.decode(encoded)
	if !ok {
		return false
	}
	want := h.params()
	changed := got.variety != want.variety || got.version != argon2.Version ||
		got.memory != want.memory || got.time != want.time || got.threads != want.threads ||
		len(got.hash) != argon2KeyLen
	return changed || mustUpdateSalt(string(got.salt))
}

// HardenRuntime does nothing: the runtime of Argon2 does not grow in a way
// that a number of extra runs could bridge.
func (Argon2Hasher) HardenRuntime(string, string) {}

// params returns the configured parameters.
func (h Argon2Hasher) params() argon2Params {
	p := argon2Params{variety: argon2id, version: argon2.Version, time: h.TimeCost, memory: h.MemoryCost, threads: h.Parallelism}
	if p.time <= 0 {
		p.time = Argon2TimeCost
	}
	if p.memory <= 0 {
		p.memory = Argon2MemoryCost
	}
	if p.threads <= 0 {
		p.threads = Argon2Parallelism
	}
	return p
}

// decode parses the PHC string behind the algorithm name. A hash without the
// version field is of version 1.0 (16).
func (h Argon2Hasher) decode(encoded string) (argon2Params, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) < 5 || parts[0] != h.Algorithm() {
		return argon2Params{}, false
	}
	p := argon2Params{variety: parts[1], version: 16}
	rest := parts[2:]
	if v, ok := strings.CutPrefix(rest[0], "v="); ok {
		version, err := strconv.Atoi(v)
		if err != nil {
			return argon2Params{}, false
		}
		p.version = version
		rest = rest[1:]
	}
	if len(rest) != 3 {
		return argon2Params{}, false
	}
	for field := range strings.SplitSeq(rest[0], ",") {
		name, value, _ := strings.Cut(field, "=")
		n, err := strconv.Atoi(value)
		if err != nil {
			return argon2Params{}, false
		}
		switch name {
		case "m":
			p.memory = n
		case "t":
			p.time = n
		case "p":
			p.threads = n
		default:
			return argon2Params{}, false
		}
	}
	var err error
	if p.salt, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(rest[1], "=")); err != nil {
		return argon2Params{}, false
	}
	if p.hash, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(rest[2], "=")); err != nil {
		return argon2Params{}, false
	}
	return p, true
}

// argon2Params is a decoded Argon2 hash, or the parameters to make one.
type argon2Params struct {
	// variety is "argon2id", "argon2i" or "argon2d".
	variety string
	// version is the Argon2 version, 19 for 1.3.
	version int
	// time, memory (KiB) and threads are the cost parameters.
	time, memory, threads int
	// salt and hash are the raw bytes.
	salt, hash []byte
}

// valid reports whether the key function accepts the parameters. It panics on
// a time or thread count below one, so a stored value is checked first.
func (p argon2Params) valid() bool {
	if p.variety != argon2id && p.variety != argon2i {
		return false
	}
	return p.time >= 1 && p.time <= math.MaxUint32 &&
		p.memory >= 1 && p.memory <= math.MaxUint32 &&
		p.threads >= 1 && p.threads <= math.MaxUint8
}

// key derives keyLen bytes from password with the parameters. Call valid
// first.
func (p argon2Params) key(password string, keyLen int) []byte {
	if p.variety == argon2i {
		return argon2.Key([]byte(password), p.salt, uint32(p.time), uint32(p.memory), uint8(p.threads), uint32(keyLen))
	}
	return argon2.IDKey([]byte(password), p.salt, uint32(p.time), uint32(p.memory), uint8(p.threads), uint32(keyLen))
}
