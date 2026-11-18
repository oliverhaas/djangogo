package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/queries/models.py behind the tests on join
// types. Node.parent points at the primary key; Django points it at Node.num
// with to_field, which the ORM does not have.

type qFK1 struct {
	ID int64
	F1 string `orm:"type=text"`
	F2 string `orm:"type=text"`
}

type qFK2 struct {
	ID int64
	F1 string `orm:"type=text"`
	F2 string `orm:"type=text"`
}

type qFK3 struct {
	ID int64
	F1 string `orm:"type=text"`
	F2 string `orm:"type=text"`
}

type qBaseA struct {
	ID int64
	A  orm.FK[qFK1] `orm:"null;on_delete=set_null"`
	B  orm.FK[qFK2] `orm:"null;on_delete=set_null"`
	C  orm.FK[qFK3] `orm:"null;on_delete=set_null"`
}

type qModelD struct {
	ID   int64
	Name string `orm:"type=text"`
}

type qModelC struct {
	ID   int64
	Name string `orm:"type=text"`
}

type qModelB struct {
	ID   int64
	Name string `orm:"type=text"`
	C    orm.FK[qModelC]
}

type qModelA struct {
	ID   int64
	Name string          `orm:"type=text"`
	B    orm.FK[qModelB] `orm:"null;on_delete=set_null"`
	D    orm.FK[qModelD]
}

type qNode struct {
	ID     int64
	Num    int64         `orm:"unique"`
	Parent orm.FK[qNode] `orm:"null;on_delete=set_null"`
}

type qObjectA struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type qObjectB struct {
	ID      int64
	Name    string `orm:"max_length=50"`
	Objecta orm.FK[qObjectA]
	Num     int64
}

type qObjectC struct {
	ID      int64
	Name    string           `orm:"max_length=50"`
	Objecta orm.FK[qObjectA] `orm:"null;on_delete=set_null"`
	Objectb orm.FK[qObjectB] `orm:"null;on_delete=set_null"`
}

// qBaseAs returns the BaseA queryset, the start of every join promotion test.
func qBaseAs(db *orm.DB) *orm.QuerySet[qBaseA] { return orm.Query[qBaseA](db) }

