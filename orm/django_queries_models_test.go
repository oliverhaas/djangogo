package orm_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/queries/models.py that the ported tests use,
// with these differences:
//
//   - A model has no default ordering here, so a port adds the OrderBy that
//     the Meta.ordering of the Django model implies.
//   - The many-to-many relations (Item.tags, Annotation.notes) are left out.
//   - Report.creator points at the primary key of Author; Django points it at
//     Author.num with to_field, which the ORM does not have.
//   - NamedCategory is a plain model; in Django it inherits DumbCategory.

type qNamedCategory struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

type qTag struct {
	ID       int64
	Name     string                 `orm:"max_length=10"`
	Parent   orm.FK[qTag]           `orm:"null;on_delete=set_null"`
	Category orm.FK[qNamedCategory] `orm:"null;on_delete=set_null"`
}

type qNote struct {
	ID     int64
	Note   string       `orm:"max_length=100"`
	Misc   string       `orm:"max_length=25"`
	Tag    orm.FK[qTag] `orm:"null;on_delete=set_null"`
	Negate bool
}

type qAnnotation struct {
	ID   int64
	Name string `orm:"max_length=10"`
	Tag  orm.FK[qTag]
}

type qExtraInfo struct {
	ID         int64
	Info       string        `orm:"max_length=100"`
	Note       orm.FK[qNote] `orm:"null"`
	Value      int64         `orm:"null"`
	Filterable bool
}

type qAuthor struct {
	ID    int64
	Name  string `orm:"max_length=10"`
	Num   int64  `orm:"unique"`
	Extra orm.FK[qExtraInfo]
}

type qItem struct {
	ID       int64
	Name     string `orm:"max_length=10"`
	Created  time.Time
	Modified time.Time `orm:"null"`
	Creator  orm.FK[qAuthor]
	Note     orm.FK[qNote]
}

type qReport struct {
	ID      int64
	Name    string          `orm:"max_length=10"`
	Creator orm.FK[qAuthor] `orm:"null;on_delete=set_null"`
}

type qRanking struct {
	ID     int64
	Rank   int64
	Author orm.FK[qAuthor]
}

type qNumber struct {
	ID         int64
	Num        int64
	OtherNum   int64 `orm:"null"`
	AnotherNum int64 `orm:"null"`
}

type qNullableName struct {
	ID   int64
	Name string `orm:"max_length=20;null"`
}

type qArticle struct {
	ID      int64
	Name    string `orm:"max_length=20"`
	Created time.Time
}

type qLeafA struct {
	ID   int64
	Data string `orm:"max_length=10"`
}

type qLeafB struct {
	ID   int64
	Data string `orm:"max_length=10"`
}

type qJoin struct {
	ID int64
	A  orm.FK[qLeafA]
	B  orm.FK[qLeafB]
}

// qModels returns the models of the queries app.
func qModels() []any {
	return []any{djangotest.App("queries",
		&qNamedCategory{}, &qTag{}, &qNote{}, &qAnnotation{}, &qExtraInfo{}, &qAuthor{}, &qItem{},
		&qReport{}, &qRanking{}, &qNumber{}, &qNullableName{}, &qArticle{}, &qLeafA{}, &qLeafB{}, &qJoin{},
		&qFK1{}, &qFK2{}, &qFK3{}, &qBaseA{}, &qModelA{}, &qModelB{}, &qModelC{}, &qModelD{},
		&qNode{}, &qObjectA{}, &qObjectB{}, &qObjectC{},
	)}
}

// qFixture holds the rows of Queries1Tests.setUpTestData.
type qFixture struct {
	nc1                *qNamedCategory
	t1, t2, t3, t4, t5 *qTag
	n1, n2, n3         *qNote
	e1, e2             *qExtraInfo
	a1, a2, a3, a4     *qAuthor
	i1, i2, i3, i4     *qItem
	r1, r2, r3         *qReport
	time1, time2       time.Time
}

