package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Holder stands in for tests/admin_inlines/models.py::Holder.
type Holder struct {
	ID    int64
	Dummy int64
}

// Inner stands in for tests/admin_inlines/models.py::Inner.
type Inner struct {
	ID       int64
	Dummy    int64
	Holder   orm.FK[Holder]
	Readonly string `orm:"max_length=1;verbose_name=Inner readonly label"`
}

// GetAbsoluteURL is Inner.get_absolute_url.
func (Inner) GetAbsoluteURL() string { return "/inner/" }

// Holder2 stands in for tests/admin_inlines/models.py::Holder2.
type Holder2 struct {
	ID    int64
	Dummy int64
}

// Inner2 stands in for tests/admin_inlines/models.py::Inner2.
type Inner2 struct {
	ID     int64
	Dummy  int64
	Holder orm.FK[Holder2]
}

// Holder3 stands in for tests/admin_inlines/models.py::Holder3.
type Holder3 struct {
	ID    int64
	Dummy int64
}

// Inner3 stands in for tests/admin_inlines/models.py::Inner3.
type Inner3 struct {
	ID     int64
	Dummy  int64
	Holder orm.FK[Holder3]
}

// Holder4 stands in for tests/admin_inlines/models.py::Holder4.
type Holder4 struct {
	ID    int64
	Dummy int64
}

// Inner4Stacked stands in for tests/admin_inlines/models.py::Inner4Stacked,
// without the unique constraint on dummy and holder, which no ported test
// reaches.
type Inner4Stacked struct {
	ID     int64
	Dummy  int64 `orm:"help_text=Awesome stacked help text is awesome."`
	Holder orm.FK[Holder4]
}

// Inner4Tabular stands in for tests/admin_inlines/models.py::Inner4Tabular,
// without the unique constraint on dummy and holder.
type Inner4Tabular struct {
	ID     int64
	Dummy  int64 `orm:"help_text=Awesome tabular help text is awesome."`
	Holder orm.FK[Holder4]
}

// TitleCollection stands in for tests/admin_inlines/models.py::TitleCollection.
type TitleCollection struct {
	ID int64
}

// Title stands in for tests/admin_inlines/models.py::Title.
type Title struct {
	ID         int64
	Collection orm.FK[TitleCollection] `orm:"null;blank;on_delete=set_null"`
	Title1     string                  `orm:"max_length=100"`
	Title2     string                  `orm:"max_length=100"`
}

// Poll stands in for tests/admin_inlines/models.py::Poll.
type Poll struct {
	ID   int64
	Name string `orm:"max_length=40"`
}

// PollQuestion stands in for tests/admin_inlines/models.py::Question. The
// admin_views Question of this package takes the name; the related name keeps
// Django's form prefix "question_set".
type PollQuestion struct {
	ID   int64
	Text string       `orm:"max_length=40"`
	Poll orm.FK[Poll] `orm:"related_name=question_set"`
}

// Clean rejects every question, as Django's Question.clean does.
func (PollQuestion) Clean() error { return orm.NewValidationError("Always invalid model.", "") }

// Novel stands in for tests/admin_inlines/models.py::Novel.
type Novel struct {
	ID   int64
	Name string `orm:"max_length=40"`
}

// Chapter stands in for tests/admin_inlines/models.py::Chapter.
type Chapter struct {
	ID    int64
	Name  string `orm:"max_length=40"`
	Novel orm.FK[Novel]
}

// FootNote stands in for tests/admin_inlines/models.py::FootNote.
type FootNote struct {
	ID      int64
	Chapter orm.FK[Chapter] `orm:"on_delete=protect"`
	Note    string          `orm:"max_length=40"`
}

// CapoFamiglia stands in for tests/admin_inlines/models.py::CapoFamiglia.
type CapoFamiglia struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Consigliere stands in for tests/admin_inlines/models.py::Consigliere.
type Consigliere struct {
	ID           int64
	Name         string               `orm:"max_length=100;help_text=Help text for Consigliere"`
	CapoFamiglia orm.FK[CapoFamiglia] `orm:"related_name=+"`
}

// SottoCapo stands in for tests/admin_inlines/models.py::SottoCapo.
type SottoCapo struct {
	ID           int64
	Name         string               `orm:"max_length=100"`
	CapoFamiglia orm.FK[CapoFamiglia] `orm:"related_name=+"`
}

// ReadOnlyInline stands in for tests/admin_inlines/models.py::ReadOnlyInline.
type ReadOnlyInline struct {
	ID           int64
	Name         string `orm:"max_length=100;help_text=Help text for ReadOnlyInline"`
	CapoFamiglia orm.FK[CapoFamiglia]
}

// BinaryTree stands in for tests/admin_inlines/models.py::BinaryTree.
type BinaryTree struct {
	ID     int64
	Name   string             `orm:"max_length=100"`
	Parent orm.FK[BinaryTree] `orm:"null;blank;on_delete=set_null"`
}

// SomeParentModel stands in for tests/admin_inlines/models.py::SomeParentModel.
type SomeParentModel struct {
	ID   int64
	Name string `orm:"max_length=1"`
}

// SomeChildModel stands in for tests/admin_inlines/models.py::SomeChildModel.
type SomeChildModel struct {
	ID            int64
	Name          string `orm:"max_length=1"`
	Position      uint32 `orm:"help_text=Position help_text."`
	Parent        orm.FK[SomeParentModel]
	ReadonlyField string `orm:"max_length=1"`
}

// ShowInlineParent stands in for
// tests/admin_inlines/models.py::ShowInlineParent.
type ShowInlineParent struct {
	ID          int64
	ShowInlines bool `orm:"default=false"`
}

// ShowInlineChild stands in for tests/admin_inlines/models.py::ShowInlineChild.
type ShowInlineChild struct {
	ID     int64
	Parent orm.FK[ShowInlineParent]
}

// ProfileCollection stands in for
// tests/admin_inlines/models.py::ProfileCollection.
type ProfileCollection struct {
	ID int64
}

// Profile stands in for tests/admin_inlines/models.py::Profile.
type Profile struct {
	ID         int64
	Collection orm.FK[ProfileCollection] `orm:"null;blank;on_delete=set_null"`
	FirstName  string                    `orm:"max_length=100"`
	LastName   string                    `orm:"max_length=100"`
}

// VerboseNameProfile stands in for
// tests/admin_inlines/models.py::VerboseNameProfile. Django derives it from
// Profile by multi-table inheritance, which the ORM does not have; the copy of
// the fields keeps what the inline shows.
type VerboseNameProfile struct {
	ID         int64
	Collection orm.FK[ProfileCollection] `orm:"null;blank;on_delete=set_null"`
	FirstName  string                    `orm:"max_length=100"`
	LastName   string                    `orm:"max_length=100"`
}

// Meta sets the verbose name.
func (VerboseNameProfile) Meta() orm.Meta {
	return orm.Meta{VerboseName: "Model with verbose name only"}
}

// VerboseNamePluralProfile stands in for
// tests/admin_inlines/models.py::VerboseNamePluralProfile, a copy of Profile
// for the same reason.
type VerboseNamePluralProfile struct {
	ID         int64
	Collection orm.FK[ProfileCollection] `orm:"null;blank;on_delete=set_null"`
	FirstName  string                    `orm:"max_length=100"`
	LastName   string                    `orm:"max_length=100"`
}

// Meta sets the plural verbose name.
func (VerboseNamePluralProfile) Meta() orm.Meta {
	return orm.Meta{VerboseNamePlural: "Model with verbose name plural only"}
}

// BothVerboseNameProfile stands in for
// tests/admin_inlines/models.py::BothVerboseNameProfile, a copy of Profile for
// the same reason.
type BothVerboseNameProfile struct {
	ID         int64
	Collection orm.FK[ProfileCollection] `orm:"null;blank;on_delete=set_null"`
	FirstName  string                    `orm:"max_length=100"`
	LastName   string                    `orm:"max_length=100"`
}

// Meta sets both verbose names.
func (BothVerboseNameProfile) Meta() orm.Meta {
	return orm.Meta{VerboseName: "Model with both - name", VerboseNamePlural: "Model with both - plural name"}
}

// Photographer stands in for tests/admin_inlines/models.py::Photographer.
// Django derives it from Person by multi-table inheritance; the firstname
// field of Person is copied in.
type Photographer struct {
	ID          int64
	Firstname   string `orm:"max_length=15"`
	Fullname    string `orm:"max_length=100"`
	Nationality string `orm:"max_length=100"`
	Residency   string `orm:"max_length=100"`
	Siblings    int64
	Children    int64
}

// Photo stands in for tests/admin_inlines/models.py::Photo.
type Photo struct {
	ID           int64
	Photographer orm.FK[Photographer]
	Image        string `orm:"max_length=100"`
	Title        string `orm:"max_length=100"`
	Description  string `orm:"type=text"`
	CreationDate orm.Date
	UpdateDate   orm.Date
	UpdatedBy    string `orm:"max_length=100"`
}

