package forms_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// todayCalls counts the calls of todayCallableDict.
var todayCalls atomic.Int64

// todayCallableDict is the today_callable_dict of Django's
// tests/model_forms/models.py: the characters whose last action is not in the
// past.
func todayCallableDict() orm.Cond {
	todayCalls.Add(1)
	return orm.Q("last_action__gte", time.Now())
}

// todayCallableQ is the today_callable_q of Django's
// tests/model_forms/models.py.
func todayCallableQ() orm.Cond { return orm.Q("last_action__gte", time.Now()) }

// StumpJoke is the StumpJoke model of Django's tests/model_forms/models.py.
type StumpJoke struct {
	ID                 int64
	MostRecentlyFooled orm.FK[Character]  `orm:"on_delete=cascade;related_name=jokes"`
	HasFooledToday     orm.M2M[Character] `orm:"related_name=jokes_today"`
	Funny              bool
}

// Meta sets the limit_choices_to of the relations.
func (StumpJoke) Meta() orm.Meta {
	return orm.Meta{LimitChoicesToFunc: map[string]func() orm.Cond{
		"MostRecentlyFooled": todayCallableDict,
		"HasFooledToday":     todayCallableQ,
	}}
}

// CharacterDetails is the CharacterDetails model of Django's
// LimitChoicesToTests.test_limit_choices_to_no_duplicates. Django's
// character2 limits with a dict, the Go limit is always a Cond.
type CharacterDetails struct {
	ID         int64
	Character1 orm.FK[Character]  `orm:"on_delete=cascade;related_name=details_fk_1"`
	Character2 orm.FK[Character]  `orm:"on_delete=cascade;related_name=details_fk_2"`
	Character3 orm.M2M[Character] `orm:"related_name=details_m2m_1"`
}

// Meta sets the limit_choices_to of the relations.
func (CharacterDetails) Meta() orm.Meta {
	funny := orm.Q("jokes__funny", true, "jokes_today__funny", true)
	return orm.Meta{LimitChoicesTo: map[string]orm.Cond{
		"Character1": funny,
		"Character2": funny,
		"Character3": funny,
	}}
}

// Number is the Number model of Django's tests/model_forms/models.py.
type Number struct {
	ID    int64
	Value int
}

// NumbersToDice is the NumbersToDice model of Django's
// tests/model_forms/models.py.
type NumbersToDice struct {
	ID     int64
	Number orm.FK[Number] `orm:"on_delete=cascade"`
	Die    orm.FK[Dice]   `orm:"on_delete=cascade"`
}

// Dice is the Dice model of Django's tests/model_forms/models.py.
type Dice struct {
	ID      int64
	Numbers orm.M2M[Number] `orm:"through=NumbersToDice"`
}

// Meta sets the limit_choices_to of Numbers.
func (Dice) Meta() orm.Meta {
	return orm.Meta{LimitChoicesTo: map[string]orm.Cond{"Numbers": orm.Q("value__gte", 1)}}
}

// eachLimit runs fn on every database with the models of the ported
// LimitChoicesToTests, after creating its characters: threepwood, whose last
// action is tomorrow, and marley, whose last action was yesterday.
func eachLimit(t *testing.T, fn func(t *testing.T, db *orm.DB, threepwood, marley *Character)) {
	t.Helper()
	models := []any{&Character{}, &StumpJoke{}, &CharacterDetails{}, &Number{}, &NumbersToDice{}, &Dice{}}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		now := time.Now().UTC()
		threepwood := &Character{Username: "threepwood", LastAction: now.AddDate(0, 0, 1)}
		marley := &Character{Username: "marley", LastAction: now.AddDate(0, 0, -1)}
		for _, c := range []*Character{threepwood, marley} {
			if err := orm.Query[Character](db).Create(context.Background(), c); err != nil {
				t.Fatal(err)
			}
		}
		fn(t, db, threepwood, marley)
	})
}

// optionLabels returns the labels of the options of the field name of form,
// without the empty option, in sorted order: the rows of Django's
// form.fields[name].queryset, compared with assertCountEqual.
func optionLabels(form *forms.Form, name string) []string {
	var out []string
	for _, m := range choiceOptionRe.FindAllStringSubmatch(renderedField(form, name), -1) {
		if m[1] != "" {
			out = append(out, m[2])
		}
	}
	slices.Sort(out)
	return out
}

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_limit_choices_to_callable_for_fk_rel
func TestLimitChoicesToCallableForFKRel(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, _, _ *Character) {
		form := forms.NewModelForm[StumpJoke](db, forms.WithFields(forms.AllFields))
		djangotest.AssertEqual(t, optionLabels(form.Form, "most_recently_fooled"), []string{"threepwood"})
	})
}

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_limit_choices_to_callable_for_m2m_rel
//
// Django's test reads most_recently_fooled again; the port reads the
// many-to-many field that the test names.
func TestLimitChoicesToCallableForM2MRel(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, _, _ *Character) {
		form := forms.NewModelForm[StumpJoke](db, forms.WithFields(forms.AllFields))
		djangotest.AssertEqual(t, optionLabels(form.Form, "has_fooled_today"), []string{"threepwood"})
	})
}