// qSetUp creates the rows of Queries1Tests.setUpTestData, without the
// many-to-many links and the Ranking and Cover rows, which no port reads.
func qSetUp(t *testing.T, db *orm.DB) *qFixture {
	t.Helper()
	f := &qFixture{}
	f.nc1 = create(t, db, &qNamedCategory{Name: "Generic"})
	generic := fkTo[qNamedCategory](f.nc1.ID)
	f.t1 = create(t, db, &qTag{Name: "t1", Category: generic})
	f.t2 = create(t, db, &qTag{Name: "t2", Parent: fkTo[qTag](f.t1.ID), Category: generic})
	f.t3 = create(t, db, &qTag{Name: "t3", Parent: fkTo[qTag](f.t1.ID)})
	f.t4 = create(t, db, &qTag{Name: "t4", Parent: fkTo[qTag](f.t3.ID)})
	f.t5 = create(t, db, &qTag{Name: "t5", Parent: fkTo[qTag](f.t3.ID)})

	f.n1 = create(t, db, &qNote{Note: "n1", Misc: "foo", Negate: true})
	f.n2 = create(t, db, &qNote{Note: "n2", Misc: "bar", Negate: true})
	f.n3 = create(t, db, &qNote{Note: "n3", Misc: "foo", Negate: false})

	create(t, db, &qAnnotation{Name: "a1", Tag: fkTo[qTag](f.t1.ID)})
	create(t, db, &qAnnotation{Name: "a2", Tag: fkTo[qTag](f.t4.ID)})

	// Django creates these out of order so that sorting by id differs from
	// sorting by info.
	f.e2 = create(t, db, &qExtraInfo{Info: "e2", Note: fkTo[qNote](f.n2.ID), Value: 41, Filterable: false})
	f.e1 = create(t, db, &qExtraInfo{Info: "e1", Note: fkTo[qNote](f.n1.ID), Value: 42, Filterable: true})

	f.a1 = create(t, db, &qAuthor{Name: "a1", Num: 1001, Extra: fkTo[qExtraInfo](f.e1.ID)})
	f.a2 = create(t, db, &qAuthor{Name: "a2", Num: 2002, Extra: fkTo[qExtraInfo](f.e1.ID)})
	f.a3 = create(t, db, &qAuthor{Name: "a3", Num: 3003, Extra: fkTo[qExtraInfo](f.e2.ID)})
	f.a4 = create(t, db, &qAuthor{Name: "a4", Num: 4004, Extra: fkTo[qExtraInfo](f.e2.ID)})

	f.time1 = time.Date(2007, 12, 19, 22, 25, 0, 0, time.UTC)
	f.time2 = time.Date(2007, 12, 19, 21, 0, 0, 0, time.UTC)
	time3 := time.Date(2007, 12, 20, 22, 25, 0, 0, time.UTC)
	time4 := time.Date(2007, 12, 20, 21, 0, 0, 0, time.UTC)
	item := func(name string, created time.Time, creator *qAuthor, note *qNote) *qItem {
		return create(t, db, &qItem{
			Name: name, Created: created, Creator: fkTo[qAuthor](creator.ID), Note: fkTo[qNote](note.ID),
		})
	}
	f.i1 = create(t, db, &qItem{
		Name: "one", Created: f.time1, Modified: f.time1,
		Creator: fkTo[qAuthor](f.a1.ID), Note: fkTo[qNote](f.n3.ID),
	})
	f.i2 = item("two", f.time2, f.a2, f.n2)
	f.i3 = item("three", time3, f.a2, f.n3)
	f.i4 = item("four", time4, f.a4, f.n3)
	for _, it := range []*qItem{f.i2, f.i3, f.i4} {
		lkSetNull[qItem](t, db, it.ID, "modified")
	}

	f.r1 = create(t, db, &qReport{Name: "r1", Creator: fkTo[qAuthor](f.a1.ID)})
	f.r2 = create(t, db, &qReport{Name: "r2", Creator: fkTo[qAuthor](f.a3.ID)})
	f.r3 = create(t, db, &qReport{Name: "r3"})
	return f
}

// The transforms of the ports: each maps a row to the name the Django test
// would show in a failure.

func qTagName(x qTag) string                   { return x.Name }
func qNoteName(x qNote) string                 { return x.Note }
func qAuthorName(x qAuthor) string             { return x.Name }
func qItemName(x qItem) string                 { return x.Name }
func qReportName(x qReport) string             { return x.Name }
func qNumberNum(x qNumber) int64               { return x.Num }
func qNullableNameName(x qNullableName) string { return x.Name }

// qAssertJoins checks the number of INNER and LEFT OUTER joins in the SELECT
// of qs, the counterpart of counting them in Django's str(qs.query).
func qAssertJoins[T any](t *testing.T, qs *orm.QuerySet[T], inner, outer int) {
	t.Helper()
	sql, _, err := orm.CompileSelectForTest(qs)
	if err != nil {
		t.Errorf("compile: %v", err)
		return
	}
	gotInner, gotOuter := strings.Count(sql, "INNER JOIN"), strings.Count(sql, "LEFT OUTER JOIN")
	if gotInner != inner || gotOuter != outer {
		t.Errorf("joins: %d INNER and %d LEFT OUTER, want %d and %d\n%s", gotInner, gotOuter, inner, outer, sql)
	}
}
