package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/m2m_regress/models.py.

type m2mrSelfRefer struct {
	ID         int64
	Name       string `orm:"max_length=10"`
	References orm.M2M[m2mrSelfRefer]
	Related    orm.M2M[m2mrSelfRefer]
}

type m2mrTag struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

type m2mrEntry struct {
	ID      int64
	Name    string `orm:"max_length=10"`
	Topics  orm.M2M[m2mrTag]
	Related orm.M2M[m2mrTag] `orm:"related_name=similar"`
}

type m2mrLine struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type m2mrWorksheet struct {
	ID    string            `orm:"pk;max_length=100"`
	Lines orm.M2M[m2mrLine] `orm:"blank"`
}

type m2mrPost struct {
	ID             int64
	PrimaryLines   orm.M2M[m2mrLine] `orm:"related_name=+"`
	SecondaryLines orm.M2M[m2mrLine] `orm:"related_name=+"`
}

// m2mrUserB lives in a separate app with the same Go struct name pattern tested
// via explicit app registration for cross-app same-name M2M column naming.
type m2mrShared struct {
	ID   int64
	Name string `orm:"max_length=30"`
}

// m2mrModels returns the model list for the m2m_regress test app.
func m2mrModels() []any {
	return []any{djangotest.App("m2m_regress",
		&m2mrSelfRefer{}, &m2mrTag{}, &m2mrEntry{},
		&m2mrLine{}, &m2mrWorksheet{}, &m2mrPost{}, &m2mrShared{},
	)}
}

// Ported from Django: tests/m2m_regress/tests.py::M2MRegressionTests.test_multiple_m2m
func TestM2MRegressMultipleM2M(t *testing.T) {
	djangotest.EachDB(t, m2mrModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		s1 := create(t, db, &m2mrSelfRefer{Name: "s1"})
		s2 := create(t, db, &m2mrSelfRefer{Name: "s2"})
		s3 := create(t, db, &m2mrSelfRefer{Name: "s3"})
		if err := orm.Many[m2mrSelfRefer](db, s1, "references").Add(ctx, s2); err != nil {
			t.Fatalf("s1.references.Add: %v", err)
		}
		if err := orm.Many[m2mrSelfRefer](db, s1, "related").Add(ctx, s3); err != nil {
			t.Fatalf("s1.related.Add: %v", err)
		}

		e1 := create(t, db, &m2mrEntry{Name: "e1"})
		t1 := create(t, db, &m2mrTag{Name: "t1"})
		t2 := create(t, db, &m2mrTag{Name: "t2"})
		if err := orm.Many[m2mrTag](db, e1, "topics").Add(ctx, t1); err != nil {
			t.Fatalf("e1.topics.Add: %v", err)
		}
		if err := orm.Many[m2mrTag](db, e1, "related").Add(ctx, t2); err != nil {
			t.Fatalf("e1.related.Add: %v", err)
		}

		refs, _ := orm.Many[m2mrSelfRefer](db, s1, "references").All(ctx)
		djangotest.AssertEqual(t, len(refs), 1)
		djangotest.AssertEqual(t, refs[0].Name, "s2")

		// Symmetrical self-ref adds mirror entry on s2.
		s2Refs, _ := orm.Many[m2mrSelfRefer](db, s2, "references").All(ctx)
		djangotest.AssertEqual(t, len(s2Refs), 1)
		djangotest.AssertEqual(t, s2Refs[0].Name, "s1")

		rels, _ := orm.Many[m2mrSelfRefer](db, s1, "related").All(ctx)
		djangotest.AssertEqual(t, len(rels), 1)
		djangotest.AssertEqual(t, rels[0].Name, "s3")

		topics, _ := orm.Many[m2mrTag](db, e1, "topics").All(ctx)
		djangotest.AssertEqual(t, len(topics), 1)
		djangotest.AssertEqual(t, topics[0].Name, "t1")

		eRels, _ := orm.Many[m2mrTag](db, e1, "related").All(ctx)
		djangotest.AssertEqual(t, len(eRels), 1)
		djangotest.AssertEqual(t, eRels[0].Name, "t2")
	})
}

// Ported from Django: tests/m2m_regress/tests.py::M2MRegressionTests.test_internal_related_name_not_in_error_msg
func TestM2MRegressInternalRelatedNameNotInErrorMsg(t *testing.T) {
	djangotest.EachDB(t, m2mrModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[m2mrSelfRefer](db).Filter("porcupine", "fred").All(ctx)
		if !errors.Is(err, orm.ErrFieldError) {
			t.Fatalf("error = %v, want ErrFieldError", err)
		}
		want := "Choices are: id, name, references, related"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want substring %q", err.Error(), want)
		}
	})
}

// Ported from Django: tests/m2m_regress/tests.py::M2MRegressionTests.test_m2m_pk_field_type
func TestM2MRegressPKFieldType(t *testing.T) {
	djangotest.EachDB(t, m2mrModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		w := &m2mrWorksheet{ID: "abc"}
		if err := orm.Query[m2mrWorksheet](db).Save(ctx, w); err != nil {
			t.Fatalf("save worksheet: %v", err)
		}
		l := create(t, db, &m2mrLine{Name: "line1"})
		if err := orm.Many[m2mrLine](db, w, "lines").Add(ctx, l); err != nil {
			t.Fatalf("add line to char-pk worksheet: %v", err)
		}
		lines, err := orm.Many[m2mrLine](db, w, "lines").All(ctx)
		if err != nil {
			t.Fatalf("all lines: %v", err)
		}
		djangotest.AssertEqual(t, len(lines), 1)
		djangotest.AssertEqual(t, lines[0].Name, "line1")

		if _, err := orm.Query[m2mrWorksheet](db).Filter("pk", "abc").Delete(ctx); err != nil {
			t.Fatalf("delete worksheet: %v", err)
		}
	})
}

// Ported from Django: tests/m2m_regress/tests.py::M2MRegressionTests.test_multiple_forwards_only_m2m
func TestM2MRegressMultipleForwardsOnlyM2M(t *testing.T) {
	djangotest.EachDB(t, m2mrModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		foo := create(t, db, &m2mrLine{Name: "foo"})
		bar := create(t, db, &m2mrLine{Name: "bar"})
		post := create(t, db, &m2mrPost{})

		if err := orm.Many[m2mrLine](db, post, "primary_lines").Add(ctx, foo); err != nil {
			t.Fatalf("primary_lines.Add: %v", err)
		}
		if err := orm.Many[m2mrLine](db, post, "secondary_lines").Add(ctx, bar); err != nil {
			t.Fatalf("secondary_lines.Add: %v", err)
		}
		prim, _ := orm.Many[m2mrLine](db, post, "primary_lines").All(ctx)
		djangotest.AssertEqual(t, len(prim), 1)
		djangotest.AssertEqual(t, prim[0].Name, "foo")

		sec, _ := orm.Many[m2mrLine](db, post, "secondary_lines").All(ctx)
		djangotest.AssertEqual(t, len(sec), 1)
		djangotest.AssertEqual(t, sec[0].Name, "bar")
	})
}
