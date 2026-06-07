// conf/active.go
package conf

// active is the process-wide settings object, set by Configure. It gives the
// Django-familiar global accessor (conf.Active()). The application configures it
// once at boot; tests may reconfigure freely.
var active *Settings

// Configure sets the active settings (applying defaults) and returns them.
func Configure(s Settings) *Settings {
	s.applyDefaults()
	active = &s
	return active
}

// Active returns the configured settings. It panics if Configure was never called,
// which always indicates a boot-ordering bug rather than a runtime condition.
func Active() *Settings {
	if active == nil {
		panic("conf: settings not configured; call conf.Configure first")
	}
	return active
}