// No Go counterpart: tests/model_forms/tests.py::LimitChoicesToTests.test_custom_field_with_queryset_but_no_limit_choices_to
// checks that a declared field with a queryset attribute but no
// get_limit_choices_to() is left alone. A Go form applies the limit only in
// the fields it builds for relations.

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_fields_for_model_applies_limit_choices_to
func TestLimitChoicesToFieldsForModel(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, _, _ *Character) {
		form := forms.NewModelForm[StumpJoke](db, forms.WithFields("has_fooled_today"))
		djangotest.AssertEqual(t, optionLabels(form.Form, "has_fooled_today"), []string{"threepwood"})
	})
}

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_callable_called_each_time_form_is_instantiated
//
// Django evaluates the limit when it builds the form. A Go form evaluates it
// each time it queries the options, here once per render of each form.
func TestLimitChoicesToCallableCalledEachTime(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, _, _ *Character) {
		before := todayCalls.Load()
		for i := range int64(3) {
			form := forms.NewModelForm[StumpJoke](db, forms.WithFields(forms.AllFields))
			renderedField(form.Form, "most_recently_fooled")
			djangotest.AssertEqual(t, todayCalls.Load()-before, i+1)
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_limit_choices_to_no_duplicates
func TestLimitChoicesToNoDuplicates(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, threepwood, marley *Character) {
		ctx := context.Background()
		joke := func(funny bool, fooled *Character) *StumpJoke {
			j := &StumpJoke{Funny: funny}
			j.MostRecentlyFooled.SetPK(fooled.ID)
			if err := orm.Query[StumpJoke](db).Create(ctx, j); err != nil {
				t.Fatal(err)
			}
			return j
		}
		fooledToday := func(j *StumpJoke, cs ...*Character) {
			if err := orm.Many[Character](db, j, "has_fooled_today").Add(ctx, cs...); err != nil {
				t.Fatal(err)
			}
		}
		joke1 := joke(true, threepwood)
		joke2 := joke(true, threepwood)
		joke3 := joke(true, marley)
		joke(false, marley)
		fooledToday(joke1, marley, threepwood)
		fooledToday(joke2, marley)
		fooledToday(joke3, marley, threepwood)

		form := forms.NewModelForm[CharacterDetails](db, forms.WithFields(forms.AllFields))
		for _, name := range []string{"character1", "character2", "character3"} {
			djangotest.AssertEqual(t, optionLabels(form.Form, name), []string{"marley", "threepwood"})
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::LimitChoicesToTests.test_limit_choices_to_m2m_through
func TestLimitChoicesToM2MThrough(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, _, _ *Character) {
		ctx := context.Background()
		var keys []string
		for _, v := range []int{0, 1, 2} {
			n := &Number{Value: v}
			if err := orm.Query[Number](db).Create(ctx, n); err != nil {
				t.Fatal(err)
			}
			if v >= 1 {
				keys = append(keys, choiceID(n.ID))
			}
		}
		form := forms.NewModelForm[Dice](db, forms.WithFields("numbers"))
		matches := choiceOptionRe.FindAllStringSubmatch(renderedField(form.Form, "numbers"), -1)
		got := make([]string, 0, len(matches))
		for _, m := range matches {
			got = append(got, m[1])
		}
		slices.Sort(got)
		djangotest.AssertEqual(t, got, keys)
	})
}

// A submitted key outside the limit is not a valid choice, as Django's
// ModelChoiceField looks the key up in the limited queryset. No Django test
// exists.
func TestLimitChoicesToRejectsKeyOutsideLimit(t *testing.T) {
	eachLimit(t, func(t *testing.T, db *orm.DB, threepwood, marley *Character) {
		form := forms.NewModelForm[StumpJoke](db, forms.WithFields("most_recently_fooled")).
			Bind(vals("most_recently_fooled", choiceID(marley.ID)))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, form.Errors().Messages()["most_recently_fooled"],
			[]string{"Select a valid choice. That choice is not one of the available choices."})
		form = forms.NewModelForm[StumpJoke](db, forms.WithFields("most_recently_fooled")).
			Bind(vals("most_recently_fooled", choiceID(threepwood.ID)))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}
