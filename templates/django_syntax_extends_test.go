package templates_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// inheritanceTemplates are the named templates of Django's InheritanceTests.
var inheritanceTemplates = map[string]string{
	"inheritance01":     `1{% block first %}&{% endblock %}3{% block second %}_{% endblock %}`,
	"inheritance02":     `{% extends 'inheritance01' %}{% block first %}2{% endblock %}{% block second %}4{% endblock %}`,
	"inheritance03":     `{% extends 'inheritance02' %}`,
	"inheritance04":     `{% extends 'inheritance01' %}`,
	"inheritance05":     `{% extends 'inheritance02' %}`,
	"inheritance06":     `{% extends foo %}`,
	"inheritance07":     `{% extends 'inheritance01' %}{% block second %}5{% endblock %}`,
	"inheritance08":     `{% extends 'inheritance02' %}{% block second %}5{% endblock %}`,
	"inheritance09":     `{% extends 'inheritance04' %}`,
	"inheritance10":     `{% extends 'inheritance04' %}      `,
	"inheritance11":     `{% extends 'inheritance04' %}{% block first %}2{% endblock %}{% block second %}4{% endblock %}`,
	"inheritance12":     `{% extends 'inheritance07' %}{% block first %}2{% endblock %}`,
	"inheritance13":     `{% extends 'inheritance02' %}{% block first %}a{% endblock %}{% block second %}b{% endblock %}`,
	"inheritance14":     `{% extends 'inheritance01' %}{% block newblock %}NO DISPLAY{% endblock %}`,
	"inheritance15":     `{% extends 'inheritance01' %}{% block first %}2{% block inner %}inner{% endblock %}{% endblock %}`,
	"inheritance16":     `{% extends 'inheritance15' %}{% block inner %}out{% endblock %}`,
	"inheritance17":     `{% load testtags %}{% block first %}1234{% endblock %}`,
	"inheritance18":     `{% load testtags %}{% echo this that theother %}5678`,
	"inheritance19":     `{% extends 'inheritance01' %}{% block first %}{% load testtags %}{% echo 400 %}5678{% endblock %}`,
	"inheritance20":     `{% extends 'inheritance01' %}{% block first %}{{ block.super }}a{% endblock %}`,
	"inheritance21":     `{% extends 'inheritance02' %}{% block first %}{{ block.super }}a{% endblock %}`,
	"inheritance22":     `{% extends 'inheritance04' %}{% block first %}{{ block.super }}a{% endblock %}`,
	"inheritance23":     `{% extends 'inheritance20' %}{% block first %}{{ block.super }}b{% endblock %}`,
	"inheritance24":     `{% extends context_template %}{% block first %}2{% endblock %}{% block second %}4{% endblock %}`,
	"inheritance25":     `{% extends context_template.1 %}{% block first %}2{% endblock %}{% block second %}4{% endblock %}`,
	"inheritance26":     `no tags`,
	"inheritance27":     `{% extends 'inheritance26' %}`,
	"inheritance 28":    `{% block first %}!{% endblock %}`,
	"inheritance29":     `{% extends 'inheritance 28' %}`,
	"inheritance30":     `1{% if optional %}{% block opt %}2{% endblock %}{% endif %}3`,
	"inheritance31":     `{% extends 'inheritance30' %}{% block opt %}two{% endblock %}`,
	"inheritance32":     `{% extends 'inheritance30' %}{% block opt %}two{% endblock %}`,
	"inheritance33":     `1{% if optional == 1 %}{% block opt %}2{% endblock %}{% endif %}3`,
	"inheritance34":     `{% extends 'inheritance33' %}{% block opt %}two{% endblock %}`,
	"inheritance35":     `{% extends 'inheritance33' %}{% block opt %}two{% endblock %}`,
	"inheritance36":     `{% for n in numbers %}_{% block opt %}{{ n }}{% endblock %}{% endfor %}_`,
	"inheritance37":     `{% extends 'inheritance36' %}{% block opt %}X{% endblock %}`,
	"inheritance38":     `{% extends 'inheritance36' %}{% block opt %}X{% endblock %}`,
	"inheritance39":     `{% extends 'inheritance30' %}{% block opt %}new{{ block.super }}{% endblock %}`,
	"inheritance40":     `{% extends 'inheritance33' %}{% block opt %}new{{ block.super }}{% endblock %}`,
	"inheritance41":     `{% extends 'inheritance36' %}{% block opt %}new{{ block.super }}{% endblock %}`,
	"inheritance42":     `{% extends 'inheritance02'|cut:' ' %}`,
	"inheritance_empty": `{% extends %}`,
	"extends_duplicate": `{% extends 'base.html' %}{% extends 'base.html' %}`,
	"duplicate_block":   `{% extends 'base.html' %}{% block content %}2{% endblock %}{% block content %}4{% endblock %}`,
}

