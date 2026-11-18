package orm_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's select_related test app: a taxonomy tree where every
// level has a forward foreign key to the level above. Topping, Pizza,
// TaggedItem and Bookmark need M2M and generic relations and are left out.

type selDomain struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

// Meta names the table as Django does.
func (selDomain) Meta() orm.Meta { return orm.Meta{Table: "select_related_domain"} }

type selKingdom struct {
	ID     int64
	Name   string `orm:"max_length=50"`
	Domain orm.FK[selDomain]
}

// Meta names the table as Django does.
func (selKingdom) Meta() orm.Meta { return orm.Meta{Table: "select_related_kingdom"} }

type selPhylum struct {
	ID      int64
	Name    string `orm:"max_length=50"`
	Kingdom orm.FK[selKingdom]
}

// Meta names the table as Django does.
func (selPhylum) Meta() orm.Meta { return orm.Meta{Table: "select_related_phylum"} }

type selKlass struct {
	ID     int64
	Name   string `orm:"max_length=50"`
	Phylum orm.FK[selPhylum]
}

// Meta names the table as Django does.
func (selKlass) Meta() orm.Meta { return orm.Meta{Table: "select_related_klass"} }

type selOrder struct {
	ID    int64
	Name  string `orm:"max_length=50"`
	Klass orm.FK[selKlass]
}

// Meta names the table as Django does.
func (selOrder) Meta() orm.Meta { return orm.Meta{Table: "select_related_order"} }

type selFamily struct {
	ID    int64
	Name  string `orm:"max_length=50"`
	Order orm.FK[selOrder]
}

// Meta names the table as Django does.
func (selFamily) Meta() orm.Meta { return orm.Meta{Table: "select_related_family"} }

type selGenus struct {
	ID     int64
	Name   string `orm:"max_length=50"`
	Family orm.FK[selFamily]
}

// Meta names the table as Django does.
func (selGenus) Meta() orm.Meta { return orm.Meta{Table: "select_related_genus"} }

type selSpecies struct {
	ID    int64
	Name  string `orm:"max_length=50"`
	Genus orm.FK[selGenus]
}

// Meta names the table as Django does.
func (selSpecies) Meta() orm.Meta { return orm.Meta{Table: "select_related_species"} }

// selHybridSpecies is the model with two foreign keys to the same model.
type selHybridSpecies struct {
	ID       int64
	Name     string             `orm:"max_length=50"`
	Parent_1 orm.FK[selSpecies] `orm:"column=parent_1_id"` //nolint:revive,staticcheck // the name must snake-case to Django's parent_1
	Parent_2 orm.FK[selSpecies] `orm:"column=parent_2_id"` //nolint:revive,staticcheck // the name must snake-case to Django's parent_2
}

// Meta names the table as Django does.
func (selHybridSpecies) Meta() orm.Meta { return orm.Meta{Table: "select_related_hybridspecies"} }

// selModels returns the models of Django's select_related test app.
func selModels() []any {
	return []any{djangotest.App("select_related",
		&selDomain{}, &selKingdom{}, &selPhylum{}, &selKlass{}, &selOrder{},
		&selFamily{}, &selGenus{}, &selSpecies{}, &selHybridSpecies{})}
}

// selLevel gets the row of a tree level by name or starts a new one, lets set
// fill the name and the parent, and saves it, as one step of Django's
// create_tree.
func selLevel[T any](t *testing.T, db *orm.DB, name string, set func(*T)) *T {
	t.Helper()
	ctx := context.Background()
	obj, err := orm.Query[T](db).Filter("name", name).First(ctx)
	switch {
	case err == nil:
		set(&obj)
		if err := orm.Query[T](db).Save(ctx, &obj); err != nil {
			t.Fatalf("Save %q: %v", name, err)
		}
	case errors.Is(err, orm.ErrDoesNotExist):
		obj = *new(T)
		set(&obj)
		if err := orm.Query[T](db).Create(ctx, &obj); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	default:
		t.Fatalf("First %q: %v", name, err)
	}
	return &obj
}

