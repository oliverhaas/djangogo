package contenttypes_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// RegressLink is tests/generic_relations_regress/models.py::Link.
type RegressLink struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// RegressPlace is tests/generic_relations_regress/models.py::Place without
// link_proxy, a relation to a proxy model.
type RegressPlace struct {
	ID    int64
	Name  string                                    `orm:"max_length=100"`
	Links contenttypes.GenericRelation[RegressLink] `orm:"related_query_name=places"`
}

// RegressAddress is tests/generic_relations_regress/models.py::Address.
type RegressAddress struct {
	ID            int64
	Street        string `orm:"max_length=80"`
	City          string `orm:"max_length=50"`
	State         string `orm:"max_length=2"`
	Zipcode       string `orm:"max_length=5"`
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// RegressPerson is tests/generic_relations_regress/models.py::Person, whose
// primary key is not named id.
type RegressPerson struct {
	Account   int64  `orm:"pk;type=int"`
	Name      string `orm:"max_length=128"`
	Addresses contenttypes.GenericRelation[RegressAddress]
}

// RegressCharLink is tests/generic_relations_regress/models.py::CharLink, a
// generic foreign key with a varchar object id.
type RegressCharLink struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      string `orm:"max_length=100"`
	ContentObject contenttypes.GenericForeignKey
	Value         string `orm:"max_length=250"`
}

// RegressTextLink is tests/generic_relations_regress/models.py::TextLink, a
// generic foreign key with a text object id.
type RegressTextLink struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      string `orm:"type=text"`
	ContentObject contenttypes.GenericForeignKey
	Value         string `orm:"max_length=250"`
}

// RegressOddRelation1 is tests/generic_relations_regress/models.py::OddRelation1.
type RegressOddRelation1 struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Clinks contenttypes.GenericRelation[RegressCharLink]
}

// RegressOddRelation2 is tests/generic_relations_regress/models.py::OddRelation2.
type RegressOddRelation2 struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Tlinks contenttypes.GenericRelation[RegressTextLink]
}

// RegressNote is tests/generic_relations_regress/models.py::Note.
type RegressNote struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
	Note          string `orm:"type=text"`
}

// RegressContact is tests/generic_relations_regress/models.py::Contact.
type RegressContact struct {
	ID    int64
	Notes contenttypes.GenericRelation[RegressNote]
}

// RegressOrganization is tests/generic_relations_regress/models.py::Organization.
type RegressOrganization struct {
	ID       int64
	Name     string                  `orm:"max_length=255"`
	Contacts orm.M2M[RegressContact] `orm:"related_name=organizations"`
}

// RegressCompany is tests/generic_relations_regress/models.py::Company.
type RegressCompany struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Links contenttypes.GenericRelation[RegressLink]
}

// RegressTag is tests/generic_relations_regress/models.py::Tag.
type RegressTag struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType] `orm:"related_name=g_r_r_tags"`
	ObjectID      string                           `orm:"max_length=15"`
	ContentObject contenttypes.GenericForeignKey
	Label         string `orm:"max_length=15"`
}

// RegressBoard is tests/generic_relations_regress/models.py::Board, a model
// with a text primary key.
type RegressBoard struct {
	Name string `orm:"pk;max_length=25"`
}

// RegressHasLinkThing is tests/generic_relations_regress/models.py::HasLinkThing
// with the links of its abstract base HasLinks. The orm has no abstract
// models and no GenericRelation subclasses, so the field is a plain
// GenericRelation.
type RegressHasLinkThing struct {
	ID    int64
	Links contenttypes.GenericRelation[RegressLink] `orm:"related_query_name=targets"`
}

