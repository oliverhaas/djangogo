package templates_test

import (
	"testing"
)

// TestJsonScript ports Django's JsonScriptTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_json_script.py::JsonScriptTests.test_basic
// Ported from Django: tests/template_tests/filter_tests/test_json_script.py::JsonScriptTests.test_without_id
func TestJsonScript(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_basic", src: `{{ value|json_script:"test_id" }}`, ctx: map[string]any{"value": map[string]any{"a": "testing\r\njson 'string\" <b>escaping</b>"}}, want: `<script id="test_id" type="application/json">{"a": "testing\r\njson 'string\" \u003Cb\u003Eescaping\u003C/b\u003E"}</script>`},
		{name: "test_without_id", src: "{{ value|json_script }}", ctx: map[string]any{"value": map[string]any{}}, want: `<script type="application/json">{}</script>`},
	})
}
