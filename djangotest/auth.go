package djangotest

import (
	"testing"

	"github.com/oliverhaas/djangogo/auth"
)

// init makes the salted MD5 hasher the preferred password hasher of every test
// binary that imports djangotest, as Django's own test settings do. Hashing
// with the default work factor costs more than a second per password under the
// race detector. The default hashers stay configured behind it, so stored
// hashes of the real algorithms still verify. A test of the hashers themselves
// sets auth.Config.PasswordHashers and restores it afterwards.
func init() {
	if !testing.Testing() {
		return
	}
	cfg := auth.CurrentConfig()
	if len(cfg.PasswordHashers) > 0 {
		return
	}
	cfg.PasswordHashers = append([]auth.Hasher{auth.MD5Hasher{}}, auth.DefaultPasswordHashers()...)
	auth.Configure(cfg)
}
