// Package checks is the system check framework (django.core.checks): messages
// with a level, a registry of tagged check functions, the report of the check
// command, and the built-in checks.
//
// # Messages
//
// A check function returns Messages. Debug, Info, Warning, Error and Critical
// build one, with Hint, Obj and ID for the optional parts:
//
//	checks.Error("Primary keys must not have null=True.",
//		checks.Hint("Set null=False on the field, or remove primary_key=True argument."),
//		checks.Obj("blog.Post.slug"),
//		checks.ID("fields.E007"))
//
// A message prints in Django's format, "blog.Post.slug: (fields.E007) Primary
// keys must not have null=True.", followed by a HINT line.
//
// # Registry
//
// A Registry holds check functions with their tags. Default is the registry of
// the process: it holds the built-in checks, and a package adds its own in an
// init function:
//
//	func init() { checks.Register(checkSomething, checks.TagModels) }
//
// Run executes the checks and returns their messages. A check registered with
// RegisterDeploy runs only when RunOptions.Deploy is set (check --deploy).
// Where a Django check reads the settings and the app registry from globals,
// a check function reads them from its Context.
//
// # Report
//
// Summarize turns the messages of a run into the output of Django's check
// command: the messages grouped by level, the line with the number of issues,
// and the decision whether the run failed.
//
//	msgs := checks.Default.Run(ctx, checks.RunOptions{Deploy: deploy})
//	report := checks.Summarize(msgs, settings.SilencedSystemChecks, checks.LevelError, true)
//	if report.Failed {
//		return &checks.SystemCheckError{Report: report}
//	}
//
// # Built-in checks
//
// The deployment checks tagged security are Django's, with its ids and texts.
// A text names a setting in Django's spelling (SECURE_HSTS_SECONDS), which is
// also the name of its environment variable; the field of conf.Settings is the
// same name in Go spelling (SecureHSTSSeconds).
//
//	security.W004  SecureHSTSSeconds is not above 0
//	security.W005  SecureHSTSIncludeSubdomains is off while HSTS is on
//	security.W006  SecureContentTypeNosniff is off
//	security.W008  SecureSSLRedirect is off
//	security.W009  SecretKey is short, has few distinct characters or is a generated development key
//	security.W012  SessionCookieSecure is off
//	security.W015  SessionCookieHTTPOnly is off
//	security.W016  CSRFCookieSecure is off and CSRFUseSessions is off
//	security.W018  Debug is on
//	security.W019  XFrameOptions is neither empty nor "DENY" in any letter case
//	security.W020  AllowedHosts is empty
//	security.W021  SecureHSTSPreload is off while HSTS is on
//	security.W022  SecureReferrerPolicy is empty
//	security.E023  SecureReferrerPolicy holds an unknown value
//	security.E024  SecureCrossOriginOpenerPolicy holds an unknown value
//	security.W025  an entry of SecretKeyFallbacks is as weak as W009 describes
//
// Django's checks for a missing middleware (security.W001, W002, W003) and its
// variants of the cookie checks for a project without the session app or
// middleware (W010, W011, W013, W014) do not exist: the middleware stack of a
// project always holds the security, clickjacking, session and CSRF
// middleware.
//
// The settings checks run in every run. They report what
// conf.Settings.Validate and conf.Settings.NamespaceErrors find, as errors,
// under Django's id where Django has a check for the setting:
//
//	urls.E006      StaticURL does not end with a slash (tags urls and settings)
//	4_0.E001       an entry of CSRFTrustedOrigins has no scheme (tags compatibility and settings)
//	settings.E001  another setting is invalid; the text is "<SETTING>: <problem>" (tag settings)
//	settings.E002  a settings namespace has an error (tag settings)
//
// Django's staticfiles checks of the settings are tagged staticfiles:
//
//	staticfiles.E002  StaticFilesDirs holds the StaticRoot
//	staticfiles.W004  an entry of StaticFilesDirs is no directory
//
// The model checks tagged models read the metadata of the model registry:
// field names (fields.E001, E002, E003), a nullable primary key (fields.E007),
// choices longer than max_length (fields.E009), the max_length of a string
// field (fields.E120, E121), a relation to a model that is not registered
// (fields.E300), on_delete=set_null on a field that is not nullable
// (fields.E320), model names (models.E023, E024), two fields with one column
// (models.E007) and two models with one table (models.E028). Context.Apps
// limits them to the models of the named apps.
package checks