// RegressA is tests/generic_relations_regress/models.py::A.
type RegressA struct {
	ID            int64
	Flag          *bool
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// RegressB is tests/generic_relations_regress/models.py::B.
type RegressB struct {
	ID int64
	A  contenttypes.GenericRelation[RegressA]
}

// Meta orders the rows as Django's B.Meta does.
func (RegressB) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// RegressC is tests/generic_relations_regress/models.py::C.
type RegressC struct {
	ID int64
	B  orm.FK[RegressB] `orm:"on_delete=cascade"`
}

// Meta orders the rows as Django's C.Meta does.
func (RegressC) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// RegressD is tests/generic_relations_regress/models.py::D.
type RegressD struct {
	ID int64
	B  orm.FK[RegressB] `orm:"null;on_delete=set_null"`
}

// Meta orders the rows as Django's D.Meta does.
func (RegressD) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// RegressNode is tests/generic_relations_regress/models.py::Node.
type RegressNode struct {
	ID          int64
	ContentType orm.FK[contenttypes.ContentType]
	ObjectID    int64                          `orm:"type=positive_int"`
	Content     contenttypes.GenericForeignKey `orm:"ct_field=content_type;fk_field=object_id"`
}

// RegressContent is tests/generic_relations_regress/models.py::Content.
type RegressContent struct {
	ID         int64
	Nodes      contenttypes.GenericRelation[RegressNode]
	RelatedObj orm.FK[RegressRelated] `orm:"on_delete=cascade"`
}

// RegressRelated is tests/generic_relations_regress/models.py::Related.
type RegressRelated struct {
	ID int64
}

// regressModels is the registry of the ported generic_relations_regress
// tests.
func regressModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("generic_relations_regress",
			&RegressLink{}, &RegressPlace{}, &RegressAddress{}, &RegressPerson{},
			&RegressCharLink{}, &RegressTextLink{}, &RegressOddRelation1{}, &RegressOddRelation2{},
			&RegressNote{}, &RegressContact{}, &RegressOrganization{}, &RegressCompany{},
			&RegressTag{}, &RegressBoard{}, &RegressHasLinkThing{},
			&RegressA{}, &RegressB{}, &RegressC{}, &RegressD{},
			&RegressNode{}, &RegressContent{}, &RegressRelated{}),
	}
}

// createWithObject sets the generic foreign key content_object of obj to
// target and creates obj (Django's Model.objects.create(content_object=target)).
func createWithObject[T any](t *testing.T, db *orm.DB, obj *T, target any) *T {
	t.Helper()
	setObject(t, db, obj, "content_object", target)
	return mustCreate(t, db, obj)
}

// idOf returns the primary key of a model with an ID field, for
// AssertQuerySetEqual.
func idOf[T interface {
	RegressLink | RegressHasLinkThing | RegressB | RegressC | RegressD |
		RegressOddRelation1 | RegressOddRelation2 | RegressContact
}](obj T) int64 {
	switch o := any(obj).(type) {
	case RegressLink:
		return o.ID
	case RegressHasLinkThing:
		return o.ID
	case RegressB:
		return o.ID
	case RegressC:
		return o.ID
	case RegressD:
		return o.ID
	case RegressOddRelation1:
		return o.ID
	case RegressOddRelation2:
		return o.ID
	case RegressContact:
		return o.ID
	}
	return 0
}

