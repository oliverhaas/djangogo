package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/queries/models.py for multi-valued and
// reverse relation join tests.

type mvSimpleCategory struct {
	ID   int64
	Name string `orm:"max_length=25"`
}

type mvCategoryItem struct {
	ID       int64
	Category orm.FK[mvSimpleCategory] `orm:"on_delete=cascade;related_name=categoryitem"`
}

type mvOneToOneCategory struct {
	Category orm.O2O[mvSimpleCategory] `orm:"pk;on_delete=cascade;related_name=onetoonecategory"`
	NewName  string                    `orm:"max_length=15"`
}

type mvCategoryRelationship struct {
	ID     int64
	First  orm.FK[mvSimpleCategory] `orm:"on_delete=cascade;related_name=first_rel"`
	Second orm.FK[mvSimpleCategory] `orm:"on_delete=cascade;related_name=second_rel"`
}

type mvFood struct {
	ID   int64
	Name string `orm:"max_length=20;unique"`
}

type mvEaten struct {
	ID   int64
	Food orm.FK[mvFood] `orm:"null;on_delete=set_null;related_name=eaten"`
	Meal string         `orm:"max_length=20"`
}

type mvNode struct {
	ID     int64
	Num    int64          `orm:"unique"`
	Parent orm.FK[mvNode] `orm:"null;on_delete=set_null;related_name=node"`
}

type mvTag struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

type mvNote struct {
	ID   int64
	Note string        `orm:"max_length=100"`
	Tag  orm.FK[mvTag] `orm:"null;on_delete=set_null;related_name=note"`
}

type mvAnnotation struct {
	ID   int64
	Name string        `orm:"max_length=10"`
	Tag  orm.FK[mvTag] `orm:"on_delete=cascade;related_name=annotation"`
}

type mvObjectA struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type mvObjectB struct {
	ID      int64
	Name    string            `orm:"max_length=50"`
	Objecta orm.FK[mvObjectA] `orm:"on_delete=cascade;related_name=objectb"`
}

type mvOrder struct {
	ID int64 `orm:"pk"`
}

// Meta orders by ID as in Django's queries.Order.
func (mvOrder) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type mvOrderItem struct {
	ID     int64
	Order  orm.FK[mvOrder] `orm:"on_delete=cascade;related_name=items"`
	Status int64
}

type mvDetail struct {
	ID   int64
	Data string `orm:"max_length=50"`
}

type mvMember struct {
	ID     int64
	Name   string            `orm:"max_length=50"`
	Detail orm.O2O[mvDetail] `orm:"on_delete=cascade;related_name=member"`
}

type mvAuthor struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type mvReport struct {
	ID      int64
	Name    string           `orm:"max_length=50"`
	Creator orm.FK[mvAuthor] `orm:"null;on_delete=set_null;related_name=report"`
}

type mvSchool struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type mvStudent struct {
	ID     int64
	School orm.FK[mvSchool] `orm:"on_delete=cascade"`
}

type mvClassroom struct {
	ID            int64
	Name          string           `orm:"max_length=20"`
	HasBlackboard *bool            `orm:"null"`
	School        orm.FK[mvSchool] `orm:"on_delete=cascade;related_name=classroom"`
}

// mvModels returns the model list for the multi-valued query tests.
func mvModels() []any {
	return []any{djangotest.App("queries_mv",
		&mvSimpleCategory{}, &mvCategoryItem{}, &mvOneToOneCategory{}, &mvCategoryRelationship{},
		&mvFood{}, &mvEaten{}, &mvNode{}, &mvTag{}, &mvNote{}, &mvAnnotation{},
		&mvObjectA{}, &mvObjectB{}, &mvOrder{}, &mvOrderItem{},
		&mvDetail{}, &mvMember{}, &mvAuthor{}, &mvReport{},
		&mvSchool{}, &mvStudent{}, &mvClassroom{},
	)}
}

