package forms_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Person is the Person model of Django's tests/model_forms/models.py.
type Person struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Category is the Category model of Django's tests/model_forms/models.py. The
// Go ORM has no SlugField: categoryOptions gives the form the slug field.
type Category struct {
	ID   int64
	Name string `orm:"max_length=20"`
	Slug string `orm:"max_length=20"`
	URL  string `orm:"max_length=40;verbose_name=The URL"`
}

// String returns the name, the __str__ of Django's Category.
func (c Category) String() string { return c.Name }

// Writer is the Writer model of Django's tests/model_forms/models.py. The Go
// ORM has no custom managers, so the manager that hides archived writers is
// not modelled.
type Writer struct {
	ID       int64
	Name     string `orm:"max_length=50;help_text=Use both first and last names."`
	Archived bool   `orm:"editable=false"`
}

// String returns the name, the __str__ of Django's Writer.
func (w Writer) String() string { return w.Name }

// Article is the Article model of Django's tests/model_forms/models.py, without
// the many-to-many field categories. PubDate and Created are datetimes, there
// is no DateField, and Status holds the choice values as strings, as the ORM
// accepts choices on string fields only.
type Article struct {
	ID       int64
	Headline string `orm:"max_length=50"`
	Slug     string `orm:"max_length=50"`
	PubDate  time.Time
	Created  time.Time `orm:"editable=false"`
	Writer   orm.FK[Writer]
	Article  string `orm:"type=text"`
	Status   string `orm:"max_length=1;blank"`
}

// Choices returns the choices of Article.ARTICLE_STATUS.
func (Article) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{"Status": {{Value: "1", Label: "Draft"}, {Value: "2", Label: "Pending"}, {Value: "3", Label: "Live"}}}
}

// Product is the Product model of Django's tests/model_forms/models.py, with a
// char field where Django has a SlugField.
type Product struct {
	ID   int64
	Slug string `orm:"max_length=50;unique"`
}

// Price is the Price model of Django's tests/model_forms/models.py. The ORM
// has no DecimalField, so the price is a string, and no unique_together.
type Price struct {
	ID       int64
	Price    string `orm:"max_length=12"`
	Quantity int
}

// Book is the Book model of Django's tests/model_forms/models.py, without the
// unique_together of title and author.
type Book struct {
	ID        int64
	Title     string         `orm:"max_length=40"`
	Author    orm.FK[Writer] `orm:"null;blank"`
	SpecialID int            `orm:"blank;null;unique"`
}

// ExplicitPK is the ExplicitPK model of Django's tests/model_forms/models.py,
// without the unique_together of key and desc.
type ExplicitPK struct {
	Key  string `orm:"max_length=20;pk"`
	Desc string `orm:"max_length=20;blank;unique"`
}

// Post is the Post model of Django's tests/model_forms/models.py, without the
// unique_for_date, unique_for_year and unique_for_month options.
type Post struct {
	ID       int64
	Title    string `orm:"max_length=50;blank"`
	Slug     string `orm:"max_length=50;blank"`
	Subtitle string `orm:"max_length=50;blank"`
	Posted   time.Time
}

// Character is the Character model of Django's tests/model_forms/models.py.
type Character struct {
	ID         int64
	Username   string `orm:"max_length=100"`
	LastAction time.Time
}

// String returns the user name, the __str__ of Django's Character.
func (c Character) String() string { return c.Username }

// Student is the Student model of Django's tests/model_forms/models.py.
type Student struct {
	ID        int64
	Character orm.FK[Character]
	Study     string `orm:"max_length=30"`
}

// Award is the Award model of Django's tests/model_forms/models.py, a model
// with ForeignKey(blank=False, null=True).
type Award struct {
	ID        int64
	Name      string            `orm:"max_length=30"`
	Character orm.FK[Character] `orm:"null;on_delete=set_null"`
}

// PublicationDefaults is the PublicationDefaults model of Django's
// tests/model_forms/models.py with the defaults a tag can express: the ORM has
// no callable defaults, DateField or FileField.
type PublicationDefaults struct {
	ID     int64
	Title  string `orm:"max_length=30"`
	Mode   string `orm:"max_length=2;default=di"`
	Active bool   `orm:"default=true"`
}

// Choices returns the choices of PublicationDefaults.MODE_CHOICES.
func (PublicationDefaults) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{"Mode": {{Value: "di", Label: "direct"}, {Value: "de", Label: "delayed"}}}
}

// ArticleStatus is the ArticleStatus model of Django's
// tests/model_forms/models.py.
type ArticleStatus struct {
	ID     int64
	Status string `orm:"max_length=2;blank"`
}

// Choices returns the choices of ArticleStatus.ARTICLE_STATUS_CHAR.
func (ArticleStatus) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{"Status": {{Value: "d", Label: "Draft"}, {Value: "p", Label: "Pending"}, {Value: "l", Label: "Live"}}}
}

// CustomErrorMessage is the CustomErrorMessage model of Django's
// tests/model_forms/models.py without its validators, error messages and
// clean method, which the ORM lacks.
type CustomErrorMessage struct {
	ID    int64
	Name1 string `orm:"max_length=50"`
	Name2 string `orm:"max_length=50"`
}

// mfModels returns the models of the ported tests/model_forms tests.
func mfModels() []any {
	return []any{
		&Person{}, &Category{}, &Writer{}, &Article{}, &Product{}, &Price{}, &Book{},
		&ExplicitPK{}, &Post{}, &Character{}, &Student{}, &Award{},
		&PublicationDefaults{}, &ArticleStatus{}, &CustomErrorMessage{},
	}
}

// eachMF runs fn on every database with the model_forms models.
func eachMF(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	djangotest.EachDB(t, mfModels(), fn)
}

// categoryOptions is the Meta of Django's CategoryForm variants: every
// field, with the slug field that the ORM cannot express as a model field.
func categoryOptions(extra ...forms.ModelFormOption) []forms.ModelFormOption {
	return append([]forms.ModelFormOption{
		forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(20))),
	}, extra...)
}

// articleOptions gives the Article form the date field of Django's pub_date.
func articleOptions(extra ...forms.ModelFormOption) []forms.ModelFormOption {
	return append([]forms.ModelFormOption{
		forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(50))),
		forms.WithFormField("pub_date", forms.NewDateField()),
	}, extra...)
}

// assertErrContains fails the test unless err is an error whose message
// contains want, the Go side of assertRaisesMessage.
func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error %v, want one containing %q", err, want)
	}
}
