package conf_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The registry of namespaces is process-wide, so every test registers under a
// name of its own.

// blogSettings is the settings struct of an app, with a field of every kind
// the environment can set and some it cannot.
type blogSettings struct {
	PageSize int
	Title    string
	APIKey   string
	HTTPOnly bool
	Tags     []string
	Timeout  time.Duration
	Ratio    float64
	Workers  uint8
	Ports    []int
	Addr     netip.Addr
	Renamed  string `env:"CUSTOM_NAME"`
	Excluded string `env:"-"`
	Cache    cacheSettings
	Labels   map[string]string
}

// cacheSettings is a struct nested in the settings of an app.
type cacheSettings struct {
	TTL     time.Duration
	Backend string
}

// otherSettings is a settings struct that no test registers under the name it
// asks for.
type otherSettings struct {
	Name string
}

// register registers a namespace and fails the test when that is refused.
func register[T any](t *testing.T, name string, defaults T) {
	t.Helper()
	if err := conf.Register(name, defaults); err != nil {
		t.Fatal(err)
	}
}

// overlaid returns the default settings overlaid with the given variables
// under the prefix "P_".
func overlaid(t *testing.T, vars map[string]string) *conf.Settings {
	t.Helper()
	s := conf.Defaults()
	if err := s.ApplyEnvFunc("P_", mapEnv(vars)); err != nil {
		t.Fatal(err)
	}
	return &s
}

// A project that sets nothing gets the defaults the app registered.
func TestGetReturnsTheRegisteredDefaults(t *testing.T) {
	defaults := blogSettings{PageSize: 10, Title: "Blog", Tags: []string{"news"}, Cache: cacheSettings{TTL: time.Minute}}
	register(t, "ns_defaults", defaults)

	fromDefaults := conf.Defaults()
	got, err := conf.Get[blogSettings](&fromDefaults, "ns_defaults")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, defaults)

	// Settings that were neither completed nor overlaid answer the same.
	got, err = conf.Get[blogSettings](&conf.Settings{}, "ns_defaults")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, defaults)
}

// A value set in code replaces the defaults as a whole; it is not merged.
func TestSetNamespaceReplacesTheDefaults(t *testing.T) {
	register(t, "ns_code", blogSettings{PageSize: 10, Title: "Blog"})
	s := conf.Defaults()
	if err := s.SetNamespace("ns_code", blogSettings{PageSize: 20}); err != nil {
		t.Fatal(err)
	}

	got, err := conf.Get[blogSettings](&s, "ns_code")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{PageSize: 20})
}

// Every kind of field the environment can set is read from the variable that
// its name or its tag gives, and no other variable is.
func TestGetOverlaysTheEnvironment(t *testing.T) {
	register(t, "ns_env", blogSettings{PageSize: 10, Title: "Blog", Excluded: "default", Labels: map[string]string{"a": "b"}})
	s := overlaid(t, map[string]string{
		"P_NS_ENV_PAGE_SIZE":     "50",
		"P_NS_ENV_API_KEY":       "k-123",
		"P_NS_ENV_HTTP_ONLY":     "yes",
		"P_NS_ENV_TAGS":          "go, django",
		"P_NS_ENV_TIMEOUT":       "30",
		"P_NS_ENV_RATIO":         "0.25",
		"P_NS_ENV_WORKERS":       "4",
		"P_NS_ENV_PORTS":         "8000, 8001",
		"P_NS_ENV_ADDR":          "192.0.2.1",
		"P_NS_ENV_CUSTOM_NAME":   "by its tag",
		"P_NS_ENV_CACHE_TTL":     "5m",
		"P_NS_ENV_CACHE_BACKEND": "redis",
		// Not read: an excluded field, the Go name of a field with a tag, a
		// variable without the prefix and one of another namespace.
		"P_NS_ENV_EXCLUDED":  "from the environment",
		"P_NS_ENV_RENAMED":   "by its Go name",
		"NS_ENV_TITLE":       "without the prefix",
		"P_NS_OTHER_TITLE":   "another namespace",
		"P_NS_ENV_PAGE_SIZ":  "1",
		"P_NS_ENVPAGE_SIZE":  "2",
		"P_NS_ENV_PAGESIZE":  "3",
		"P_NS_ENV_CACHETTL":  "4",
		"P_NS_ENV_CACHE":     "5",
		"P_NS_ENV_CACHE_TTl": "6",
	})

	got, err := conf.Get[blogSettings](s, "ns_env")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{
		PageSize: 50,
		Title:    "Blog",
		APIKey:   "k-123",
		HTTPOnly: true,
		Tags:     []string{"go", "django"},
		Timeout:  30 * time.Second,
		Ratio:    0.25,
		Workers:  4,
		Ports:    []int{8000, 8001},
		Addr:     netip.MustParseAddr("192.0.2.1"),
		Renamed:  "by its tag",
		Excluded: "default",
		Cache:    cacheSettings{TTL: 5 * time.Minute, Backend: "redis"},
		Labels:   map[string]string{"a": "b"},
	})
}

