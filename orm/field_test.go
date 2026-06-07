// Package orm provides the metadata primitives for the Djan-Go-Go ORM.
package orm

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// sample is a representative struct used in tests.
type sample struct {
	ID      int64
	Title   string `orm:"max_length=200"`
	Body    string `orm:"type=text"`
	Slug    string `orm:"column=url_slug;unique"`
	Active  bool
	Created time.Time
	Note    string `orm:"null"`
	Code    string `orm:"pk"`
	Ignored string `orm:"-"`
	secret  string //nolint:unused // intentionally unexported to test skipping
}

// fieldIndex returns the index of the struct field with the given name.
func fieldIndex(t reflect.Type, name string) int {
	for i := range t.NumField() {
		if t.Field(i).Name == name {
			return i
		}
	}
	return -1
}

func TestParseStructField_ID(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "ID")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Name != "ID" {
		t.Errorf("Name: got %q, want %q", f.Name, "ID")
	}
	if f.Column != "id" {
		t.Errorf("Column: got %q, want %q", f.Column, "id")
	}
	if f.Kind != KindInt {
		t.Errorf("Kind: got %v, want KindInt", f.Kind)
	}
	if f.PrimaryKey {
		t.Error("PrimaryKey should be false (not promoted here)")
	}
	if f.Index != i {
		t.Errorf("Index: got %d, want %d", f.Index, i)
	}
}

func TestParseStructField_Title(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Title")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Kind != KindChar {
		t.Errorf("Kind: got %v, want KindChar", f.Kind)
	}
	if f.MaxLength != 200 {
		t.Errorf("MaxLength: got %d, want 200", f.MaxLength)
	}
	if f.Column != "title" {
		t.Errorf("Column: got %q, want %q", f.Column, "title")
	}
}

func TestParseStructField_Body(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Body")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Kind != KindText {
		t.Errorf("Kind: got %v, want KindText", f.Kind)
	}
	if f.MaxLength != 0 {
		t.Errorf("MaxLength: got %d, want 0", f.MaxLength)
	}
}

func TestParseStructField_Slug(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Slug")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Kind != KindChar {
		t.Errorf("Kind: got %v, want KindChar", f.Kind)
	}
	if f.Column != "url_slug" {
		t.Errorf("Column: got %q, want %q", f.Column, "url_slug")
	}
	if !f.Unique {
		t.Error("Unique should be true")
	}
	if f.MaxLength != 255 {
		t.Errorf("MaxLength: got %d, want 255", f.MaxLength)
	}
}

func TestParseStructField_Active(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Active")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Kind != KindBool {
		t.Errorf("Kind: got %v, want KindBool", f.Kind)
	}
}

func TestParseStructField_Created(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Created")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if f.Kind != KindDateTime {
		t.Errorf("Kind: got %v, want KindDateTime", f.Kind)
	}
}

func TestParseStructField_Note(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Note")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if !f.Null {
		t.Error("Null should be true")
	}
	if f.Kind != KindChar {
		t.Errorf("Kind: got %v, want KindChar", f.Kind)
	}
}

func TestParseStructField_Code(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Code")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keep {
		t.Fatal("expected keep=true")
	}
	if !f.PrimaryKey {
		t.Error("PrimaryKey should be true")
	}
	if f.Kind != KindChar {
		t.Errorf("Kind: got %v, want KindChar", f.Kind)
	}
}

func TestParseStructField_Ignored(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "Ignored")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep {
		t.Errorf("expected keep=false for orm:\"-\", got field: %+v", f)
	}
}

func TestParseStructField_Secret(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(sample{})
	i := fieldIndex(typ, "secret")
	sf := typ.Field(i)

	f, keep, err := parseStructField(sf, i)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep {
		t.Errorf("expected keep=false for unexported field, got field: %+v", f)
	}
}

// --- Negative / error cases ---

func TestParseStructField_UnknownTag(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Name string `orm:"bogus"`
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for unknown tag option")
	}
	if !strings.Contains(err.Error(), "unknown tag option") {
		t.Errorf("error message should mention 'unknown tag option': %v", err)
	}
}

func TestParseStructField_MaxLengthNonNumeric(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Name string `orm:"max_length=abc"`
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for non-numeric max_length")
	}
}

func TestParseStructField_MaxLengthZero(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Name string `orm:"max_length=0"`
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for max_length=0")
	}
}

func TestParseStructField_EmptyColumn(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Name string `orm:"column="`
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for empty column name")
	}
}

func TestParseStructField_TypeTextOnInt(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Count int `orm:"type=text"`
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for type=text on non-string field")
	}
	if !strings.Contains(err.Error(), "type=text") {
		t.Errorf("error message should mention 'type=text': %v", err)
	}
}

func TestParseStructField_UnsupportedType(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(struct {
		Data []byte
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for unsupported type []byte")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("error message should mention 'unsupported type': %v", err)
	}
}

func TestParseStructField_UnsupportedStruct(t *testing.T) {
	t.Parallel()
	type myStruct struct{ X int }
	typ := reflect.TypeOf(struct {
		Nested myStruct
	}{})
	sf := typ.Field(0)

	_, _, err := parseStructField(sf, 0)
	if err == nil {
		t.Fatal("expected error for unsupported struct type")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("error message should mention 'unsupported type': %v", err)
	}
}

// --- Kind.String ---

func TestKindString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    Kind
		want string
	}{
		{KindAuto, "Auto"},
		{KindInt, "Int"},
		{KindChar, "Char"},
		{KindText, "Text"},
		{KindBool, "Bool"},
		{KindDateTime, "DateTime"},
		{Kind(99), "Kind(99)"},
	}
	for _, c := range cases {
		got := c.k.String()
		if got != c.want {
			t.Errorf("Kind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}
