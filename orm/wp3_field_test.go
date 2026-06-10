package orm

import (
	"strings"
	"testing"
	"time"
)

// fieldOf registers ptr in a fresh registry and returns the named field, failing
// the test on a registration error.
func fieldOf(t *testing.T, ptr any, name string) *Field {
	t.Helper()
	r := NewRegistry()
	m, err := r.Register(ptr)
	if err != nil {
		t.Fatalf("Register(%T): %v", ptr, err)
	}
	f, ok := m.FieldByName(name)
	if !ok {
		t.Fatalf("field %q not found on %T", name, ptr)
	}
	return f
}

// registerErr registers ptr and returns the error (nil on success).
func registerErr(ptr any) error {
	_, err := NewRegistry().Register(ptr)
	return err
}

func TestDefaultIntParsed(t *testing.T) {
	type M struct {
		ID    int64
		Count int `orm:"default=42"`
	}
	f := fieldOf(t, &M{}, "Count")
	if !f.HasDefault || f.Default != int64(42) {
		t.Errorf("HasDefault=%v Default=%#v, want true / int64(42)", f.HasDefault, f.Default)
	}
}

func TestDefaultBoolParsed(t *testing.T) {
	type M struct {
		ID     int64
		Active bool `orm:"default=true"`
	}
	f := fieldOf(t, &M{}, "Active")
	if !f.HasDefault || f.Default != true {
		t.Errorf("Default=%#v, want true", f.Default)
	}
}

func TestDefaultStringParsed(t *testing.T) {
	type M struct {
		ID     int64
		Status string `orm:"default=draft"`
	}
	f := fieldOf(t, &M{}, "Status")
	if !f.HasDefault || f.Default != "draft" {
		t.Errorf("Default=%#v, want \"draft\"", f.Default)
	}
}

func TestDefaultEmptyStringIsLegal(t *testing.T) {
	type M struct {
		ID     int64
		Status string `orm:"default="`
	}
	f := fieldOf(t, &M{}, "Status")
	if !f.HasDefault || f.Default != "" {
		t.Errorf("HasDefault=%v Default=%#v, want true / \"\"", f.HasDefault, f.Default)
	}
}

func TestDefaultIntInvalid(t *testing.T) {
	type M struct {
		ID    int64
		Count int `orm:"default=abc"`
	}
	if err := registerErr(&M{}); err == nil {
		t.Error("expected an error for non-integer int default")
	}
}

func TestDefaultBoolInvalid(t *testing.T) {
	type M struct {
		ID  int64
		Yes bool `orm:"default=yes"`
	}
	if err := registerErr(&M{}); err == nil {
		t.Error("expected an error for non-bool bool default")
	}
}

func TestDefaultOnDateTimeRejected(t *testing.T) {
	type M struct {
		ID  int64
		At  time.Time `orm:"default=now"`
		Pad string
	}
	err := registerErr(&M{})
	if err == nil || !strings.Contains(err.Error(), "datetime") {
		t.Errorf("err = %v, want a datetime-related error", err)
	}
}

func TestDefaultOnFKRejected(t *testing.T) {
	type Target struct{ ID int64 }
	type M struct {
		ID  int64
		Ref FK[Target] `orm:"default=1"`
	}
	err := registerErr(&M{})
	if err == nil || !strings.Contains(err.Error(), "relation field") {
		t.Errorf("err = %v, want a relation-field error", err)
	}
}

func TestAutoNowAddSetsFlag(t *testing.T) {
	type M struct {
		ID        int64
		CreatedAt time.Time `orm:"auto_now_add"`
	}
	f := fieldOf(t, &M{}, "CreatedAt")
	if !f.AutoNowAdd || f.AutoNow {
		t.Errorf("AutoNowAdd=%v AutoNow=%v, want true/false", f.AutoNowAdd, f.AutoNow)
	}
}

func TestAutoNowSetsFlag(t *testing.T) {
	type M struct {
		ID        int64
		UpdatedAt time.Time `orm:"auto_now"`
	}
	f := fieldOf(t, &M{}, "UpdatedAt")
	if !f.AutoNow || f.AutoNowAdd {
		t.Errorf("AutoNow=%v AutoNowAdd=%v, want true/false", f.AutoNow, f.AutoNowAdd)
	}
}

func TestAutoNowOnNonDateTimeRejected(t *testing.T) {
	type M struct {
		ID   int64
		Name string `orm:"auto_now"`
	}
	if err := registerErr(&M{}); err == nil {
		t.Error("expected an error for auto_now on a non-datetime field")
	}
}

func TestAutoNowMutuallyExclusive(t *testing.T) {
	type M struct {
		ID int64
		At time.Time `orm:"auto_now;auto_now_add"`
	}
	err := registerErr(&M{})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("err = %v, want a mutual-exclusion error", err)
	}
}

func TestUnknownTagStillErrors(t *testing.T) {
	type M struct {
		ID   int64
		Name string `orm:"bogus"`
	}
	if err := registerErr(&M{}); err == nil {
		t.Error("expected an error for an unknown tag option (hard-error invariant)")
	}
}

// choiceModel declares choices on its Status field via the withChoices hook.
type choiceModel struct {
	ID     int64
	Status string `orm:"max_length=20"`
}

func (choiceModel) Choices() map[string][]Choice {
	return map[string][]Choice{
		"Status": {{Value: "draft", Label: "Draft"}, {Value: "published", Label: "Published"}},
	}
}

func TestChoicesFromMethodPopulateField(t *testing.T) {
	f := fieldOf(t, &choiceModel{}, "Status")
	if len(f.Choices) != 2 {
		t.Fatalf("len(Choices) = %d, want 2", len(f.Choices))
	}
	if f.Choices[0].Value != "draft" || f.Choices[0].Label != "Draft" {
		t.Errorf("Choices[0] = %+v, want {draft Draft}", f.Choices[0])
	}
}

// choicesUnknownField names a field that does not exist.
type choicesUnknownField struct {
	ID   int64
	Name string
}

func (choicesUnknownField) Choices() map[string][]Choice {
	return map[string][]Choice{"Nope": {{Value: "x", Label: "X"}}}
}

func TestChoicesUnknownFieldRejected(t *testing.T) {
	err := registerErr(&choicesUnknownField{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("err = %v, want an unknown-field error", err)
	}
}

// choicesOnInt declares choices on a non-string field.
type choicesOnInt struct {
	ID    int64
	Level int
}

func (choicesOnInt) Choices() map[string][]Choice {
	return map[string][]Choice{"Level": {{Value: "1", Label: "One"}}}
}

func TestChoicesOnNonStringRejected(t *testing.T) {
	err := registerErr(&choicesOnInt{})
	if err == nil || !strings.Contains(err.Error(), "string field") {
		t.Errorf("err = %v, want a string-field error", err)
	}
}

func TestNowDefaultIsUTCAndCurrent(t *testing.T) {
	db := &DB{}
	got := db.now()
	if got.Location() != time.UTC {
		t.Errorf("now() location = %v, want UTC", got.Location())
	}
	if d := time.Since(got); d > time.Minute || d < -time.Minute {
		t.Errorf("now() = %v, not close to current time", got)
	}
}

func TestNowOverrideHonored(t *testing.T) {
	fixed := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	db := &DB{Now: func() time.Time { return fixed }}
	if got := db.now(); !got.Equal(fixed) {
		t.Errorf("now() = %v, want %v", got, fixed)
	}
}
