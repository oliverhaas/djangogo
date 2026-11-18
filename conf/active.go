package conf

import "sync/atomic"

// active is the process-wide settings object, set by Configure. It gives the
// Django-familiar global accessor (conf.Active()). The application configures
// it once at boot; tests may reconfigure freely, also from parallel tests,
// which is why the pointer is atomic.
var active atomic.Pointer[Settings]

// Configure sets the active settings (applying defaults) and returns them.
func Configure(s Settings) *Settings {
	s.applyDefaults()
	active.Store(&s)
	return &s
}

// Active returns the configured settings. It panics if Configure was never
// called, which is a boot-ordering bug and not a runtime condition.
func Active() *Settings {
	s := active.Load()
	if s == nil {
		panic("conf: settings not configured; call conf.Configure first")
	}
	return s
}