// selCreateTree is Django's create_tree: eight names, one per level, from the
// domain down to the species. Levels that exist already are reused.
func selCreateTree(t *testing.T, db *orm.DB, names ...string) {
	t.Helper()
	if len(names) != 8 {
		t.Fatalf("create_tree needs 8 names, got %d", len(names))
	}
	domain := selLevel(t, db, names[0], func(o *selDomain) { o.Name = names[0] })
	kingdom := selLevel(t, db, names[1], func(o *selKingdom) {
		o.Name, o.Domain = names[1], fkTo[selDomain](domain.ID)
	})
	phylum := selLevel(t, db, names[2], func(o *selPhylum) {
		o.Name, o.Kingdom = names[2], fkTo[selKingdom](kingdom.ID)
	})
	klass := selLevel(t, db, names[3], func(o *selKlass) {
		o.Name, o.Phylum = names[3], fkTo[selPhylum](phylum.ID)
	})
	order := selLevel(t, db, names[4], func(o *selOrder) {
		o.Name, o.Klass = names[4], fkTo[selKlass](klass.ID)
	})
	family := selLevel(t, db, names[5], func(o *selFamily) {
		o.Name, o.Order = names[5], fkTo[selOrder](order.ID)
	})
	genus := selLevel(t, db, names[6], func(o *selGenus) {
		o.Name, o.Family = names[6], fkTo[selFamily](family.ID)
	})
	selLevel(t, db, names[7], func(o *selSpecies) {
		o.Name, o.Genus = names[7], fkTo[selGenus](genus.ID)
	})
}

// selSetUp is setUpTestData of SelectRelatedTests: four trees that share
// their domain and, for two of them, their kingdom.
func selSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	selCreateTree(t, db, "Eukaryota", "Animalia", "Anthropoda", "Insecta", "Diptera",
		"Drosophilidae", "Drosophila", "melanogaster")
	selCreateTree(t, db, "Eukaryota", "Animalia", "Chordata", "Mammalia", "Primates",
		"Hominidae", "Homo", "sapiens")
	selCreateTree(t, db, "Eukaryota", "Plantae", "Magnoliophyta", "Magnoliopsida", "Fabales",
		"Fabaceae", "Pisum", "sativum")
	selCreateTree(t, db, "Eukaryota", "Fungi", "Basidiomycota", "Homobasidiomycatae", "Agaricales",
		"Amanitacae", "Amanita", "muscaria")
}

// selFetch runs Fetch of a foreign key and fails the test on an error.
func selFetch[T any](t *testing.T, db *orm.DB, fk *orm.FK[T]) *T {
	t.Helper()
	obj, err := fk.Fetch(context.Background(), db)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	return obj
}

// selFamilyOf follows species.genus.family, as Django's attribute access does.
func selFamilyOf(t *testing.T, db *orm.DB, s *selSpecies) *selFamily {
	t.Helper()
	return selFetch(t, db, &selFetch(t, db, &s.Genus).Family)
}

// selOrderOf follows species.genus.family.order.
func selOrderOf(t *testing.T, db *orm.DB, s *selSpecies) *selOrder {
	t.Helper()
	return selFetch(t, db, &selFamilyOf(t, db, s).Order)
}

// selSortedNames sorts names, as Django's sorted().
func selSortedNames(names []string) []string {
	slices.Sort(names)
	return names
}

