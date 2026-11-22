package templates_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// relativeTemplates are the templates of Django's
// tests/template_tests/relative_templates.
var relativeTemplates = map[string]string{
	"dir1/dir2/inc1.html":            `{% include "./../../three.html" %}` + "\n",
	"dir1/dir2/inc2.html":            `{% include "./include_content.html" %}` + "\n",
	"dir1/dir2/inc3.html":            "{% include tmpl %}\n",
	"dir1/dir2/include_content.html": "dir2 include\n",
	"dir1/dir2/one.html":             `{% extends "./../../one.html" %}` + "\n\n{% block content %}{{ block.super }} dir2 one{% endblock %}\n",
	"dir1/looped.html":               `{% extends "./dir2/../looped.html" %}` + "\n\n{% block content %}{{ block.super }} dir1 three{% endblock %}\n",
	"dir1/one.html":                  `{% extends "./../one.html" %}` + "\n\n{% block content %}{{ block.super }} dir1 one{% endblock %}\n",
	"dir1/one1.html":                 `{% extends './../one.html' %}` + "\n\n{% block content %}{{ block.super }} dir1 one{% endblock %}\n",
	"dir1/one2.html":                 `{% extends '../one.html' %}` + "\n\n{% block content %}{{ block.super }} dir1 one{% endblock %}\n",
	"dir1/one3.html":                 `{% extends "../one.html" %}` + "\n\n{% block content %}{{ block.super }} dir1 one{% endblock %}\n",
	"dir1/three.html":                `{% extends "./dir2/../../three.html" %}` + "\n\n{% block content %}{{ block.super }} dir1 three{% endblock %}\n",
	"dir1/two.html":                  `{% extends "./dir2/one.html" %}` + "\n\n{% block content %}{{ block.super }} dir1 two{% endblock %}\n",
	"error_extends.html":             `{% extends "./../two.html" %}` + "\n\n{% block content %}{{ block.super }} one{% endblock %}\n",
	"error_include.html":             `{% include "./../three.html" %}` + "\n",
	"one.html":                       `{% extends "./two.html" %}` + "\n\n{% block content %}{{ block.super }} one{% endblock %}\n",
	"one_var.html":                   "{% extends tmpl %}\n\n{% block content %}{{ block.super }} one{% endblock %}\n",
	"three.html":                     "{% block content %}three{% endblock %}\n",
	"two.html":                       `{% extends "./three.html" %}` + "\n\n{% block content %}{{ block.super }} two{% endblock %}\n",
}

// relativeEngine returns an engine over a directory with relativeTemplates.
func relativeEngine(t *testing.T) *templates.Engine {
	t.Helper()
	return templates.New(templates.DirSource(writeTemplates(t, relativeTemplates)))
}

