package checks

// init registers the built-in checks on Default. One function does it for all
// of them, so their order does not depend on the names of the files that
// define them: the model checks, the settings checks, the staticfiles checks,
// then the deployment checks.
//
// The two settings checks with a Django id carry Django's tag and TagSettings,
// so a run for TagSettings shows every invalid setting.
func init() {
	Default.Register(checkModels, TagModels)
	Default.Register(onSettings(checkStaticURL), TagURLs, TagSettings)
	Default.Register(onSettings(checkCSRFTrustedOrigins), TagCompatibility, TagSettings)
	Default.Register(onSettings(checkSettings), TagSettings)
	Default.Register(onSettings(checkStaticFilesDirs), TagStaticfiles)
	for _, check := range securityChecks {
		Default.RegisterDeploy(onSettings(check), TagSecurity)
	}
}
