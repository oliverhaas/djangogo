package djangogo

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/middleware"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
)

// This file turns the settings into the configuration of the contrib
// packages: the auth app, the session and CSRF middleware, the context
// processors of the template engine and the post-migrate hook that creates
// content types and permissions.

// configureAuth applies the auth settings to the auth package. It starts from
// the active configuration, so the backends a project sets in code before New
// stay, and replaces the hashers and validators only when the settings name
// some.
func (a *Application) configureAuth() error {
	s := a.Settings
	cfg := auth.CurrentConfig()
	cfg.SecretKey = s.SecretKey
	cfg.SecretKeyFallbacks = s.SecretKeyFallbacks
	cfg.LoginURL = s.LoginURL
	cfg.LoginRedirectURL = s.LoginRedirectURL
	cfg.LogoutRedirectURL = s.LogoutRedirectURL
	cfg.IsSecure = a.isSecure
	if len(s.PasswordHashers) > 0 {
		hashers := make([]auth.Hasher, 0, len(s.PasswordHashers))
		for _, name := range s.PasswordHashers {
			h, err := passwordHasher(name)
			if err != nil {
				return err
			}
			hashers = append(hashers, h)
		}
		cfg.PasswordHashers = hashers
	}
	if len(s.AuthPasswordValidators) > 0 {
		validators := make([]auth.PasswordValidator, 0, len(s.AuthPasswordValidators))
		for _, v := range s.AuthPasswordValidators {
			pv, err := passwordValidator(v)
			if err != nil {
				return err
			}
			validators = append(validators, pv)
		}
		cfg.PasswordValidators = validators
	}
	auth.Configure(cfg)
	return nil
}

// isSecure reports whether r reached the site over HTTPS, Django's
// request.is_secure with SECURE_PROXY_SSL_HEADER.
func (a *Application) isSecure(r *http.Request) bool {
	return middleware.IsSecure(r, a.Settings.SecureProxySSLHeader)
}

// djangoHashers pairs Django's class name of each password hasher with the
// hasher of the auth package.
var djangoHashers = []struct {
	class  string
	hasher auth.Hasher
}{
	{"PBKDF2PasswordHasher", auth.PBKDF2Hasher{}},
	{"PBKDF2SHA1PasswordHasher", auth.PBKDF2SHA1Hasher{}},
	{"Argon2PasswordHasher", auth.Argon2Hasher{}},
	{"BCryptSHA256PasswordHasher", auth.BCryptSHA256Hasher{}},
	{"BCryptPasswordHasher", auth.BCryptHasher{}},
	{"ScryptPasswordHasher", auth.ScryptHasher{}},
	{"MD5PasswordHasher", auth.MD5Hasher{}},
}

// passwordHasher returns the hasher an entry of PASSWORD_HASHERS names: its
// algorithm ("pbkdf2_sha256") or Django's class, with or without the module
// ("django.contrib.auth.hashers.PBKDF2PasswordHasher").
func passwordHasher(name string) (auth.Hasher, error) {
	class := lastSegment(name)
	for _, h := range djangoHashers {
		if name == h.hasher.Algorithm() || class == h.class {
			return h.hasher, nil
		}
	}
	return nil, &conf.ImproperlyConfigured{Msg: fmt.Sprintf("The PASSWORD_HASHERS setting names an unknown hasher %q.", name)}
}

// passwordValidator returns the validator an entry of AUTH_PASSWORD_VALIDATORS
// names, by Django's class with or without the module, with its options.
func passwordValidator(v conf.PasswordValidator) (auth.PasswordValidator, error) {
	opts := validatorOptions{name: v.Name, values: v.Options}
	var pv auth.PasswordValidator
	switch lastSegment(v.Name) {
	case "UserAttributeSimilarityValidator":
		pv = auth.UserAttributeSimilarityValidator{
			UserAttributes: opts.stringList("user_attributes"),
			MaxSimilarity:  opts.floatValue("max_similarity"),
		}
	case "MinimumLengthValidator":
		pv = auth.MinimumLengthValidator{MinLength: opts.intValue("min_length")}
	case "CommonPasswordValidator":
		pv = auth.CommonPasswordValidator{}
		if path := opts.stringValue("password_list_path"); path != "" && opts.err == nil {
			passwords, err := readPasswordList(path)
			if err != nil {
				return nil, &conf.ImproperlyConfigured{Msg: fmt.Sprintf("AUTH_PASSWORD_VALIDATORS: %s: %v", v.Name, err)}
			}
			pv = auth.NewCommonPasswordValidator(passwords)
		}
	case "NumericPasswordValidator":
		pv = auth.NumericPasswordValidator{}
	default:
		return nil, &conf.ImproperlyConfigured{Msg: fmt.Sprintf(
			"The AUTH_PASSWORD_VALIDATORS setting names an unknown validator %q.", v.Name)}
	}
	if err := opts.finish(); err != nil {
		return nil, err
	}
	return pv, nil
}

