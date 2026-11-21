package admin_test

import (
	"context"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/admin_widgets/tests.py::FilteredSelectMultipleWidgetTest.test_render
func TestFilteredSelectMultipleWidgetRender(t *testing.T) {
	// Backslash in verbose_name to ensure it is JavaScript escaped.
	w := admin.FilteredSelectMultiple{VerboseName: "test\\"}
	djangotest.AssertHTMLEqual(t, w.Render("test", "test", nil),
		`<select multiple name="test" class="selectfilter" `+
			`data-field-name="test\" data-is-stacked="0">`+"\n</select>")
}

// Ported from Django: tests/admin_widgets/tests.py::FilteredSelectMultipleWidgetTest.test_stacked_render
func TestFilteredSelectMultipleWidgetStackedRender(t *testing.T) {
	// Backslash in verbose_name to ensure it is JavaScript escaped.
	w := admin.FilteredSelectMultiple{VerboseName: "test\\", IsStacked: true}
	djangotest.AssertHTMLEqual(t, w.Render("test", "test", nil),
		`<select multiple name="test" class="selectfilterstacked" `+
			`data-field-name="test\" data-is-stacked="1">`+"\n</select>")
}

// Company stands in for tests/admin_widgets/models.py::Company.
type Company struct {
	ID   int64
	Name string `orm:"max_length=20"`
}

// String returns the company name.
func (c Company) String() string { return c.Name }

// Advisor stands in for tests/admin_widgets/models.py::Advisor.
type Advisor struct {
	ID        int64
	Name      string           `orm:"max_length=20"`
	Companies orm.M2M[Company] `orm:"related_name=advisors"`
}

// adminWidgetsModels returns the model set of the ported admin_widgets tests.
func adminWidgetsModels() []any {
	return []any{auth.App{}, contenttypes.App{}, djangotest.App("admin_widgets", &Company{}, &Advisor{})}
}

// advisorAddPage renders the add page of Advisor under ma for user.
func advisorAddPage(t *testing.T, db *orm.DB, user *auth.User, ma admin.ModelAdmin) *djangotest.Response {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Advisor](site, ma)
	})
	env.client.ForceLogin(user)
	resp := env.client.Get(mustReverse(t, env.router, "admin:admin_widgets_advisor_add"))
	djangotest.AssertStatus(t, resp, 200)
	return resp
}

// The Django tests ask formfield_for_dbfield for the widget of Band.members;
// the Go admin has no such hook, so these tests read the widget off the
// rendered add page. Advisor.companies has the shape of Band.members (a plain
// many-to-many field), and a Go Band type already stands in for the Band of
// tests/modeladmin.
//
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_many_to_many
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_filtered_many_to_many
// Ported from Django: tests/admin_widgets/tests.py::AdminFormfieldForDBFieldTests.test_m2m_widgets
func TestAdminFormfieldForDBFieldManyToMany(t *testing.T) {
	djangotest.EachDB(t, adminWidgetsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, name := range []string{"Acme", "Globex"} {
			if err := orm.Query[Company](db).Create(ctx, &Company{Name: name}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}
		superuser := createTestUser(t, db, "super", "secret", true, true)

		t.Run("test_many_to_many", func(t *testing.T) {
			resp := advisorAddPage(t, db, superuser, admin.ModelAdmin{})
			djangotest.AssertContains(t, resp,
				`<select name="companies" id="id_companies" multiple required aria-describedby="id_companies_helptext" `+
					`data-context="available-source">`+
					`<option value="1">Acme</option><option value="2">Globex</option></select>`,
				djangotest.HTML())
		})

		t.Run("test_filtered_many_to_many", func(t *testing.T) {
			resp := advisorAddPage(t, db, superuser, admin.ModelAdmin{FilterVertical: []string{"companies"}})
			djangotest.AssertContains(t, resp,
				`<select name="companies" id="id_companies" multiple required `+
					`class="selectfilterstacked" data-context="available-source" data-field-name="companies" data-is-stacked="1">`+
					`<option value="1">Acme</option><option value="2">Globex</option></select>`,
				djangotest.HTML())
		})

		t.Run("test_m2m_widgets", func(t *testing.T) {
			resp := advisorAddPage(t, db, superuser, admin.ModelAdmin{FilterVertical: []string{"companies"}})
			djangotest.AssertContains(t, resp, `class="selectfilterstacked"`)
			djangotest.AssertContains(t, resp,
				"Hold down “Control”, or “Command” on a Mac, to select more than one.")
		})

		// test_raw_id_many_to_many is in TestAdminFormfieldForDBFieldRawID.
		// FIXME-PORT: test_formfield_overrides needs formfield_overrides, and
		// test_m2m_widgets_no_allow_multiple_selected needs a widget with
		// allow_multiple_selected = False; the Go admin has neither.
	})
}

// Topping stands in for tests/admin_views/models.py::Topping.
type Topping struct {
	ID   int64
	Name string `orm:"max_length=20"`
}

// String returns the topping name.
func (tp Topping) String() string { return tp.Name }

// Pizza stands in for tests/admin_views/models.py::Pizza.
type Pizza struct {
	ID       int64
	Name     string           `orm:"max_length=20"`
	Toppings orm.M2M[Topping] `orm:"related_name=pizzas"`
}

// String returns the pizza name.
func (p Pizza) String() string { return p.Name }

// pizzaModels returns the model set of the ported Pizza and Topping tests.
func pizzaModels() []any {
	return []any{auth.App{}, contenttypes.App{}, djangotest.App("admin_views", &Topping{}, &Pizza{})}
}

