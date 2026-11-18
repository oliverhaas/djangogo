package checks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oliverhaas/djangogo/conf"
)

// The settings whose problem Django reports with a check of its own, and the
// id of that check. Every other problem of conf.Settings.Validate is reported
// as settings.E001.
const (
	staticURLSetting          = "STATIC_URL"
	staticURLID               = "urls.E006"
	csrfTrustedOriginsSetting = "CSRF_TRUSTED_ORIGINS"
	csrfTrustedOriginsID      = "4_0.E001"
)

// settingProblems returns the problems conf.Settings.Validate reports for one
// setting as errors with the given id and the text of the problem.
func settingProblems(s *conf.Settings, setting, id string) []Message {
	var messages []Message
	for _, p := range s.Validate() {
		if p.Setting == setting {
			messages = append(messages, Error(p.Msg, ID(id)))
		}
	}
	return messages
}

// checkStaticURL reports a StaticURL that does not end with a slash
// (urls.E006, Django's check_url_settings).
func checkStaticURL(s *conf.Settings) []Message {
	return settingProblems(s, staticURLSetting, staticURLID)
}

// checkCSRFTrustedOrigins reports every entry of CSRFTrustedOrigins without a
// scheme (4_0.E001, Django's check_csrf_trusted_origins).
func checkCSRFTrustedOrigins(s *conf.Settings) []Message {
	return settingProblems(s, csrfTrustedOriginsSetting, csrfTrustedOriginsID)
}

// checkSettings reports the invalid settings Django has no check for: every
// other problem of conf.Settings.Validate as settings.E001, with the text
// "<SETTING>: <problem>", and every error of a settings namespace as
// settings.E002. Django raises ImproperlyConfigured for most of them where it
// reads the setting.
func checkSettings(s *conf.Settings) []Message {
	var messages []Message
	for _, p := range s.Validate() {
		if p.Setting == staticURLSetting || p.Setting == csrfTrustedOriginsSetting {
			continue
		}
		messages = append(messages, Error(p.Setting+": "+p.Msg, ID("settings.E001")))
	}
	for _, err := range s.NamespaceErrors() {
		messages = append(messages, Error(err.Error(), ID("settings.E002")))
	}
	return messages
}

// checkStaticFilesDirs is the part of Django's staticfiles checks that reads
// the settings alone (FileSystemFinder.check): an entry of StaticFilesDirs that
// is the StaticRoot, which collectstatic would copy into itself
// (staticfiles.E002), and an entry that is no directory (staticfiles.W004).
func checkStaticFilesDirs(s *conf.Settings) []Message {
	var messages []Message
	for _, dir := range s.StaticFilesDirs {
		if s.StaticRoot != "" && absPath(s.StaticRoot) == absPath(dir) {
			messages = append(messages, Error(
				"The STATICFILES_DIRS setting should not contain the STATIC_ROOT setting.",
				ID("staticfiles.E002"),
			))
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			messages = append(messages, Warning(
				fmt.Sprintf("The directory '%s' in the STATICFILES_DIRS setting does not exist.", dir),
				ID("staticfiles.W004"),
			))
		}
	}
	return messages
}

// absPath returns path as Python's os.path.abspath does: absolute and cleaned,
// without following links. A relative path stays relative when the working
// directory cannot be read.
func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
