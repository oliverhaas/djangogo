package auth_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
)

// useHashers sets Config.PasswordHashers for the test (Django's
// override_settings(PASSWORD_HASHERS=...)).
func useHashers(t *testing.T, hashers ...auth.Hasher) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordHashers = hashers
	t.Cleanup(auth.Configure(cfg))
}

// cheapDefaults is Django's default hasher list with small work factors. The
// ports that check behavior and not a stored value use it: one PBKDF2 run at
// 1,200,000 iterations takes more than a second under the race detector.
func cheapDefaults() []auth.Hasher {
	return []auth.Hasher{
		auth.PBKDF2Hasher{Iterations: 1000},
		auth.PBKDF2SHA1Hasher{Iterations: 1000},
		auth.Argon2Hasher{TimeCost: 1, MemoryCost: 64, Parallelism: 1},
		auth.BCryptSHA256Hasher{Rounds: 4},
		auth.ScryptHasher{WorkFactor: 1 << 4},
	}
}

// makePassword is auth.MakePasswordWith that fails the test on an error.
func makePassword(t *testing.T, raw, salt, algorithm string) string {
	t.Helper()
	encoded, err := auth.MakePasswordWith(raw, salt, algorithm)
	must(t, err)
	return encoded
}

// algorithmOf returns the algorithm of the hasher that made encoded.
func algorithmOf(t *testing.T, encoded string) string {
	t.Helper()
	h, err := auth.IdentifyHasher(encoded)
	must(t, err)
	return h.Algorithm()
}

// salted returns the configured hasher of the algorithm as a SaltedHasher.
func salted(t *testing.T, algorithm string) auth.SaltedHasher {
	t.Helper()
	h, err := auth.GetHasher(algorithm)
	must(t, err)
	s, ok := h.(auth.SaltedHasher)
	if !ok {
		t.Fatalf("hasher %q takes no salt", algorithm)
	}
	return s
}

// upgradeRecorder is the setter of the Django tests: it records that the
// password was handed back for a new hash.
type upgradeRecorder struct{ upgraded bool }

// set is the setter.
func (r *upgradeRecorder) set(string) error {
	r.upgraded = true
	return nil
}