// inlinesModels returns the model set of the ported admin_inlines tests.
func inlinesModels() []any {
	return []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_inlines",
		&Holder{}, &Inner{}, &Holder2{}, &Inner2{}, &Holder3{}, &Inner3{},
		&Holder4{}, &Inner4Stacked{}, &Inner4Tabular{}, &TitleCollection{}, &Title{},
		&Poll{}, &PollQuestion{}, &Novel{}, &Chapter{}, &FootNote{},
		&CapoFamiglia{}, &Consigliere{}, &SottoCapo{}, &ReadOnlyInline{}, &BinaryTree{},
		&SomeParentModel{}, &SomeChildModel{}, &ShowInlineParent{}, &ShowInlineChild{},
		&ProfileCollection{}, &Profile{}, &VerboseNameProfile{}, &VerboseNamePluralProfile{},
		&BothVerboseNameProfile{}, &Photographer{}, &Photo{},
	)}
}

// customNumberWidget is CustomNumberWidget of tests/admin_inlines/admin.py: a
// number input with a script.
type customNumberWidget struct{ forms.NumberInput }

// Media returns the widget's script.
func (customNumberWidget) Media() forms.Media {
	return forms.NewMedia(nil, []string{"custom_number.js"})
}

// callable returns a display column whose value is text, a method of an
// admin in Django.
func callable(text string) map[string]admin.DisplayColumn {
	return map[string]admin.DisplayColumn{"call_me": {Value: func(any) any { return text }}}
}

// holderAdminMedia is the Media class of HolderAdmin.
func holderAdminMedia() forms.Media {
	return forms.NewMedia(nil, []string{"my_awesome_admin_scripts.js"})
}

// inlineScripts is the Media class of InnerInline2 and InnerInline3.
func inlineScripts() forms.Media {
	return forms.NewMedia(nil, []string{"my_awesome_inline_scripts.js"})
}

// titleForm returns the options of TitleForm: a declared title1 field and a
// clean that wants both titles equal.
func titleForm() []forms.ModelFormOption {
	title1 := forms.NewCharField()
	title1.MaxLength = 100
	return []forms.ModelFormOption{
		forms.WithFormField("title1", title1),
		forms.WithClean(func(form *forms.Form) error {
			data := form.CleanedData()
			if data["title1"] != data["title2"] {
				return forms.NewValidationError("The two titles must be the same", "")
			}
			return nil
		}),
	}
}

// childHiddenFieldForm returns the options of ChildHiddenFieldForm: a hidden
// position whose value 1 adds a non-field error.
func childHiddenFieldForm() []forms.ModelFormOption {
	return []forms.ModelFormOption{
		forms.WithFields("name", "position", "parent"),
		forms.WithWidget("position", forms.HiddenInput{}),
		forms.WithClean(func(form *forms.Form) error {
			if pos, ok := form.CleanedData()["position"]; ok && fmt.Sprint(pos) == "1" {
				return forms.NewValidationError("A non-field error", "")
			}
			return nil
		}),
	}
}

// chapterCount returns the number of children of obj, a *BinaryTree, Django's
// obj.binarytree_set.count().
func childCount(db *orm.DB, obj any) int {
	n, err := orm.Query[BinaryTree](db).Filter("parent", obj.(*BinaryTree).ID).Count(context.Background())
	if err != nil {
		panic(err)
	}
	return int(n)
}

// registerInlinesAdmin registers the admins of the site "admin" of
// tests/admin_inlines/admin.py that the ported tests reach.
func registerInlinesAdmin(site *admin.AdminSite, db *orm.DB) {
	admin.Register[TitleCollection](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[TitleCollection, Title](admin.InlineModelAdmin{Extra: new(1), FormOptions: titleForm()}),
	}})
	admin.Register[Holder](site, admin.ModelAdmin{Media: holderAdminMedia(), Inlines: []admin.Inline{
		admin.StackedInline[Holder, Inner](admin.InlineModelAdmin{CanDelete: new(false), ReadonlyFields: []string{"readonly"}}),
	}})
	admin.Register[Holder2](site, admin.ModelAdmin{Media: holderAdminMedia(), Inlines: []admin.Inline{
		admin.StackedInline[Holder2, Inner2](admin.InlineModelAdmin{Media: inlineScripts()}),
		admin.TabularInline[Holder2, Inner2](admin.InlineModelAdmin{}),
	}})
	admin.Register[Holder3](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.StackedInline[Holder3, Inner3](admin.InlineModelAdmin{
			Media:       inlineScripts(),
			FormOptions: []forms.ModelFormOption{forms.WithWidget("dummy", customNumberWidget{})},
		}),
	}})
	admin.Register[Poll](site, admin.ModelAdmin{DisplayColumns: callable("Callable in PollAdmin"), Inlines: []admin.Inline{
		admin.TabularInline[Poll, PollQuestion](admin.InlineModelAdmin{
			ReadonlyFields: []string{"call_me"}, DisplayColumns: callable("Callable in QuestionInline"),
		}),
	}})
	admin.Register[Novel](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[Novel, Chapter](admin.InlineModelAdmin{
			ReadonlyFields:               []string{"call_me"},
			DisplayColumns:               callable("Callable in ChapterInline"),
			DeleteConfirmationMaxDisplay: new(3),
		}),
	}})
	admin.Register[Holder4](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.StackedInline[Holder4, Inner4Stacked](admin.InlineModelAdmin{ShowChangeLink: true}),
		admin.TabularInline[Holder4, Inner4Tabular](admin.InlineModelAdmin{ShowChangeLink: true}),
	}})
	admin.Register[CapoFamiglia](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[CapoFamiglia, Consigliere](admin.InlineModelAdmin{}),
		admin.TabularInline[CapoFamiglia, SottoCapo](admin.InlineModelAdmin{}),
		admin.TabularInline[CapoFamiglia, ReadOnlyInline](admin.InlineModelAdmin{ReadonlyFields: []string{"name"}}),
	}})
	admin.Register[ProfileCollection](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[ProfileCollection, Profile](admin.InlineModelAdmin{Extra: new(1)}),
	}})
	admin.Register[BinaryTree](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[BinaryTree, BinaryTree](admin.InlineModelAdmin{
			GetExtra: func(_ *http.Request, obj any) int {
				if obj != nil {
					return 2 - childCount(db, obj)
				}
				return 2
			},
			GetMaxNum: func(_ *http.Request, obj any) int {
				if obj != nil {
					return 3 - childCount(db, obj)
				}
				return 3
			},
		}),
	}})
	admin.Register[SomeParentModel](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[SomeParentModel, SomeChildModel](admin.InlineModelAdmin{
			ReadonlyFields: []string{"readonly_field"},
			FormOptions: []forms.ModelFormOption{
				forms.WithFields(forms.AllFields),
				forms.WithWidget("position", forms.HiddenInput{}),
				forms.WithLabel("readonly_field", "Label from ModelForm.Meta"),
				forms.WithHelpText("readonly_field", "Help text from ModelForm.Meta"),
				forms.WithLabel("name", "new label"),
			},
		}),
	}})
	admin.Register[PollQuestion](site, admin.ModelAdmin{})
	admin.Register[Inner4Stacked](site, admin.ModelAdmin{})
	admin.Register[Inner4Tabular](site, admin.ModelAdmin{})
	extra := forms.NewCharField()
	admin.Register[Chapter](site, admin.ModelAdmin{Inlines: []admin.Inline{
		admin.TabularInline[Chapter, FootNote](admin.InlineModelAdmin{
			FormOptions:         []forms.ModelFormOption{forms.WithFields(forms.AllFields), forms.WithFormField("extra_field", extra)},
			HasChangePermission: func(*http.Request, any) bool { return false },
		}),
	}})
	admin.Register[Photographer](site, admin.ModelAdmin{
		Fieldsets: []admin.Fieldset{
			{Fields: []any{"firstname", "fullname"}},
			{Name: "Advanced options", Fields: []any{"nationality", "residency"}},
			{Name: "Advanced options", Fields: []any{"siblings", "children"}, Classes: []string{"collapse"}},
		},
		Inlines: []admin.Inline{
			admin.TabularInline[Photographer, Photo](photoInline(2)),
			admin.StackedInline[Photographer, Photo](photoInline(2)),
			admin.StackedInline[Photographer, Photo](photoInline(3)),
			admin.StackedInline[Photographer, Photo](admin.InlineModelAdmin{Extra: new(2), Classes: []string{"collapse"}}),
		},
	})
	admin.Register[ShowInlineParent](site, admin.ModelAdmin{
		GetInlines: func(_ *http.Request, obj any) []admin.Inline {
			if p, ok := obj.(*ShowInlineParent); ok && p.ShowInlines {
				return []admin.Inline{admin.StackedInline[ShowInlineParent, ShowInlineChild](admin.InlineModelAdmin{})}
			}
			return nil
		},
	})
}

// photoInline returns PhotoInlineMixin with the given extra.
func photoInline(extra int) admin.InlineModelAdmin {
	return admin.InlineModelAdmin{
		Extra: new(extra),
		Fieldsets: []admin.Fieldset{
			{Fields: []any{"image", "title"}, Description: "First group"},
			{Name: "Details", Fields: []any{"description", "creation_date"}, Classes: []string{"collapse"}, Description: "Second group"},
			{Name: "Details", Fields: []any{"update_date", "updated_by"}, Description: "Third group"},
		},
	}
}