// mvSQL captures and returns the single SQL statement executed by qs.All.
func mvSQL[T any](t *testing.T, db *orm.DB, qs *orm.QuerySet[T]) string {
	t.Helper()
	capture := djangotest.CaptureQueries(db)
	_ = mustAll(t, qs)
	capture.Stop()
	if capture.Len() != 1 {
		t.Fatalf("expected 1 query, got %d", capture.Len())
	}
	return capture.Queries()[0].SQL
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_order_by_reverse_fk
func TestQueriesOrderByReverseFK(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c1 := create(t, db, &mvSimpleCategory{Name: "category1"})
		c2 := create(t, db, &mvSimpleCategory{Name: "category2"})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c2.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[mvSimpleCategory](db).OrderBy("categoryitem", "pk"),
			[]int64{c1.ID, c2.ID, c1.ID},
			func(c mvSimpleCategory) int64 { return c.ID },
		)
	})
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_ticket15316_one2one_filter_false
func TestQueriesTicket15316One2OneFilterFalse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c := create(t, db, &mvSimpleCategory{Name: "cat"})
		c0 := create(t, db, &mvSimpleCategory{Name: "cat0"})
		c1 := create(t, db, &mvSimpleCategory{Name: "category1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c1.ID), NewName: "new1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c0.ID), NewName: "new2"})

		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c.ID)})
		ci2 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c0.ID)})
		ci3 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})

		qs := orm.Query[mvCategoryItem](db).Filter("category__onetoonecategory__isnull", false).OrderBy("pk")
		djangotest.AssertEqual(t, mustCount(t, qs), int64(2))
		djangotest.AssertQuerySetEqual(t, qs, []int64{ci2.ID, ci3.ID}, func(x mvCategoryItem) int64 { return x.ID })
	})
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_ticket15316_one2one_exclude_false
func TestQueriesTicket15316One2OneExcludeFalse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c := create(t, db, &mvSimpleCategory{Name: "cat"})
		c0 := create(t, db, &mvSimpleCategory{Name: "cat0"})
		c1 := create(t, db, &mvSimpleCategory{Name: "category1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c1.ID), NewName: "new1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c0.ID), NewName: "new2"})

		ci1 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c0.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})

		qs := orm.Query[mvCategoryItem](db).Exclude("category__onetoonecategory__isnull", false)
		djangotest.AssertEqual(t, mustCount(t, qs), int64(1))
		djangotest.AssertQuerySetEqual(t, qs, []int64{ci1.ID}, func(x mvCategoryItem) int64 { return x.ID })
	})
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_ticket15316_one2one_filter_true
func TestQueriesTicket15316One2OneFilterTrue(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c := create(t, db, &mvSimpleCategory{Name: "cat"})
		c0 := create(t, db, &mvSimpleCategory{Name: "cat0"})
		c1 := create(t, db, &mvSimpleCategory{Name: "category1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c1.ID), NewName: "new1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c0.ID), NewName: "new2"})

		ci1 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c0.ID)})
		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})

		qs := orm.Query[mvCategoryItem](db).Filter("category__onetoonecategory__isnull", true)
		djangotest.AssertEqual(t, mustCount(t, qs), int64(1))
		djangotest.AssertQuerySetEqual(t, qs, []int64{ci1.ID}, func(x mvCategoryItem) int64 { return x.ID })
	})
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_ticket15316_one2one_exclude_true
func TestQueriesTicket15316One2OneExcludeTrue(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c := create(t, db, &mvSimpleCategory{Name: "cat"})
		c0 := create(t, db, &mvSimpleCategory{Name: "cat0"})
		c1 := create(t, db, &mvSimpleCategory{Name: "category1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c1.ID), NewName: "new1"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c0.ID), NewName: "new2"})

		create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c.ID)})
		ci2 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c0.ID)})
		ci3 := create(t, db, &mvCategoryItem{Category: fkTo[mvSimpleCategory](c1.ID)})

		qs := orm.Query[mvCategoryItem](db).Exclude("category__onetoonecategory__isnull", true).OrderBy("pk")
		djangotest.AssertEqual(t, mustCount(t, qs), int64(2))
		djangotest.AssertQuerySetEqual(t, qs, []int64{ci2.ID, ci3.ID}, func(x mvCategoryItem) int64 { return x.ID })
	})
}

// Ported from Django: tests/queries/tests.py::ToFieldTests.test_reverse_in
func TestQueriesReverseIn(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		apple := create(t, db, &mvFood{Name: "apple"})
		pear := create(t, db, &mvFood{Name: "pear"})
		lunchApple := create(t, db, &mvEaten{Food: fkTo[mvFood](apple.ID), Meal: "lunch"})
		lunchPear := create(t, db, &mvEaten{Food: fkTo[mvFood](pear.ID), Meal: "dinner"})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[mvFood](db).Filter("eaten__in", []any{lunchApple, lunchPear}),
			[]string{"apple", "pear"},
			func(f mvFood) string { return f.Name },
			djangotest.Unordered(),
		)
	})
}

