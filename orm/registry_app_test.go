package orm

import (
	"strings"
	"testing"
)

type appLabelPost struct {
	ID        int64
	Title     string
	CreatedAt int64
	Author    FK[appLabelAuthor] `orm:"column=writer_id"`
}

type appLabelAuthor struct {
	ID   int64
	Name string
}

type appLabelCustomTable struct {
	ID int64
}

func (appLabelCustomTable) Meta() Meta { return Meta{Table: "custom_things"} }

func TestRegisterAppScopesLabelAndTable(t *testing.T) {
	r := NewRegistry()
	author, err := r.RegisterApp("blog", &appLabelAuthor{})
	if err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	if got := author.App(); got != "blog" {
		t.Errorf("App() = %q, want blog", got)
	}
	if got := author.Label(); got != "blog.appLabelAuthor" {
		t.Errorf("Label() = %q, want blog.appLabelAuthor", got)
	}
	if got := author.Table(); got != "blog_applabelauthor" {
		t.Errorf("Table() = %q, want blog_applabelauthor", got)
	}

	custom, err := r.RegisterApp("blog", &appLabelCustomTable{})
	if err != nil {
		t.Fatalf("RegisterApp(custom): %v", err)
	}
	if got := custom.Table(); got != "custom_things" {
		t.Errorf("Meta().Table should win over the app prefix, got %q", got)
	}

	if m, ok := r.Get("blog.appLabelAuthor"); !ok || m != author {
		t.Errorf("Get by label = %v, %v; want the author model", m, ok)
	}
	if m, ok := r.Get("appLabelAuthor"); !ok || m != author {
		t.Errorf("Get by unambiguous bare name = %v, %v; want the author model", m, ok)
	}
	if got := r.AppModels("blog"); len(got) != 2 || got[0] != author || got[1] != custom {
		t.Errorf("AppModels(blog) = %v, want [author custom]", got)
	}
	if got := r.AppModels("shop"); len(got) != 0 {
		t.Errorf("AppModels(shop) = %v, want none", got)
	}
}

func TestRegisterWithoutAppKeepsBareNames(t *testing.T) {
	r := NewRegistry()
	m, err := r.Register(&appLabelAuthor{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if m.App() != "" || m.Label() != "appLabelAuthor" || m.Table() != "applabelauthor" {
		t.Errorf("app=%q label=%q table=%q; want empty app, bare label, bare table", m.App(), m.Label(), m.Table())
	}
}

func TestRegisterAppRejectsDuplicates(t *testing.T) {
	r := NewRegistry()
	if _, err := r.RegisterApp("blog", &appLabelAuthor{}); err != nil {
		t.Fatalf("RegisterApp: %v", err)
	}
	// The same Go type may not be registered twice, even under another app.
	if _, err := r.RegisterApp("shop", &appLabelAuthor{}); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("re-registering a type: err = %v, want already registered", err)
	}
}

type appLabelClashA struct {
	ID int64
}

func (appLabelClashA) Meta() Meta { return Meta{Table: "clash"} }

type appLabelClashB struct {
	ID int64
}

func (appLabelClashB) Meta() Meta { return Meta{Table: "clash"} }

func TestRegisterRejectsDuplicateTable(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Register(&appLabelClashA{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := r.Register(&appLabelClashB{}); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("duplicate table: err = %v, want already used", err)
	}
}

func TestFieldAttrAndLookups(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Register(&appLabelAuthor{}); err != nil {
		t.Fatalf("Register author: %v", err)
	}
	m, err := r.Register(&appLabelPost{})
	if err != nil {
		t.Fatalf("Register post: %v", err)
	}

	created, ok := m.FieldByAttr("created_at")
	if !ok || created.Name != "CreatedAt" {
		t.Fatalf("FieldByAttr(created_at) = %v, %v", created, ok)
	}

	// A foreign key's attribute name is the relation name; its column carries the
	// _id suffix (or a column= override) and never changes the attribute name.
	fk, ok := m.FieldByAttr("author")
	if !ok || fk.Name != "Author" {
		t.Fatalf("FieldByAttr(author) = %v, %v", fk, ok)
	}
	if fk.Column != "writer_id" {
		t.Errorf("FK column = %q, want writer_id", fk.Column)
	}
	if byCol, ok := m.FieldByColumn("writer_id"); !ok || byCol != fk {
		t.Errorf("FieldByColumn(writer_id) = %v, %v; want the FK field", byCol, ok)
	}
	if _, ok := m.FieldByAttr("writer_id"); ok {
		t.Error("FieldByAttr must not resolve a column name")
	}
}
