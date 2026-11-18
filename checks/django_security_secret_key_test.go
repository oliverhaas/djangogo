package checks_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are the ports of CheckSecretKeyTest and
// CheckSecretKeyFallbacksTest of Django's
// tests/check_framework/test_security.py. The other classes of that file are
// in django_security_test.go.

// secW025 is base.W025 with its placeholder filled, as the Django tests build
// it: Warning(base.W025.msg % name, id=base.W025.id).
func secW025(name string) checks.Message {
	const msg = "Your %s has less than 50 characters, less than 5 unique characters, or it's prefixed with 'django-insecure-' indicating that it was generated automatically by Django. Please generate a long and random value, otherwise many of Django's security-critical features will be vulnerable to attack."
	return checks.Warning(fmt.Sprintf(msg, name), checks.ID("security.W025"))
}

// The limits of Django's secret key check (base.SECRET_KEY_MIN_LENGTH,
// SECRET_KEY_MIN_UNIQUE_CHARACTERS and SECRET_KEY_INSECURE_PREFIX), which the
// secret key tests assert their data against.
const (
	secretKeyMinLength           = 50
	secretKeyMinUniqueCharacters = 5
	secretKeyInsecurePrefix      = "django-insecure-"
)

// A key as Django's get_random_secret_key returns one: 50 random characters.
const randomSecretKey = "x6k!z2@w#q9$e8%r7^t5&y4*u3(i1)o0-p_a=s+dfghjklmnbvc"

// uniqueCharacters returns the number of distinct characters of s.
func uniqueCharacters(s string) int {
	seen := map[rune]bool{}
	for _, r := range s {
		seen[r] = true
	}
	return len(seen)
}

// TestOkaySecretKey accepts a key of exactly the minimum length with enough
// distinct characters.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_okay_secret_key
func TestOkaySecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "ab"
	djangotest.AssertEqual(t, len(s.SecretKey), secretKeyMinLength)
	if uniqueCharacters(s.SecretKey) <= secretKeyMinUniqueCharacters {
		t.Fatalf("the key has %d distinct characters", uniqueCharacters(s.SecretKey))
	}
	assertMessages(t, deployMessages(s, "security.W009"))
}

// TestEmptySecretKey warns about an empty key.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_empty_secret_key
func TestEmptySecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = ""
	assertMessages(t, deployMessages(s, "security.W009"), secW009)
}

// TestMissingSecretKey warns about settings without a SECRET_KEY. Django
// deletes the setting; the counterpart is settings whose field was never set.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_missing_secret_key
func TestMissingSecretKey(t *testing.T) {
	assertMessages(t, deployMessages(conf.Settings{}, "security.W009"), secW009)
}

// TestInsecureSecretKey warns about a key with the prefix of a generated
// development key, however long and random the rest is.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_insecure_secret_key
func TestInsecureSecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = secretKeyInsecurePrefix + randomSecretKey
	assertMessages(t, deployMessages(s, "security.W009"), secW009)
}

// TestLowLengthSecretKey warns about a key one character below the minimum
// length.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_low_length_secret_key
func TestLowLengthSecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "a"
	djangotest.AssertEqual(t, len(s.SecretKey), secretKeyMinLength-1)
	assertMessages(t, deployMessages(s, "security.W009"), secW009)
}

// TestLowEntropySecretKey warns about a long key with too few distinct
// characters.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyTest.test_low_entropy_secret_key
func TestLowEntropySecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = strings.Repeat("abcd", 20)
	if len(s.SecretKey) <= secretKeyMinLength {
		t.Fatalf("the key has %d characters", len(s.SecretKey))
	}
	if uniqueCharacters(s.SecretKey) >= secretKeyMinUniqueCharacters {
		t.Fatalf("the key has %d distinct characters", uniqueCharacters(s.SecretKey))
	}
	assertMessages(t, deployMessages(s, "security.W009"), secW009)
}

// TestOkaySecretKeyFallbacks accepts a fallback key of exactly the minimum
// length with enough distinct characters.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_okay_secret_key_fallbacks
func TestOkaySecretKeyFallbacks(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "ab"}
	djangotest.AssertEqual(t, len(s.SecretKeyFallbacks[0]), secretKeyMinLength)
	if uniqueCharacters(s.SecretKeyFallbacks[0]) <= secretKeyMinUniqueCharacters {
		t.Fatalf("the key has %d distinct characters", uniqueCharacters(s.SecretKeyFallbacks[0]))
	}
	assertMessages(t, deployMessages(s, "security.W025"))
}

// TestInsecureSecretKeyFallbacks warns about a fallback key with the prefix of
// a generated development key.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_insecure_secret_key_fallbacks
func TestInsecureSecretKeyFallbacks(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{secretKeyInsecurePrefix + randomSecretKey}
	assertMessages(t, deployMessages(s, "security.W025"), secW025("SECRET_KEY_FALLBACKS[0]"))
}

// TestLowLengthSecretKeyFallbacks warns about a fallback key one character
// below the minimum length.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_low_length_secret_key_fallbacks
func TestLowLengthSecretKeyFallbacks(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "a"}
	djangotest.AssertEqual(t, len(s.SecretKeyFallbacks[0]), secretKeyMinLength-1)
	assertMessages(t, deployMessages(s, "security.W025"), secW025("SECRET_KEY_FALLBACKS[0]"))
}

// TestLowEntropySecretKeyFallbacks warns about a long fallback key with too
// few distinct characters.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_low_entropy_secret_key_fallbacks
func TestLowEntropySecretKeyFallbacks(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{strings.Repeat("abcd", 20)}
	if len(s.SecretKeyFallbacks[0]) <= secretKeyMinLength {
		t.Fatalf("the key has %d characters", len(s.SecretKeyFallbacks[0]))
	}
	if uniqueCharacters(s.SecretKeyFallbacks[0]) >= secretKeyMinUniqueCharacters {
		t.Fatalf("the key has %d distinct characters", uniqueCharacters(s.SecretKeyFallbacks[0]))
	}
	assertMessages(t, deployMessages(s, "security.W025"), secW025("SECRET_KEY_FALLBACKS[0]"))
}

// TestMultipleKeys names the index of the one weak fallback key.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_multiple_keys
func TestMultipleKeys(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{
		strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "ab",
		"badkey",
	}
	assertMessages(t, deployMessages(s, "security.W025"), secW025("SECRET_KEY_FALLBACKS[1]"))
}

// TestMultipleBadKeys warns once for every weak fallback key.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSecretKeyFallbacksTest.test_multiple_bad_keys
func TestMultipleBadKeys(t *testing.T) {
	s := conf.Defaults()
	s.SecretKeyFallbacks = []string{
		strings.Repeat("abcdefghijklmnopqrstuvwx", 2) + "ab",
		"badkey1",
		"badkey2",
	}
	assertMessages(t, deployMessages(s, "security.W025"),
		secW025("SECRET_KEY_FALLBACKS[1]"),
		secW025("SECRET_KEY_FALLBACKS[2]"),
	)
}