// TestInheritance ports Django's InheritanceTests. Each row renders one named
// template of inheritanceTemplates.
//
// An error row gives the text the message holds.
//
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance01
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance02
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance03
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance04
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance05
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance06
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance07
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance08
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance09
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance10
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance11
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance12
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance13
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance14
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance15
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance16
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance17
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance18
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance19
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance20
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance21
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance22
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance23
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance24
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance25
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance26
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance27
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance_28
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance29
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance30
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance31
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance32
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance33
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance34
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance35
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance36
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance37
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance38
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance39
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance40
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance41
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance42
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_inheritance_empty
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_extends_duplicate
// Ported from Django: tests/template_tests/syntax_tests/test_extends.py::InheritanceTests.test_duplicate_block
func TestInheritance(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		ctx     map[string]any
		want    string
		skip    string
		wantErr string
	}{
		{"inheritance01", "inheritance01", nil, "1&3_", "", ""},
		{"inheritance02", "inheritance02", nil, "1234", "", ""},
		{"inheritance03", "inheritance03", nil, "1234", "", ""},
		{"inheritance04", "inheritance04", nil, "1&3_", "", ""},
		{"inheritance05", "inheritance05", nil, "1234", "", ""},
		{"inheritance06", "inheritance06", map[string]any{"foo": "inheritance02"}, "1234", "", ""},
		{"inheritance07", "inheritance07", nil, "1&35", "", ""},
		{"inheritance08", "inheritance08", nil, "1235", "", ""},
		{"inheritance09", "inheritance09", nil, "1&3_", "", ""},
		{"inheritance10", "inheritance10", nil, "1&3_", "", ""},
		{"inheritance11", "inheritance11", nil, "1234", "", ""},
		{"inheritance12", "inheritance12", nil, "1235", "", ""},
		{"inheritance13", "inheritance13", nil, "1a3b", "", ""},
		{"inheritance14", "inheritance14", nil, "1&3_", "", ""},
		{"inheritance15", "inheritance15", nil, "12inner3_", "", ""},
		{"inheritance16", "inheritance16", nil, "12out3_", "", ""},
		{"inheritance17", "inheritance17", nil, "1234", "", ""},
		{"inheritance18", "inheritance18", nil, "this that theother5678", "the testtags library ({% echo %}) is a Python tag library with no Go equivalent here", ""},
		{"inheritance19", "inheritance19", nil, "140056783_", "the testtags library ({% echo %}) is a Python tag library with no Go equivalent here", ""},
		{"inheritance20", "inheritance20", nil, "1&a3_", "", ""},
		{"inheritance21", "inheritance21", nil, "12a34", "", ""},
		{"inheritance22", "inheritance22", nil, "1&a3_", "", ""},
		{"inheritance23", "inheritance23", nil, "1&ab3_", "", ""},
		{"inheritance24", "inheritance24", nil, "1234", "No Go counterpart: the parent is a Django Template object in the context", ""},
		{"inheritance25", "inheritance25", nil, "1234", "No Go counterpart: the parent is a Django Template object in the context", ""},
		{"inheritance26", "inheritance26", nil, "no tags", "", ""},
		{"inheritance27", "inheritance27", nil, "no tags", "", ""},
		{"inheritance_28", "inheritance 28", nil, "!", "", ""},
		{"inheritance29", "inheritance29", nil, "!", "", ""},
		{"inheritance30", "inheritance30", map[string]any{"optional": true}, "123", "", ""},
		{"inheritance31", "inheritance31", map[string]any{"optional": true}, "1two3", "", ""},
		{"inheritance32", "inheritance32", nil, "13", "", ""},
		{"inheritance33", "inheritance33", map[string]any{"optional": 1}, "123", "", ""},
		{"inheritance34", "inheritance34", map[string]any{"optional": 1}, "1two3", "", ""},
		{"inheritance35", "inheritance35", map[string]any{"optional": 2}, "13", "", ""},
		{"inheritance36", "inheritance36", map[string]any{"numbers": "123"}, "_1_2_3_", "", ""},
		{"inheritance37", "inheritance37", map[string]any{"numbers": "123"}, "_X_X_X_", "", ""},
		{"inheritance38", "inheritance38", nil, "_", "", ""},
		{"inheritance39", "inheritance39", map[string]any{"optional": true}, "1new23", "", ""},
		{"inheritance40", "inheritance40", map[string]any{"optional": 1}, "1new23", "", ""},
		{"inheritance41", "inheritance41", map[string]any{"numbers": "123"}, "_new1_new2_new3_", "", ""},
		{"inheritance42", "inheritance42", nil, "1234", "", ""},
		{"inheritance_empty", "inheritance_empty", nil, "", "", "'extends' takes one argument"},
		{"extends_duplicate", "extends_duplicate", nil, "", "", "'extends' cannot appear more than once in the same template"},
		{"duplicate_block", "duplicate_block", nil, "", "", "'block' tag with name 'content' appears more than once"},
	}
	eng := templates.New(templates.MapSource("locmem", inheritanceTemplates))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			got, err := eng.Render(tt.tmpl, tt.ctx)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("render %q: error = %v, want one with %q", tt.tmpl, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("render %q: %v", tt.tmpl, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNamedEndblock ports Django's NamedEndblockTests: {% endblock name %} must
// match the innermost open block.
//
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks01
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks02
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks03
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks04
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks05
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks06
// Ported from Django: tests/template_tests/syntax_tests/test_named_endblock.py::NamedEndblockTests.test_namedendblocks07
func TestNamedEndblock(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    string
		skip    string
		wantErr bool
	}{
		{"namedendblocks01", "1{% block first %}_{% block second %}2{% endblock second %}_{% endblock first %}3", "1_2_3", "", false},
		{"namedendblocks02", "1{% block first %}_{% block second %}2{% endblock first %}_{% endblock second %}3", "", "", true},
		{"namedendblocks03", "1{% block first %}_{% block second %}2{% endblock %}_{% endblock second %}3", "", "", true},
		{"namedendblocks04", "1{% block first %}_{% block second %}2{% endblock second %}_{% endblock third %}3", "", "", true},
		{"namedendblocks05", "1{% block first %}_{% block second %}2{% endblock first %}", "", "", true},
		{"namedendblocks06", "1{% block first %}_{% block second %}2{% endblock %}_{% endblock first %}3", "1_2_3", "", false},
		{"namedendblocks07", "1{% block first %}_{% block second %}2{% endblock second %}_{% endblock %}3", "1_2_3", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			eng := templates.New(templates.MapSource("locmem", map[string]string{tt.name: tt.src}))
			got, err := eng.Render(tt.name, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("render: want an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMultiline checks that a multi-line template renders unchanged.
//
// Ported from Django: tests/template_tests/syntax_tests/test_multiline.py::MultilineTests.test_multiline01
func TestMultiline(t *testing.T) {
	const multilineString = `
Hello,
boys.
How
are
you
gentlemen.
`
	eng := templates.New(templates.MapSource("locmem", map[string]string{"multiline01": multilineString}))
	got, err := eng.Render("multiline01", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != multilineString {
		t.Errorf("got %q, want %q", got, multilineString)
	}
}

// TestExtendsVariable checks {% extends %} with a variable beyond Django's
// InheritanceTests: a parent that extends a variable, two variables in one
// chain, an include, a template given as a string, the recorded templates
// and the errors. An error never shows the internal name of a variant.
//
// No Django test exists for these cases.
func TestExtendsVariable(t *testing.T) {
	eng := templates.New(templates.MapSource("locmem", map[string]string{
		"base.html":   `<{% block content %}base{% endblock %}>`,
		"other.html":  `[{% block content %}other{% endblock %}]`,
		"layout.html": `{% extends layout %}{% block content %}layout {{ block.super }}{% endblock %}`,
		"page.html":   `{% extends "layout.html" %}{% block content %}page {{ block.super }}{% endblock %}`,
		"chain.html":  `{% extends first %}{% block content %}chain {{ block.super }}{% endblock %}`,
		"inc.html":    `{% include "layout.html" %}|{% include "layout.html" with layout="other.html" %}`,
		"broken.html": `{% extends layout %}{% block content %}{% partial missing %}{% endblock %}`,
		"self.html":   `{% extends name %}`,
		"a.html":      `{% extends "b.html" %}`,
		"b.html":      `{% extends name %}`,
	}))
	tests := []struct {
		name    string
		tmpl    string
		ctx     map[string]any
		want    string
		wantErr string
	}{
		{"variable", "layout.html", map[string]any{"layout": "base.html"}, "<layout base>", ""},
		{"other value", "layout.html", map[string]any{"layout": "other.html"}, "[layout other]", ""},
		{"parent extends a variable", "page.html", map[string]any{"layout": "other.html"}, "[page layout other]", ""},
		{"two variables", "chain.html", map[string]any{"first": "layout.html", "layout": "base.html"}, "<chain layout base>", ""},
		{"include", "inc.html", map[string]any{"layout": "base.html"}, "<layout base>|[layout other]", ""},
		{"missing variable", "layout.html", nil, "", "Invalid template name in 'extends' tag: ''. Got this from the 'layout' variable."},
		{"missing parent", "layout.html", map[string]any{"layout": "nope.html"}, "", "TemplateDoesNotExist: nope.html"},
		{"error in a block", "broken.html", map[string]any{"layout": "base.html"}, "", "Partial 'missing' is not defined in the current template."},
		{"extends itself", "self.html", map[string]any{"name": "self.html"}, "", "TemplateDoesNotExist: self.html (tried: locmem:self.html (Skipped to avoid recursion))"},
		{"cycle", "a.html", map[string]any{"name": "a.html"}, "", "TemplateDoesNotExist: a.html (tried: locmem:a.html (Skipped to avoid recursion))"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eng.Render(tt.tmpl, tt.ctx)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one with %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "\x00") {
					t.Errorf("error %q shows the internal name of a variant", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("string", func(t *testing.T) {
		got, err := eng.RenderString(`{% extends parent %}{% block content %}s {{ block.super }}{% endblock %}`,
			map[string]any{"parent": "layout.html", "layout": "base.html"})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if got != "<s layout base>" {
			t.Errorf("got %q, want %q", got, "<s layout base>")
		}
	})

	t.Run("recorded", func(t *testing.T) {
		rec := &templates.Recorder{}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(templates.WithRecorder(r.Context(), rec))
		if _, err := eng.RenderRequest(r, "page.html", map[string]any{"layout": "other.html"}); err != nil {
			t.Fatalf("RenderRequest: %v", err)
		}
		rendered := rec.Rendered()
		names := make([]string, 0, len(rendered))
		for _, rd := range rendered {
			names = append(names, rd.Name)
		}
		if want := []string{"page.html", "layout.html", "other.html"}; !slices.Equal(names, want) {
			t.Errorf("recorded %q, want %q", names, want)
		}
	})
}