// TestExtendsRelative checks {% extends %} with a name relative to the
// template of the tag.
//
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_normal_extend
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_normal_extend_variable
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_dir1_extend
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_dir1_extend1
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_dir1_extend2
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_dir1_extend3
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_dir2_extend
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsMixedBehaviorTests.test_mixing1
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsMixedBehaviorTests.test_mixing2
func TestExtendsRelative(t *testing.T) {
	engine := relativeEngine(t)
	for _, tc := range []struct {
		name string
		ctx  map[string]any
		want string
	}{
		{"one.html", nil, "three two one"},
		{"one_var.html", map[string]any{"tmpl": "./two.html"}, "three two one"},
		{"dir1/one.html", nil, "three two one dir1 one"},
		{"dir1/one1.html", nil, "three two one dir1 one"},
		{"dir1/one2.html", nil, "three two one dir1 one"},
		{"dir1/one3.html", nil, "three two one dir1 one"},
		{"dir1/dir2/one.html", nil, "three two one dir2 one"},
		{"dir1/two.html", nil, "three two one dir2 one dir1 two"},
		{"dir1/three.html", nil, "three dir1 three"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.TrimSpace(mustRender(t, engine, tc.name, tc.ctx)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIncludeRelative checks {% include %} with a name relative to the
// template of the tag, as a literal and from a variable.
//
// Ported from Django: tests/template_tests/test_extends_relative.py::IncludeRelativeBehaviorTests.test_normal_include
// Ported from Django: tests/template_tests/test_extends_relative.py::IncludeRelativeBehaviorTests.test_normal_include_variable
// Ported from Django: tests/template_tests/test_extends_relative.py::IncludeRelativeBehaviorTests.test_dir2_include
func TestIncludeRelative(t *testing.T) {
	engine := relativeEngine(t)
	for _, tc := range []struct {
		name string
		ctx  map[string]any
		want string
	}{
		{"dir1/dir2/inc2.html", nil, "dir2 include"},
		{"dir1/dir2/inc3.html", map[string]any{"tmpl": "./include_content.html"}, "dir2 include"},
		{"dir1/dir2/inc1.html", nil, "three"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.TrimSpace(mustRender(t, engine, tc.name, tc.ctx)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRelativeErrors checks the errors of a relative name that leaves the
// directories of the sources, that names the template of the tag itself or
// that is in a template given as a string.
//
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsRelativeBehaviorTests.test_extend_error
// Ported from Django: tests/template_tests/test_extends_relative.py::IncludeRelativeBehaviorTests.test_include_error
// Ported from Django: tests/template_tests/test_extends_relative.py::ExtendsMixedBehaviorTests.test_mixing_loop
func TestRelativeErrors(t *testing.T) {
	engine := relativeEngine(t)
	for _, tc := range []struct {
		name string
		ctx  map[string]any
		want string
	}{
		{"error_extends.html", nil, `The relative path '"./../two.html"' points outside the file hierarchy that template 'error_extends.html' is in.`},
		{"error_include.html", nil, `The relative path '"./../three.html"' points outside the file hierarchy that template 'error_include.html' is in.`},
		{"dir1/looped.html", nil, `The relative path '"./dir2/../looped.html"' was translated to template name 'dir1/looped.html', the same template in which the tag appears.`},
		// No Django test exists for the rows below.
		{"dir1/dir2/inc3.html", map[string]any{"tmpl": "../../../x.html"}, `The relative path '../../../x.html' points outside the file hierarchy that template 'dir1/dir2/inc3.html' is in.`},
		{"dir1/dir2/inc3.html", map[string]any{"tmpl": "./inc3.html"}, `The relative path './inc3.html' was translated to template name 'dir1/dir2/inc3.html', the same template in which the tag appears.`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Render(tc.name, tc.ctx)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one with %q", err, tc.want)
			}
		})
	}
	// No Django test exists for a relative name in a template given as a
	// string.
	for src, want := range map[string]string{
		`{% extends "./base.html" %}`: `The relative path "./base.html" cannot be evaluated due to an unknown template origin.`,
		`{% include '../x.html' %}`:   `The relative path '../x.html' cannot be evaluated due to an unknown template origin.`,
		`{% include tmpl %}`:          `The relative path ./x.html cannot be evaluated due to an unknown template origin.`,
	} {
		_, err := engine.RenderString(src, map[string]any{"tmpl": "./x.html"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want one with %q", src, err, want)
		}
	}
}

// TestIncludeRelativeSelf checks that a literal {% include %} may name the
// template of the tag through a relative name, for a recursive include.
//
// No Django test exists for a relative include of the template itself.
func TestIncludeRelativeSelf(t *testing.T) {
	engine := templates.New(templates.MapSource("locmem", map[string]string{
		"tree/node.html": `{{ n }}{% if n %}{% include "./node.html" with n=n|add:"-1" %}{% endif %}`,
	}))
	if got := mustRender(t, engine, "tree/node.html", map[string]any{"n": 2}); got != "210" {
		t.Errorf("got %q, want %q", got, "210")
	}
}