// registerHiddenFieldAdmin registers SomeParentModel with the inline of one of
// the hidden field sites of tests/admin_inlines/admin.py.
func registerHiddenFieldAdmin(site *admin.AdminSite) {
	var inline admin.Inline
	switch site.Name {
	case "tabular_inline_hidden_field_admin":
		inline = admin.TabularInline[SomeParentModel, SomeChildModel](admin.InlineModelAdmin{FormOptions: childHiddenFieldForm()})
	case "stacked_inline_hidden_field_in_group_admin":
		inline = admin.StackedInline[SomeParentModel, SomeChildModel](admin.InlineModelAdmin{
			FormOptions: childHiddenFieldForm(), Fields: []any{[]string{"name", "position"}},
		})
	case "stacked_inline_hidden_field_on_single_line_admin":
		inline = admin.StackedInline[SomeParentModel, SomeChildModel](admin.InlineModelAdmin{
			FormOptions: childHiddenFieldForm(), Fields: []any{"name", "position"},
		})
	}
	admin.Register[SomeParentModel](site, admin.ModelAdmin{Inlines: []admin.Inline{inline}})
}

// inlinesEnv returns an environment with the admins of the named site of
// tests/admin_inlines/admin.py.
func inlinesEnv(t *testing.T, db *orm.DB, siteName string) *adminTestEnv {
	t.Helper()
	return newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.Name = siteName
		switch siteName {
		case "admin":
			registerInlinesAdmin(site, db)
		case "zero_display_site":
			admin.Register[Novel](site, admin.ModelAdmin{Inlines: []admin.Inline{
				admin.TabularInline[Novel, Chapter](admin.InlineModelAdmin{DeleteConfirmationMaxDisplay: new(0)}),
			}})
		default:
			registerHiddenFieldAdmin(site)
		}
	})
}