// pizzaEnv registers Pizza and Topping with the readonly fields of
// tests/admin_views/admin.py (PizzaAdmin and ToppingAdmin), or with none when
// readonly is false, and logs in a superuser.
func pizzaEnv(t *testing.T, db *orm.DB, readonly bool) *adminTestEnv {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		pizza, topping := admin.ModelAdmin{}, admin.ModelAdmin{}
		if readonly {
			pizza.ReadonlyFields = []string{"toppings"}
			topping.ReadonlyFields = []string{"pizzas"}
		}
		admin.Register[Pizza](site, pizza)
		admin.Register[Topping](site, topping)
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	return env
}

// pizzaWithTopping creates the pizza "Americano" with the topping "Salami".
func pizzaWithTopping(t *testing.T, db *orm.DB) (*Pizza, *Topping) {
	t.Helper()
	ctx := context.Background()
	topping := &Topping{Name: "Salami"}
	if err := orm.Query[Topping](db).Create(ctx, topping); err != nil {
		t.Fatalf("Create topping: %v", err)
	}
	pizza := &Pizza{Name: "Americano"}
	if err := orm.Query[Pizza](db).Create(ctx, pizza); err != nil {
		t.Fatalf("Create pizza: %v", err)
	}
	if err := orm.Many[Topping](db, pizza, "toppings").Add(ctx, topping); err != nil {
		t.Fatalf("Add topping: %v", err)
	}
	return pizza, topping
}

// toppingNames returns the names of the toppings of pizza.
func toppingNames(t *testing.T, db *orm.DB, pizza *Pizza) []string {
	t.Helper()
	rows, err := orm.Many[Topping](db, pizza, "toppings").QuerySet().OrderBy("name").All(context.Background())
	if err != nil {
		t.Fatalf("toppings: %v", err)
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return names
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_manytomany
func TestReadonlyManyToMany(t *testing.T) {
	djangotest.EachDB(t, pizzaModels(), func(t *testing.T, db *orm.DB) {
		env := pizzaEnv(t, db, true)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_pizza_add"))
		djangotest.AssertStatus(t, resp, 200)
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_manytomany_backwards_ref
func TestReadonlyManyToManyBackwardsRef(t *testing.T) {
	djangotest.EachDB(t, pizzaModels(), func(t *testing.T, db *orm.DB) {
		env := pizzaEnv(t, db, true)
		pizzaWithTopping(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_topping_add"))
		djangotest.AssertStatus(t, resp, 200)
	})
}

// Ported from Django: tests/admin_views/tests.py::ReadonlyTest.test_readonly_manytomany_forwards_ref
func TestReadonlyManyToManyForwardsRef(t *testing.T) {
	djangotest.EachDB(t, pizzaModels(), func(t *testing.T, db *orm.DB) {
		env := pizzaEnv(t, db, true)
		pizza, _ := pizzaWithTopping(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_pizza_change", pizza.ID))
		djangotest.AssertContains(t, resp, "<label>Toppings:</label>", djangotest.HTML())
		djangotest.AssertContains(t, resp, `<div class="readonly">Salami</div>`, djangotest.HTML())
	})
}

// Django posts to the change view of ReadablePizza, a proxy of Pizza whose
// admin leaves toppings editable; the Go test registers Pizza without readonly
// fields instead.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewBasicTest.test_change_view_logs_m2m_field_changes
func TestChangeViewLogsManyToManyFieldChanges(t *testing.T) {
	djangotest.EachDB(t, pizzaModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := pizzaEnv(t, db, false)
		pizza := &Pizza{Name: "Cheese"}
		if err := orm.Query[Pizza](db).Create(ctx, pizza); err != nil {
			t.Fatalf("Create pizza: %v", err)
		}
		cheese := &Topping{Name: "cheese"}
		if err := orm.Query[Topping](db).Create(ctx, cheese); err != nil {
			t.Fatalf("Create topping: %v", err)
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_pizza_change", pizza.ID), url.Values{
			"name":     {pizza.Name},
			"toppings": {strconv.FormatInt(cheese.ID, 10)},
		})
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:admin_views_pizza_changelist"))
		djangotest.AssertEqual(t, toppingNames(t, db, pizza), []string{"cheese"})
		// FIXME-PORT: the change message "Changed Toppings." of the LogEntry
		// needs the admin log (django_admin_log), which the Go admin lacks.
	})
}

// No Django counterpart: the add view saves the many-to-many field with the
// new row, in one transaction.
func TestAddViewSavesManyToManyField(t *testing.T) {
	djangotest.EachDB(t, pizzaModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := pizzaEnv(t, db, false)
		pks := make([]string, 0, 3)
		for _, name := range []string{"ham", "olive", "onion"} {
			tp := &Topping{Name: name}
			if err := orm.Query[Topping](db).Create(ctx, tp); err != nil {
				t.Fatalf("Create topping: %v", err)
			}
			pks = append(pks, strconv.FormatInt(tp.ID, 10))
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_pizza_add"), url.Values{
			"name":     {"Capricciosa"},
			"toppings": {pks[0], pks[2]},
		})
		djangotest.AssertRedirects(t, resp, mustReverse(t, env.router, "admin:admin_views_pizza_changelist"))
		pizza, err := orm.Query[Pizza](db).Get(ctx, "name", "Capricciosa")
		if err != nil {
			t.Fatalf("Get pizza: %v", err)
		}
		djangotest.AssertEqual(t, toppingNames(t, db, &pizza), []string{"ham", "onion"})

		// The change form selects the saved toppings.
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_pizza_change", pizza.ID))
		djangotest.AssertContains(t, resp, `<option value="`+pks[0]+`" selected>ham</option>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<option value="`+pks[1]+`">olive</option>`, djangotest.HTML())
	})
}