// validatorOptions reads the options of one password validator and records
// the first option of the wrong type and the options nobody read.
type validatorOptions struct {
	name   string
	values map[string]any
	used   []string
	err    error
}

// get returns the value of an option and marks it as read.
func (o *validatorOptions) get(key string) (any, bool) {
	o.used = append(o.used, key)
	v, ok := o.values[key]
	return v, ok
}

// fail records that an option has the wrong type.
func (o *validatorOptions) fail(key, want string) {
	if o.err == nil {
		o.err = &conf.ImproperlyConfigured{Msg: fmt.Sprintf(
			"AUTH_PASSWORD_VALIDATORS: the option %q of %s must be %s.", key, o.name, want)}
	}
}

// intValue returns an integer option, 0 when it is not set.
func (o *validatorOptions) intValue(key string) int {
	v, ok := o.get(key)
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		if n == math.Trunc(n) {
			return int(n)
		}
	}
	o.fail(key, "an integer")
	return 0
}

// floatValue returns a number option, 0 when it is not set.
func (o *validatorOptions) floatValue(key string) float64 {
	v, ok := o.get(key)
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	o.fail(key, "a number")
	return 0
}

// stringValue returns a string option, "" when it is not set.
func (o *validatorOptions) stringValue(key string) string {
	v, ok := o.get(key)
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		o.fail(key, "a string")
	}
	return s
}

