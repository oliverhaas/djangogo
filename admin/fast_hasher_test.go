package admin_test

// Importing djangotest makes the fast MD5 password hasher the preferred one
// for this test binary. The tests log in several times, and one PBKDF2 hash
// with the default work factor takes more than a second under the race
// detector.
import _ "github.com/oliverhaas/djangogo/djangotest"
