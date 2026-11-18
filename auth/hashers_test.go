package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// TestVerifiesHashesMadeByDjango checks values that Django 6.0.3 produced for
// the password "lètmein" with its default parameters: make_password with a
// random salt for PBKDF2 and hasher.encode with a bcrypt salt for the bcrypt
// hashers.
func TestVerifiesHashesMadeByDjango(t *testing.T) {
	cases := []struct {
		hasher  auth.Hasher
		encoded string
	}{
		{auth.PBKDF2Hasher{}, "pbkdf2_sha256$1200000$UYZyoHm23dyIoZdtNfgez1$yq222VJ/mRaxyaWdou03vqv8crC9i32x2Iba5YrOQ2o="},
		{auth.BCryptSHA256Hasher{}, "bcrypt_sha256$$2b$12$mSnmLQyyRDbb9ZXVnmHJseINsyeY5K5yPZkjSjZvjSiX6AKuB0ydG"},
		{auth.BCryptHasher{}, "bcrypt$$2b$12$mSnmLQyyRDbb9ZXVnmHJselyewBFkAvvt0eJ852575dkovTpu4fUm"},
	}
	for _, c := range cases {
		// The hashers are used directly, without the shared configuration,
		// so the slow checks can run side by side.
		t.Run(c.hasher.Algorithm(), func(t *testing.T) {
			t.Parallel()
			if !c.hasher.Verify("lètmein", c.encoded) {
				t.Error("the password of the Django hash does not verify")
			}
			if c.hasher.Verify("lètmeinz", c.encoded) {
				t.Error("a wrong password verifies")
			}
			if c.hasher.MustUpdate(c.encoded) {
				t.Error("a hash with Django's current parameters must not be updated")
			}
		})
	}
}

func TestDefaultHasherIsDjangos(t *testing.T) {
	useHashers(t)
	hasher, err := auth.GetHasher("default")
	must(t, err)
	djangotest.AssertEqual(t, hasher.Algorithm(), "pbkdf2_sha256")
	algorithms := make([]string, 0, 5)
	for _, h := range auth.DefaultPasswordHashers() {
		algorithms = append(algorithms, h.Algorithm())
	}
	djangotest.AssertSequenceEqual(t, algorithms,
		[]string{"pbkdf2_sha256", "pbkdf2_sha1", "argon2", "bcrypt_sha256", "scrypt"})
}

func TestCheckPasswordMalformed(t *testing.T) {
	useHashers(t, append(cheapDefaults(), auth.BCryptHasher{Rounds: 4}, auth.MD5Hasher{})...)
	cases := []string{
		"",
		"plaintext",
		"$$$",
		"pbkdf2_sha256$1000$salt", // too few fields
		"pbkdf2_sha256$notanint$salt$hash",
		"pbkdf2_sha256$-5$salt$hash",
		"pbkdf2_sha256$0$salt$hash",
		"pbkdf2_sha1$1000",
		"md5$salt",
		"bcrypt$",
		"bcrypt$not-a-bcrypt-hash",
		"bcrypt_sha256$$2b$99$short",
		"scrypt$16$salt$8$5",
		"scrypt$x$salt$8$5$hash",
		"scrypt$3$salt$8$5$hash", // not a power of two
		"argon2$argon2id$v=19$m=64,t=0,p=1$c2FsdHNhbHQ$YC9+jJCrQhs5R6db7LlN8Q",
		"argon2$argon2id$v=19$m=64,t=1,p=0$c2FsdHNhbHQ$YC9+jJCrQhs5R6db7LlN8Q",
		"argon2$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$",
		"argon2$argon2d$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$YC9+jJCrQhs5R6db7LlN8Q",
		"argon2$argon2id$v=19$m=64,t=1$c2FsdHNhbHQ",
		"argon2$argon2id$v=19$x=1$c2FsdHNhbHQ$YC9+jJCrQhs5R6db7LlN8Q",
		"argon2$",
	}
	for _, encoded := range cases {
		if auth.CheckPassword("anything", encoded) {
			t.Errorf("CheckPassword(%q) = true, want false", encoded)
		}
		if h, err := auth.IdentifyHasher(encoded); err == nil {
			// Neither call may panic on a malformed value.
			h.MustUpdate(encoded)
			h.HardenRuntime("anything", encoded)
		}
	}
}

func TestMakePasswordWithSaltNeedsSaltedHasher(t *testing.T) {
	useHashers(t, auth.BCryptSHA256Hasher{Rounds: 4})
	_, err := auth.MakePasswordWith("secret", "seasalt", "bcrypt_sha256")
	djangotest.AssertErrorIs(t, err, auth.ErrSaltNotSupported)
}

func TestPlainBCryptRejectsLongPassword(t *testing.T) {
	hasher := auth.BCryptHasher{Rounds: 4}
	long := strings.Repeat("a", 73)
	if _, err := hasher.Encode(long); err == nil {
		t.Fatal("Encode accepted a password of more than 72 bytes")
	}
	// bcrypt reads the first 72 bytes when it checks, as in Django.
	encoded, err := hasher.Encode(long[:72])
	must(t, err)
	djangotest.AssertEqual(t, hasher.Verify(long[:72], encoded), true)
	djangotest.AssertEqual(t, hasher.Verify(long, encoded), true)
	djangotest.AssertEqual(t, hasher.Verify(long[:71], encoded), false)
}

func TestHardenRuntimeKeepsTheStoredValue(t *testing.T) {
	// The extra work is not observable from outside; the calls must at
	// least accept a value with a lower work factor.
	pbkdf2Old, err := auth.PBKDF2Hasher{Iterations: 1}.Encode("letmein")
	must(t, err)
	auth.PBKDF2Hasher{Iterations: 6}.HardenRuntime("wrong_password", pbkdf2Old)

	bcryptOld, err := auth.BCryptHasher{Rounds: 4}.Encode("letmein")
	must(t, err)
	auth.BCryptHasher{Rounds: 6}.HardenRuntime("wrong_password", bcryptOld)
	djangotest.AssertEqual(t, auth.BCryptHasher{Rounds: 6}.MustUpdate(bcryptOld), true)
}

func TestAuthenticateUpgradesOutdatedHash(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		useHashers(t, auth.MD5Hasher{})
		u := createUser(t, db, "old", "s3cret")
		djangotest.AssertEqual(t, strings.HasPrefix(u.Password, "md5$"), true)

		useHashers(t, auth.PBKDF2Hasher{Iterations: 1000}, auth.MD5Hasher{})

		// A wrong password changes nothing.
		_, err := auth.Authenticate(ctx, db, "old", "wrong")
		djangotest.AssertErrorIs(t, err, auth.ErrInvalidCredentials)
		djangotest.AssertEqual(t, reload(t, db, u.ID).Password, u.Password)

		got, err := auth.Authenticate(ctx, db, "old", "s3cret")
		must(t, err)
		stored := reload(t, db, u.ID).Password
		djangotest.AssertEqual(t, strings.HasPrefix(stored, "pbkdf2_sha256$1000$"), true)
		djangotest.AssertEqual(t, got.Password, stored)
		djangotest.AssertEqual(t, auth.CheckPassword("s3cret", stored), true)

		// A current hash is left alone.
		_, err = auth.Authenticate(ctx, db, "old", "s3cret")
		must(t, err)
		djangotest.AssertEqual(t, reload(t, db, u.ID).Password, stored)
	})
}
