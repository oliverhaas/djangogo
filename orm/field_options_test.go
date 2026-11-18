package orm

import (
	"strings"
	"testing"
	"time"
)

type fieldOptsTarget struct {
	ID int64
}

type fieldOptsModel struct {
	ID       int64
	Title    string              `orm:"max_length=120;verbose_name=headline;help_text=Shown on the list page"`
	Subtitle string              `orm:"blank;index"`
	Slug     string              `orm:"editable=false"`
	Created  time.Time           `orm:"auto_now_add"`
	Updated  time.Time           `orm:"auto_now"`
	Owner    FK[fieldOptsTarget] `orm:"null;blank;index;verbose_name=owning user;help_text=Who owns it;editable=false"`
}

func TestCommonFieldTagOptions(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Register(&fieldOptsTarget{}); err != nil {
		t.Fatalf("Register target: %v", err)
	}
	m, err := r.Register(&fieldOptsModel{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	field := func(name string) *Field {
		t.Helper()
		f, ok := m.FieldByName(name)
		if !ok {
			t.Fatalf("no field %s", name)
		}
		return f
	}

	title := field("Title")
	if title.VerboseName != "headline" || title.HelpText != "Shown on the list page" {
		t.Errorf("Title verbose=%q help=%q", title.VerboseName, title.HelpText)
	}
	if title.Blank || !title.Editable || title.DBIndex {
		t.Errorf("Title defaults: blank=%v editable=%v index=%v; want false true false", title.Blank, title.Editable, title.DBIndex)
	}

	sub := field("Subtitle")
	if !sub.Blank || !sub.DBIndex || sub.Null {
		t.Errorf("Subtitle blank=%v index=%v null=%v; want true true false (blank is independent of null)", sub.Blank, sub.DBIndex, sub.Null)
	}

	if field("Slug").Editable {
		t.Error("editable=false should clear Editable")
	}

	// Auto-stamped fields are owned by the ORM: not editable, and blank.
	for _, name := range []string{"Created", "Updated"} {
		f := field(name)
		if f.Editable || !f.Blank {
			t.Errorf("%s editable=%v blank=%v; want false true", name, f.Editable, f.Blank)
		}
	}

	owner := field("Owner")
	if !owner.Null || !owner.Blank || !owner.DBIndex || owner.Editable {
		t.Errorf("Owner null=%v blank=%v index=%v editable=%v", owner.Null, owner.Blank, owner.DBIndex, owner.Editable)
	}
	if owner.VerboseName != "owning user" || owner.HelpText != "Who owns it" {
		t.Errorf("Owner verbose=%q help=%q", owner.VerboseName, owner.HelpText)
	}
}

type fieldOptsBadEditable struct {
	ID   int64
	Name string `orm:"editable=maybe"`
}

func TestEditableTagRejectsUnknownValue(t *testing.T) {
	_, err := NewRegistry().Register(&fieldOptsBadEditable{})
	if err == nil || !strings.Contains(err.Error(), "editable must be true or false") {
		t.Errorf("err = %v, want editable must be true or false", err)
	}
}
