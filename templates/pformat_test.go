package templates

import (
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// TestPformat compares pformat with what Python 3.12's pprint.pformat gives
// for the same values.
//
// No Django test exists for the pprint filter.
func TestPformat(t *testing.T) {
	items := make([]any, 8)
	for i := range items {
		items[i] = "item number " + string(rune('0'+i))
	}
	dict := map[string]any{}
	for i := range 5 {
		dict["key"+string(rune('0'+i))] = "value number " + string(rune('0'+i))
	}
	rows := make([]any, 3)
	for r := range rows {
		row := make([]any, 4)
		for c := range row {
			row[c] = "cell " + string(rune('0'+r)) + "." + string(rune('0'+c))
		}
		rows[r] = row
	}
	lorem := strings.Repeat("lorem ipsum dolor sit amet ", 5)
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"str", "abc", "'abc'"},
		{"quote", "it's", `"it's"`},
		{"both", `it's "x"`, `'it\'s "x"'`},
		{"none", nil, "None"},
		{"list", []any{"a", 1, 2.5, nil, true}, "['a', 1, 2.5, None, True]"},
		{"dict", map[string]any{"b": []any{1, 2}, "a": map[string]any{"x": 1e16}}, "{'a': {'x': 1e+16}, 'b': [1, 2]}"},
		{"empty_list", []any{}, "[]"},
		{"empty_dict", map[string]any{}, "{}"},
		{
			"long_list", items,
			"['item number 0',\n 'item number 1',\n 'item number 2',\n 'item number 3',\n" +
				" 'item number 4',\n 'item number 5',\n 'item number 6',\n 'item number 7']",
		},
		{
			"long_dict", dict,
			"{'key0': 'value number 0',\n 'key1': 'value number 1',\n 'key2': 'value number 2',\n" +
				" 'key3': 'value number 3',\n 'key4': 'value number 4'}",
		},
		{
			"nested",
			map[string]any{"title": "x", "rows": rows},
			"{'rows': [['cell 0.0', 'cell 0.1', 'cell 0.2', 'cell 0.3'],\n" +
				"          ['cell 1.0', 'cell 1.1', 'cell 1.2', 'cell 1.3'],\n" +
				"          ['cell 2.0', 'cell 2.1', 'cell 2.2', 'cell 2.3']],\n 'title': 'x'}",
		},
		{
			"long_str", lorem,
			"('lorem ipsum dolor sit amet lorem ipsum dolor sit amet lorem ipsum dolor sit '\n" +
				" 'amet lorem ipsum dolor sit amet lorem ipsum dolor sit amet ')",
		},
		{
			"long_str_lines", "first line of the text\nsecond line that is rather long, really long, much longer than the first one\n",
			"('first line of the text\\n'\n" +
				" 'second line that is rather long, really long, much longer than the first '\n 'one\\n')",
		},
		{
			"list_of_long_str",
			[]any{strings.Repeat("lorem ipsum dolor sit amet ", 4), "x"},
			"['lorem ipsum dolor sit amet lorem ipsum dolor sit amet lorem ipsum dolor sit '\n" +
				" 'amet lorem ipsum dolor sit amet ',\n 'x']",
		},
		{"float_small", 1e-05, "1e-05"},
		{"date", orm.Date{Year: 2012, Month: 2, Day: 14}, "datetime.date(2012, 2, 14)"},
		{
			"datetime", time.Date(2012, 2, 14, 3, 4, 5, 0, time.UTC),
			"datetime.datetime(2012, 2, 14, 3, 4, 5, tzinfo=datetime.timezone.utc)",
		},
		{"timedelta", -24*time.Hour + 5*time.Second, "datetime.timedelta(days=-1, seconds=5)"},
		{"escapes", "tab\there\x00\u00e9\u2028", "'tab\\there\\x00\u00e9\\u2028'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pformat(tc.in); got != tc.want {
				t.Errorf("pformat(%#v) =\n%s\nwant\n%s", tc.in, got, tc.want)
			}
		})
	}
}

// TestPprintFilter renders the filter: autoescape escapes its output, and a
// safe input gives a safe output, as for a filter Django registers with
// is_safe=True.
//
// No Django test exists for the pprint filter.
func TestPprintFilter(t *testing.T) {
	engine := New(MapSource("locmem", map[string]string{
		"plain": "{{ v|pprint }}",
		"safe":  "{{ v|safe|pprint }}",
	}))
	for _, tc := range []struct{ name, want string }{
		{"plain", "[&#x27;&lt;b&gt;&#x27;, 1]"},
		{"safe", "'<b>'"},
	} {
		v := any([]any{"<b>", 1})
		if tc.name == "safe" {
			v = "<b>"
		}
		got, err := engine.Render(tc.name, map[string]any{"v": v})
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}