// checkWithSetter is check_password(password, encoded, setter).
func checkWithSetter(t *testing.T, raw, encoded string, r *upgradeRecorder) bool {
	t.Helper()
	ok, err := auth.CheckPasswordAndUpgrade(raw, encoded, r.set)
	must(t, err)
	return ok
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_simple
func TestHashPassSimple(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	encoded := makePassword(t, "lètmein", "", "default")
	djangotest.AssertEqual(t, strings.HasPrefix(encoded, "pbkdf2_sha256$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	// Blank passwords
	blank := makePassword(t, "", "", "default")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "pbkdf2_sha256$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_pbkdf2
//
// The expected value was made by Django 6.0.3, whose work factor is 1,200,000
// iterations. The blank password and the salt checks run with a small work
// factor.
func TestHashPassPBKDF2(t *testing.T) {
	useHashers(t, auth.DefaultPasswordHashers()...)
	encoded := makePassword(t, "lètmein", "seasalt", "pbkdf2_sha256")
	djangotest.AssertEqual(t, encoded, "pbkdf2_sha256$1200000$seasalt$6sTlFi4QohxXLuZigqDIUNX8xG9NxrTmV8+flFQdBqE=")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "pbkdf2_sha256")

	useHashers(t, cheapDefaults()...)
	// Blank passwords
	blank := makePassword(t, "", "seasalt", "pbkdf2_sha256")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "pbkdf2_sha256$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
	// Salt entropy check.
	hasher := salted(t, "pbkdf2_sha256")
	weak := makePassword(t, "lètmein", "iodizedsalt", "pbkdf2_sha256")
	salt, err := hasher.Salt()
	must(t, err)
	strong := makePassword(t, "lètmein", salt, "pbkdf2_sha256")
	djangotest.AssertEqual(t, hasher.MustUpdate(weak), true)
	djangotest.AssertEqual(t, hasher.MustUpdate(strong), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_md5
func TestHashPassMD5(t *testing.T) {
	useHashers(t, auth.MD5Hasher{})
	encoded := makePassword(t, "lètmein", "seasalt", "md5")
	djangotest.AssertEqual(t, encoded, "md5$seasalt$3f86d0d3d465b7b458c231bf3555c0e3")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "md5")
	// Blank passwords
	blank := makePassword(t, "", "seasalt", "md5")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "md5$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
	// Salt entropy check.
	hasher := salted(t, "md5")
	weak := makePassword(t, "lètmein", "iodizedsalt", "md5")
	salt, err := hasher.Salt()
	must(t, err)
	strong := makePassword(t, "lètmein", salt, "md5")
	djangotest.AssertEqual(t, hasher.MustUpdate(weak), true)
	djangotest.AssertEqual(t, hasher.MustUpdate(strong), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bcrypt_sha256
//
// The hashes are made with 4 rounds to keep the test fast.
func TestHashPassBCryptSHA256(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	encoded := makePassword(t, "lètmein", "", "bcrypt_sha256")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, strings.HasPrefix(encoded, "bcrypt_sha256$"), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "bcrypt_sha256")

	// password truncation no longer works
	password := "VSK0UYV6FFQVZ0KG88DYN9WADAADZO1CTSIVDJUNZSUML6IBX7LN7ZS3R5" +
		"JGB3RGZ7VI7G7DJQ9NI8BQFSRPTG6UWTTVESA5ZPUN"
	encoded = makePassword(t, password, "", "bcrypt_sha256")
	djangotest.AssertEqual(t, auth.CheckPassword(password, encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword(password[:72], encoded), false)
	// Blank passwords
	blank := makePassword(t, "", "", "bcrypt_sha256")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "bcrypt_sha256$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bcrypt
//
// The hashes are made with 4 rounds to keep the test fast.
func TestHashPassBCrypt(t *testing.T) {
	useHashers(t, auth.BCryptHasher{Rounds: 4})
	encoded := makePassword(t, "lètmein", "", "bcrypt")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, strings.HasPrefix(encoded, "bcrypt$"), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "bcrypt")
	// Blank passwords
	blank := makePassword(t, "", "", "bcrypt")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "bcrypt$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bcrypt_upgrade
//
// Django changes the rounds of the hasher in place. Here the hasher list is
// configured twice, with 4 rounds and with the default.
func TestHashPassBCryptUpgrade(t *testing.T) {
	useHashers(t, auth.BCryptHasher{Rounds: 4})
	// Generate a password with 4 rounds.
	encoded := makePassword(t, "letmein", "", "bcrypt")
	djangotest.AssertEqual(t, strings.HasPrefix(encoded, "bcrypt$$2a$04$"), true)

	var state upgradeRecorder
	// No upgrade is triggered.
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, false)

	// Revert to the old rounds count and ...
	useHashers(t, auth.BCryptHasher{})
	djangotest.AssertNotEqual(t, auth.BCryptRounds, 4)

	// ... check if the password would get updated to the new count.
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_unusable
//
// Django's check_password(None, encoded) has no counterpart: a Go string is
// never None.
func TestHashPassUnusable(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	encoded, err := auth.MakeUnusablePassword()
	must(t, err)
	djangotest.AssertEqual(t, len(encoded), len("!")+40)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword(encoded, encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword("!", encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword("", encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	_, err = auth.IdentifyHasher(encoded)
	djangotest.AssertErrorIs(t, err, auth.ErrUnknownHasher)
	// Assert that the unusable passwords actually contain a random part.
	// This might fail one day due to a hash collision.
	other, err := auth.MakeUnusablePassword()
	must(t, err)
	djangotest.AssertNotEqual(t, encoded, other)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bad_algorithm
//
// Django raises ValueError with "Unknown password hashing algorithm 'lolcat'.
// Did you specify it in the PASSWORD_HASHERS setting?". The Go error wraps
// ErrUnknownHasher and names the algorithm.
func TestHashPassBadAlgorithm(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	_, err := auth.MakePasswordWith("lètmein", "", "lolcat")
	djangotest.AssertErrorIs(t, err, auth.ErrUnknownHasher)
	djangotest.AssertEqual(t, strings.Contains(err.Error(), `"lolcat"`), true)
	_, err = auth.IdentifyHasher("lolcat$salt$hash")
	djangotest.AssertErrorIs(t, err, auth.ErrUnknownHasher)
	djangotest.AssertEqual(t, strings.Contains(err.Error(), `"lolcat"`), true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_is_password_usable
//
// Django also passes None, which a Go string cannot be.
func TestHashPassIsPasswordUsable(t *testing.T) {
	for _, password := range []string{"lètmein_badencoded", ""} {
		djangotest.AssertEqual(t, auth.IsPasswordUsable(password), true)
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_low_level_pbkdf2
//
// The expected value was made by Django 6.0.3 (1,200,000 iterations).
func TestHashPassLowLevelPBKDF2(t *testing.T) {
	t.Parallel()
	hasher := auth.PBKDF2Hasher{}
	encoded, err := hasher.EncodeWithSalt("lètmein", "seasalt2")
	must(t, err)
	djangotest.AssertEqual(t, encoded, "pbkdf2_sha256$1200000$seasalt2$hPlIUc6GqWsws6cZV1K8OuOARm1UrbZ3vLGFoHkH0ZI=")
	djangotest.AssertEqual(t, hasher.Verify("lètmein", encoded), true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_low_level_pbkdf2_sha1
//
// The expected value was made by Django 6.0.3 (1,200,000 iterations).
func TestHashPassLowLevelPBKDF2SHA1(t *testing.T) {
	t.Parallel()
	hasher := auth.PBKDF2SHA1Hasher{}
	encoded, err := hasher.EncodeWithSalt("lètmein", "seasalt2")
	must(t, err)
	djangotest.AssertEqual(t, encoded, "pbkdf2_sha1$1200000$seasalt2$RGU4BAy93u+JDPtuMamdllndh+c=")
	djangotest.AssertEqual(t, hasher.Verify("lètmein", encoded), true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bcrypt_salt_check
//
// The bcrypt library draws the salt, so Encode takes none.
func TestHashPassBCryptSaltCheck(t *testing.T) {
	hasher := auth.BCryptHasher{Rounds: 4}
	encoded, err := hasher.Encode("lètmein")
	must(t, err)
	djangotest.AssertEqual(t, hasher.MustUpdate(encoded), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_bcryptsha256_salt_check
//
// The bcrypt library draws the salt, so Encode takes none.
func TestHashPassBCryptSHA256SaltCheck(t *testing.T) {
	hasher := auth.BCryptSHA256Hasher{Rounds: 4}
	encoded, err := hasher.Encode("lètmein")
	must(t, err)
	djangotest.AssertEqual(t, hasher.MustUpdate(encoded), false)
}

// upgradeHashers is the hasher list of the upgrade tests, with a small work
// factor.
func upgradeHashers() []auth.Hasher {
	return []auth.Hasher{
		auth.PBKDF2Hasher{Iterations: 1000},
		auth.PBKDF2SHA1Hasher{Iterations: 1000},
		auth.MD5Hasher{},
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_upgrade
func TestHashPassUpgrade(t *testing.T) {
	useHashers(t, upgradeHashers()...)
	preferred, err := auth.GetHasher("default")
	must(t, err)
	djangotest.AssertEqual(t, preferred.Algorithm(), "pbkdf2_sha256")
	for _, algo := range []string{"pbkdf2_sha1", "md5"} {
		encoded := makePassword(t, "lètmein", "", algo)
		var state upgradeRecorder
		djangotest.AssertEqual(t, checkWithSetter(t, "lètmein", encoded, &state), true)
		djangotest.AssertEqual(t, state.upgraded, true)
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_no_upgrade
func TestHashPassNoUpgrade(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	encoded := makePassword(t, "lètmein", "", "default")
	var state upgradeRecorder
	djangotest.AssertEqual(t, checkWithSetter(t, "WRONG", encoded, &state), false)
	djangotest.AssertEqual(t, state.upgraded, false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_no_upgrade_on_incorrect_pass
func TestHashPassNoUpgradeOnIncorrectPass(t *testing.T) {
	useHashers(t, upgradeHashers()...)
	for _, algo := range []string{"pbkdf2_sha1", "md5"} {
		encoded := makePassword(t, "lètmein", "", algo)
		var state upgradeRecorder
		djangotest.AssertEqual(t, checkWithSetter(t, "WRONG", encoded, &state), false)
		djangotest.AssertEqual(t, state.upgraded, false)
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_pbkdf2_upgrade
//
// Django changes the iterations of the hasher in place. Here the hasher list
// is configured twice.
func TestHashPassPBKDF2Upgrade(t *testing.T) {
	// Generate a password with 1 iteration.
	useHashers(t, auth.PBKDF2Hasher{Iterations: 1})
	encoded := makePassword(t, "letmein", "", "default")
	djangotest.AssertEqual(t, strings.Split(encoded, "$")[1], "1")

	var state upgradeRecorder
	// No upgrade is triggered
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, false)

	// Revert to the old iteration count and ...
	useHashers(t, auth.PBKDF2Hasher{Iterations: 1000})

	// ... check if the password would get updated to the new iteration
	// count.
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_pbkdf2_upgrade_new_hasher
//
// Two hashers of one algorithm cannot be told apart by the encoded value, in
// Django as here: the first one of the list decides. The second list of the
// Django test therefore reduces to the hasher with the current work factor.
func TestHashPassPBKDF2UpgradeNewHasher(t *testing.T) {
	var state upgradeRecorder

	useHashers(t, auth.PBKDF2Hasher{Iterations: 1})
	encoded := makePassword(t, "letmein", "", "default")
	djangotest.AssertEqual(t, strings.Split(encoded, "$")[1], "1")

	// No upgrade is triggered
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, false)

	// Revert to the old iteration count and check if the password would get
	// updated to the new iteration count.
	useHashers(t, auth.PBKDF2Hasher{Iterations: 1000}, auth.PBKDF2Hasher{Iterations: 1})
	djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, true)
}

// spyHasher is the preferred hasher of the timing tests. It counts the calls
// that Django's tests count with mocks.
type spyHasher struct {
	auth.MD5Hasher
	// mustUpdate is the answer of MustUpdate.
	mustUpdate bool
	// encodes, verifies and hardens count the calls.
	encodes, verifies, hardens *int
}

// Encode counts and hashes.
func (h spyHasher) Encode(password string) (string, error) {
	*h.encodes++
	return h.MD5Hasher.Encode(password)
}

// Verify counts and checks.
func (h spyHasher) Verify(password, encoded string) bool {
	*h.verifies++
	return h.MD5Hasher.Verify(password, encoded)
}

// MustUpdate returns the configured answer.
func (h spyHasher) MustUpdate(string) bool { return h.mustUpdate }

// HardenRuntime counts.
func (h spyHasher) HardenRuntime(string, string) { *h.hardens++ }

// newSpyHasher returns a spy with fresh counters.
func newSpyHasher(mustUpdate bool) spyHasher {
	return spyHasher{mustUpdate: mustUpdate, encodes: new(int), verifies: new(int), hardens: new(int)}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_check_password_calls_harden_runtime
func TestHashPassCheckPasswordCallsHardenRuntime(t *testing.T) {
	hasher := newSpyHasher(true)
	useHashers(t, hasher)
	encoded := makePassword(t, "letmein", "", "default")

	// Correct password supplied, no hardening needed
	auth.CheckPassword("letmein", encoded)
	djangotest.AssertEqual(t, *hasher.hardens, 0)

	// Wrong password supplied, hardening needed
	auth.CheckPassword("wrong_password", encoded)
	djangotest.AssertEqual(t, *hasher.hardens, 1)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_check_password_calls_make_password_to_fake_runtime
//
// Django's first case passes None for both arguments, which Go strings cannot
// be. The third case makes identify_hasher fail with a mock; here the value
// comes from a hasher that is not configured.
func TestHashPassCheckPasswordCallsMakePasswordToFakeRuntime(t *testing.T) {
	unusable, err := auth.MakeUnusablePassword()
	must(t, err)
	foreign, err := auth.PBKDF2Hasher{Iterations: 1}.Encode("letmein")
	must(t, err)
	cases := []struct{ password, encoded string }{
		{"foo", unusable},    // unusable encoded
		{"letmein", foreign}, // valid encoded of a hasher that is not installed
	}
	for _, c := range cases {
		hasher := newSpyHasher(false)
		useHashers(t, hasher)
		djangotest.AssertEqual(t, auth.CheckPassword(c.password, c.encoded), false)
		// Ensure make_password is called to standardize timing.
		djangotest.AssertEqual(t, *hasher.verifies, 0)
		djangotest.AssertEqual(t, *hasher.encodes, 1)
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPass.test_encode_invalid_salt
//
// Django raises ValueError("salt must be provided and cannot contain $.");
// the Go hashers return ErrInvalidSalt. None is not a Go string.
func TestHashPassEncodeInvalidSalt(t *testing.T) {
	hashers := []auth.SaltedHasher{
		auth.MD5Hasher{},
		auth.PBKDF2Hasher{},
		auth.PBKDF2SHA1Hasher{},
		auth.ScryptHasher{},
	}
	for _, hasher := range hashers {
		for _, salt := range []string{"", "sea$salt"} {
			_, err := hasher.EncodeWithSalt("password", salt)
			djangotest.AssertErrorIs(t, err, auth.ErrInvalidSalt)
		}
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPassArgon2.test_argon2
//
// New hashes are made with small parameters. The hash of Argon2 version 1.0
// (no "v=" field) does not verify: the Go library implements version 1.3 only.
func TestHashPassArgon2(t *testing.T) {
	useHashers(t, cheapDefaults()...)
	encoded := makePassword(t, "lètmein", "", "argon2")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, strings.HasPrefix(encoded, "argon2$argon2id$"), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "argon2")
	// Blank passwords
	blank := makePassword(t, "", "", "argon2")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "argon2$argon2id$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
	// Old hashes without version attribute
	encoded = "argon2$argon2i$m=8,t=1,p=1$c29tZXNhbHQ$gwQOXSNhxiOxPOA0+PY10P9QFO" +
		"4NAYysnqRt1GSQLE55m+2GYDt9FEjPMHhP2Cuf0nOEXXMocVrsJAtNSsKyfg"
	djangotest.AssertEqual(t, auth.CheckPassword("secret", encoded), false)
	djangotest.AssertEqual(t, auth.CheckPassword("wrong", encoded), false)
	// Old hashes with version attribute.
	encoded = "argon2$argon2i$v=19$m=8,t=1,p=1$c2FsdHNhbHQ$YC9+jJCrQhs5R6db7LlN8Q"
	djangotest.AssertEqual(t, auth.CheckPassword("secret", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("wrong", encoded), false)
	// Salt entropy check.
	hasher := salted(t, "argon2")
	weak := makePassword(t, "lètmein", "iodizedsalt", "argon2")
	salt, err := hasher.Salt()
	must(t, err)
	strong := makePassword(t, "lètmein", salt, "argon2")
	djangotest.AssertEqual(t, hasher.MustUpdate(weak), true)
	djangotest.AssertEqual(t, hasher.MustUpdate(strong), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPassArgon2.test_argon2_upgrade
//
// Django changes one attribute of the hasher in place. Here the hasher list is
// configured twice per attribute; the current parameters are kept small.
func TestHashPassArgon2Upgrade(t *testing.T) {
	current := auth.Argon2Hasher{TimeCost: 2, MemoryCost: 128, Parallelism: 2}
	olds := []auth.Argon2Hasher{
		{TimeCost: 1, MemoryCost: 128, Parallelism: 2},
		{TimeCost: 2, MemoryCost: 64, Parallelism: 2},
		{TimeCost: 2, MemoryCost: 128, Parallelism: 1},
	}
	for _, old := range olds {
		useHashers(t, old)
		encoded := makePassword(t, "letmein", "", "argon2")

		var state upgradeRecorder
		// No upgrade is triggered.
		djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
		djangotest.AssertEqual(t, state.upgraded, false)

		// Revert to the old rounds count and ...
		useHashers(t, current)

		// ... check if the password would get updated to the new count.
		djangotest.AssertEqual(t, checkWithSetter(t, "letmein", encoded, &state), true)
		djangotest.AssertEqual(t, state.upgraded, true)
	}
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPassArgon2.test_argon2_version_upgrade
//
// The stored value is the one of the Django test, made by argon2-cffi with
// Django's default parameters. Verifying it takes 100 MiB of memory.
func TestHashPassArgon2VersionUpgrade(t *testing.T) {
	useHashers(t, auth.Argon2Hasher{TimeCost: 1, MemoryCost: 8, Parallelism: 1})
	encoded := "argon2$argon2id$v=19$m=102400,t=2,p=8$Y041dExhNkljRUUy$TMa6A8fPJh" +
		"CAUXRhJXCXdw"
	var state upgradeRecorder
	djangotest.AssertEqual(t, checkWithSetter(t, "secret", encoded, &state), true)
	djangotest.AssertEqual(t, state.upgraded, true)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPassScrypt.test_scrypt
//
// The blank password is hashed with a small work factor.
func TestHashPassScrypt(t *testing.T) {
	useHashers(t, auth.DefaultPasswordHashers()...)
	encoded := makePassword(t, "lètmein", "seasalt", "scrypt")
	djangotest.AssertEqual(t, encoded, "scrypt$16384$seasalt$8$5$ECMIUp+LMxMSK8xB/IVyba+KYGTI7FTnet025q/1f"+
		"/vBAVnnP3hdYqJuRi+mJn6ji6ze3Fbb7JEFPKGpuEf5vw==")
	djangotest.AssertEqual(t, auth.IsPasswordUsable(encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmein", encoded), true)
	djangotest.AssertEqual(t, auth.CheckPassword("lètmeinz", encoded), false)
	djangotest.AssertEqual(t, algorithmOf(t, encoded), "scrypt")
	useHashers(t, cheapDefaults()...)
	// Blank passwords.
	blank := makePassword(t, "", "seasalt", "scrypt")
	djangotest.AssertEqual(t, strings.HasPrefix(blank, "scrypt$"), true)
	djangotest.AssertEqual(t, auth.IsPasswordUsable(blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword("", blank), true)
	djangotest.AssertEqual(t, auth.CheckPassword(" ", blank), false)
}

// Ported from Django: tests/auth_tests/test_hashers.py::TestUtilsHashPassScrypt.test_scrypt_upgrade
//
// Django changes one attribute of the hasher in place. Here the hasher list is
// configured twice per attribute; the current parameters are kept small.
func TestHashPassScryptUpgrade(t *testing.T) {
	current := auth.ScryptHasher{WorkFactor: 1 << 5, BlockSize: 8, Parallelism: 3}
	olds := []auth.ScryptHasher{
		{WorkFactor: 1 << 4, BlockSize: 8, Parallelism: 3},
		{WorkFactor: 1 << 5, BlockSize: 10, Parallelism: 3},
		{WorkFactor: 1 << 5, BlockSize: 8, Parallelism: 2},
	}
	for _, old := range olds {
		useHashers(t, old)
		encoded := makePassword(t, "lètmein", "seasalt", "scrypt")

		var state upgradeRecorder
		// No update is triggered.
		djangotest.AssertEqual(t, checkWithSetter(t, "lètmein", encoded, &state), true)
		djangotest.AssertEqual(t, state.upgraded, false)
		// Revert to the old value.
		useHashers(t, current)
		// Password is updated.
		djangotest.AssertEqual(t, checkWithSetter(t, "lètmein", encoded, &state), true)
		djangotest.AssertEqual(t, state.upgraded, true)
	}
}