// A field name becomes a variable name in upper snake case, with an acronym
// kept together.
func TestNamespaceVariableNames(t *testing.T) {
	type names struct {
		PageSize      string
		APIKey        string
		HTTPOnly      string
		URL           string
		UserID        string
		MaxTTLSeconds string
		Level2Cache   string
		X             string
	}
	register(t, "Ns_Names", names{})
	s := overlaid(t, map[string]string{
		"P_NS_NAMES_PAGE_SIZE":       "PageSize",
		"P_NS_NAMES_API_KEY":         "APIKey",
		"P_NS_NAMES_HTTP_ONLY":       "HTTPOnly",
		"P_NS_NAMES_URL":             "URL",
		"P_NS_NAMES_USER_ID":         "UserID",
		"P_NS_NAMES_MAX_TTL_SECONDS": "MaxTTLSeconds",
		"P_NS_NAMES_LEVEL2_CACHE":    "Level2Cache",
		"P_NS_NAMES_X":               "X",
	})

	got, err := conf.Get[names](s, "Ns_Names")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, names{
		PageSize:      "PageSize",
		APIKey:        "APIKey",
		HTTPOnly:      "HTTPOnly",
		URL:           "URL",
		UserID:        "UserID",
		MaxTTLSeconds: "MaxTTLSeconds",
		Level2Cache:   "Level2Cache",
		X:             "X",
	})
}

// The fields of an embedded struct are read without its name, those of a
// named nested struct with it, and a tag on the struct replaces the name.
func TestNamespaceNestedStructs(t *testing.T) {
	type nested struct {
		cacheSettings
		Primary cacheSettings
		Tagged  cacheSettings `env:"SECOND"`
	}
	register(t, "ns_nested", nested{})
	s := overlaid(t, map[string]string{
		"P_NS_NESTED_TTL":            "1",
		"P_NS_NESTED_PRIMARY_TTL":    "2",
		"P_NS_NESTED_SECOND_TTL":     "3",
		"P_NS_NESTED_SECOND_BACKEND": "redis",
		"P_NS_NESTED_TAGGED_TTL":     "4",
	})

	got, err := conf.Get[nested](s, "ns_nested")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, nested{
		cacheSettings: cacheSettings{TTL: time.Second},
		Primary:       cacheSettings{TTL: 2 * time.Second},
		Tagged:        cacheSettings{TTL: 3 * time.Second, Backend: "redis"},
	})
}

// The environment wins over a value set in code, whichever comes first.
func TestNamespaceEnvironmentWinsOverCode(t *testing.T) {
	register(t, "ns_order", blogSettings{PageSize: 10})
	vars := map[string]string{"P_NS_ORDER_PAGE_SIZE": "50"}
	code := blogSettings{PageSize: 20, Title: "From code"}
	want := blogSettings{PageSize: 50, Title: "From code"}

	t.Run("set before the overlay", func(t *testing.T) {
		s := conf.Defaults()
		if err := s.SetNamespace("ns_order", code); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyEnvFunc("P_", mapEnv(vars)); err != nil {
			t.Fatal(err)
		}
		got, err := conf.Get[blogSettings](&s, "ns_order")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got, want)
	})
	t.Run("set after the overlay", func(t *testing.T) {
		s := overlaid(t, vars)
		if err := s.SetNamespace("ns_order", code); err != nil {
			t.Fatal(err)
		}
		got, err := conf.Get[blogSettings](s, "ns_order")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got, want)
	})
	// The app may be registered only after the project built its settings.
	t.Run("registered after the overlay", func(t *testing.T) {
		s := overlaid(t, map[string]string{"P_NS_LATE_PAGE_SIZE": "50"})
		register(t, "ns_late", blogSettings{PageSize: 10})
		got, err := conf.Get[blogSettings](s, "ns_late")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got, blogSettings{PageSize: 50})
	})
}

// A namespace reads every environment the settings were overlaid with, the
// later one over the earlier one.
func TestNamespaceReadsEveryOverlay(t *testing.T) {
	register(t, "ns_chain", blogSettings{})
	s := conf.Defaults()
	first := map[string]string{"A_NS_CHAIN_PAGE_SIZE": "1", "A_NS_CHAIN_TITLE": "first"}
	second := map[string]string{"B_NS_CHAIN_TITLE": "second"}
	if err := s.ApplyEnvFunc("A_", mapEnv(first)); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEnvFunc("B_", mapEnv(second)); err != nil {
		t.Fatal(err)
	}

	got, err := conf.Get[blogSettings](&s, "ns_chain")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{PageSize: 1, Title: "second"})
}