// capturedSQL returns the SQL of the single statement fn runs.
func capturedSQL(t *testing.T, db *orm.DB, fn func() error) string {
	t.Helper()
	qc := djangotest.CaptureQueries(db)
	err := fn()
	qc.Stop()
	if err != nil {
		t.Fatal(err)
	}
	queries := qc.Queries()
	if len(queries) != 1 {
		t.Fatalf("%d queries, want 1", len(queries))
	}
	return queries[0].SQL
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_reverse_relation_pk
func TestGenericRegressReverseRelationPK(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := mustCreate(t, db, &RegressPerson{Account: 23, Name: "Chef"})
		createWithObject(t, db, &RegressAddress{
			Street: "123 Anywhere Place", City: "Conifer", State: "CO", Zipcode: "80433",
		}, p)
		qs := orm.Query[RegressPerson](db).Filter("addresses__zipcode", "80433")
		n, err := qs.Count(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, int64(1))
		first, err := qs.First(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, first.Name, "Chef")
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_charlink_delete
func TestGenericRegressCharlinkDelete(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		oddrel := mustCreate(t, db, &RegressOddRelation1{Name: "clink"})
		createWithObject(t, db, &RegressCharLink{}, oddrel)
		_, err := orm.Query[RegressOddRelation1](db).Filter("pk", oddrel.ID).Delete(context.Background())
		djangotest.AssertEqual(t, err, nil)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_textlink_delete
func TestGenericRegressTextlinkDelete(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		oddrel := mustCreate(t, db, &RegressOddRelation2{Name: "tlink"})
		createWithObject(t, db, &RegressTextLink{}, oddrel)
		_, err := orm.Query[RegressOddRelation2](db).Filter("pk", oddrel.ID).Delete(context.Background())
		djangotest.AssertEqual(t, err, nil)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_charlink_filter
func TestGenericRegressCharlinkFilter(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		oddrel := mustCreate(t, db, &RegressOddRelation1{Name: "clink"})
		createWithObject(t, db, &RegressCharLink{Value: "value"}, oddrel)
		djangotest.AssertQuerySetEqual(t, orm.Query[RegressOddRelation1](db).Filter("clinks__value", "value"),
			[]int64{oddrel.ID}, idOf[RegressOddRelation1])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_textlink_filter
func TestGenericRegressTextlinkFilter(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		oddrel := mustCreate(t, db, &RegressOddRelation2{Name: "clink"})
		createWithObject(t, db, &RegressTextLink{Value: "value"}, oddrel)
		djangotest.AssertQuerySetEqual(t, orm.Query[RegressOddRelation2](db).Filter("tlinks__value", "value"),
			[]int64{oddrel.ID}, idOf[RegressOddRelation2])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_q_object_or
func TestGenericRegressQObjectOr(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		noteContact := mustCreate(t, db, &RegressContact{})
		orgContact := mustCreate(t, db, &RegressContact{})
		createWithObject(t, db, &RegressNote{Note: "note"}, noteContact)
		org := mustCreate(t, db, &RegressOrganization{Name: "org name"})
		if err := orm.Many[RegressContact](db, org, "contacts").Add(ctx, orgContact); err != nil {
			t.Fatal(err)
		}
		note := orm.Q("notes__note__icontains", "other note")
		orgName := orm.Q("organizations__name__icontains", "org name")
		// A non-matching note and a matching organization name, then the same
		// conditions in reverse order.
		for _, cond := range []orm.Cond{note.Or(orgName), orgName.Or(note)} {
			contacts, err := orm.Query[RegressContact](db).Filter(cond).All(ctx)
			djangotest.AssertEqual(t, err, nil)
			found := false
			for _, c := range contacts {
				found = found || c.ID == orgContact.ID
			}
			djangotest.AssertEqual(t, found, true)
		}
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_join_reuse
func TestGenericRegressJoinReuse(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[RegressPerson](db).Filter("addresses__street", "foo").Filter("addresses__street", "bar")
		sql := capturedSQL(t, db, func() error {
			_, err := qs.All(context.Background())
			return err
		})
		djangotest.AssertEqual(t, strings.Count(sql, "JOIN"), 2)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_generic_relation_ordering
func TestGenericRegressGenericRelationOrdering(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		p1 := mustCreate(t, db, &RegressPlace{Name: "South Park"})
		p2 := mustCreate(t, db, &RegressPlace{Name: "The City"})
		c := mustCreate(t, db, &RegressCompany{Name: "Chubby's Intl."})
		createWithObject(t, db, &RegressLink{}, p1)
		createWithObject(t, db, &RegressLink{}, c)
		places, err := orm.Query[RegressPlace](db).OrderBy("links__id").All(context.Background())
		djangotest.AssertEqual(t, err, nil)
		count := func(place *RegressPlace) int {
			n := 0
			for _, p := range places {
				if p.ID == place.ID {
					n++
				}
			}
			return n
		}
		djangotest.AssertEqual(t, len(places), 2)
		djangotest.AssertEqual(t, count(p1), 1)
		djangotest.AssertEqual(t, count(p2), 1)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_gfk_to_model_with_empty_pk
func TestGenericRegressGFKToModelWithEmptyPK(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		b1 := mustCreate(t, db, &RegressBoard{Name: ""})
		createWithObject(t, db, &RegressTag{Label: "VP"}, b1)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_ticket_20378
func TestGenericRegressTicket20378(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		// Create extra things so that the primary keys of the links and the
		// things differ.
		hs1 := mustCreate(t, db, &RegressHasLinkThing{})
		hs2 := mustCreate(t, db, &RegressHasLinkThing{})
		hs3 := mustCreate(t, db, &RegressHasLinkThing{})
		hs4 := mustCreate(t, db, &RegressHasLinkThing{})
		l1 := createWithObject(t, db, &RegressLink{}, hs3)
		l2 := createWithObject(t, db, &RegressLink{}, hs4)
		things := orm.Query[RegressHasLinkThing](db)
		djangotest.AssertQuerySetEqual(t, things.Filter("links", l1), []int64{hs3.ID}, idOf[RegressHasLinkThing])
		djangotest.AssertQuerySetEqual(t, things.Filter("links", l2), []int64{hs4.ID}, idOf[RegressHasLinkThing])
		djangotest.AssertQuerySetEqual(t, things.Exclude("links", l2),
			[]int64{hs1.ID, hs2.ID, hs3.ID}, idOf[RegressHasLinkThing], djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, things.Exclude("links", l1),
			[]int64{hs1.ID, hs2.ID, hs4.ID}, idOf[RegressHasLinkThing], djangotest.Unordered())
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_ticket_20564
func TestGenericRegressTicket20564(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		b1 := mustCreate(t, db, &RegressB{})
		b2 := mustCreate(t, db, &RegressB{})
		b3 := mustCreate(t, db, &RegressB{})
		c1 := mustCreate(t, db, &RegressC{B: fkOf[RegressB](b1.ID)})
		c2 := mustCreate(t, db, &RegressC{B: fkOf[RegressB](b2.ID)})
		c3 := mustCreate(t, db, &RegressC{B: fkOf[RegressB](b3.ID)})
		createWithObject(t, db, &RegressA{Flag: nil}, b1)
		createWithObject(t, db, &RegressA{Flag: new(true)}, b2)
		cs := orm.Query[RegressC](db)
		djangotest.AssertQuerySetEqual(t, cs.Filter("b__a__flag", nil), []int64{c1.ID, c3.ID}, idOf[RegressC])
		djangotest.AssertQuerySetEqual(t, cs.Exclude("b__a__flag", nil), []int64{c2.ID}, idOf[RegressC])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_ticket_20564_nullable_fk
func TestGenericRegressTicket20564NullableFK(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		b1 := mustCreate(t, db, &RegressB{})
		b2 := mustCreate(t, db, &RegressB{})
		b3 := mustCreate(t, db, &RegressB{})
		d1 := mustCreate(t, db, &RegressD{B: fkOf[RegressB](b1.ID)})
		d2 := mustCreate(t, db, &RegressD{B: fkOf[RegressB](b2.ID)})
		d3 := mustCreate(t, db, &RegressD{B: fkOf[RegressB](b3.ID)})
		d4 := mustCreate(t, db, &RegressD{})
		createWithObject(t, db, &RegressA{Flag: nil}, b1)
		createWithObject(t, db, &RegressA{Flag: new(true)}, b1)
		createWithObject(t, db, &RegressA{Flag: new(true)}, b2)
		ds, bs := orm.Query[RegressD](db), orm.Query[RegressB](db)
		djangotest.AssertQuerySetEqual(t, ds.Exclude("b__a__flag", nil), []int64{d2.ID}, idOf[RegressD])
		djangotest.AssertQuerySetEqual(t, ds.Filter("b__a__flag", nil), []int64{d1.ID, d3.ID, d4.ID}, idOf[RegressD])
		djangotest.AssertQuerySetEqual(t, bs.Filter("a__flag", nil), []int64{b1.ID, b3.ID}, idOf[RegressB])
		djangotest.AssertQuerySetEqual(t, bs.Exclude("a__flag", nil), []int64{b2.ID}, idOf[RegressB])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_extra_join_condition
func TestGenericRegressExtraJoinCondition(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sqlOf := func(qs *orm.QuerySet[RegressB]) string {
			return strings.ToLower(capturedSQL(t, db, func() error {
				_, err := qs.All(ctx)
				return err
			}))
		}
		// The content type takes part in the join or subquery condition.
		djangotest.AssertEqual(t, strings.Contains(sqlOf(orm.Query[RegressB](db).Exclude("a__flag", nil)), "content_type_id"), true)
		// The subquery needs no join here, as opposed to the query above, where
		// a b without any a would then fail.
		sql := sqlOf(orm.Query[RegressB](db).Exclude("a__flag", true))
		djangotest.AssertEqual(t, strings.Contains(sql, " join "), false)
		djangotest.AssertEqual(t, strings.Contains(sql, "content_type_id"), true)
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_annotate
func TestGenericRegressAnnotate(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		hs1 := mustCreate(t, db, &RegressHasLinkThing{})
		hs2 := mustCreate(t, db, &RegressHasLinkThing{})
		mustCreate(t, db, &RegressHasLinkThing{})
		b := mustCreate(t, db, &RegressBoard{Name: strconv.FormatInt(hs1.ID, 10)})
		createWithObject(t, db, &RegressLink{}, hs2)
		link := createWithObject(t, db, &RegressLink{}, hs1)
		createWithObject(t, db, &RegressLink{}, b)
		qs := orm.Query[RegressHasLinkThing](db).Annotate(orm.Sum("links")).Filter("pk", hs1.ID)
		// Without the content type in the join condition, the link to b would
		// match as well: b and hs1 have equal primary keys.
		sum := func() any {
			t.Helper()
			n, err := qs.Count(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, n, int64(1))
			rows, err := qs.AllAnnotated(ctx)
			djangotest.AssertEqual(t, err, nil)
			return rows[0].Annotations["links__sum"]
		}
		djangotest.AssertEqual(t, sum(), any(link.ID))
		if _, err := orm.Query[RegressLink](db).Filter("pk", link.ID).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		// Without a left join there would be no row at all.
		djangotest.AssertEqual(t, sum(), nil)
		for _, c := range []struct {
			isNull bool
			want   int64
		}{{true, 1}, {false, 0}} {
			n, err := qs.Filter("links__sum__isnull", c.isNull).Count(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, n, c.want)
		}
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_filter_targets_related_pk
func TestGenericRegressFilterTargetsRelatedPK(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		// Fixed primary keys make those of the link and of hs2 differ.
		mustCreate(t, db, &RegressHasLinkThing{ID: 1})
		hs2 := mustCreate(t, db, &RegressHasLinkThing{ID: 2})
		link := createWithObject(t, db, &RegressLink{ID: 1}, hs2)
		if link.ObjectID == link.ID {
			t.Fatalf("object id %d equals the primary key of the link", link.ObjectID)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[RegressHasLinkThing](db).Filter("links", link.ID),
			[]int64{hs2.ID}, idOf[RegressHasLinkThing])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_ticket_22998
func TestGenericRegressTicket22998(t *testing.T) {
	cancel := orm.OnPreDelete(func(_ context.Context, node *RegressNode) error {
		return &orm.ProtectedError{Message: "Not allowed to delete.", ProtectedObjects: []any{*node}}
	})
	defer cancel()
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		related := mustCreate(t, db, &RegressRelated{})
		content := mustCreate(t, db, &RegressContent{RelatedObj: fkOf[RegressRelated](related.ID)})
		node := &RegressNode{}
		setObject(t, db, node, "content", content)
		mustCreate(t, db, node)
		// Deleting related cascades to content and then to node, whose
		// pre_delete handler prevents the deletion.
		_, err := orm.Query[RegressRelated](db).Filter("pk", related.ID).Delete(context.Background())
		if !errors.Is(err, orm.ErrProtected) {
			t.Fatalf("delete error %v, want a protected error", err)
		}
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_generic_reverse_relation_with_mti
func TestGenericRegressReverseRelationFilter(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		place := mustCreate(t, db, &RegressPlace{Name: "Test Place"})
		link := createWithObject(t, db, &RegressLink{}, place)
		djangotest.AssertQuerySetEqual(t, orm.Query[RegressLink](db).Filter("places", place),
			[]int64{link.ID}, idOf[RegressLink], djangotest.Unordered())
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_generic_reverse_relation_with_abc
func TestGenericRegressReverseRelationAccessor(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		thing := mustCreate(t, db, &RegressHasLinkThing{})
		link := createWithObject(t, db, &RegressLink{}, thing)
		djangotest.AssertQuerySetEqual(t, orm.Related[RegressHasLinkThing](db, link, "targets"),
			[]int64{thing.ID}, idOf[RegressHasLinkThing], djangotest.Unordered())
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_generic_reverse_relation_exclude_filter
func TestGenericRegressReverseRelationExcludeFilter(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		place1 := mustCreate(t, db, &RegressPlace{Name: "Test Place 1"})
		place2 := mustCreate(t, db, &RegressPlace{Name: "Test Place 2"})
		createWithObject(t, db, &RegressLink{}, place1)
		link2 := createWithObject(t, db, &RegressLink{}, place2)
		links := orm.Query[RegressLink](db)
		djangotest.AssertQuerySetEqual(t, links.Filter(orm.Not(orm.Q("places__name", "Test Place 1"))),
			[]int64{link2.ID}, idOf[RegressLink])
		djangotest.AssertQuerySetEqual(t, links.Exclude("places__name", "Test Place 1"),
			[]int64{link2.ID}, idOf[RegressLink])
	})
}

// Ported from Django: tests/generic_relations_regress/tests.py::GenericRelationTests.test_check_cached_value_pk_different_type
func TestGenericRegressCheckCachedValuePKDifferentType(t *testing.T) {
	djangotest.EachDB(t, regressModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		board := mustCreate(t, db, &RegressBoard{Name: "some test"})
		oddrel := mustCreate(t, db, &RegressOddRelation1{Name: "clink"})
		created := createWithObject(t, db, &RegressCharLink{}, oddrel)
		charlink, err := orm.Query[RegressCharLink](db).Get(ctx, "pk", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertSameObject(t, getObject(t, db, &charlink, "content_object"), oddrel)
		boardCT, err := contenttypes.GetForModel(ctx, db, mustModel(t, db, RegressBoard{}))
		if err != nil {
			t.Fatal(err)
		}
		// The primary key of the cached object is not compared when the
		// content type does not match.
		charlink.ObjectID = board.Name
		charlink.ContentType.SetPK(boardCT.ID)
		got, ok := getObject(t, db, &charlink, "content_object").(*RegressBoard)
		if !ok || got.Name != board.Name {
			t.Fatalf("object %#v, want the board %q", got, board.Name)
		}
	})
}

// fkOf returns a foreign key to the row with primary key pk.
func fkOf[T any](pk int64) orm.FK[T] {
	var fk orm.FK[T]
	fk.SetPK(pk)
	return fk
}

// No Go counterpart: tests/generic_relations_regress/tests.py::GenericRelationTests.test_inherited_models_content_type
// and test_coerce_object_id_remote_field_cache_persistence use multi-table
// inheritance (Restaurant and Cafe), which the orm does not have.
//
// No Go counterpart: tests/generic_relations_regress/tests.py::GenericRelationTests.test_target_model_len_zero
// and test_target_model_bool_false give a model __len__ and __bool__, which
// Go structs do not have.
//
// No Go counterpart: tests/generic_relations_regress/tests.py::GenericRelationTests.test_editable_generic_rel
// subclasses GenericRelation to make it editable with save_form_data.
//
// No Go counterpart: tests/generic_relations_regress/tests.py::GenericRelationTests.test_ticket_22982
// checks the repr of a Python manager.
//
// No Go counterpart: tests/generic_relations_regress/tests.py::GenericRelationTests.test_filter_on_related_proxy_model
// follows a generic relation to a proxy model, and the orm has no proxy
// models.