// inlinesSuperEnv returns the environment of the site "admin" with a
// superuser logged in, TestDataMixin and the setUp of the test classes.
func inlinesSuperEnv(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env := inlinesEnv(t, db, "admin")
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// inlineSets returns the inline_admin_formsets entry of the template context.
func inlineSets(t *testing.T, resp *djangotest.Response) []map[string]any {
	t.Helper()
	return maps(ctxVal(t, resp, "inline_admin_formsets"))
}

// maps returns a list of the template context as maps. The context keeps a
// []map[string]any as a []any.
func maps(v any) []map[string]any {
	if m, ok := v.([]map[string]any); ok {
		return m
	}
	raw := v.([]any)
	out := make([]map[string]any, len(raw))
	for i, item := range raw {
		out[i] = item.(map[string]any)
	}
	return out
}

// inlineFormSet returns the formset of the i-th inline as the template sees
// it, Django's InlineAdminFormSet.formset.
func inlineFormSet(t *testing.T, resp *djangotest.Response, i int) forms.TemplateFormSet {
	t.Helper()
	return inlineSets(t, resp)[i]["formset"].(forms.TemplateFormSet)
}

// scriptSrc matches the src attribute of a script tag.
var scriptSrc = regexp.MustCompile(`<script src="([^"]+)"`)

// inlineFields returns the column headers of an inline, Django's
// InlineAdminFormSet.fields().
func inlineFields(set map[string]any) []map[string]any {
	return maps(set["fields"])
}

// fk returns a foreign key value that points at pk.
func fk[T any](pk int64) orm.FK[T] {
	var f orm.FK[T]
	f.SetPK(pk)
	return f
}

// inlineFixture holds the rows of TestInline.setUpTestData.
type inlineFixture struct {
	holder       *Holder
	parent       *SomeParentModel
	viewOnlyUser *auth.User
}

// newInlineFixture creates the rows of TestInline.setUpTestData.
func newInlineFixture(t *testing.T, db *orm.DB) inlineFixture {
	t.Helper()
	f := inlineFixture{holder: create(t, db, &Holder{Dummy: 13})}
	create(t, db, &Inner{Dummy: 42, Holder: fk[Holder](f.holder.ID)})
	f.parent = create(t, db, &SomeParentModel{Name: "a"})
	create(t, db, &SomeChildModel{Name: "b", Position: 0, Parent: fk[SomeParentModel](f.parent.ID)})
	create(t, db, &SomeChildModel{Name: "c", Position: 1, Parent: fk[SomeParentModel](f.parent.ID)})
	f.viewOnlyUser = createTestUser(t, db, "user", "pwd", true, false)
	grantPerm(t, db, f.viewOnlyUser, "admin_inlines", "view_someparentmodel")
	grantPerm(t, db, f.viewOnlyUser, "admin_inlines", "view_somechildmodel")
	return f
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_can_delete
func TestInlineCanDelete(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		f := newInlineFixture(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder_change", f.holder.ID))
		djangotest.AssertEqual(t, inlineFormSet(t, resp, 0)["can_delete"], any(false))
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_readonly_stacked_inline_label
func TestInlineReadonlyStackedInlineLabel(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder{Dummy: 42})
		create(t, db, &Inner{Holder: fk[Holder](holder.ID), Dummy: 42})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder_change", holder.ID))
		djangotest.AssertContains(t, resp, "<label>Inner readonly label:</label>")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_excluded_id_for_inlines_uses_hidden_field
func TestInlineExcludedIDUsesHiddenField(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no UUID primary key with a callable default, " +
		"Django: an excluded UUID id is rendered as a hidden input")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_many_to_many_inlines
func TestInlineManyToManyInlines(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit the auto-created through model of a " +
		"many-to-many field, Django: Author.books.through as an inline model")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inline_primary
func TestInlinePrimary(t *testing.T) {
	t.Skip("FIXME-PORT: Go: orm.O2O as the primary key and many-to-many through models, " +
		"Django: Fashionista with a one-to-one primary key")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_inline_column_css_class
func TestInlineTabularColumnCSSClass(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_poll_add"))
		fields := inlineFields(inlineSets(t, resp)[0])
		djangotest.AssertEqual(t, len(fields), 2)
		// Editable field.
		djangotest.AssertEqual(t, fields[0]["name"], any("text"))
		djangotest.AssertContains(t, resp, `<th class="column-text required">`)
		// Read-only field.
		djangotest.AssertEqual(t, fields[1]["name"], any("call_me"))
		djangotest.AssertContains(t, resp, `<th class="column-call_me">`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_form_tabular_inline_label
func TestInlineCustomFormTabularLabel(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_titlecollection_add"))
		djangotest.AssertContains(t, resp, `<th class="column-title1 required">Title1</th>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_form_tabular_inline_extra_field_label
func TestInlineCustomFormTabularExtraFieldLabel(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline of OutfitItem edits ShoppingWeakness, a many-to-many " +
		"through model, Django: WeaknessForm.extra_field")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_non_editable_custom_form_tabular_inline_extra_field_label
func TestInlineNonEditableCustomFormExtraFieldLabel(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_chapter_add"))
		fields := inlineFields(inlineSets(t, resp)[0])
		djangotest.AssertEqual(t, len(fields), 2)
		djangotest.AssertEqual(t, fields[1]["label"], any("Extra field"))
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_form_tabular_inline_overridden_label
func TestInlineCustomFormTabularOverriddenLabel(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_someparentmodel_add"))
		field := inlineFields(inlineSets(t, resp)[0])[0]
		djangotest.AssertEqual(t, field["label"], any("new label"))
		djangotest.AssertContains(t, resp, `<th class="column-name required">New label</th>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_non_field_errors
func TestInlineTabularNonFieldErrors(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		data := url.Values{
			"title_set-TOTAL_FORMS":   {"1"},
			"title_set-INITIAL_FORMS": {"0"},
			"title_set-MAX_NUM_FORMS": {"0"},
			"_save":                   {"Save"},
			"title_set-0-title1":      {"a title"},
			"title_set-0-title2":      {"a different title"},
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_inlines_titlecollection_add"), data)
		// Here colspan is "4": two fields (title1 and title2), one hidden field
		// and the delete checkbox.
		djangotest.AssertContains(t, resp, `<tr class="row-form-errors"><td colspan="4">`+
			`<ul class="errorlist nonfield"><li>The two titles must be the same</li></ul></td></tr>`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_no_parent_callable_lookup
func TestInlineNoParentCallableLookup(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_novel_add"))
		djangotest.AssertContains(t, resp, `<div class="js-inline-admin-formset inline-group" id="chapter_set-group"`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_callable_lookup
func TestInlineCallableLookup(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_poll_add"))
		djangotest.AssertContains(t, resp, `<div class="js-inline-admin-formset inline-group" id="question_set-group"`)
		djangotest.AssertContains(t, resp, "<p>Callable in QuestionInline</p>")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_model_error_inline_with_readonly_field
func TestInlineModelErrorWithReadonlyField(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		poll := create(t, db, &Poll{Name: "Test poll"})
		data := url.Values{
			"question_set-TOTAL_FORMS":   {"1"},
			"question_set-INITIAL_FORMS": {"0"},
			"question_set-MAX_NUM_FORMS": {"0"},
			"_save":                      {"Save"},
			"question_set-0-text":        {"Question"},
			"question_set-0-poll":        {fmt.Sprint(poll.ID)},
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_inlines_poll_change", poll.ID), data)
		djangotest.AssertContains(t, resp, "Always invalid model.")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_help_text
func TestInlineHelpText(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder4_add"))
		djangotest.AssertContains(t, resp, "Awesome stacked help text is awesome.", djangotest.Count(4))
		djangotest.AssertContains(t, resp, `<img src="/static/admin/img/icon-unknown.svg" `+
			`class="help help-tooltip" width="10" height="10" `+
			`alt="(Awesome tabular help text is awesome.)" `+
			`title="Awesome tabular help text is awesome.">`, djangotest.Count(1))
		// ReadOnly fields
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_capofamiglia_add"))
		djangotest.AssertContains(t, resp, `<img src="/static/admin/img/icon-unknown.svg" `+
			`class="help help-tooltip" width="10" height="10" `+
			`alt="(Help text for ReadOnlyInline)" `+
			`title="Help text for ReadOnlyInline">`, djangotest.Count(1))
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_model_form_meta_readonly_field
func TestInlineTabularModelFormMetaReadonlyField(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_someparentmodel_add"))
		djangotest.AssertContains(t, resp, `<img src="/static/admin/img/icon-unknown.svg" `+
			`class="help help-tooltip" width="10" height="10" `+
			`alt="(Help text from ModelForm.Meta)" `+
			`title="Help text from ModelForm.Meta">`)
		djangotest.AssertContains(t, resp, "Label from ModelForm.Meta")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inline_hidden_field_no_column
func TestInlineHiddenFieldNoColumn(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		parent := create(t, db, &SomeParentModel{Name: "a"})
		create(t, db, &SomeChildModel{Name: "b", Position: 0, Parent: fk[SomeParentModel](parent.ID)})
		create(t, db, &SomeChildModel{Name: "c", Position: 1, Parent: fk[SomeParentModel](parent.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_someparentmodel_change", parent.ID))
		djangotest.AssertNotContains(t, resp, `<td class="field-position">`)
		djangotest.AssertInHTML(t, `<input id="id_somechildmodel_set-1-position" `+
			`name="somechildmodel_set-1-position" type="hidden" value="1">`, resp.Content())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_inline_hidden_field_with_view_only_permissions
func TestInlineTabularHiddenFieldViewOnly(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesEnv(t, db, "tabular_inline_hidden_field_admin")
		f := newInlineFixture(t, db)
		env.client.ForceLogin(f.viewOnlyUser)
		resp := env.client.Get(mustReverse(t, env.router,
			"tabular_inline_hidden_field_admin:admin_inlines_someparentmodel_change", f.parent.ID))
		djangotest.AssertInHTML(t, `<th class="column-position hidden">Position`+
			`<img src="/static/admin/img/icon-unknown.svg" `+
			`class="help help-tooltip" width="10" height="10" `+
			`alt="(Position help_text.)" `+
			`title="Position help_text.">`+
			`</th>`, resp.Content())
		djangotest.AssertInHTML(t, `<td class="field-position hidden"><p>0</p></td>`, resp.Content())
		djangotest.AssertInHTML(t, `<td class="field-position hidden"><p>1</p></td>`, resp.Content())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_stacked_inline_hidden_field_with_view_only_permissions
func TestInlineStackedHiddenFieldViewOnly(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesEnv(t, db, "stacked_inline_hidden_field_in_group_admin")
		f := newInlineFixture(t, db)
		env.client.ForceLogin(f.viewOnlyUser)
		resp := env.client.Get(mustReverse(t, env.router,
			"stacked_inline_hidden_field_in_group_admin:admin_inlines_someparentmodel_change", f.parent.ID))
		// The whole line containing name + position fields is not hidden.
		djangotest.AssertContains(t, resp,
			`<div class="form-row flex-container form-multiline field-name field-position">`)
		// The div containing the position field is hidden.
		for _, pos := range []string{"0", "1"} {
			djangotest.AssertInHTML(t, `<div class="flex-container field-position fieldBox hidden">`+
				`<label class="inline">Position:</label>`+
				`<div class="help hidden"><div>Position help_text.</div></div>`+
				`<div class="readonly">`+pos+`</div></div>`, resp.Content())
		}
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_stacked_inline_single_hidden_field_in_line_with_view_only_permissions
func TestInlineStackedSingleHiddenFieldViewOnly(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesEnv(t, db, "stacked_inline_hidden_field_on_single_line_admin")
		f := newInlineFixture(t, db)
		env.client.ForceLogin(f.viewOnlyUser)
		resp := env.client.Get(mustReverse(t, env.router,
			"stacked_inline_hidden_field_on_single_line_admin:admin_inlines_someparentmodel_change", f.parent.ID))
		// The whole line containing position field is hidden.
		for _, pos := range []string{"0", "1"} {
			djangotest.AssertInHTML(t, `<div class="form-row hidden field-position">`+
				`<div class="flex-container"><label>Position:</label>`+
				`<div class="help hidden"><div>Position help_text.</div></div>`+
				`<div class="readonly">`+pos+`</div>`+
				`</div></div>`, resp.Content())
		}
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_inline_with_hidden_field_non_field_errors_has_correct_colspan
func TestInlineTabularHiddenFieldNonFieldErrorsColspan(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesEnv(t, db, "tabular_inline_hidden_field_admin")
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		parent := create(t, db, &SomeParentModel{Name: "a"})
		child := create(t, db, &SomeChildModel{Name: "b", Position: 0, Parent: fk[SomeParentModel](parent.ID)})
		data := url.Values{
			"name":                             {parent.Name},
			"somechildmodel_set-TOTAL_FORMS":   {"1"},
			"somechildmodel_set-INITIAL_FORMS": {"1"},
			"somechildmodel_set-MIN_NUM_FORMS": {"0"},
			"somechildmodel_set-MAX_NUM_FORMS": {"1000"},
			"_save":                            {"Save"},
			"somechildmodel_set-0-id":          {fmt.Sprint(child.ID)},
			"somechildmodel_set-0-parent":      {fmt.Sprint(parent.ID)},
			"somechildmodel_set-0-name":        {child.Name},
			"somechildmodel_set-0-position":    {"1"},
		}
		resp := env.client.Post(mustReverse(t, env.router,
			"tabular_inline_hidden_field_admin:admin_inlines_someparentmodel_change", parent.ID), data)
		// Form has 3 visible columns and 1 hidden column.
		djangotest.AssertInHTML(t, `<thead><tr><th class="original"></th>`+
			`<th class="column-name required">Name</th>`+
			`<th class="column-position required hidden">Position`+
			`<img src="/static/admin/img/icon-unknown.svg" `+
			`class="help help-tooltip" width="10" height="10" `+
			`alt="(Position help_text.)" `+
			`title="Position help_text.">`+
			`</th>`+
			`<th>Delete?</th></tr></thead>`, resp.Content())
		// The non-field error must be spanned on 3 (visible) columns.
		djangotest.AssertInHTML(t, `<tr class="row-form-errors"><td colspan="3">`+
			`<ul class="errorlist nonfield"><li>A non-field error</li></ul></td></tr>`, resp.Content())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_non_related_name_inline
func TestInlineNonRelatedName(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_capofamiglia_add"))
		html := djangotest.HTML()
		djangotest.AssertContains(t, resp, `<input type="hidden" name="-1-0-id" id="id_-1-0-id">`, html)
		djangotest.AssertContains(t, resp,
			`<input type="hidden" name="-1-0-capo_famiglia" id="id_-1-0-capo_famiglia">`, html)
		djangotest.AssertContains(t, resp,
			`<input id="id_-1-0-name" type="text" class="vTextField" name="-1-0-name" `+
				`maxlength="100" aria-describedby="id_-1-0-name_helptext">`, html)
		djangotest.AssertContains(t, resp, `<input type="hidden" name="-2-0-id" id="id_-2-0-id">`, html)
		djangotest.AssertContains(t, resp,
			`<input type="hidden" name="-2-0-capo_famiglia" id="id_-2-0-capo_famiglia">`, html)
		djangotest.AssertContains(t, resp,
			`<input id="id_-2-0-name" type="text" class="vTextField" name="-2-0-name" maxlength="100">`, html)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_localize_pk_shortcut
func TestInlineLocalizePKShortcut(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder{ID: 123456789, Dummy: 42})
		inner := create(t, db, &Inner{ID: 987654321, Holder: fk[Holder](holder.ID), Dummy: 42})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder_change", holder.ID))
		djangotest.AssertContains(t, resp, fmt.Sprintf("r/%d/%d/", contentTypeID(t, db, &Inner{}), inner.ID))
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_pk_shortcut
func TestInlineCustomPKShortcut(t *testing.T) {
	t.Skip("FIXME-PORT: Go: orm.FK and the admin need an integer primary key, " +
		"Django: a CharField primary key")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_create_inlines_on_inherited_model
func TestInlineCreateOnInheritedModel(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, Django: ExtraTerrestrial(LifeForm)")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_get_extra_form
func TestInlineCustomGetExtraForm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		head := create(t, db, &BinaryTree{Name: "Tree Head"})
		create(t, db, &BinaryTree{Name: "First Child", Parent: fk[BinaryTree](head.ID)})
		// The maximum number of forms should respect GetMaxNum.
		maxForms := `<input id="id_binarytree_set-MAX_NUM_FORMS" ` +
			`name="binarytree_set-MAX_NUM_FORMS" type="hidden" value="%d">`
		// The total number of forms will remain the same in either case.
		totalForms := `<input id="id_binarytree_set-TOTAL_FORMS" ` +
			`name="binarytree_set-TOTAL_FORMS" type="hidden" value="2">`
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_binarytree_add"))
		djangotest.AssertInHTML(t, fmt.Sprintf(maxForms, 3), resp.Content())
		djangotest.AssertInHTML(t, totalForms, resp.Content())

		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_binarytree_change", head.ID))
		djangotest.AssertInHTML(t, fmt.Sprintf(maxForms, 2), resp.Content())
		djangotest.AssertInHTML(t, totalForms, resp.Content())
	})
}

// minNumEnv returns an environment with BinaryTree registered with the
// inline cfg and a superuser logged in. Django builds the ModelAdmin by hand
// and calls changeform_view with a request from RequestFactory.
func minNumEnv(t *testing.T, db *orm.DB, cfg admin.InlineModelAdmin) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[BinaryTree](site, admin.ModelAdmin{Inlines: []admin.Inline{
			admin.TabularInline[BinaryTree, BinaryTree](cfg),
		}})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_min_num
func TestInlineMinNum(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := minNumEnv(t, db, admin.InlineModelAdmin{MinNum: 2, Extra: new(3)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_binarytree_add"))
		djangotest.AssertInHTML(t, `<input id="id_binarytree_set-MIN_NUM_FORMS" `+
			`name="binarytree_set-MIN_NUM_FORMS" type="hidden" value="2">`, resp.Content())
		djangotest.AssertInHTML(t, `<input id="id_binarytree_set-TOTAL_FORMS" `+
			`name="binarytree_set-TOTAL_FORMS" type="hidden" value="5">`, resp.Content())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_custom_min_num
func TestInlineCustomMinNum(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		head := create(t, db, &BinaryTree{Name: "Tree Head"})
		create(t, db, &BinaryTree{Name: "First Child", Parent: fk[BinaryTree](head.ID)})
		env := minNumEnv(t, db, admin.InlineModelAdmin{
			Extra: new(3),
			GetMinNum: func(_ *http.Request, obj any) int {
				if obj != nil {
					return 5
				}
				return 2
			},
		})
		minForms := `<input id="id_binarytree_set-MIN_NUM_FORMS" ` +
			`name="binarytree_set-MIN_NUM_FORMS" type="hidden" value="%d">`
		totalForms := `<input id="id_binarytree_set-TOTAL_FORMS" ` +
			`name="binarytree_set-TOTAL_FORMS" type="hidden" value="%d">`
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_binarytree_add"))
		djangotest.AssertInHTML(t, fmt.Sprintf(minForms, 2), resp.Content())
		djangotest.AssertInHTML(t, fmt.Sprintf(totalForms, 5), resp.Content())

		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_binarytree_change", head.ID))
		djangotest.AssertInHTML(t, fmt.Sprintf(minForms, 5), resp.Content())
		djangotest.AssertInHTML(t, fmt.Sprintf(totalForms, 8), resp.Content())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inline_nonauto_noneditable_pk
func TestInlineNonAutoNonEditablePK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an integer primary key that a save hook fills is not supported, " +
		"Django: NonAutoPKBook.save picks a random rand_pk")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inline_nonauto_noneditable_inherited_pk
func TestInlineNonAutoNonEditableInheritedPK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, Django: NonAutoPKBookChild")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inline_editable_pk
func TestInlineEditablePK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: Author has a many-to-many field and a one-to-one to Person that the " +
		"other skipped tests need, Django: AuthorAdmin with six inlines")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_stacked_inline_edit_form_contains_has_original_class
func TestInlineStackedEditFormHasOriginalClass(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder{Dummy: 1})
		create(t, db, &Inner{Dummy: 1, Holder: fk[Holder](holder.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder_change", holder.ID))
		djangotest.AssertContains(t, resp, `<div class="inline-related has_original" id="inner_set-0">`, djangotest.Count(1))
		djangotest.AssertContains(t, resp, `<div class="inline-related" id="inner_set-1">`, djangotest.Count(1))
	})
}

// inlineChangeLinkHTML is INLINE_CHANGELINK_HTML.
const inlineChangeLinkHTML = `class="inlinechangelink">Change</a>`

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inlines_show_change_link_registered
func TestInlineShowChangeLinkRegistered(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder4{Dummy: 1})
		item1 := create(t, db, &Inner4Stacked{Dummy: 1, Holder: fk[Holder4](holder.ID)})
		item2 := create(t, db, &Inner4Tabular{Dummy: 1, Holder: fk[Holder4](holder.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder4_change", holder.ID))
		djangotest.AssertEqual(t, inlineSets(t, resp)[0]["has_registered_model"], any(true))
		for _, item := range []struct {
			model string
			pk    int64
		}{{"inner4stacked", item1.ID}, {"inner4tabular", item2.ID}} {
			u := mustReverse(t, env.router, "admin:admin_inlines_"+item.model+"_change", item.pk)
			djangotest.AssertContains(t, resp, fmt.Sprintf(`<a href="%s" %s`, u, inlineChangeLinkHTML))
		}
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inlines_show_change_link_unregistered
func TestInlineShowChangeLinkUnregistered(t *testing.T) {
	t.Skip("FIXME-PORT: Go: orm.FK needs an integer primary key, Django: ParentModelWithCustomPk " +
		"has a CharField primary key")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_tabular_inline_show_change_link_false_registered
func TestInlineTabularShowChangeLinkFalseRegistered(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		poll := create(t, db, &Poll{Name: "New poll"})
		create(t, db, &PollQuestion{Poll: fk[Poll](poll.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_poll_change", poll.ID))
		djangotest.AssertEqual(t, inlineSets(t, resp)[0]["has_registered_model"], any(true))
		djangotest.AssertNotContains(t, resp, inlineChangeLinkHTML)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_noneditable_inline_has_field_inputs
//
// Django registers the proxy NovelReadonlyChapter of Novel a second time; the
// ORM has no proxy models, so Novel goes on a site of its own with the same
// inline.
func TestInlineNonEditableHasFieldInputs(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[Novel](site, admin.ModelAdmin{Inlines: []admin.Inline{
				admin.TabularInline[Novel, Chapter](admin.InlineModelAdmin{
					HasChangePermission: func(*http.Request, any) bool { return false },
				}),
			}})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_novel_add"))
		djangotest.AssertContains(t, resp, `<input type="text" name="chapter_set-0-name" `+
			`class="vTextField" maxlength="40" id="id_chapter_set-0-name">`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inlines_plural_heading_foreign_key
func TestInlinePluralHeadingForeignKey(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder4_add"))
		djangotest.AssertContains(t, resp,
			`<h2 id="inner4stacked_set-heading" class="inline-heading">Inner4 stackeds</h2>`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<h2 id="inner4tabular_set-heading" class="inline-heading">Inner4 tabulars</h2>`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inlines_singular_heading_one_to_one
func TestInlineSingularHeadingOneToOne(t *testing.T) {
	t.Skip("FIXME-PORT: Go: Author and Fashionista need many-to-many fields and a one-to-one " +
		"primary key, Django: inlines of Person over one-to-one fields")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_inlines_based_on_model_state
func TestInlineBasedOnModelState(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		parent := create(t, db, &ShowInlineParent{ShowInlines: false})
		data := url.Values{"show_inlines": {"on"}, "_save": {"Save"}}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_inlines_showinlineparent_change", parent.ID), data)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		got, err := orm.Query[ShowInlineParent](db).Get(context.Background(), "pk", parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got.ShowInlines, true)
	})
}

// protectedChapter creates a novel with a chapter that n footnotes protect.
func protectedChapter(t *testing.T, db *orm.DB, n int) (*Novel, *Chapter) {
	t.Helper()
	novel := create(t, db, &Novel{})
	chapter := create(t, db, &Chapter{Novel: fk[Novel](novel.ID)})
	for range n {
		create(t, db, &FootNote{Chapter: fk[Chapter](chapter.ID)})
	}
	return novel, chapter
}

// deleteChapterData is the POST data of the delete_protected tests.
func deleteChapterData(novel *Novel, chapter *Chapter) url.Values {
	return url.Values{
		"show_inlines":              {"on"},
		"chapter_set-TOTAL_FORMS":   {"1"},
		"chapter_set-INITIAL_FORMS": {"1"},
		"chapter_set-MAX_NUM_FORMS": {"1000"},
		"chapter_set-MIN_NUM_FORMS": {"0"},
		"chapter_set-0-id":          {fmt.Sprint(chapter.ID)},
		"chapter_set-0-name":        {chapter.Name},
		"chapter_set-0-novel":       {fmt.Sprint(novel.ID)},
		"chapter_set-0-DELETE":      {"on"},
	}
}

// nonFormErrors returns the non-form errors of the first inline.
func nonFormErrors(t *testing.T, resp *djangotest.Response) []string {
	t.Helper()
	errs := inlineFormSet(t, resp, 0)["non_form_errors"].(forms.TemplateErrors)
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Message
	}
	return out
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_delete_protected_message_limits_number_of_objects_displayed
func TestInlineDeleteProtectedMessageLimitsObjects(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		// admin limits the number of displayed objects to 3, so we create 5
		// footnotes.
		novel, chapter := protectedChapter(t, db, 5)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_inlines_novel_change", novel.ID),
			deleteChapterData(novel, chapter))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		errs := nonFormErrors(t, resp)
		djangotest.AssertEqual(t, len(errs), 1)
		msg := errs[0]
		want := fmt.Sprintf("Deleting chapter Chapter object (%d) would "+
			"require deleting the following protected related objects:", chapter.ID)
		if !strings.HasPrefix(msg, want) {
			t.Errorf("message %q does not start with %q", msg, want)
		}
		djangotest.AssertEqual(t, strings.Count(msg, "FootNote"), 3)
		if !strings.HasSuffix(msg, "…and 2 more objects.") {
			t.Errorf("message %q does not end with the count of the others", msg)
		}
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_delete_protected_message_does_not_limit_small_amount_of_objects
func TestInlineDeleteProtectedMessageSmallAmount(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		novel, chapter := protectedChapter(t, db, 3)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_inlines_novel_change", novel.ID),
			deleteChapterData(novel, chapter))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		errs := nonFormErrors(t, resp)
		djangotest.AssertEqual(t, len(errs), 1)
		msg := errs[0]
		want := fmt.Sprintf("Deleting chapter Chapter object (%d) would require "+
			"deleting the following protected related objects:", chapter.ID)
		if !strings.HasPrefix(msg, want) {
			t.Errorf("message %q does not start with %q", msg, want)
		}
		djangotest.AssertEqual(t, strings.Count(msg, "FootNote object"), 3)
		if strings.Contains(msg, "more") {
			t.Errorf("message %q counts more objects", msg)
		}
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInline.test_delete_protected_message_lists_nothing_when_limit_zero
func TestInlineDeleteProtectedMessageLimitZero(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesEnv(t, db, "zero_display_site")
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		novel, chapter := protectedChapter(t, db, 3)
		resp := env.client.Post(mustReverse(t, env.router, "zero_display_site:admin_inlines_novel_change", novel.ID),
			deleteChapterData(novel, chapter))
		djangotest.AssertStatus(t, resp, http.StatusOK)
		errs := nonFormErrors(t, resp)
		djangotest.AssertEqual(t, len(errs), 1)
		djangotest.AssertEqual(t, errs[0], fmt.Sprintf("Deleting chapter Chapter object (%d) would require "+
			"deleting some protected related objects.", chapter.ID))
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlineMedia.test_inline_media_only_base
func TestInlineMediaOnlyBase(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder{Dummy: 13})
		create(t, db, &Inner{Dummy: 42, Holder: fk[Holder](holder.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder_change", holder.ID))
		djangotest.AssertContains(t, resp, "my_awesome_admin_scripts.js")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlineMedia.test_inline_media_only_inline
func TestInlineMediaOnlyInline(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder3{Dummy: 13})
		create(t, db, &Inner3{Dummy: 42, Holder: fk[Holder3](holder.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder3_change", holder.ID))
		media := inlineSets(t, resp)[0]["media"].(forms.TemplateMedia)["js"].(forms.SafeHTML)
		matches := scriptSrc.FindAllStringSubmatch(string(media), -1)
		scripts := make([]string, 0, len(matches))
		for _, m := range matches {
			scripts = append(scripts, m[1])
		}
		djangotest.AssertEqual(t, scripts, []string{
			"/static/admin/js/vendor/jquery/jquery.min.js",
			"/static/my_awesome_inline_scripts.js",
			"/static/custom_number.js",
			"/static/admin/js/jquery.init.js",
			"/static/admin/js/inlines.js",
		})
		djangotest.AssertContains(t, resp, "my_awesome_inline_scripts.js")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlineMedia.test_all_inline_media
func TestInlineMediaAll(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		holder := create(t, db, &Holder2{Dummy: 13})
		create(t, db, &Inner2{Dummy: 42, Holder: fk[Holder2](holder.ID)})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_holder2_change", holder.ID))
		djangotest.AssertContains(t, resp, "my_awesome_admin_scripts.js")
		djangotest.AssertContains(t, resp, "my_awesome_inline_scripts.js")
	})
}

// No Go counterpart: tests/admin_inlines/tests.py::TestInlineAdminForm.test_immutable_content_type
// builds an InlineAdminForm by hand to check that it leaves the content type
// of a generic relation alone; the Go admin has neither the helper class nor
// generic relations.

// Ported from Django: tests/admin_inlines/tests.py::TestInlineProtectedOnDelete.test_deleting_inline_with_protected_delete_does_not_validate
func TestInlineProtectedOnDeleteDoesNotValidate(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		lotr := create(t, db, &Novel{Name: "Lord of the rings"})
		chapter := create(t, db, &Chapter{Novel: fk[Novel](lotr.ID), Name: "Many Meetings"})
		footNote := create(t, db, &FootNote{Chapter: fk[Chapter](chapter.ID), Note: "yadda yadda"})
		changeURL := mustReverse(t, env.router, "admin:admin_inlines_novel_change", lotr.ID)
		env.client.Get(changeURL)
		data := url.Values{
			"name":                      {lotr.Name},
			"chapter_set-TOTAL_FORMS":   {"1"},
			"chapter_set-INITIAL_FORMS": {"1"},
			"chapter_set-MAX_NUM_FORMS": {"1000"},
			"_save":                     {"Save"},
			"chapter_set-0-id":          {fmt.Sprint(chapter.ID)},
			"chapter_set-0-name":        {chapter.Name},
			"chapter_set-0-novel":       {fmt.Sprint(lotr.ID)},
			"chapter_set-0-DELETE":      {"on"},
		}
		resp := env.client.Post(changeURL, data)
		djangotest.AssertContains(t, resp, fmt.Sprintf("Deleting chapter Chapter object (%d) would require deleting "+
			"the following protected related objects: foot note FootNote object (%d)", chapter.ID, footNote.ID))
	})
}

// inlinePermsFixture holds the rows of TestInlinePermissions.setUpTestData.
type inlinePermsFixture struct {
	env             *adminTestEnv
	user            *auth.User
	holder          *Holder2
	inner2          *Inner2
	holderChangeURL string
}

// newInlinePermsFixture creates the rows of TestInlinePermissions and logs
// the staff user in. The user may add and change Holder2 rows; the tests
// grant permissions on Inner2.
func newInlinePermsFixture(t *testing.T, db *orm.DB) inlinePermsFixture {
	t.Helper()
	f := inlinePermsFixture{env: inlinesEnv(t, db, "admin")}
	f.user = createTestUser(t, db, "admin", "secret", true, false)
	grantPerm(t, db, f.user, "admin_inlines", "add_holder2")
	grantPerm(t, db, f.user, "admin_inlines", "change_holder2")
	f.holder = create(t, db, &Holder2{Dummy: 13})
	f.inner2 = create(t, db, &Inner2{Dummy: 42, Holder: fk[Holder2](f.holder.ID)})
	f.holderChangeURL = mustReverse(t, f.env.router, "admin:admin_inlines_holder2_change", f.holder.ID)
	f.env.client.ForceLogin(f.user)
	return f
}

// The Author and Book cases of TestInlinePermissions edit the auto-created
// through model of a many-to-many field, which an inline cannot.

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_add_m2m_noperm
func TestInlinePermissionsAddM2MNoPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_add_fk_noperm
func TestInlinePermissionsAddFKNoPerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		resp := f.env.client.Get(mustReverse(t, f.env.router, "admin:admin_inlines_holder2_add"))
		// No permissions on Inner2s, so no inline.
		djangotest.AssertNotContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, "Add another Inner2")
		djangotest.AssertNotContains(t, resp, `id="id_inner2_set-TOTAL_FORMS"`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_m2m_noperm
func TestInlinePermissionsChangeM2MNoPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_noperm
func TestInlinePermissionsChangeFKNoPerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		resp := f.env.client.Get(f.holderChangeURL)
		// No permissions on Inner2s, so no inline.
		djangotest.AssertNotContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, "Add another Inner2")
		djangotest.AssertNotContains(t, resp, `id="id_inner2_set-TOTAL_FORMS"`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_add_m2m_view_only_perm
func TestInlinePermissionsAddM2MViewOnlyPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_add_m2m_add_perm
func TestInlinePermissionsAddM2MAddPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_add_fk_add_perm
func TestInlinePermissionsAddFKAddPerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "add_inner2")
		resp := f.env.client.Get(mustReverse(t, f.env.router, "admin:admin_inlines_holder2_add"))
		// Add permission on inner2s, so we get the inline.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, "Add another Inner2")
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" `+
			`value="3" name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_m2m_add_perm
func TestInlinePermissionsChangeM2MAddPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_m2m_view_only_perm
func TestInlinePermissionsChangeM2MViewOnlyPerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_m2m_change_perm
func TestInlinePermissionsChangeM2MChangePerm(t *testing.T) {
	t.Skip("FIXME-PORT: Go: an inline cannot edit a many-to-many through model, Django: Author.books.through")
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_add_perm
func TestInlinePermissionsChangeFKAddPerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "add_inner2")
		resp := f.env.client.Get(f.holderChangeURL)
		// Add permission on inner2s, so we can add but not modify existing.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, "Add another Inner2")
		// 3 extra forms only, not the existing instance form.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" value="3" `+
			`name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, fmt.Sprintf(`<input type="hidden" id="id_inner2_set-0-id" value="%d" `+
			`name="inner2_set-0-id">`, f.inner2.ID), djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_change_perm
func TestInlinePermissionsChangeFKChangePerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "change_inner2")
		resp := f.env.client.Get(f.holderChangeURL)
		// Change permission on inner2s, so we can change existing but not add
		// new.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		// Just the one form for existing instances.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" value="1" `+
			`name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="hidden" id="id_inner2_set-0-id" value="%d" `+
			`name="inner2_set-0-id">`, f.inner2.ID), djangotest.HTML())
		// max-num 0 means we can't add new ones.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-MAX_NUM_FORMS" value="0" `+
			`name="inner2_set-MAX_NUM_FORMS">`, djangotest.HTML())
		// TabularInline
		djangotest.AssertContains(t, resp, `<th class="column-dummy required">Dummy</th>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="number" name="inner2_set-2-0-dummy" value="%d" `+
			`class="vIntegerField" id="id_inner2_set-2-0-dummy">`, f.inner2.Dummy), djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_add_change_perm
func TestInlinePermissionsChangeFKAddChangePerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "add_inner2")
		grantPerm(t, db, f.user, "admin_inlines", "change_inner2")
		resp := f.env.client.Get(f.holderChangeURL)
		// Add/change perm, so we can add new and change existing.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		// One form for existing instance and three extra for new.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" value="4" `+
			`name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="hidden" id="id_inner2_set-0-id" value="%d" `+
			`name="inner2_set-0-id">`, f.inner2.ID), djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_change_del_perm
func TestInlinePermissionsChangeFKChangeDelPerm(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "change_inner2")
		grantPerm(t, db, f.user, "admin_inlines", "delete_inner2")
		resp := f.env.client.Get(f.holderChangeURL)
		// Change/delete perm on inner2s, so we can change/delete existing.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		// One form for existing instance only, no new.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" value="1" `+
			`name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="hidden" id="id_inner2_set-0-id" value="%d" `+
			`name="inner2_set-0-id">`, f.inner2.ID), djangotest.HTML())
		djangotest.AssertContains(t, resp, `id="id_inner2_set-0-DELETE"`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestInlinePermissions.test_inline_change_fk_all_perms
func TestInlinePermissionsChangeFKAllPerms(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		f := newInlinePermsFixture(t, db)
		grantPerm(t, db, f.user, "admin_inlines", "add_inner2")
		grantPerm(t, db, f.user, "admin_inlines", "change_inner2")
		grantPerm(t, db, f.user, "admin_inlines", "delete_inner2")
		resp := f.env.client.Get(f.holderChangeURL)
		// All perms on inner2s, so we can add/change/delete.
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		djangotest.AssertContains(t, resp,
			`<h2 id="inner2_set-2-heading" class="inline-heading">Inner2s</h2>`, djangotest.HTML())
		// One form for existing instance only, three for new.
		djangotest.AssertContains(t, resp, `<input type="hidden" id="id_inner2_set-TOTAL_FORMS" value="4" `+
			`name="inner2_set-TOTAL_FORMS">`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="hidden" id="id_inner2_set-0-id" value="%d" `+
			`name="inner2_set-0-id">`, f.inner2.ID), djangotest.HTML())
		djangotest.AssertContains(t, resp, `id="id_inner2_set-0-DELETE"`)
		// TabularInline
		djangotest.AssertContains(t, resp, `<th class="column-dummy required">Dummy</th>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<input type="number" name="inner2_set-2-0-dummy" value="%d" `+
			`class="vIntegerField" id="id_inner2_set-2-0-dummy">`, f.inner2.Dummy), djangotest.HTML())
	})
}

// readOnlyPollFixture is the setUpTestData of
// TestReadOnlyChangeViewInlinePermissions: a staff user who may view polls
// and do everything with questions, logged in.
func readOnlyPollFixture(t *testing.T, db *orm.DB) (env *adminTestEnv, poll *Poll, addURL, changeURL string) {
	t.Helper()
	env = inlinesEnv(t, db, "admin")
	user := createTestUser(t, db, "testing", "password", true, false)
	grantPerm(t, db, user, "admin_inlines", "view_poll")
	for _, action := range []string{"add", "change", "delete", "view"} {
		grantPerm(t, db, user, "admin_inlines", action+"_pollquestion")
	}
	poll = create(t, db, &Poll{Name: "Survey"})
	env.client.ForceLogin(user)
	return env, poll, mustReverse(t, env.router, "admin:admin_inlines_poll_add"),
		mustReverse(t, env.router, "admin:admin_inlines_poll_change", poll.ID)
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_add_url_not_allowed
func TestReadOnlyChangeViewInlineAddURLNotAllowed(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, _, addURL, _ := readOnlyPollFixture(t, db)
		djangotest.AssertStatus(t, env.client.Get(addURL), http.StatusForbidden)
		djangotest.AssertStatus(t, env.client.Post(addURL, url.Values{}), http.StatusForbidden)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_post_to_change_url_not_allowed
func TestReadOnlyChangeViewInlinePostNotAllowed(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, _, _, changeURL := readOnlyPollFixture(t, db)
		djangotest.AssertStatus(t, env.client.Post(changeURL, url.Values{}), http.StatusForbidden)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_get_to_change_url_is_allowed
func TestReadOnlyChangeViewInlineGetAllowed(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, _, _, changeURL := readOnlyPollFixture(t, db)
		djangotest.AssertStatus(t, env.client.Get(changeURL), http.StatusOK)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_main_model_is_rendered_as_read_only
func TestReadOnlyChangeViewInlineMainModelReadOnly(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, poll, _, changeURL := readOnlyPollFixture(t, db)
		resp := env.client.Get(changeURL)
		djangotest.AssertContains(t, resp, `<div class="readonly">`+poll.Name+`</div>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `<input type="text" name="name" value="`+poll.Name+`" class="vTextField" `+
			`maxlength="40" required id="id_name">`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_inlines_are_rendered_as_read_only
func TestReadOnlyChangeViewInlinesReadOnly(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, poll, _, changeURL := readOnlyPollFixture(t, db)
		question := create(t, db, &PollQuestion{Text: "How will this be rendered?", Poll: fk[Poll](poll.ID)})
		resp := env.client.Get(changeURL)
		djangotest.AssertContains(t, resp, `<td class="field-text"><p>`+question.Text+`</p></td>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `id="id_question_set-0-text"`)
		djangotest.AssertNotContains(t, resp, `id="id_related_objs-0-DELETE"`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_submit_line_shows_only_close_button
func TestReadOnlyChangeViewInlineSubmitLineClose(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, poll, _, changeURL := readOnlyPollFixture(t, db)
		resp := env.client.Get(changeURL)
		djangotest.AssertContains(t, resp,
			`<a role="button" href="/admin/admin_inlines/poll/" class="closelink">Close</a>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, fmt.Sprintf(`<a role="button" href="/admin/admin_inlines/poll/%d/delete/" `+
			`class="deletelink">Delete</a>`, poll.ID), djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `<input type="submit" value="Save and add another" name="_addanother">`)
		djangotest.AssertNotContains(t, resp, `<input type="submit" value="Save and continue editing" name="_continue">`)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_inline_delete_buttons_are_not_shown
func TestReadOnlyChangeViewInlineNoDeleteButtons(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, poll, _, changeURL := readOnlyPollFixture(t, db)
		create(t, db, &PollQuestion{Text: "How will this be rendered?", Poll: fk[Poll](poll.ID)})
		resp := env.client.Get(changeURL)
		djangotest.AssertNotContains(t, resp,
			`<input type="checkbox" name="question_set-0-DELETE" id="id_question_set-0-DELETE">`, djangotest.HTML())
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestReadOnlyChangeViewInlinePermissions.test_extra_inlines_are_not_shown
func TestReadOnlyChangeViewInlineNoExtraForms(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env, _, _, changeURL := readOnlyPollFixture(t, db)
		resp := env.client.Get(changeURL)
		djangotest.AssertNotContains(t, resp, `id="id_question_set-0-text"`)
	})
}

// profileInlines returns the four inlines of the TestVerboseNameInlineForms
// tests with the names that names returns for each model.
func profileInlines(names func(model string) (singular, plural string)) []admin.Inline {
	cfg := func(model string) admin.InlineModelAdmin {
		singular, plural := names(model)
		return admin.InlineModelAdmin{VerboseName: singular, VerboseNamePlural: plural}
	}
	return []admin.Inline{
		admin.TabularInline[ProfileCollection, Profile](cfg("NonVerbose")),
		admin.TabularInline[ProfileCollection, VerboseNameProfile](cfg("VerboseName")),
		admin.TabularInline[ProfileCollection, VerboseNamePluralProfile](cfg("VerboseNamePlural")),
		admin.TabularInline[ProfileCollection, BothVerboseNameProfile](cfg("BothVerboseName")),
	}
}

// profileCollectionPage registers ProfileCollection with the inlines and
// returns its change page for a superuser. Django calls changeform_view of a
// ModelAdmin built by hand.
func profileCollectionPage(t *testing.T, db *orm.DB, inlines []admin.Inline) *djangotest.Response {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[ProfileCollection](site, admin.ModelAdmin{Inlines: inlines})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	obj := create(t, db, &ProfileCollection{})
	return env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_profilecollection_change", obj.ID))
}

// heading returns the heading of the inline with the given prefix.
func heading(prefix, text string) string {
	return `<h2 id="` + prefix + `-heading" class="inline-heading">` + text + `</h2>`
}

// Ported from Django: tests/admin_inlines/tests.py::TestVerboseNameInlineForms.test_verbose_name_inline
func TestVerboseNameInline(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		resp := profileCollectionPage(t, db, profileInlines(func(model string) (string, string) {
			return map[string]string{
				"NonVerbose":        "Non-verbose childs",
				"VerboseName":       "Childs with verbose name",
				"VerboseNamePlural": "Childs with verbose name plural",
				"BothVerboseName":   "Childs with both verbose names",
			}[model], ""
		}))
		html := djangotest.HTML()
		djangotest.AssertNotContains(t, resp, "Add another Profile")
		// Non-verbose model.
		djangotest.AssertContains(t, resp, heading("profile_set", "Non-verbose childss"), html)
		djangotest.AssertContains(t, resp, "Add another Non-verbose child")
		djangotest.AssertNotContains(t, resp, heading("profile_set", "Profiles"), html)
		// Model with verbose name.
		djangotest.AssertContains(t, resp, heading("verbosenameprofile_set", "Childs with verbose names"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with verbose name")
		djangotest.AssertNotContains(t, resp, heading("verbosenameprofile_set", "Model with verbose name onlys"), html)
		djangotest.AssertNotContains(t, resp, "Add another Model with verbose name only")
		// Model with verbose name plural.
		djangotest.AssertContains(t, resp,
			heading("verbosenamepluralprofile_set", "Childs with verbose name plurals"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with verbose name plural")
		djangotest.AssertNotContains(t, resp,
			heading("verbosenamepluralprofile_set", "Model with verbose name plural only"), html)
		// Model with both verbose names.
		djangotest.AssertContains(t, resp,
			heading("bothverbosenameprofile_set", "Childs with both verbose namess"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with both verbose names")
		djangotest.AssertNotContains(t, resp,
			heading("bothverbosenameprofile_set", "Model with both - plural name"), html)
		djangotest.AssertNotContains(t, resp, "Add another Model with both - name")
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestVerboseNameInlineForms.test_verbose_name_plural_inline
func TestVerboseNamePluralInline(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		resp := profileCollectionPage(t, db, profileInlines(func(model string) (string, string) {
			return "", map[string]string{
				"NonVerbose":        "Non-verbose childs",
				"VerboseName":       "Childs with verbose name",
				"VerboseNamePlural": "Childs with verbose name plural",
				"BothVerboseName":   "Childs with both verbose names",
			}[model]
		}))
		html := djangotest.HTML()
		// Non-verbose model.
		djangotest.AssertContains(t, resp, heading("profile_set", "Non-verbose childs"), html)
		djangotest.AssertContains(t, resp, "Add another Profile")
		djangotest.AssertNotContains(t, resp, heading("profile_set", "Profiles"), html)
		// Model with verbose name.
		djangotest.AssertContains(t, resp, heading("verbosenameprofile_set", "Childs with verbose name"), html)
		djangotest.AssertContains(t, resp, "Add another Model with verbose name only")
		djangotest.AssertNotContains(t, resp, heading("verbosenameprofile_set", "Model with verbose name onlys"), html)
		// Model with verbose name plural.
		djangotest.AssertContains(t, resp,
			heading("verbosenamepluralprofile_set", "Childs with verbose name plural"), html)
		djangotest.AssertContains(t, resp, "Add another Profile")
		djangotest.AssertNotContains(t, resp,
			heading("verbosenamepluralprofile_set", "Model with verbose name plural only"), html)
		// Model with both verbose names.
		djangotest.AssertContains(t, resp,
			heading("bothverbosenameprofile_set", "Childs with both verbose names"), html)
		djangotest.AssertContains(t, resp, "Add another Model with both - name")
		djangotest.AssertNotContains(t, resp,
			heading("bothverbosenameprofile_set", "Model with both - plural name"), html)
	})
}

// Ported from Django: tests/admin_inlines/tests.py::TestVerboseNameInlineForms.test_both_verbose_names_inline
func TestBothVerboseNamesInline(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		resp := profileCollectionPage(t, db, profileInlines(func(model string) (string, string) {
			names := map[string][2]string{
				"NonVerbose":        {"Non-verbose childs - name", "Non-verbose childs - plural name"},
				"VerboseName":       {"Childs with verbose name - name", "Childs with verbose name - plural name"},
				"VerboseNamePlural": {"Childs with verbose name plural - name", "Childs with verbose name plural - plural name"},
				"BothVerboseName":   {"Childs with both - name", "Childs with both - plural name"},
			}[model]
			return names[0], names[1]
		}))
		html := djangotest.HTML()
		djangotest.AssertNotContains(t, resp, "Add another Profile")
		// Non-verbose model.
		djangotest.AssertContains(t, resp, heading("profile_set", "Non-verbose childs - plural name"), html)
		djangotest.AssertContains(t, resp, "Add another Non-verbose childs - name")
		djangotest.AssertNotContains(t, resp, heading("profile_set", "Profiles"), html)
		// Model with verbose name.
		djangotest.AssertContains(t, resp,
			heading("verbosenameprofile_set", "Childs with verbose name - plural name"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with verbose name - name")
		djangotest.AssertNotContains(t, resp, heading("verbosenameprofile_set", "Model with verbose name onlys"), html)
		// Model with verbose name plural.
		djangotest.AssertContains(t, resp,
			heading("verbosenamepluralprofile_set", "Childs with verbose name plural - plural name"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with verbose name plural - name")
		djangotest.AssertNotContains(t, resp,
			heading("verbosenamepluralprofile_set", "Model with verbose name plural only"), html)
		// Model with both verbose names.
		djangotest.AssertContains(t, resp,
			heading("bothverbosenameprofile_set", "Childs with both - plural name"), html)
		djangotest.AssertContains(t, resp, "Add another Childs with both - name")
		djangotest.AssertNotContains(t, resp,
			heading("bothverbosenameprofile_set", "Model with both - plural name"), html)
		djangotest.AssertNotContains(t, resp, "Add another Model with both - name")
	})
}

// No Go counterpart: tests/admin_inlines/tests.py::TestInlineWithFieldsets.test_fieldset_context_fully_set
// asserts that rendering logs no missing template variable; pongo2 has no
// such log.

// Ported from Django: tests/admin_inlines/tests.py::TestInlineWithFieldsets.test_inline_headings
func TestInlineWithFieldsetsHeadings(t *testing.T) {
	djangotest.EachDB(t, inlinesModels(), func(t *testing.T, db *orm.DB) {
		env := inlinesSuperEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_inlines_photographer_add"))
		// Page main title.
		djangotest.AssertContains(t, resp, "<h1>Add photographer</h1>", djangotest.HTML())

		// Headings for the toplevel fieldsets. The first one has no name.
		djangotest.AssertContains(t, resp, `<fieldset class="module aligned ">`)
		// The second and third have the same "Advanced options" name, but the
		// second one has the "collapse" class.
		for x, classes := range map[int]string{1: "", 2: "collapse"} {
			headingID := fmt.Sprintf("fieldset-0-%d-heading", x)
			djangotest.AssertContains(t, resp,
				fmt.Sprintf(`<fieldset class="module aligned %s" aria-labelledby="%s">`, classes, headingID))
			djangotest.AssertContains(t, resp,
				fmt.Sprintf(`<h2 id="%s" class="fieldset-heading">Advanced options</h2>`, headingID))
			djangotest.AssertContains(t, resp, fmt.Sprintf(`id="%s"`, headingID), djangotest.Count(1))
		}

		// Headings and subheadings for all the inlines.
		for _, set := range inlineSets(t, resp) {
			prefix := set["formset"].(forms.TemplateFormSet)["prefix"].(string)
			headingID := prefix + "-heading"
			formsetHeading := fmt.Sprintf(`<h2 id="%s" class="inline-heading">Photos</h2>`, headingID)
			djangotest.AssertContains(t, resp, formsetHeading, djangotest.HTML())
			djangotest.AssertContains(t, resp, fmt.Sprintf(`id="%s"`, headingID), djangotest.Count(1))

			// If this is a TabularInline, do not make further asserts since
			// fieldsets are not shown as such in this table layout.
			if strings.Contains(set["template"].(string), "tabular") {
				continue
			}
			if strings.Contains(set["classes"].(string), "collapse") {
				djangotest.AssertContains(t, resp, "<summary>"+formsetHeading+"</summary>",
					djangotest.HTML(), djangotest.Count(1))
			}

			// Headings for every formset (the amount depends on extra).
			for y, form := range maps(set["forms"]) {
				djangotest.AssertContains(t, resp, fmt.Sprintf(
					`<h3><b>Photo:</b> <span class="inline_label">#%d</span></h3>`, y+1), djangotest.HTML())

				// Every fieldset defined for an inline's form.
				for z, fieldset := range maps(form["fieldsets"]) {
					classes, _ := fieldset["classes"].(string)
					if name, _ := fieldset["name"].(string); name == "" {
						djangotest.AssertContains(t, resp, fmt.Sprintf(`<fieldset class="module aligned %s">`, classes))
						continue
					}
					headingID := fmt.Sprintf("%s-%d-%d-heading", prefix, y, z)
					djangotest.AssertContains(t, resp, fmt.Sprintf(
						`<fieldset class="module aligned %s" aria-labelledby="%s">`, classes, headingID))
					fieldsetHeading := fmt.Sprintf(`<h4 id="%s" class="fieldset-heading">Details</h4>`, headingID)
					djangotest.AssertContains(t, resp, fieldsetHeading)
					if strings.Contains(classes, "collapse") {
						djangotest.AssertContains(t, resp, "<summary>"+fieldsetHeading+"</summary>", djangotest.HTML())
					}
					djangotest.AssertContains(t, resp, fmt.Sprintf(`id="%s"`, headingID), djangotest.Count(1))
				}
			}
		}
	})
}