// stringList returns a list of strings option, nil when it is not set.
func (o *validatorOptions) stringList(key string) []string {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, e := range list {
			s, ok := e.(string)
			if !ok {
				o.fail(key, "a list of strings")
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	o.fail(key, "a list of strings")
	return nil
}

// finish returns the first type error, or an error for an option the
// validator does not take (Django's TypeError for an unexpected keyword).
func (o *validatorOptions) finish() error {
	if o.err != nil {
		return o.err
	}
	for key := range o.values {
		if !slices.Contains(o.used, key) {
			return &conf.ImproperlyConfigured{Msg: fmt.Sprintf(
				"AUTH_PASSWORD_VALIDATORS: %s has no option %q.", o.name, key)}
		}
	}
	return nil
}

// readPasswordList reads the password list of CommonPasswordValidator: one
// password per line, gzip compressed when the name ends in ".gz", as Django
// reads password_list_path.
func readPasswordList(path string) (passwords []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		r = gz
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if p := strings.ToLower(strings.TrimSpace(sc.Text())); p != "" {
			passwords = append(passwords, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return passwords, nil
}

// sessionStore returns the store of SESSION_ENGINE. The database engine needs
// a database and the installed sessions app, whose table holds the sessions;
// without them the sessions live in signed cookies.
func (a *Application) sessionStore() sessions.Store {
	s := a.Settings
	if s.SessionEngine == conf.SessionEngineDB && a.DB != nil && a.Apps.IsInstalled(sessions.App{}.Name()) {
		return sessions.NewDBStore(a.DB)
	}
	return sessions.NewSignedCookieStore([]byte(s.SecretKey))
}

// sessionOptions returns the options of the session middleware from the
// SESSION_* settings.
func (a *Application) sessionOptions() sessions.Options {
	s := a.Settings
	return sessions.Options{
		CookieName:           s.SessionCookieName,
		CookieAge:            s.SessionCookieAge,
		CookieDomain:         s.SessionCookieDomain,
		CookiePath:           s.SessionCookiePath,
		CookieSecure:         s.SessionCookieSecure,
		DisableHTTPOnly:      !s.SessionCookieHTTPOnly,
		CookieSameSite:       sameSite(s.SessionCookieSameSite),
		SaveEveryRequest:     s.SessionSaveEveryRequest,
		ExpireAtBrowserClose: s.SessionExpireAtBrowserClose,
	}
}

// messageOptions returns the options of the message middleware from
// MESSAGE_STORAGE. The messages cookie is signed with SECRET_KEY and takes the
// SESSION_COOKIE_* attributes, as in Django's CookieStorage.
func (a *Application) messageOptions() messages.Options {
	s := a.Settings
	storage := messages.StorageFallback
	switch s.MessageStorage {
	case conf.MessageStorageCookie:
		storage = messages.StorageCookie
	case conf.MessageStorageSession:
		storage = messages.StorageSession
	}
	fallbacks := make([][]byte, len(s.SecretKeyFallbacks))
	for i, key := range s.SecretKeyFallbacks {
		fallbacks[i] = []byte(key)
	}
	return messages.Options{
		Storage: storage,
		Cookie: messages.CookieOptions{
			Secret:          []byte(s.SecretKey),
			Fallbacks:       fallbacks,
			Domain:          s.SessionCookieDomain,
			Secure:          s.SessionCookieSecure,
			DisableHTTPOnly: !s.SessionCookieHTTPOnly,
			SameSite:        sameSite(s.SessionCookieSameSite),
		},
	}
}

// csrfOptions returns the options of the CSRF middleware from the CSRF_*
// settings. resolve, when not nil, finds the view of a request so that a view
// marked with csrf.Exempt is not checked.
func (a *Application) csrfOptions(resolve func(*http.Request) http.Handler) csrf.Options {
	s := a.Settings
	return csrf.Options{
		UseSessions:    s.CSRFUseSessions,
		CookieName:     s.CSRFCookieName,
		CookieAge:      s.CSRFCookieAge,
		CookieDomain:   s.CSRFCookieDomain,
		CookiePath:     s.CSRFCookiePath,
		CookieSecure:   s.CSRFCookieSecure,
		CookieHTTPOnly: s.CSRFCookieHTTPOnly,
		CookieSameSite: sameSite(s.CSRFCookieSameSite),
		HeaderName:     s.CSRFHeaderName,
		TrustedOrigins: s.CSRFTrustedOrigins,
		IsSecure:       a.isSecure,
		Resolve:        resolve,
	}
}

// sameSite converts a SameSite setting, which the settings checks have
// validated, to the cookie attribute. The empty string leaves the attribute
// out, as Django's False does.
func sameSite(value string) http.SameSite {
	switch strings.ToLower(value) {
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	}
	return http.SameSiteDefaultMode
}

// contextProcessors returns the context processors of
// TEMPLATES_CONTEXT_PROCESSORS, named by Django's dotted path or its last
// segment, after the CSRF processor that Django always runs. Without the
// setting they are those of a new Django project: request, auth when the
// auth app is installed, and messages.
func (a *Application) contextProcessors() ([]templates.ContextProcessor, error) {
	names := a.Settings.Templates.ContextProcessors
	if names == nil {
		names = []string{"request"}
		if a.Apps.IsInstalled(auth.App{}.Name()) {
			names = append(names, "auth")
		}
		names = append(names, "messages")
	}
	out := make([]templates.ContextProcessor, 0, len(names)+1)
	out = append(out, csrf.ContextProcessor)
	for _, name := range names {
		switch lastSegment(name) {
		case "csrf":
			// Always runs, as in Django.
		case "request":
			out = append(out, templates.RequestProcessor)
		case "static":
			out = append(out, templates.StaticProcessor)
		case "debug":
			out = append(out, templates.DebugProcessor(a.Settings.Debug))
		case "auth":
			out = append(out, auth.ContextProcessor)
		case "messages":
			out = append(out, messages.ContextProcessor)
		default:
			return nil, &conf.ImproperlyConfigured{Msg: fmt.Sprintf(
				"The TEMPLATES_CONTEXT_PROCESSORS setting names an unknown context processor %q.", name)}
		}
	}
	return out, nil
}

// syncContentTypes is the post-migrate hook of the contenttypes and auth
// apps: it creates the content type of every model and, with the auth app
// installed, the default permissions of every model (Django's
// create_contenttypes and create_permissions receivers). As in Django it does
// nothing until the migrations of these apps are applied.
func (a *Application) syncContentTypes(ctx context.Context, db *orm.DB) error {
	applied, err := migrations.NewRecorder(db).AppliedMigrations(ctx)
	if err != nil {
		return err
	}
	migrated := func(app string) bool {
		for key := range applied {
			if key.App == app {
				return true
			}
		}
		return false
	}
	ct, au := contenttypes.App{}.Name(), auth.App{}.Name()
	if !migrated(ct) {
		return nil
	}
	if a.Apps.IsInstalled(au) && migrated(au) {
		_, err = auth.SyncPermissions(ctx, db)
		return err
	}
	_, err = contenttypes.Sync(ctx, db)
	return err
}

// lastSegment returns the part of a dotted name after its last dot.
func lastSegment(name string) string {
	return name[strings.LastIndex(name, ".")+1:]
}
