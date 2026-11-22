package templates_test

import (
	"testing"
	"time"
)

// TestRegroupTag ports Django's RegroupTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup01
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup02
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup03
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup04
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup05
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup06
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup07
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup08
// Ported from Django: tests/template_tests/syntax_tests/test_regroup.py::RegroupTagTests.test_regroup_unpack
func TestRegroupTag(t *testing.T) {
	data := []any{
		map[string]any{"foo": "c", "bar": 1},
		map[string]any{"foo": "d", "bar": 1},
		map[string]any{"foo": "a", "bar": 2},
		map[string]any{"foo": "b", "bar": 2},
		map[string]any{"foo": "x", "bar": 3},
	}
	day := func(m time.Month, d int) time.Time { return time.Date(2012, m, d, 0, 0, 0, 0, time.UTC) }
	runRenderCases(t, []renderCase{
		{
			name: "test_regroup01",
			src: "{% regroup data by bar as grouped %}" +
				"{% for group in grouped %}{{ group.grouper }}:" +
				"{% for item in group.list %}{{ item.foo }}{% endfor %},{% endfor %}",
			ctx:  map[string]any{"data": data},
			want: "1:cd,2:ab,3:x,",
		},
		{
			// A missing target variable gives an empty list and no error.
			name: "test_regroup02",
			src: "{% regroup data by bar as grouped %}" +
				"{% for group in grouped %}{{ group.grouper }}:" +
				"{% for item in group.list %}{{ item.foo }}{% endfor %}{% endfor %}",
			ctx:  map[string]any{},
			want: "",
		},
		{
			name: "test_regroup03",
			src: `{% regroup data by at|date:"m" as grouped %}` +
				"{% for group in grouped %}{{ group.grouper }}:" +
				`{% for item in group.list %}{{ item.at|date:"d" }}{% endfor %},{% endfor %}`,
			ctx: map[string]any{"data": []any{
				map[string]any{"at": day(time.February, 14)},
				map[string]any{"at": day(time.February, 28)},
				map[string]any{"at": day(time.July, 4)},
			}},
			want: "02:1428,07:04,",
		},
		{
			name: "test_regroup04",
			src: `{% regroup data by bar|join:"" as grouped %}` +
				"{% for group in grouped %}{{ group.grouper }}:" +
				"{% for item in group.list %}{{ item.foo|first }}{% endfor %},{% endfor %}",
			ctx: map[string]any{"data": []any{
				map[string]any{"foo": "x", "bar": []any{"ab", "c"}},
				map[string]any{"foo": "y", "bar": []any{"a", "bc"}},
				map[string]any{"foo": "z", "bar": []any{"a", "d"}},
			}},
			want: "abc:xy,ad:z,",
		},
		{name: "test_regroup05", src: "{% regroup data by bar as %}", wantErr: true},
		{name: "test_regroup06", src: "{% regroup data by bar thisaintright grouped %}", wantErr: true},
		{name: "test_regroup07", src: "{% regroup data thisaintright bar as grouped %}", wantErr: true},
		{name: "test_regroup08", src: "{% regroup data by bar as grouped toomanyargs %}", wantErr: true},
		{
			name: "test_regroup_unpack",
			src: "{% regroup data by bar as grouped %}" +
				"{% for grouper, group in grouped %}{{ grouper }}:" +
				"{% for item in group %}{{ item.foo }}{% endfor %},{% endfor %}",
			ctx:  map[string]any{"data": data},
			want: "1:cd,2:ab,3:x,",
		},
	})
}

// TestRegroupByRelatedObject groups by a value that is not a string and
// reads an attribute of the grouper, the way a template groups rows by a
// foreign key.
//
// No Django test exists for grouping by an object; the docs show
// {% regroup cities by country as country_list %} with country.grouper.
func TestRegroupByRelatedObject(t *testing.T) {
	de := map[string]any{"name": "Germany"}
	fr := map[string]any{"name": "France"}
	runRenderCases(t, []renderCase{{
		name: "grouper_attribute",
		src: "{% regroup cities by country as countries %}" +
			"{% for c in countries %}{{ c.grouper.name }}={{ c.list|length }};{% endfor %}",
		ctx: map[string]any{"cities": []any{
			map[string]any{"name": "Berlin", "country": de},
			map[string]any{"name": "Hamburg", "country": de},
			map[string]any{"name": "Paris", "country": fr},
		}},
		want: "Germany=2;France=1;",
	}})
}