// A malformed variable is reported by name and its field keeps its value; the
// value of a variable whose name suggests a secret is not shown.
func TestGetMalformedVariables(t *testing.T) {
	type secrets struct {
		Limit    int
		APIToken int
		Password time.Duration
		Enabled  bool
		Addr     netip.Addr
		Labels   map[string]string
		Matrix   [][]string
		Title    string
	}
	register(t, "ns_bad", secrets{Limit: 5, Enabled: true})
	s := overlaid(t, map[string]string{
		"P_NS_BAD_LIMIT":     "many",
		"P_NS_BAD_API_TOKEN": "hunter2",
		"P_NS_BAD_PASSWORD":  "hunter2",
		"P_NS_BAD_ENABLED":   "perhaps",
		"P_NS_BAD_ADDR":      "nowhere",
		"P_NS_BAD_LABELS":    "a=b",
		"P_NS_BAD_MATRIX":    "a,b",
		"P_NS_BAD_TITLE":     "well-formed",
	})

	got, err := conf.Get[secrets](s, "ns_bad")

	djangotest.AssertEqual(t, got, secrets{Limit: 5, Enabled: true, Title: "well-formed"})
	djangotest.AssertEqual(t, errorLines(err), []string{
		`conf: P_NS_BAD_LIMIT: invalid integer "many"`,
		`conf: P_NS_BAD_API_TOKEN: invalid integer (the value is not shown)`,
		`conf: P_NS_BAD_PASSWORD: invalid duration (the value is not shown) ` +
			`(want a number of seconds or a Go duration such as "36h")`,
		`conf: P_NS_BAD_ENABLED: invalid boolean "perhaps"`,
		`conf: P_NS_BAD_ADDR: invalid value "nowhere": ParseAddr("nowhere"): unable to parse IP`,
		`conf: P_NS_BAD_LABELS: a field of the type map[string]string cannot be set from the environment`,
		`conf: P_NS_BAD_MATRIX: a field of the type [][]string cannot be set from the environment`,
	})
}

// Get refuses an unregistered name, a type other than the registered one and
// nil settings.
func TestGetErrors(t *testing.T) {
	register(t, "ns_typed", blogSettings{PageSize: 10})
	s := conf.Defaults()

	_, err := conf.Get[blogSettings](&s, "ns_unknown")
	djangotest.AssertErrorIs(t, err, conf.ErrNamespaceNotRegistered)
	djangotest.AssertEqual(t, err.Error(), `conf: settings namespace is not registered: "ns_unknown"`)

	got, err := conf.Get[otherSettings](&s, "ns_typed")
	djangotest.AssertEqual(t, errorLines(err), []string{
		`conf: namespace "ns_typed" holds a conf_test.blogSettings, not a conf_test.otherSettings`,
	})
	djangotest.AssertEqual(t, got, otherSettings{})

	_, err = conf.Get[*blogSettings](&s, "ns_typed")
	djangotest.AssertEqual(t, errorLines(err), []string{
		`conf: namespace "ns_typed" holds a conf_test.blogSettings, not a *conf_test.blogSettings`,
	})

	_, err = conf.Get[blogSettings](nil, "ns_typed")
	djangotest.AssertEqual(t, errorLines(err), []string{"conf: Get called with nil settings"})
}

// SetNamespace refuses an unregistered name and a value of another type.
func TestSetNamespaceErrors(t *testing.T) {
	register(t, "ns_set", blogSettings{PageSize: 10})
	s := conf.Defaults()

	err := s.SetNamespace("ns_unknown", blogSettings{})
	djangotest.AssertErrorIs(t, err, conf.ErrNamespaceNotRegistered)

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"another type", otherSettings{}, `conf: namespace "ns_set": the value is a conf_test.otherSettings, want a conf_test.blogSettings`},
		{"a pointer", &blogSettings{}, `conf: namespace "ns_set": the value is a *conf_test.blogSettings, want a conf_test.blogSettings`},
		{"nil", nil, `conf: namespace "ns_set": the value is a <nil>, want a conf_test.blogSettings`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.SetNamespace("ns_set", tt.value)
			djangotest.AssertEqual(t, errorLines(err), []string{tt.want})
		})
	}

	// A refused value leaves the namespace as it was.
	got, err := conf.Get[blogSettings](&s, "ns_set")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{PageSize: 10})
}
