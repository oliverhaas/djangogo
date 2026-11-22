package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
)

// fixedClock returns a Now function for PasswordResetTokenGenerator that
// always returns t (Django's MockedPasswordResetTokenGenerator).
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// useResetTimeout sets Config.PasswordResetTimeout for the test.
func useResetTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordResetTimeout = d
	t.Cleanup(auth.Configure(cfg))
}

// tokenUser creates the user of the token tests.
func tokenUser(t *testing.T, db *orm.DB, username, email string) *auth.User {
	t.Helper()
	u, err := auth.CreateUser(context.Background(), db, username, email, "testpw", false, false, true)
	must(t, err)
	return u
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_make_token
func TestDjangoTokenGeneratorMakeToken(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		p0 := &auth.PasswordResetTokenGenerator{}
		tk1 := p0.MakeToken(user)
		if !p0.CheckToken(user, tk1) {
			t.Errorf("CheckToken(%q) = false, want true", tk1)
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_10265
//
// The token generated for a user created in the same request will work
// correctly.
func TestDjangoTokenGenerator10265(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "comebackkid", "test3@example.com")
		userReload := reload(t, db, user.ID)
		p0 := &auth.PasswordResetTokenGenerator{Now: fixedClock(time.Now())}
		if tk1, tk2 := p0.MakeToken(user), p0.MakeToken(userReload); tk1 != tk2 {
			t.Errorf("tokens differ: %q and %q", tk1, tk2)
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_token_with_different_email
//
// Updating the user email address invalidates the token. The subtests with
// the CustomEmailField model need a custom user model.
func TestDjangoTokenGeneratorTokenWithDifferentEmail(t *testing.T) {
	t.Run("CustomEmailField", func(t *testing.T) {
		t.Skip("needs a custom user model with another email field, which djangogo does not have")
	})
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "changeemailuser", "test4@example.com")
		p0 := &auth.PasswordResetTokenGenerator{}
		tk1 := p0.MakeToken(user)
		if !p0.CheckToken(user, tk1) {
			t.Fatalf("CheckToken(%q) = false, want true", tk1)
		}
		user.Email = "test4new@example.com"
		_, err := orm.Query[auth.User](db).Filter("id", user.ID).Update(context.Background(), "email", user.Email)
		must(t, err)
		if p0.CheckToken(user, tk1) {
			t.Error("CheckToken after the email change = true, want false")
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_timeout
//
// The token is valid after n seconds, but no greater.
func TestDjangoTokenGeneratorTimeout(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		now := time.Now()
		p0 := &auth.PasswordResetTokenGenerator{Now: fixedClock(now)}
		tk1 := p0.MakeToken(user)
		check := func(after time.Duration, want bool) {
			t.Helper()
			p := &auth.PasswordResetTokenGenerator{Now: fixedClock(now.Add(after))}
			if got := p.CheckToken(user, tk1); got != want {
				t.Errorf("CheckToken after %v = %v, want %v", after, got, want)
			}
		}
		check(auth.DefaultPasswordResetTimeout, true)
		check(auth.DefaultPasswordResetTimeout+time.Second, false)
		useResetTimeout(t, time.Hour)
		check(time.Hour, true)
		check(time.Hour+time.Second, false)
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_check_token_with_nonexistent_token_and_user
func TestDjangoTokenGeneratorCheckTokenWithNonexistentTokenAndUser(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		p0 := &auth.PasswordResetTokenGenerator{}
		tk1 := p0.MakeToken(user)
		if p0.CheckToken(nil, tk1) {
			t.Error("CheckToken(nil, token) = true, want false")
		}
		if p0.CheckToken(user, "") {
			t.Error(`CheckToken(user, "") = true, want false`)
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_token_with_different_secret
//
// A valid token can be created with a secret other than SECRET_KEY by using
// the PasswordResetTokenGenerator.Secret field.
func TestDjangoTokenGeneratorTokenWithDifferentSecret(t *testing.T) {
	useSecret(t, "settings-secret-key")
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		p0 := &auth.PasswordResetTokenGenerator{Secret: "abcdefghijkl"}
		tk0 := p0.MakeToken(user)
		if !p0.CheckToken(user, tk0) {
			t.Fatal("CheckToken with the new secret = false, want true")
		}
		p1 := &auth.PasswordResetTokenGenerator{}
		tk1 := p1.MakeToken(user)
		if p0.CheckToken(user, tk1) {
			t.Error("a token of SECRET_KEY checks out with the new secret")
		}
		if p1.CheckToken(user, tk0) {
			t.Error("a token of the new secret checks out with SECRET_KEY")
		}
	})
}

// No Go counterpart: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_token_with_different_secret_subclass
// Go has no subclass with a class attribute; the Secret field is what
// test_token_with_different_secret covers.

// No Go counterpart: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_secret_lazy_validation
// An empty Config.SecretKey means a random key per process, not an
// ImproperlyConfigured error.

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_check_token_secret_fallbacks
func TestDjangoTokenGeneratorCheckTokenSecretFallbacks(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		p1 := &auth.PasswordResetTokenGenerator{Secret: "secret"}
		tk := p1.MakeToken(user)
		p2 := &auth.PasswordResetTokenGenerator{Secret: "newsecret", SecretFallbacks: []string{"secret"}}
		if !p1.CheckToken(user, tk) {
			t.Error("CheckToken with the secret = false, want true")
		}
		if !p2.CheckToken(user, tk) {
			t.Error("CheckToken with the fallback = false, want true")
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_check_token_secret_key_fallbacks
func TestDjangoTokenGeneratorCheckTokenSecretKeyFallbacks(t *testing.T) {
	useSecret(t, "secret", "oldsecret")
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		tk := (&auth.PasswordResetTokenGenerator{Secret: "oldsecret"}).MakeToken(user)
		if !(&auth.PasswordResetTokenGenerator{}).CheckToken(user, tk) {
			t.Error("CheckToken with SECRET_KEY_FALLBACKS = false, want true")
		}
	})
}

// Ported from Django: tests/auth_tests/test_tokens.py::TokenGeneratorTest.test_check_token_secret_key_fallbacks_override
func TestDjangoTokenGeneratorCheckTokenSecretKeyFallbacksOverride(t *testing.T) {
	useSecret(t, "secret", "oldsecret")
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := tokenUser(t, db, "tokentestuser", "test2@example.com")
		tk := (&auth.PasswordResetTokenGenerator{Secret: "oldsecret"}).MakeToken(user)
		p2 := &auth.PasswordResetTokenGenerator{SecretFallbacks: []string{}}
		if p2.CheckToken(user, tk) {
			t.Error("CheckToken with empty SecretFallbacks = true, want false")
		}
	})
}

// No Django test exists: a token that Django 6.0.3 made checks out here and
// the generator makes the same token, with and without a last login. The
// tokens come from MockedPasswordResetTokenGenerator at 2026-01-02 03:04:05
// with SECRET_KEY "oracle-secret" and TIME_ZONE "Europe/Berlin".
func TestTokenGeneratorMatchesDjango(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	must(t, err)
	cfg := auth.CurrentConfig()
	cfg.Location = berlin
	t.Cleanup(auth.Configure(cfg))
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, berlin)
	user := &auth.User{ID: 7, Password: "pbkdf2_sha256$1000000$salt$hash=", Email: "alice@example.com"}
	g := &auth.PasswordResetTokenGenerator{Secret: "oracle-secret", Now: fixedClock(now)}
	if got, want := g.MakeToken(user), "d1rcit-82b88db026298f538b6e328f1a71d75a"; got != want {
		t.Errorf("MakeToken without last login = %q, want %q", got, want)
	}
	login := time.Date(2025, 3, 4, 5, 6, 7, 890000000, time.UTC)
	user.LastLogin = &login
	want := "d1rcit-f0fe21000cb3a26b7c8571fbed110975"
	if got := g.MakeToken(user); got != want {
		t.Errorf("MakeToken with last login = %q, want %q", got, want)
	}
	later := &auth.PasswordResetTokenGenerator{Secret: "oracle-secret", Now: fixedClock(now.Add(time.Hour))}
	if !later.CheckToken(user, want) {
		t.Errorf("CheckToken(%q) = false, want true", want)
	}
}
