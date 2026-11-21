package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of Queries1Tests of Django's tests/queries/tests.py. The models
// and the rows of its setUpTestData are in django_queries_models_test.go.

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket6074
func TestQueriesTicket6074(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		authors := orm.Query[qAuthor](db)
		empty := orm.Q("id__in", []int64{})
		djangotest.AssertQuerySetEqual(t, authors.Filter(empty), []string{}, qAuthorName)
		djangotest.AssertQuerySetEqual(t, authors.Filter(empty.Or(empty)), []string{}, qAuthorName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_tickets_1878_2939
//
// The assertions that select a column with extra() are not ported.
func TestQueriesTickets1878And2939(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		ctx := context.Background()
		items := orm.Query[qItem](db)
		n, err := items.Values("creator").Distinct().Count(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, 3)

		// A second item named "four" makes the two-column case differ from the
		// one-column case.
		create(t, db, &qItem{
			Name: "four", Created: f.time1, Creator: fkTo[qAuthor](f.a2.ID), Note: fkTo[qNote](f.n1.ID),
		})
		n, err = items.Exclude("name", "two").Values("creator", "name").Distinct().Count(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, 4)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket7323
func TestQueriesTicket7323(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		n, err := orm.Query[qItem](db).Values("creator", "name").Count(context.Background())
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, 4)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_tickets_5321_7070
func TestQueriesTickets5321And7070(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		// The ordering columns are part of what DISTINCT compares, so rows that
		// look equal in the output stay apart.
		assertValues(t, orm.Query[qNote](db).Values("misc").Distinct().OrderBy("note", "-misc"),
			[]map[string]any{{"misc": "foo"}, {"misc": "bar"}, {"misc": "foo"}})
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket7096
//
// The assertions on Item.tags, a many-to-many relation, are not ported.
func TestQueriesTicket7096(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		tags := orm.Query[qTag](db)
		djangotest.AssertQuerySetEqual(t, tags.Filter("parent", f.t1, "name", "t3").OrderBy("name"),
			[]string{"t3"}, qTagName)
		djangotest.AssertQuerySetEqual(t, tags.Exclude("parent", f.t1, "name", "t3").OrderBy("name"),
			[]string{"t1", "t2", "t4", "t5"}, qTagName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket7181
//
// The assertions that merge querysets with | and & are not ported: the ORM
// combines conditions with Q, not querysets.
func TestQueriesTicket7181(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		// Where NULL sorts depends on the database, so only the row count is
		// checked: ordering through a nullable relation must not drop rows.
		djangotest.AssertEqual(t, len(mustAll(t, orm.Query[qTag](db).OrderBy("parent__name"))), 5)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket9926
func TestQueriesTicket9926(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		tags := orm.Query[qTag](db)
		want := []string{"t1", "t2", "t3", "t4", "t5"}
		djangotest.AssertQuerySetEqual(t, tags.SelectRelated("parent", "category").OrderBy("name"), want, qTagName)
		djangotest.AssertQuerySetEqual(t,
			tags.SelectRelated("parent", "parent__category").OrderBy("name"), want, qTagName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket10205
func TestQueriesTicket10205(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		n, err := orm.Query[qTag](db).Filter("name__in", []string{}).Update(context.Background(), "name", "foo")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket10742
//
// Django checks that the inner queryset was not evaluated by looking at its
// result cache; a queryset here has none, so the port counts the queries.
func TestQueriesTicket10742(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		authors := orm.Query[qAuthor](db)
		subq := authors.Filter("num__lt", 3000)
		djangotest.AssertNumQueries(t, db, 3, func() {
			djangotest.AssertQuerySetEqual(t, authors.Filter("pk__in", subq).OrderBy("name"),
				[]string{"a1", "a2"}, qAuthorName)
			djangotest.AssertQuerySetEqual(t, authors.Exclude("pk__in", subq).OrderBy("name"),
				[]string{"a3", "a4"}, qAuthorName)
			djangotest.AssertQuerySetEqual(t, authors.Filter(orm.Q("pk__in", subq).And(orm.Q("name", "a1"))),
				[]string{"a1"}, qAuthorName)
		})
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket7076
func TestQueriesTicket7076(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		// Excluding must not eliminate the rows that hold NULL.
		djangotest.AssertQuerySetEqual(t, orm.Query[qItem](db).Exclude("modified", f.time1).OrderBy("name"),
			[]string{"four", "three", "two"}, qItemName)
		djangotest.AssertQuerySetEqual(t, orm.Query[qTag](db).Exclude("parent__name", f.t1.Name).OrderBy("name"),
			[]string{"t1", "t4", "t5"}, qTagName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket_10790_1
func TestQueriesTicket10790Part1(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		tags := orm.Query[qTag](db).OrderBy("name")
		// An isnull test on a foreign key reads the local column: no join.
		q := tags.Filter("parent__isnull", true)
		djangotest.AssertQuerySetEqual(t, q, []string{"t1"}, qTagName)
		qAssertJoins(t, q, 0, 0)

		q = tags.Filter("parent__isnull", false)
		djangotest.AssertQuerySetEqual(t, q, []string{"t2", "t3", "t4", "t5"}, qTagName)
		qAssertJoins(t, q, 0, 0)

		q = tags.Exclude("parent__isnull", true)
		djangotest.AssertQuerySetEqual(t, q, []string{"t2", "t3", "t4", "t5"}, qTagName)
		qAssertJoins(t, q, 0, 0)

		q = tags.Exclude("parent__isnull", false)
		djangotest.AssertQuerySetEqual(t, q, []string{"t1"}, qTagName)
		qAssertJoins(t, q, 0, 0)

		q = tags.Exclude("parent__parent__isnull", false)
		djangotest.AssertQuerySetEqual(t, q, []string{"t1", "t2", "t3"}, qTagName)
		qAssertJoins(t, q, 0, 1)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket_10790_2
func TestQueriesTicket10790Part2(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		tags := orm.Query[qTag](db).OrderBy("name")
		// Only the last join is trimmed; the one before it stays INNER.
		q := tags.Filter("parent__parent__isnull", false)
		djangotest.AssertQuerySetEqual(t, q, []string{"t4", "t5"}, qTagName)
		qAssertJoins(t, q, 1, 0)

		q = tags.Filter("parent__parent", f.t1)
		djangotest.AssertQuerySetEqual(t, q, []string{"t4", "t5"}, qTagName)
		qAssertJoins(t, q, 1, 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket_10790_8
func TestQueriesTicket10790Part8(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		q := orm.Query[qTag](db).OrderBy("name").
			Filter(orm.Q("parent__isnull", true).Or(orm.Q("parent", f.t1)))
		djangotest.AssertQuerySetEqual(t, q, []string{"t1", "t2", "t3"}, qTagName)
		qAssertJoins(t, q, 0, 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket19672
func TestQueriesTicket19672(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		q := orm.Query[qReport](db).Filter(
			orm.Q("creator__isnull", false).And(orm.Not(orm.Q("creator__extra__value", 41))))
		djangotest.AssertQuerySetEqual(t, q, []string{"r1"}, qReportName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_lookup_constraint_fielderror
func TestQueriesLookupConstraintFieldError(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[qTag](db).Filter("unknown_field__name", "generic").All(context.Background())
		assertFieldError(t, err, "annot resolve keyword 'unknown_field' into field. Choices are: "+
			"category, category_id, id, name, parent, parent_id, qannotation, qnote, qtag")
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_field_with_filterable
func TestQueriesFieldWithFilterable(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[qAuthor](db).Filter("extra", f.e2).OrderBy("name"),
			[]string{"a3", "a4"}, qAuthorName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_negate_field
func TestQueriesNegateField(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		notes := orm.Query[qNote](db).OrderBy("note")
		djangotest.AssertQuerySetEqual(t, notes.Filter("negate", true), []string{"n1", "n2"}, qNoteName)
		djangotest.AssertQuerySetEqual(t, notes.Exclude("negate", true), []string{"n3"}, qNoteName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_tickets_2076_7256
//
// Ordering by a relation orders by its foreign-key column here; Django orders
// by the default ordering of the related model, which the ORM does not have.
// The assertions that depend on that, and the one on Item.tags, are not
// ported.
func TestQueriesTickets2076And7256(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		items := orm.Query[qItem](db)
		// Ordering on a related table works even when the table is not otherwise
		// part of the query.
		djangotest.AssertQuerySetEqual(t, items.OrderBy("note__note", "name"),
			[]string{"two", "four", "one", "three"}, qItemName)
		djangotest.AssertQuerySetEqual(t, items.OrderBy("creator", "name"),
			[]string{"one", "three", "two", "four"}, qItemName)

		// An ordering on a local column needs no join.
		qs := items.OrderBy("name")
		djangotest.AssertQuerySetEqual(t, qs, []string{"four", "one", "three", "two"}, qItemName)
		qAssertJoins(t, qs, 0, 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_tickets_7448_7707
func TestQueriesTickets7448And7707(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		qs := orm.Query[qItem](db).Filter("created__in", []time.Time{f.time1, f.time2}).OrderBy("-note", "name")
		djangotest.AssertQuerySetEqual(t, qs, []string{"one", "two"}, qItemName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_tickets_2874_3002
//
// Django reaches creator.extra.note through lazy loading; the port selects the
// whole chain, which puts the Note table into the statement twice.
func TestQueriesTickets2874And3002(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		qs := orm.Query[qItem](db).SelectRelated("creator__extra__note", "note").OrderBy("note__note", "name")
		rows := mustAll(t, qs)
		djangotest.AssertQuerySetEqual(t, qs, []string{"two", "four", "one", "three"}, qItemName)

		// The two joins to Note must not be mixed up.
		note, ok := rows[0].Note.Object()
		djangotest.AssertEqual(t, ok, true)
		djangotest.AssertEqual(t, note.Note, "n2")
		creator, _ := rows[0].Creator.Object()
		extra, _ := creator.Extra.Object()
		extraNote, ok := extra.Note.Object()
		djangotest.AssertEqual(t, ok, true)
		djangotest.AssertEqual(t, extraNote.Note, "n1")
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket4358
func TestQueriesTicket4358(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		infos := orm.Query[qExtraInfo](db).OrderBy("info")
		// Without keys a relation is returned under its "_id" name.
		rows, err := infos.Values().All(context.Background())
		djangotest.AssertEqual(t, err, nil)
		_, ok := rows[0]["note_id"]
		djangotest.AssertEqual(t, ok, true)
		// Both names of the relation can be asked for.
		assertValues(t, infos.Values("note_id"), []map[string]any{{"note_id": f.n1.ID}, {"note_id": f.n2.ID}})
		assertValues(t, infos.Values("note"), []map[string]any{{"note": f.n1.ID}, {"note": f.n2.ID}})
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket9997
func TestQueriesTicket9997(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		tags := orm.Query[qTag](db)
		// A Values queryset with one key is a one-column subquery.
		qs := tags.Filter("name__in", tags.Filter("parent", f.t1).Values("name")).OrderBy("name")
		djangotest.AssertQuerySetEqual(t, qs, []string{"t2", "t3"}, qTagName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_order_by_related_field_transform
//
// DateTimePK has an integer primary key here, so the month is taken from its
// date field: "date__date__month" stands for Django's "date__month".
func TestQueriesOrderByRelatedFieldTransform(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		d12 := create(t, db, &qDateTimePK{Date: time.Date(2021, 12, 10, 0, 0, 0, 0, time.UTC)})
		extra12 := create(t, db, &qExtraInfo{Info: "extra 12", Date: fkTo[qDateTimePK](d12.ID)})
		d11 := create(t, db, &qDateTimePK{Date: time.Date(2022, 11, 10, 0, 0, 0, 0, time.UTC)})
		extra11 := create(t, db, &qExtraInfo{Info: "extra 11", Date: fkTo[qDateTimePK](d11.ID)})
		djangotest.AssertQuerySetEqual(t,
			orm.Query[qExtraInfo](db).Filter("date__isnull", false).OrderBy("date__date__month"),
			[]int64{extra11.ID, extra12.ID}, func(e qExtraInfo) int64 { return e.ID })
	})
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_get_clears_ordering
func TestQueriesGetClearsOrdering(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		f := qSetUp(t, db)
		capture := djangotest.CaptureQueries(db)
		_, err := orm.Query[qAuthor](db).OrderBy("name").Get(context.Background(), "pk", f.a1.ID)
		capture.Stop()
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, capture.Len(), 1)
		if sql := capture.Queries()[0].SQL; strings.Contains(strings.ToLower(sql), "order by") {
			t.Errorf("Get kept the ordering: %s", sql)
		}
	})
}
