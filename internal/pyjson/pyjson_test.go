package pyjson_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/internal/pyjson"
)

// No Django test exists: the expected values are the output of Python's
// json.dumps with its default ensure_ascii.
func TestWriteString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", `""`},
		{"plain text", `"plain text"`},
		{"quote \" and backslash \\", `"quote \" and backslash \\"`},
		{"\n\r\t\b\f", `"\n\r\t\b\f"`},
		{"\x00\x1f\x7f", `"\u0000\u001f\u007f"`},
		{"caf\u00e9 \u2019", `"caf\u00e9 \u2019"`},
		{"\U0001F600", `"\ud83d\ude00"`},
		{"</script>", `"</script>"`},
	}
	for _, tt := range tests {
		var b strings.Builder
		pyjson.WriteString(&b, tt.in)
		if got := b.String(); got != tt.want {
			t.Errorf("WriteString(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

// No Django test exists: verify Object.Get returns the last matching member as a Python dict built from the document does.
func TestObjectGet(t *testing.T) {
	obj := pyjson.Object{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
		{Key: "a", Value: 3},
	}
	val, ok := obj.Get("a")
	if !ok || val != 3 {
		t.Errorf("Get(\"a\") = %v, %v; want 3, true", val, ok)
	}
	val, ok = obj.Get("b")
	if !ok || val != 2 {
		t.Errorf("Get(\"b\") = %v, %v; want 2, true", val, ok)
	}
	if _, ok := obj.Get("missing"); ok {
		t.Errorf("Get(\"missing\") found a member, want false")
	}
}

// No Django test exists: verify Decode parses primitives, arrays and ordered objects with json.Number and rejects trailing data.
func TestDecode(t *testing.T) {
	raw := "  \n\t {\"z\": [1, -2.5, true, false, null, \"hi\"], \"a\": {\"x\": 10}, \"z\": 99} \r\n "
	got, err := pyjson.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("Decode unexpected error: %v", err)
	}
	want := pyjson.Object{
		{
			Key: "z",
			Value: []any{
				json.Number("1"),
				json.Number("-2.5"),
				true,
				false,
				nil,
				"hi",
			},
		},
		{
			Key: "a",
			Value: pyjson.Object{
				{Key: "x", Value: json.Number("10")},
			},
		},
		{
			Key:   "z",
			Value: json.Number("99"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode = %#v, want %#v", got, want)
	}
}

// No Django test exists: verify Decode returns an error on trailing tokens, malformed JSON, or empty input.
func TestDecodeErrors(t *testing.T) {
	tests := []string{
		"",
		"   ",
		`{"a": 1} {"b": 2}`,
		`123 456`,
		`true false`,
		`{"a": 1} trailing`,
		`[1, 2`,
		`{"a": }`,
	}
	for _, in := range tests {
		if got, err := pyjson.Decode([]byte(in)); err == nil {
			t.Errorf("Decode(%q) = %#v, nil; want error", in, got)
		}
	}
}

// No Django test exists: verify FormatFloat matches Python repr(float) and json.dumps float formatting.
func TestFormatFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{0.1, "0.1"},
		{1.0, "1.0"},
		{-1.0, "-1.0"},
		{1.5, "1.5"},
		{1e16, "1e+16"},
		{-1e16, "-1e+16"},
		{1e15, "1000000000000000.0"},
		{999999999999999.0, "999999999999999.0"},
		{1e-05, "1e-05"},
		{-1e-05, "-1e-05"},
		{0.0001, "0.0001"},
		{0.000099999, "9.9999e-05"},
		{123456789.123, "123456789.123"},
		{1.2345678901234567e+100, "1.2345678901234567e+100"},
		{1.23e-100, "1.23e-100"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}
	for _, tt := range tests {
		if got := pyjson.FormatFloat(tt.in); got != tt.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// No Django test exists: verify Marshal string escaping with EnsureASCII on and off matches Python json.dumps.
func TestMarshalStrings(t *testing.T) {
	tests := []struct {
		in           string
		wantASCII    string
		wantNonASCII string
	}{
		{"", `""`, `""`},
		{"plain text", `"plain text"`, `"plain text"`},
		{"quote \" and backslash \\", `"quote \" and backslash \\"`, `"quote \" and backslash \\"`},
		{"\n\r\t\b\f", `"\n\r\t\b\f"`, `"\n\r\t\b\f"`},
		{"\x00\x01\x1f\x7f", `"\u0000\u0001\u001f\u007f"`, "\"\\u0000\\u0001\\u001f\x7f\""},
		{"caf\u00e9 \u2019", `"caf\u00e9 \u2019"`, "\"café ’\""},
		{"\U0001F600", `"\ud83d\ude00"`, "\"😀\""},
		{"</script>", `"</script>"`, `"</script>"`},
	}
	for _, tt := range tests {
		gotASCII, err := pyjson.Marshal(tt.in, pyjson.Options{Indent: -1, EnsureASCII: true})
		if err != nil || gotASCII != tt.wantASCII {
			t.Errorf("Marshal(%q, EnsureASCII=true) = %s, %v; want %s", tt.in, gotASCII, err, tt.wantASCII)
		}
		gotNonASCII, err := pyjson.Marshal(tt.in, pyjson.Options{Indent: -1, EnsureASCII: false})
		if err != nil || gotNonASCII != tt.wantNonASCII {
			t.Errorf("Marshal(%q, EnsureASCII=false) = %s, %v; want %s", tt.in, gotNonASCII, err, tt.wantNonASCII)
		}
	}
}

// No Django test exists: verify Marshal indentation and custom separators match Python json.dumps for nested objects and arrays.
func TestMarshalIndentAndSeparators(t *testing.T) {
	nested := pyjson.Object{
		{Key: "z", Value: []any{1, 2, []any{}, pyjson.Object{}}},
		{
			Key: "a",
			Value: pyjson.Object{
				{Key: "inner", Value: true},
				{Key: "none", Value: nil},
				{Key: "empty_list", Value: []any{}},
				{Key: "empty_dict", Value: pyjson.Object{}},
			},
		},
	}

	tests := []struct {
		name string
		opts pyjson.Options
		want string
	}{
		{
			name: "indent_none",
			opts: pyjson.Options{Indent: -1},
			want: `{"z": [1, 2, [], {}], "a": {"inner": true, "none": null, "empty_list": [], "empty_dict": {}}}`,
		},
		{
			name: "indent_0",
			opts: pyjson.Options{Indent: 0},
			want: "{\n\"z\": [\n1,\n2,\n[],\n{}\n],\n\"a\": {\n\"inner\": true,\n\"none\": null,\n\"empty_list\": [],\n\"empty_dict\": {}\n}\n}",
		},
		{
			name: "indent_2",
			opts: pyjson.Options{Indent: 2},
			want: "{\n  \"z\": [\n    1,\n    2,\n    [],\n    {}\n  ],\n  \"a\": {\n    \"inner\": true,\n    \"none\": null,\n    \"empty_list\": [],\n    \"empty_dict\": {}\n  }\n}",
		},
		{
			name: "indent_4",
			opts: pyjson.Options{Indent: 4},
			want: "{\n    \"z\": [\n        1,\n        2,\n        [],\n        {}\n    ],\n    \"a\": {\n        \"inner\": true,\n        \"none\": null,\n        \"empty_list\": [],\n        \"empty_dict\": {}\n    }\n}",
		},
		{
			name: "custom_sep_compact",
			opts: pyjson.Options{Indent: -1, ItemSep: ",", KeySep: ":"},
			want: `{"z":[1,2,[],{}],"a":{"inner":true,"none":null,"empty_list":[],"empty_dict":{}}}`,
		},
		{
			name: "custom_sep_indent_2",
			opts: pyjson.Options{Indent: 2, ItemSep: " ; ", KeySep: " = "},
			want: "{\n  \"z\" = [\n    1 ; \n    2 ; \n    [] ; \n    {}\n  ] ; \n  \"a\" = {\n    \"inner\" = true ; \n    \"none\" = null ; \n    \"empty_list\" = [] ; \n    \"empty_dict\" = {}\n  }\n}",
		},
	}

	for _, tt := range tests {
		got, err := pyjson.Marshal(nested, tt.opts)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s:\ngot:  %q\nwant: %q", tt.name, got, tt.want)
		}
	}
}

// No Django test exists: verify Marshal handles all Go integer types, floats, typed slices, arrays, and sorted string maps.
func TestMarshalTypes(t *testing.T) {
	type customKey string
	val := pyjson.Object{
		{Key: "ints", Value: []any{int(-1), int8(-8), int16(-16), int32(-32), int64(-64)}},
		{Key: "uints", Value: []any{uint(1), uint8(8), uint16(16), uint32(32), uint64(64), uintptr(128)}},
		{Key: "num", Value: json.Number("123456789012345678901234567890")},
		{Key: "f32", Value: float32(1.5)},
		{Key: "f64", Value: float64(1e16)},
		{Key: "typed_slice", Value: []int{10, 20}},
		{Key: "array", Value: [2]string{"x", "y"}},
		{Key: "map", Value: map[string]any{"b": 2, "a": 1, "c": 3}},
		{Key: "typed_map", Value: map[customKey]int{"z": 26, "a": 1}},
	}
	got, err := pyjson.Marshal(val, pyjson.Options{Indent: -1})
	if err != nil {
		t.Fatalf("Marshal unexpected error: %v", err)
	}
	want := `{"ints": [-1, -8, -16, -32, -64], "uints": [1, 8, 16, 32, 64, 128], "num": 123456789012345678901234567890, "f32": 1.5, "f64": 1e+16, "typed_slice": [10, 20], "array": ["x", "y"], "map": {"a": 1, "b": 2, "c": 3}, "typed_map": {"a": 1, "z": 26}}`
	if got != want {
		t.Errorf("Marshal =\n%s\nwant:\n%s", got, want)
	}
}

// customPayload is a test struct that requires a Default hook to serialize.
type customPayload struct {
	Label string
}

// No Django test exists: verify Default hook and Python's TypeError message when Default is nil.
func TestMarshalDefaultAndTypeError(t *testing.T) {
	_, err := pyjson.Marshal(customPayload{Label: "x"}, pyjson.Options{Indent: -1})
	if err == nil || err.Error() != "Object of type customPayload is not JSON serializable" {
		t.Errorf("Marshal without Default error = %v, want 'Object of type customPayload is not JSON serializable'", err)
	}

	_, err = pyjson.Marshal([]byte("raw"), pyjson.Options{Indent: -1})
	if err == nil || err.Error() != "Object of type []uint8 is not JSON serializable" {
		t.Errorf("Marshal([]byte) without Default error = %v, want 'Object of type []uint8 is not JSON serializable'", err)
	}

	opts := pyjson.Options{
		Indent: -1,
		Default: func(v any) (any, error) {
			switch x := v.(type) {
			case customPayload:
				return pyjson.Object{{Key: "label", Value: x.Label}}, nil
			case []byte:
				return string(x), nil
			default:
				return nil, err
			}
		},
	}
	got, err := pyjson.Marshal([]any{customPayload{Label: "ok"}, []byte("bytes")}, opts)
	if err != nil {
		t.Fatalf("Marshal with Default error: %v", err)
	}
	want := `[{"label": "ok"}, "bytes"]`
	if got != want {
		t.Errorf("Marshal with Default = %s, want %s", got, want)
	}
}

// No Django test exists: verify Decode followed by Marshal preserves object key order byte for byte.
func TestDecodeMarshalRoundTrip(t *testing.T) {
	input := "{\n  \"model\": \"auth.user\",\n  \"pk\": 1,\n  \"fields\": {\n    \"username\": \"café\",\n    \"is_active\": true,\n    \"score\": 1000000000000000.0,\n    \"small\": 1e-05,\n    \"tags\": [\n      \"z\",\n      \"a\"\n    ],\n    \"extra\": null\n  }\n}"
	decoded, err := pyjson.Decode([]byte(input))
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	got, err := pyjson.Marshal(decoded, pyjson.Options{Indent: 2, EnsureASCII: false})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if got != input {
		t.Errorf("RoundTrip mismatch:\ngot:\n%s\nwant:\n%s", got, input)
	}
}
