// conf/settings_test.go
package conf

import "testing"

func TestSettingsApplyDefaults(t *testing.T) {
	s := Settings{SecretKey: "x"}
	s.applyDefaults()

	if s.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", s.Host)
	}
	if s.Port != "8000" {
		t.Errorf("Port = %q, want 8000", s.Port)
	}
	if len(s.AllowedHosts) == 0 {
		t.Error("AllowedHosts should default to a non-empty list")
	}
}

func TestSettingsCheck(t *testing.T) {
	if err := (&Settings{SecretKey: "set"}).Check(); err != nil {
		t.Errorf("valid settings returned error: %v", err)
	}
	if err := (&Settings{}).Check(); err == nil {
		t.Error("missing SecretKey should be an error")
	}
}

func TestConfigureAndActive(t *testing.T) {
	got := Configure(Settings{SecretKey: "k"})
	if got.Host != "127.0.0.1" {
		t.Errorf("Configure should apply defaults; Host = %q", got.Host)
	}
	if Active() != got {
		t.Error("Active() should return the configured settings")
	}
}