// Django's attribute access is Fetch: each step that is not loaded runs a
// query.
//
// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_access_fks_without_select_related
func TestSelectRelatedAccessFksWithoutSelectRelated(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 8, func() {
			fly, err := orm.Query[selSpecies](db).Get(context.Background(), "name", "melanogaster")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			family := selFamilyOf(t, db, &fly)
			klass := selFetch(t, db, &selFetch(t, db, &family.Order).Klass)
			kingdom := selFetch(t, db, &selFetch(t, db, &klass.Phylum).Kingdom)
			domain := selFetch(t, db, &kingdom.Domain)
			djangotest.AssertEqual(t, domain.Name, "Eukaryota")
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_access_fks_with_select_related
func TestSelectRelatedAccessFksWithSelectRelated(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			person, err := orm.Query[selSpecies](db).
				SelectRelated("genus__family__order__klass__phylum__kingdom__domain").
				Get(context.Background(), "name", "sapiens")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			genus := selFetch(t, db, &person.Genus)
			family := selFetch(t, db, &genus.Family)
			order := selFetch(t, db, &family.Order)
			klass := selFetch(t, db, &order.Klass)
			phylum := selFetch(t, db, &klass.Phylum)
			kingdom := selFetch(t, db, &phylum.Kingdom)
			domain := selFetch(t, db, &kingdom.Domain)
			djangotest.AssertEqual(t, domain.Name, "Eukaryota")
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_list_without_select_related
func TestSelectRelatedListWithoutSelectRelated(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 9, func() {
			world := mustAll(t, orm.Query[selSpecies](db))
			families := make([]string, 0, len(world))
			for i := range world {
				families = append(families, selFamilyOf(t, db, &world[i]).Name)
			}
			djangotest.AssertEqual(t, selSortedNames(families),
				[]string{"Amanitacae", "Drosophilidae", "Fabaceae", "Hominidae"})
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_list_with_select_related
func TestSelectRelatedListWithSelectRelated(t *testing.T) {
	t.Skip("needs select_related() without arguments, which the ORM does not have")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_select_related_no_arguments_deprecated
func TestSelectRelatedSelectRelatedNoArgumentsDeprecated(t *testing.T) {
	t.Skip("needs select_related() without arguments, which the ORM does not have")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_list_with_depth
func TestSelectRelatedListWithDepth(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 5, func() {
			world := mustAll(t, orm.Query[selSpecies](db).SelectRelated("genus__family"))
			orders := make([]string, 0, len(world))
			for i := range world {
				orders = append(orders, selOrderOf(t, db, &world[i]).Name)
			}
			djangotest.AssertEqual(t, selSortedNames(orders),
				[]string{"Agaricales", "Diptera", "Fabales", "Primates"})
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_select_related_with_extra
func TestSelectRelatedSelectRelatedWithExtra(t *testing.T) {
	t.Skip("needs extra(), which the ORM does not have")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_select_related_memory_leak
func TestSelectRelatedSelectRelatedMemoryLeak(t *testing.T) {
	t.Skip("checks Python garbage collection; Go has no counterpart")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_certain_fields
func TestSelectRelatedCertainFields(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			world := mustAll(t, orm.Query[selSpecies](db).SelectRelated("genus__family"))
			families := make([]string, 0, len(world))
			for i := range world {
				families = append(families, selFamilyOf(t, db, &world[i]).Name)
			}
			djangotest.AssertEqual(t, selSortedNames(families),
				[]string{"Amanitacae", "Drosophilidae", "Fabaceae", "Hominidae"})
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_more_certain_fields
func TestSelectRelatedMoreCertainFields(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 2, func() {
			world := mustAll(t, orm.Query[selSpecies](db).
				Filter("genus__name", "Amanita").SelectRelated("genus__family"))
			orders := make([]string, 0, len(world))
			for i := range world {
				orders = append(orders, selOrderOf(t, db, &world[i]).Name)
			}
			djangotest.AssertEqual(t, orders, []string{"Agaricales"})
		})
	})
}

// Django slices with [0:1]; Go uses Offset(0).Limit(1).
//
// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_field_traversal
func TestSelectRelatedFieldTraversal(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			s, err := orm.Query[selSpecies](db).SelectRelated("genus__family__order").
				OrderBy("id").Offset(0).Limit(1).Get(context.Background())
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			djangotest.AssertEqual(t, selOrderOf(t, db, &s).Name, "Diptera")
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_none_clears_list
func TestSelectRelatedNoneClearsList(t *testing.T) {
	t.Skip("needs select_related(None), which the ORM does not have")
}

// Django unpacks an unordered slice of two species; Go orders by id to make
// the pair deterministic. The loaded parents are compared by primary key and
// name.
//
// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_chaining
func TestSelectRelatedChaining(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selSetUp(t, db)
		ctx := context.Background()
		parents := mustAll(t, orm.Query[selSpecies](db).OrderBy("id").Limit(2))
		if len(parents) != 2 {
			t.Fatalf("got %d species, want 2", len(parents))
		}
		parent1, parent2 := parents[0], parents[1]
		create(t, db, &selHybridSpecies{
			Name:     "hybrid",
			Parent_1: fkTo[selSpecies](parent1.ID),
			Parent_2: fkTo[selSpecies](parent2.ID),
		})
		queryset := orm.Query[selHybridSpecies](db).SelectRelated("parent_1").SelectRelated("parent_2")
		djangotest.AssertNumQueries(t, db, 1, func() {
			rows, err := queryset.Limit(1).All(ctx)
			if err != nil || len(rows) != 1 {
				t.Fatalf("All: %v, %d rows", err, len(rows))
			}
			obj := rows[0]
			for _, c := range []struct {
				fk   orm.FK[selSpecies]
				want selSpecies
			}{{obj.Parent_1, parent1}, {obj.Parent_2, parent2}} {
				got, ok := c.fk.Object()
				if !ok {
					t.Fatalf("parent of %q is not loaded", c.want.Name)
				}
				djangotest.AssertEqual(t, got.ID, c.want.ID)
				djangotest.AssertEqual(t, got.Name, c.want.Name)
			}
		})
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_reverse_relation_caching
func TestSelectRelatedReverseRelationCaching(t *testing.T) {
	t.Skip("needs the reverse relation species_set; the ORM has no reverse relations")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_select_related_after_values
func TestSelectRelatedSelectRelatedAfterValues(t *testing.T) {
	t.Skip("SelectRelated after Values does not compile in Go: ValuesQuerySet has no SelectRelated")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_select_related_after_values_list
func TestSelectRelatedSelectRelatedAfterValuesList(t *testing.T) {
	t.Skip("SelectRelated after Values does not compile in Go: ValuesListQuerySet has no SelectRelated")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_fetch_mode_copied_fetching_one
func TestSelectRelatedFetchModeCopiedFetchingOne(t *testing.T) {
	t.Skip("needs fetch modes")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_fetch_mode_copied_fetching_many
func TestSelectRelatedFetchModeCopiedFetchingMany(t *testing.T) {
	t.Skip("needs fetch modes")
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedTests.test_fetch_peers_for_select_related_objects
func TestSelectRelatedFetchPeersForSelectRelatedObjects(t *testing.T) {
	t.Skip("needs fetch modes")
}

// selAssertSelectRelatedError runs a queryset with a bad SelectRelated name
// and checks the FieldError.
func selAssertSelectRelatedError[T any](t *testing.T, db *orm.DB, name, want string) {
	t.Helper()
	_, err := orm.Query[T](db).SelectRelated(name).All(context.Background())
	assertFieldError(t, err, want)
}

// selNonRel builds the expected message for a field that is not a relation.
func selNonRel(name, choices string) string {
	return "on-relational field given in select_related: '" + name + "'. Choices are: " + choices
}

// selInv builds the expected message for a name that is no field.
func selInv(name, choices string) string {
	return "nvalid field name(s) given in select_related: '" + name + "'. Choices are: " + choices
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_non_relational_field
func TestSelectRelatedNonRelationalField(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selAssertSelectRelatedError[selSpecies](t, db, "name__some_field", selNonRel("name", "genus"))
		selAssertSelectRelatedError[selSpecies](t, db, "name", selNonRel("name", "genus"))
		selAssertSelectRelatedError[selDomain](t, db, "name", selNonRel("name", "(none)"))
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_non_relational_field_nested
func TestSelectRelatedNonRelationalFieldNested(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selAssertSelectRelatedError[selSpecies](t, db, "genus__name", selNonRel("name", "family"))
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_many_to_many_field
func TestSelectRelatedManyToManyField(t *testing.T) {
	t.Skip("needs ManyToManyField")
}

// Go has no related_name accessor, so child_1 is an unknown name; the message
// is the same as Django's.
//
// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_reverse_relational_field
func TestSelectRelatedReverseRelationalField(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selAssertSelectRelatedError[selSpecies](t, db, "child_1", selInv("child_1", "genus"))
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_invalid_field
func TestSelectRelatedInvalidField(t *testing.T) {
	djangotest.EachDB(t, selModels(), func(t *testing.T, db *orm.DB) {
		selAssertSelectRelatedError[selSpecies](t, db, "invalid_field", selInv("invalid_field", "genus"))
		selAssertSelectRelatedError[selSpecies](t, db, "genus__related_invalid_field",
			selInv("related_invalid_field", "family"))
		selAssertSelectRelatedError[selDomain](t, db, "invalid_field", selInv("invalid_field", "(none)"))
	})
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_generic_relations
func TestSelectRelatedGenericRelations(t *testing.T) {
	t.Skip("needs generic relations")
}