// qValuesSQL runs a Values queryset and returns the statement it ran.
func qValuesSQL(t *testing.T, db *orm.DB, vs *orm.ValuesQuerySet) string {
	t.Helper()
	capture := djangotest.CaptureQueries(db)
	_, err := vs.All(context.Background())
	capture.Stop()
	if err != nil || capture.Len() != 1 {
		t.Fatalf("Values: %d queries, error %v", capture.Len(), err)
	}
	return capture.Queries()[0].SQL
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion_select_related
func TestQueriesDisjunctionPromotionSelectRelated(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		fk1 := create(t, db, &qFK1{F1: "f1", F2: "f2"})
		basea := create(t, db, &qBaseA{A: fkTo[qFK1](fk1.ID)})
		qs := qBaseAs(db).Filter(orm.Q("a", fk1).Or(orm.Q("b", 2)))
		qAssertJoins(t, qs, 0, 0)
		qs = qs.SelectRelated("a", "b")
		qAssertJoins(t, qs, 0, 2)
		djangotest.AssertNumQueries(t, db, 1, func() {
			rows := mustAll(t, qs)
			djangotest.AssertEqual(t, len(rows), 1)
			djangotest.AssertEqual(t, rows[0].ID, basea.ID)
			a, _ := rows[0].A.Object()
			djangotest.AssertEqual(t, *a, *fk1)
			b, _ := rows[0].B.Object()
			djangotest.AssertEqual(t, b, nil)
		})
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion1
func TestQueriesDisjunctionPromotion1(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		// A join that exists, then two ORed filters on one other join: every
		// join can be INNER.
		bEither := orm.Q("b__f1", "foo").Or(orm.Q("b__f2", "foo"))
		qs := qBaseAs(db).Filter("a__f1", "foo")
		qAssertJoins(t, qs, 1, 0)
		qAssertJoins(t, qs.Filter(bEither), 2, 0)
		// The same filters in the other order.
		qs = qBaseAs(db).Filter(bEither)
		qAssertJoins(t, qs, 1, 0)
		qAssertJoins(t, qs.Filter("a__f1", "foo"), 2, 0)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion2
func TestQueriesDisjunctionPromotion2(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		// Two different joins in an ORed condition must be OUTER; the join that
		// was there before stays INNER.
		bOrC := orm.Q("b__f1", "foo").Or(orm.Q("c__f2", "foo"))
		qs := qBaseAs(db).Filter("a__f1", "foo")
		qAssertJoins(t, qs, 1, 0)
		qAssertJoins(t, qs.Filter(bOrC), 1, 2)
		// The same filters in the other order.
		qs = qBaseAs(db).Filter(bOrC)
		qAssertJoins(t, qs, 0, 2)
		qAssertJoins(t, qs.Filter("a__f1", "foo"), 1, 2)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion3
func TestQueriesDisjunctionPromotion3(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := qBaseAs(db).Filter("a__f2", "bar")
		qAssertJoins(t, qs, 1, 0)
		// The ANDed filter on a__f2 cannot be true without a row of a, so the
		// join to a stays INNER inside the ORed filter as well.
		qAssertJoins(t, qs.Filter(orm.Q("a__f1", "foo").Or(orm.Q("b__f2", "foo"))), 1, 1)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion3_demote
func TestQueriesDisjunctionPromotion3Demote(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		// The first filter joins a as OUTER, the second makes it INNER again.
		qs := qBaseAs(db).Filter(orm.Q("a__f1", "foo").Or(orm.Q("b__f2", "foo"))).Filter("a__f2", "bar")
		qAssertJoins(t, qs, 1, 1)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion4_demote
func TestQueriesDisjunctionPromotion4Demote(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := qBaseAs(db).Filter(orm.Q("a", 1).Or(orm.Q("a", 2)))
		qAssertJoins(t, qs, 0, 0)
		// The join to a was settled by the filter above, although it was trimmed
		// away; the filter below must find it as INNER.
		qAssertJoins(t, qs.Filter("a__f1", "foo"), 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion4
func TestQueriesDisjunctionPromotion4(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := qBaseAs(db).Filter("a__f1", "foo")
		qAssertJoins(t, qs, 1, 0)
		qAssertJoins(t, qs.Filter(orm.Q("a", 1).Or(orm.Q("a", 2))), 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion5_demote
func TestQueriesDisjunctionPromotion5Demote(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		aEither := orm.Q("a", 1).Or(orm.Q("a", 2))
		aOrB := orm.Q("a__f1", "foo").Or(orm.Q("b__f1", "foo"))
		// The filters on a make its join INNER even though it is trimmed, so
		// a__f1 needs no promotion afterwards, but b__f1 does.
		qs := qBaseAs(db).Filter(aEither)
		qAssertJoins(t, qs, 0, 0)
		qAssertJoins(t, qs.Filter(aOrB), 1, 1)
		// In the other order the join to a starts as OUTER and is demoted.
		qs = qBaseAs(db).Filter(aOrB)
		qAssertJoins(t, qs, 0, 2)
		qAssertJoins(t, qs.Filter(aEither), 1, 1)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion6
func TestQueriesDisjunctionPromotion6(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		aEither := orm.Q("a", 1).Or(orm.Q("a", 2))
		qAssertJoins(t, qBaseAs(db).Filter(aEither), 0, 0)
		qs := qBaseAs(db).Filter(orm.Q("a__f1", "foo").And(orm.Q("b__f1", "foo")))
		qAssertJoins(t, qs, 2, 0)
		qAssertJoins(t, qs.Filter(aEither), 2, 0)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion7
func TestQueriesDisjunctionPromotion7(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		q := orm.Q
		qAssertJoins(t, qBaseAs(db).Filter(q("a", 1).Or(q("a", 2))), 0, 0)
		qAssertJoins(t, qBaseAs(db).Filter(
			q("a__f1", "foo").Or(q("b__f1", "foo").And(q("a__f1", "bar")))), 1, 1)
		qAssertJoins(t, qBaseAs(db).Filter(
			q("a__f1", "foo").Or(q("b__f1", "foo")).And(q("a__f1", "bar").Or(q("c__f1", "foo")))), 0, 3)
		qAssertJoins(t, qBaseAs(db).Filter(
			q("a__f1", "foo").Or(q("a__f1", "bar").And(q("b__f1", "bar").Or(q("c__f1", "foo"))))), 1, 2)
	})
}

// Ported from Django: tests/queries/tests.py::DisjunctionPromotionTests.test_disjunction_promotion_fexpression
func TestQueriesDisjunctionPromotionFExpression(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		q := orm.Q
		// A join that only an F expression reads takes part in the votes too.
		qAssertJoins(t, qBaseAs(db).Filter(q("a__f1", orm.F("b__f1")).Or(q("b__f1", "foo"))), 1, 1)
		qAssertJoins(t, qBaseAs(db).Filter(q("a__f1", orm.F("c__f1")).Or(q("b__f1", "foo"))), 0, 3)
		qAssertJoins(t, qBaseAs(db).Filter(
			q("a__f1", orm.F("b__f1")).Or(q("a__f2", orm.F("b__f2"))).Or(q("c__f1", "foo"))), 0, 3)
		qAssertJoins(t, qBaseAs(db).Filter(q("a__f1", orm.F("c__f1")).Or(q("pk", 1).And(q("pk", 2)))), 0, 2)
	})
}

// qSetUpNullJoin creates the rows of NullJoinPromotionOrTest.setUpTestData.
func qSetUpNullJoin(t *testing.T, db *orm.DB) {
	t.Helper()
	d1 := create(t, db, &qModelD{Name: "foo"})
	d2 := create(t, db, &qModelD{Name: "bar"})
	create(t, db, &qModelA{Name: "a1", D: fkTo[qModelD](d1.ID)})
	c := create(t, db, &qModelC{Name: "c"})
	b := create(t, db, &qModelB{Name: "b", C: fkTo[qModelC](c.ID)})
	create(t, db, &qModelA{Name: "a2", B: fkTo[qModelB](b.ID), D: fkTo[qModelD](d2.ID)})
}

// Ported from Django: tests/queries/tests.py::NullJoinPromotionOrTest.test_isnull_filter_promotion
func TestQueriesIsNullFilterPromotion(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUpNullJoin(t, db)
		name := func(a qModelA) string { return a.Name }
		isNull, notNull := orm.Q("b__name__isnull", true), orm.Q("b__name__isnull", false)
		// A test for NULL has to keep the rows without a b, so it needs an OUTER
		// join; a test for NOT NULL can use an INNER one. A negation swaps them.
		for _, tc := range []struct {
			cond         orm.Cond
			inner, outer int
			want         string
		}{
			{isNull, 0, 1, "a1"},
			{orm.Not(isNull), 1, 0, "a2"},
			{orm.Not(orm.Not(isNull)), 0, 1, "a1"},
			{notNull, 1, 0, "a2"},
			{orm.Not(notNull), 0, 1, "a1"},
			{orm.Not(orm.Not(notNull)), 1, 0, "a2"},
		} {
			qs := orm.Query[qModelA](db).Filter(tc.cond)
			qAssertJoins(t, qs, tc.inner, tc.outer)
			djangotest.AssertQuerySetEqual(t, qs, []string{tc.want}, name)
		}
	})
}

// Ported from Django: tests/queries/tests.py::NullJoinPromotionOrTest.test_null_join_demotion
func TestQueriesNullJoinDemotion(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		as := orm.Query[qModelA](db)
		isNull, notNull := orm.Q("b__name__isnull", true), orm.Q("b__name__isnull", false)
		qAssertJoins(t, as.Filter(notNull.And(isNull)), 1, 0)
		qAssertJoins(t, as.Filter(isNull.And(notNull)), 1, 0)
		qAssertJoins(t, as.Filter(notNull.Or(isNull)), 0, 1)
		qAssertJoins(t, as.Filter(isNull.Or(notNull)), 0, 1)
	})
}

// Ported from Django: tests/queries/tests.py::ValuesJoinPromotionTests.test_values_no_promotion_for_existing
func TestQueriesValuesNoPromotionForExisting(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qNode](db).Filter("parent__parent__isnull", false)
		qAssertJoins(t, qs, 1, 0)
		if sql := qValuesSQL(t, db, qs.Values("parent__parent__id")); !strings.Contains(sql, " INNER JOIN ") {
			t.Errorf("Values promoted the join of the filter: %s", sql)
		}
		// Without the filter the join is LEFT OUTER.
		sql := qValuesSQL(t, db, orm.Query[qNode](db).Values("parent__parent__id"))
		if !strings.Contains(sql, " LEFT OUTER JOIN ") {
			t.Errorf("Values through a nullable relation must join LEFT OUTER: %s", sql)
		}
	})
}

// Ported from Django: tests/queries/tests.py::ValuesJoinPromotionTests.test_non_nullable_fk_not_promoted
func TestQueriesNonNullableFKNotPromoted(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		sql := qValuesSQL(t, db, orm.Query[qObjectB](db).Values("objecta__name"))
		if !strings.Contains(sql, " INNER JOIN ") {
			t.Errorf("Values through a non-null relation must join INNER: %s", sql)
		}
	})
}

