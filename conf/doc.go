// Package conf holds the settings of a Djan-Go-Go project (the analog of
// Django's settings module and of django.conf).
//
// Settings is a typed struct that carries Django's setting names in Go
// spelling: SESSION_COOKIE_HTTPONLY is SessionCookieHTTPOnly, X_FRAME_OPTIONS is
// XFrameOptions. Every field that the environment can set has an env tag: its
// Django name, where Django has the setting.
//
// # Defaults, code, environment
//
// The value of a setting comes from three places, each one replacing the one
// before it:
//
//  1. Defaults returns Django's default values.
//  2. The project changes fields in code.
//  3. ApplyEnv replaces a field when its environment variable is set.
//
// The settings function of a project puts them in this order:
//
//	func settings() (conf.Settings, error) {
//		s := conf.Defaults()
//		s.Debug = true
//		s.InstalledApps = []string{"blog"}
//		s.Database = conf.Database{Driver: conf.DriverSQLite, DSN: "db.sqlite3"}
//
//		// A variable of the real environment wins over the same one in .env.
//		if err := conf.LoadDotEnv(); err != nil {
//			return s, err
//		}
//		// Reads DJANGOGO_SECRET_KEY, DJANGOGO_DEBUG, DATABASE_URL and so on.
//		if err := s.ApplyEnv("DJANGOGO_"); err != nil {
//			return s, err
//		}
//		return s, nil
//	}
//
// A project that needs no environment may pass a plain literal to Configure,
// which fills every zero-valued field with its default. Go cannot tell an
// omitted boolean from an explicit false, so a literal cannot switch a setting
// off whose default is true (UseTZ, AppendSlash, SessionCookieHTTPOnly,
// SecureContentTypeNosniff, Templates.AppDirs), nor empty a string whose
// default is not empty: start from Defaults for that. The environment overlay
// applies the defaults before it reads a variable, so a variable can switch
// such a setting off in both cases.
//
// # Environment variables
//
// The variable of a field is the prefix given to ApplyEnv followed by the env
// tag: with the prefix "DJANGOGO_", SessionCookieAge is read from
// DJANGOGO_SESSION_COOKIE_AGE and Templates.Dirs from DJANGOGO_TEMPLATES_DIRS.
// ApplyEnv documents the syntax of the values: booleans such as "true", "yes"
// and "0", whole seconds or a Go duration for a time.Duration, and a
// comma-separated list for a slice.
//
// The fields without an env tag have no variable of their own. Database, the
// default database, is read from prefix+"DATABASE_URL" and, when that is
// unset, from DATABASE_URL (see ParseDatabaseURL); another alias that the code
// declares in Databases is read from prefix+"DATABASE_URL_"+ALIAS.
// AuthPasswordValidators is set in code only.
//
// A malformed variable is an error that names the variable. ApplyEnv reports
// all of them at once and leaves their fields unchanged.
//
// LoadDotEnv copies the variables of a .env file into the process environment
// without replacing a variable that is already set; ReadDotEnv parses a file
// into a map, which ApplyEnvFunc can read in place of the process environment.
//
// # Settings of apps
//
// An app that has settings of its own registers a struct with its defaults
// under a name, a settings namespace:
//
//	type Settings struct {
//		PageSize int
//		Title    string
//	}
//
//	func init() {
//		// Register refuses only a programming error, such as a name that
//		// another type holds.
//		if err := conf.Register("blog", Settings{PageSize: 10}); err != nil {
//			panic(err)
//		}
//	}
//
// The project may replace the value in code, and the app reads it back typed.
// The environment overlay of the settings applies to it as well, under the
// variables prefix + NAME + "_" + FIELD, and wins over the value set in code:
//
//	err := s.SetNamespace("blog", blog.Settings{PageSize: 20, Title: "News"})
//
//	// DJANGOGO_BLOG_PAGE_SIZE=50 gives PageSize 50.
//	cfg, err := conf.Get[blog.Settings](s, "blog")
//
// Get describes how a variable name is derived from a field name and which
// field types the environment can set. Namespaces lists the registered names.
//
// # Validation
//
// Check reports what the project cannot run with, an empty SecretKey. Validate
// returns the invalid setting values and NamespaceErrors the errors of the
// namespaces; the system checks report both.
package conf
