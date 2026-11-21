package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// ExcludeBand is a model with three fields for TestReplaceExclude.
type ExcludeBand struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Bio   string `orm:"type=text"`
	Genre string `orm:"max_length=20"`
}

// No Django test exists: the exclude argument of Django's modelform_factory
// replaces the Meta.exclude of the form it extends, which the admin relies on
// to let ModelAdmin.exclude win over the Meta.exclude of its form.
// ReplaceExclude is the Go form of that argument.
func TestReplaceExclude(t *testing.T) {
	m, err := orm.NewRegistry().Register(&ExcludeBand{})
	if err != nil {
		t.Fatalf("register ExcludeBand: %v", err)
	}
	form := forms.FromModel(m, forms.WithFields(forms.AllFields), forms.WithExclude("bio"), forms.ReplaceExclude("name"))
	djangotest.AssertEqual(t, form.FieldNames(), []string{"bio", "genre"})

	// The exclusions of later options add to those of ReplaceExclude.
	form = forms.FromModel(m, forms.WithFields(forms.AllFields), forms.ReplaceExclude("name"), forms.WithExclude("bio"))
	djangotest.AssertEqual(t, form.FieldNames(), []string{"genre"})

	// Without names, ReplaceExclude drops every earlier exclusion.
	form = forms.FromModel(m, forms.WithFields(forms.AllFields), forms.WithExclude("bio"), forms.ReplaceExclude())
	djangotest.AssertEqual(t, form.FieldNames(), []string{"name", "bio", "genre"})
}