// Ported from Django: tests/queries/tests.py::ValuesJoinPromotionTests.test_ticket_21376
func TestQueriesTicket21376(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		a := create(t, db, &qObjectA{})
		create(t, db, &qObjectC{Objecta: fkTo[qObjectA](a.ID)})
		qs := orm.Query[qObjectC](db).
			Filter(orm.Q("objecta", a).Or(orm.Q("objectb__objecta", a))).
			Filter(orm.Q("objectb", 1).Or(orm.Q("objecta", a)))
		djangotest.AssertEqual(t, mustCount(t, qs), 1)
		// The only table the statement reads is ObjectB, and the row without an
		// ObjectB must survive the join.
		qAssertJoins(t, qs, 0, 1)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_fk_reuse
func TestQueriesFKReuse(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qAnnotation](db).Filter("tag__name", "foo").Filter("tag__name", "bar")
		qAssertJoins(t, qs, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_fk_reuse_select_related
func TestQueriesFKReuseSelectRelated(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qAnnotation](db).Filter("tag__name", "foo").SelectRelated("tag")
		qAssertJoins(t, qs, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_fk_reuse_annotation
func TestQueriesFKReuseAnnotation(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qAnnotation](db).Filter("tag__name", "foo").Annotate("cnt", orm.Count("tag__name"))
		qAssertJoins(t, qs, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_fk_reuse_disjunction
func TestQueriesFKReuseDisjunction(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qAnnotation](db).Filter(orm.Q("tag__name", "foo").Or(orm.Q("tag__name", "bar")))
		qAssertJoins(t, qs, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::JoinReuseTest.test_fk_reuse_order_by
func TestQueriesFKReuseOrderBy(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qAnnotation](db).Filter("tag__name", "foo").OrderBy("tag__name")
		qAssertJoins(t, qs, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries6Tests.test_ticket_11320
func TestQueriesTicket11320(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qTag](db).Exclude("category", nil).Exclude("category__name", "foo")
		qAssertJoins(t, qs, 1, 0)
	})
}