// Ported from Django: tests/queries/tests.py::ToFieldTests.test_single_object_reverse
func TestQueriesSingleObjectReverse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		apple := create(t, db, &mvFood{Name: "apple"})
		lunch := create(t, db, &mvEaten{Food: fkTo[mvFood](apple.ID), Meal: "lunch"})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[mvFood](db).Filter("eaten", lunch),
			[]string{"apple"},
			func(f mvFood) string { return f.Name },
		)
	})
}

// Ported from Django: tests/queries/tests.py::ToFieldTests.test_recursive_fk_reverse
func TestQueriesRecursiveFKReverse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		node1 := create(t, db, &mvNode{Num: 42})
		node2 := create(t, db, &mvNode{Num: 1, Parent: fkTo[mvNode](node1.ID)})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[mvNode](db).Filter("node", node2),
			[]int64{node1.ID},
			func(n mvNode) int64 { return n.ID },
		)
	})
}

// Ported from Django: tests/queries/tests.py::ExcludeTests.test_exclude_reverse_fk_field_ref
func TestQueriesExcludeReverseFKFieldRef(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		tag := create(t, db, &mvTag{Name: "t"})
		create(t, db, &mvNote{Tag: fkTo[mvTag](tag.ID), Note: "note"})
		ann := create(t, db, &mvAnnotation{Name: "annotation", Tag: fkTo[mvTag](tag.ID)})

		got, err := orm.Query[mvAnnotation](db).Exclude("tag__note__note", orm.F("name")).Get(context.Background())
		if err != nil {
			t.Fatalf("Exclude: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, ann.ID)
	})
}

// Ported from Django: tests/queries/tests.py::ExcludeTests.test_exclude_with_circular_fk_relation
func TestQueriesExcludeWithCircularFKRelation(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		oa := create(t, db, &mvObjectA{Name: "a"})
		create(t, db, &mvObjectB{Name: "b", Objecta: fkTo[mvObjectA](oa.ID)})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[mvObjectB](db).Exclude("objecta__objectb__name", orm.F("name"))), int64(0))
	})
}

// Ported from Django: tests/queries/tests.py::ExcludeTests.test_exclude_multivalued_exists
func TestQueriesExcludeMultivaluedExists(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		a1 := create(t, db, &mvAuthor{Name: "a1"})
		a2 := create(t, db, &mvAuthor{Name: "a2"})
		create(t, db, &mvReport{Name: "Programming", Creator: fkTo[mvAuthor](a2.ID)})

		capture := djangotest.CaptureQueries(db)
		rows := mustAll(t, orm.Query[mvAuthor](db).Exclude("report__name", "Programming"))
		capture.Stop()

		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, rows[0].ID, a1.ID)
		if !strings.Contains(strings.ToLower(capture.Queries()[0].SQL), "exists") {
			t.Fatalf("expected EXISTS subquery in SQL, got: %s", capture.Queries()[0].SQL)
		}
	})
}

// mvOrderSetUp creates the ExcludeTest17600 fixture.
func mvOrderSetUp(t *testing.T, db *orm.DB) (o1, o2, o3 *mvOrder) {
	t.Helper()
	o1 = create(t, db, &mvOrder{ID: 1})
	o2 = create(t, db, &mvOrder{ID: 2})
	o3 = create(t, db, &mvOrder{ID: 3})
	for _, st := range []int64{1, 1, 1} {
		create(t, db, &mvOrderItem{Order: fkTo[mvOrder](o1.ID), Status: st})
	}
	for _, st := range []int64{1, 2, 3} {
		create(t, db, &mvOrderItem{Order: fkTo[mvOrder](o2.ID), Status: st})
	}
	for _, st := range []int64{2, 3, 4} {
		create(t, db, &mvOrderItem{Order: fkTo[mvOrder](o3.ID), Status: st})
	}
	return o1, o2, o3
}

