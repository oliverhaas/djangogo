package conf_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// Register refuses a name or a type that cannot work, and a namespace whose
// variables another reader already owns.
func TestRegisterErrors(t *testing.T) {
	const badName = " (want letters, digits and underscores, starting with a letter)"
	register(t, "ns_taken", blogSettings{})
	register(t, "ns_collide", struct{ Inner struct{ Field string } }{})

	tests := []struct {
		name     string
		register func() error
		want     string
	}{
		{
			"empty name",
			func() error { return conf.Register("", blogSettings{}) },
			`conf: invalid namespace name ""` + badName,
		},
		{
			"name that starts with a digit",
			func() error { return conf.Register("1st", blogSettings{}) },
			`conf: invalid namespace name "1st"` + badName,
		},
		{
			"name that starts with an underscore",
			func() error { return conf.Register("_private", blogSettings{}) },
			`conf: invalid namespace name "_private"` + badName,
		},
		{
			"name with a dash",
			func() error { return conf.Register("my-app", blogSettings{}) },
			`conf: invalid namespace name "my-app"` + badName,
		},
		{
			"not a struct",
			func() error { return conf.Register("ns_int", 10) },
			`conf: namespace "ns_int": int is not a struct type`,
		},
		{
			"pointer to a struct",
			func() error { return conf.Register("ns_pointer", &blogSettings{}) },
			`conf: namespace "ns_pointer": *conf_test.blogSettings is not a struct type`,
		},
		{
			"name taken by another type",
			func() error { return conf.Register("ns_taken", otherSettings{}) },
			`conf: namespace "ns_taken" is already registered with the type conf_test.blogSettings, not conf_test.otherSettings`,
		},
		{
			"variable of a setting",
			func() error { return conf.Register("session", struct{ CookieAge int }{}) },
			`conf: namespace "session": the variable SESSION_COOKIE_AGE is already read by the settings`,
		},
		{
			"variable of the default database",
			func() error { return conf.Register("database", struct{ URL string }{}) },
			`conf: namespace "database": the variable DATABASE_URL is already read by the settings`,
		},
		{
			"variable of a database alias",
			func() error { return conf.Register("database_url", struct{ Replica string }{}) },
			`conf: namespace "database_url": the variable DATABASE_URL_REPLICA is already read by the settings`,
		},
		{
			"variable of another namespace",
			func() error { return conf.Register("ns_collide_inner", struct{ Field string }{}) },
			`conf: namespace "ns_collide_inner": the variable NS_COLLIDE_INNER_FIELD is already read by the namespace "ns_collide"`,
		},
		{
			"name that differs from another only in its letter case",
			func() error { return conf.Register("NS_TAKEN", blogSettings{}) },
			`conf: namespace "NS_TAKEN": the variable NS_TAKEN_PAGE_SIZE is already read by the namespace "ns_taken"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			djangotest.AssertEqual(t, errorLines(tt.register()), []string{tt.want})
		})
	}

	// A refused registration registers nothing.
	names := conf.Namespaces()
	for _, refused := range []string{"", "ns_int", "ns_pointer", "session", "database", "database_url", "ns_collide_inner", "NS_TAKEN"} {
		if slices.Contains(names, refused) {
			t.Errorf("the refused namespace %q is registered", refused)
		}
	}
	got, err := conf.Get[blogSettings](&conf.Settings{}, "ns_taken")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{})
}

// Registering a name again with the same type replaces its defaults, which a
// test that registers in every run relies on.
func TestRegisterAgainReplacesTheDefaults(t *testing.T) {
	register(t, "ns_again", blogSettings{PageSize: 10})
	register(t, "ns_again", blogSettings{PageSize: 25})

	got, err := conf.Get[blogSettings](&conf.Settings{}, "ns_again")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, blogSettings{PageSize: 25})
}

// Namespaces lists every registered name in a stable order.
func TestNamespacesAreSorted(t *testing.T) {
	register(t, "ns_sorted_b", blogSettings{})
	register(t, "ns_sorted_a", blogSettings{})
	register(t, "Ns_Sorted_C", blogSettings{})

	names := conf.Namespaces()
	if !slices.IsSorted(names) {
		t.Errorf("Namespaces() is not sorted: %q", names)
	}
	for _, name := range []string{"ns_sorted_a", "ns_sorted_b", "Ns_Sorted_C"} {
		if !slices.Contains(names, name) {
			t.Errorf("Namespaces() lacks %q: %q", name, names)
		}
	}
}

// Get hands out a copy: a caller that changes a slice or a map of its result
// changes neither the registered defaults nor a value set in code, and the
// registry does not share them with the code that registered or set them.
func TestGetDoesNotAliasTheStoredValues(t *testing.T) {
	defaults := blogSettings{
		Tags:   []string{"a", "b"},
		Ports:  []int{1},
		Labels: map[string]string{"k": "v"},
	}
	want := blogSettings{
		Tags:   []string{"a", "b"},
		Ports:  []int{1},
		Labels: map[string]string{"k": "v"},
	}
	register(t, "ns_alias", defaults)
	defaults.Tags[0] = "changed after Register"
	defaults.Labels["k"] = "changed after Register"
	s := conf.Defaults()

	first, err := conf.Get[blogSettings](&s, "ns_alias")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, first, want)
	first.Tags[0] = "changed in the result"
	first.Ports[0] = 2
	first.Labels["k"] = "changed in the result"
	first.Labels["new"] = "added to the result"

	second, err := conf.Get[blogSettings](&s, "ns_alias")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, second, want)

	code := blogSettings{Tags: []string{"c"}, Labels: map[string]string{"k": "code"}}
	if err := s.SetNamespace("ns_alias", code); err != nil {
		t.Fatal(err)
	}
	code.Tags[0] = "changed after SetNamespace"
	code.Labels["k"] = "changed after SetNamespace"
	third, err := conf.Get[blogSettings](&s, "ns_alias")
	if err != nil {
		t.Fatal(err)
	}
	third.Tags[0] = "changed in the result"
	third.Labels["k"] = "changed in the result"

	fourth, err := conf.Get[blogSettings](&s, "ns_alias")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, fourth, blogSettings{Tags: []string{"c"}, Labels: map[string]string{"k": "code"}})
}

// The slices of an embedded struct are copied like any other.
func TestGetDoesNotAliasAnEmbeddedStruct(t *testing.T) {
	type inner struct{ Tags []string }
	type outer struct{ inner }
	register(t, "ns_alias_embedded", outer{inner{Tags: []string{"a"}}})
	s := conf.Defaults()

	first, err := conf.Get[outer](&s, "ns_alias_embedded")
	if err != nil {
		t.Fatal(err)
	}
	first.Tags[0] = "changed in the result"

	second, err := conf.Get[outer](&s, "ns_alias_embedded")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, second.Tags, []string{"a"})
}

// The race detector watches this test: readers of one settings value and
// registrations run at the same time.
func TestGetConcurrently(t *testing.T) {
	register(t, "ns_concurrent", blogSettings{Tags: []string{"a"}, Labels: map[string]string{"k": "v"}})
	s := overlaid(t, map[string]string{"P_NS_CONCURRENT_PAGE_SIZE": "7"})
	if err := s.SetNamespace("ns_concurrent", blogSettings{Tags: []string{"a"}, Labels: map[string]string{"k": "v"}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				got, err := conf.Get[blogSettings](s, "ns_concurrent")
				if err != nil || got.PageSize != 7 || !slices.Equal(got.Tags, []string{"a"}) || got.Labels["k"] != "v" {
					t.Errorf("Get returned %+v, %v", got, err)
					return
				}
				got.Tags[0] = "changed"
				got.Labels["k"] = "changed"
			}
		})
		wg.Go(func() {
			for range 50 {
				if err := conf.Register("ns_concurrent_other", otherSettings{Name: "x"}); err != nil {
					t.Error(err)
					return
				}
				conf.Namespaces()
				if errs := s.NamespaceErrors(); len(errs) != 0 {
					t.Errorf("NamespaceErrors() = %v", errs)
					return
				}
			}
		})
	}
	wg.Wait()
}

// errTooLarge is what the Validate method of limitSettings wraps.
var errTooLarge = errors.New("max must not be greater than 100")

// limitSettings validates itself through a value receiver.
type limitSettings struct {
	Max int
}

// Validate reports a limit that is out of range.
func (l limitSettings) Validate() error {
	if l.Max > 100 {
		return errTooLarge
	}
	return nil
}

// namedSettings validates itself through a pointer receiver.
type namedSettings struct {
	Name string
}

// Validate reports a missing name.
func (n *namedSettings) Validate() error {
	if n.Name == "" {
		return errors.New("name must not be empty")
	}
	return nil
}

// NamespaceErrors reports the malformed variables of every namespace and what
// the Validate method of a value returns.
func TestNamespaceErrors(t *testing.T) {
	register(t, "ns_limits", limitSettings{Max: 10})
	register(t, "ns_named", namedSettings{Name: "default"})
	// Every error is one line, so the joined text lists them.
	texts := func(errs []error) []string { return errorLines(errors.Join(errs...)) }

	t.Run("valid", func(t *testing.T) {
		s := overlaid(t, map[string]string{"P_NS_LIMITS_MAX": "100"})
		djangotest.AssertEqual(t, texts(s.NamespaceErrors()), nil)
	})
	t.Run("an environment value that fails Validate", func(t *testing.T) {
		s := overlaid(t, map[string]string{"P_NS_LIMITS_MAX": "101"})
		errs := s.NamespaceErrors()

		djangotest.AssertEqual(t, texts(errs), []string{
			`conf: namespace "ns_limits": max must not be greater than 100`,
		})
		djangotest.AssertErrorIs(t, errs[0], errTooLarge)
	})
	t.Run("a code value that fails Validate", func(t *testing.T) {
		s := conf.Defaults()
		if err := s.SetNamespace("ns_named", namedSettings{}); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, texts(s.NamespaceErrors()), []string{
			`conf: namespace "ns_named": name must not be empty`,
		})
	})
	t.Run("malformed variables, one error each", func(t *testing.T) {
		register(t, "ns_malformed", blogSettings{})
		s := overlaid(t, map[string]string{
			"P_NS_LIMITS_MAX":          "many",
			"P_NS_MALFORMED_PAGE_SIZE": "ten",
			"P_NS_MALFORMED_HTTP_ONLY": "perhaps",
		})
		djangotest.AssertEqual(t, texts(s.NamespaceErrors()), []string{
			`conf: P_NS_LIMITS_MAX: invalid integer "many"`,
			`conf: P_NS_MALFORMED_PAGE_SIZE: invalid integer "ten"`,
			`conf: P_NS_MALFORMED_HTTP_ONLY: invalid boolean "perhaps"`,
		})
	})
}