// Ported from Django: tests/queries/tests.py::ExcludeTest17600.test_exclude_plain
// Ported from Django: tests/queries/tests.py::ExcludeTest17600.test_exclude_plain_distinct
// Ported from Django: tests/queries/tests.py::ExcludeTest17600.test_exclude_with_q_object_distinct
// Ported from Django: tests/queries/tests.py::ExcludeTest17600.test_exclude_with_q_object_no_distinct
func TestQueriesExclude17600(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		_, _, o3 := mvOrderSetUp(t, db)
		oid := func(o mvOrder) int64 { return o.ID }
		want := []int64{o3.ID}

		djangotest.AssertQuerySetEqual(t, orm.Query[mvOrder](db).Exclude("items__status", 1), want, oid)
		djangotest.AssertQuerySetEqual(t, orm.Query[mvOrder](db).Exclude("items__status", 1).Distinct(), want, oid)
		djangotest.AssertQuerySetEqual(t, orm.Query[mvOrder](db).Exclude(orm.Q("items__status", 1)).Distinct(), want, oid)
		djangotest.AssertQuerySetEqual(t, orm.Query[mvOrder](db).Exclude(orm.Q("items__status", 1)), want, oid)
	})
}

// Ported from Django: tests/queries/tests.py::Exclude15786.test_ticket15786
func TestQueriesExclude15786(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		c1 := create(t, db, &mvSimpleCategory{Name: "c1"})
		c2 := create(t, db, &mvSimpleCategory{Name: "c2"})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c1.ID)})
		create(t, db, &mvOneToOneCategory{Category: o2oTo[mvSimpleCategory](c2.ID)})
		rel := create(t, db, &mvCategoryRelationship{
			First:  fkTo[mvSimpleCategory](c1.ID),
			Second: fkTo[mvSimpleCategory](c2.ID),
		})
		got, err := orm.Query[mvCategoryRelationship](db).Exclude(
			"first__onetoonecategory", orm.F("second__onetoonecategory"),
		).Get(context.Background())
		if err != nil {
			t.Fatalf("Exclude: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, rel.ID)
	})
}

// Ported from Django: tests/queries/tests.py::ReverseJoinTrimmingTest.test_reverse_trimming
func TestQueriesReverseTrimming(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		tag := create(t, db, &mvTag{Name: "t"})
		qs := orm.Query[mvTag](db).Filter("annotation__tag", tag.ID)
		capture := djangotest.CaptureQueries(db)
		rows := mustAll(t, qs)
		capture.Stop()
		if !strings.Contains(capture.Queries()[0].SQL, "INNER JOIN") {
			t.Fatalf("expected INNER JOIN, got: %s", capture.Queries()[0].SQL)
		}
		djangotest.AssertEqual(t, len(rows), 0)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_revo2o_reuse
func TestQueriesRevO2OReuse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[mvDetail](db).Filter("member__name", "foo").Filter("member__name", "foo")
		sql := mvSQL(t, db, qs)
		djangotest.AssertEqual(t, strings.Count(sql, "JOIN"), 1)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_revfk_noreuse
func TestQueriesRevFKNoReuse(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		// Chained Filter calls across a reverse FK create separate joins.
		qs := orm.Query[mvAuthor](db).Filter("report__name", "r4").Filter("report__name", "r1")
		sql := mvSQL(t, db, qs)
		djangotest.AssertEqual(t, strings.Count(sql, "JOIN"), 2)

		// Conditions in a single Filter call reuse the reverse FK join.
		qsSame := orm.Query[mvAuthor](db).Filter("report__name", "r4", "report__id__gt", 0)
		sqlSame := mvSQL(t, db, qsSame)
		djangotest.AssertEqual(t, strings.Count(sqlSame, "JOIN"), 1)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_inverted_q_across_relations
func TestQueriesInvertedQAcrossRelations(t *testing.T) {
	djangotest.EachDB(t, mvModels(), func(t *testing.T, db *orm.DB) {
		springfield := create(t, db, &mvSchool{Name: "Springfield"})
		hogwarts := create(t, db, &mvSchool{Name: "Hogwarts"})
		create(t, db, &mvStudent{School: fkTo[mvSchool](springfield.ID)})
		hp := create(t, db, &mvStudent{School: fkTo[mvSchool](hogwarts.ID)})
		create(t, db, &mvClassroom{School: fkTo[mvSchool](hogwarts.ID), Name: "Potion"})
		create(t, db, &mvClassroom{School: fkTo[mvSchool](springfield.ID), Name: "Main"})

		qs := orm.Query[mvStudent](db).Filter(orm.Not(
			orm.Q("school__classroom__name", "Main").And(orm.Q("school__classroom__has_blackboard", nil)),
		))
		djangotest.AssertQuerySetEqual(t, qs, []int64{hp.ID}, func(s mvStudent) int64 { return s.ID })
	})
}
