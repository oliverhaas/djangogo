package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// mqTag is a target model for ModelQuerySet and RelatedMany tests, ordered by
// Name by default.
type mqTag struct {
	orm.RelCache
	ID    int64
	Name  string `orm:"max_length=30"`
	Group string `orm:"max_length=30"`
}

// Meta orders mqTag rows by name.
func (mqTag) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// mqArticle owns a forward many-to-many relation to mqTag.
type mqArticle struct {
	ID    int64
	Title string         `orm:"max_length=50"`
	Tags  orm.M2M[mqTag] `orm:"related_name=articles"`
}

// mqModels returns the models for the ModelQuerySet tests.
func mqModels() []any {
	return []any{djangotest.App("modelqs", &mqTag{}, &mqArticle{})}
}

// mqTagNames extracts the Name of each *mqTag in rows.
func mqTagNames(t *testing.T, rows []any) []string {
	t.Helper()
	names := make([]string, len(rows))
	for i, r := range rows {
		tag, ok := r.(*mqTag)
		if !ok {
			t.Fatalf("row %d has type %T, want *mqTag", i, r)
		}
		names[i] = tag.Name
	}
	return names
}

// No Django test exists: QueryModel queries a runtime *Model with default
// ordering, Filter, Exclude, OrderBy, Only, All, and Count, and returns an
// error without panicking when the model is nil.
func TestQueryModel(t *testing.T) {
	djangotest.EachDB(t, mqModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &mqTag{Name: "charlie", Group: "g1"})
		create(t, db, &mqTag{Name: "alpha", Group: "g1"})
		create(t, db, &mqTag{Name: "bravo", Group: "g2"})

		tagModel, ok := db.Registry().ModelOf(mqTag{})
		if !ok {
			t.Fatal("mqTag not registered")
		}

		base := orm.QueryModel(db, tagModel)
		djangotest.AssertEqual(t, base.Model(), tagModel)

		n, err := base.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		allRows, err := base.All(ctx)
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, mqTagNames(t, allRows), []string{"alpha", "bravo", "charlie"})

		filtered := base.Filter("group", "g1")
		nFiltered, err := filtered.Count(ctx)
		if err != nil {
			t.Fatalf("Filter Count: %v", err)
		}
		djangotest.AssertEqual(t, nFiltered, int64(2))
		filteredRows, err := filtered.OrderBy("-name").All(ctx)
		if err != nil {
			t.Fatalf("Filter OrderBy All: %v", err)
		}
		djangotest.AssertEqual(t, mqTagNames(t, filteredRows), []string{"charlie", "alpha"})

		excludedRows, err := base.Exclude("group", "g1").All(ctx)
		if err != nil {
			t.Fatalf("Exclude All: %v", err)
		}
		djangotest.AssertEqual(t, mqTagNames(t, excludedRows), []string{"bravo"})

		onlyRows, err := base.Filter("name", "alpha").Only("name").All(ctx)
		if err != nil {
			t.Fatalf("Only All: %v", err)
		}
		djangotest.AssertEqual(t, len(onlyRows), 1)
		onlyTag := onlyRows[0].(*mqTag)
		djangotest.AssertEqual(t, onlyTag.Name, "alpha")
		djangotest.AssertEqual(t, onlyTag.Group, "")
		djangotest.AssertEqual(t, orm.DeferredFields(onlyTag), []string{"group"})

		nOrig, err := base.Count(ctx)
		if err != nil {
			t.Fatalf("base Count after chains: %v", err)
		}
		djangotest.AssertEqual(t, nOrig, int64(3))

		nilQS := orm.QueryModel(db, nil)
		if _, err := nilQS.All(ctx); err == nil {
			t.Error("QueryModel(db, nil).All() = nil, want error")
		}
		if _, err := nilQS.Count(ctx); err == nil {
			t.Error("QueryModel(db, nil).Count() = nil, want error")
		}
	})
}

// No Django test exists: RelatedMany returns a ModelQuerySet of the targets
// related to an owner through a forward many-to-many *Field in the target
// model's default ordering, supporting Filter, OrderBy, Only, All, and Count.
func TestRelatedMany(t *testing.T) {
	djangotest.EachDB(t, mqModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		tCharlie := create(t, db, &mqTag{Name: "charlie", Group: "g1"})
		tAlpha := create(t, db, &mqTag{Name: "alpha", Group: "g1"})
		tBravo := create(t, db, &mqTag{Name: "bravo", Group: "g2"})
		_ = create(t, db, &mqTag{Name: "delta", Group: "g2"})

		a1 := create(t, db, &mqArticle{Title: "First"})
		a2 := create(t, db, &mqArticle{Title: "Second"})
		if err := orm.Many[mqTag](db, a1, "tags").Add(ctx, tCharlie, tAlpha, tBravo); err != nil {
			t.Fatalf("Add tags to a1: %v", err)
		}
		if err := orm.Many[mqTag](db, a2, "tags").Add(ctx, tBravo); err != nil {
			t.Fatalf("Add tags to a2: %v", err)
		}

		artModel, ok := db.Registry().ModelOf(mqArticle{})
		if !ok {
			t.Fatal("mqArticle not registered")
		}
		tagsField, ok := artModel.ManyToManyField("tags")
		if !ok {
			t.Fatal("ManyToManyField(tags) not found")
		}

		qs, err := orm.RelatedMany(db, a1, tagsField)
		if err != nil {
			t.Fatalf("RelatedMany: %v", err)
		}
		n, err := qs.Count(ctx)
		if err != nil {
			t.Fatalf("RelatedMany Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		rows, err := qs.All(ctx)
		if err != nil {
			t.Fatalf("RelatedMany All: %v", err)
		}
		djangotest.AssertEqual(t, mqTagNames(t, rows), []string{"alpha", "bravo", "charlie"})

		filteredRows, err := qs.Filter("group", "g1").OrderBy("-name").All(ctx)
		if err != nil {
			t.Fatalf("RelatedMany Filter OrderBy All: %v", err)
		}
		djangotest.AssertEqual(t, mqTagNames(t, filteredRows), []string{"charlie", "alpha"})

		onlyRows, err := qs.Filter("name", "bravo").Only("name").All(ctx)
		if err != nil {
			t.Fatalf("RelatedMany Only All: %v", err)
		}
		djangotest.AssertEqual(t, len(onlyRows), 1)
		onlyTag := onlyRows[0].(*mqTag)
		djangotest.AssertEqual(t, onlyTag.Name, "bravo")
		djangotest.AssertEqual(t, onlyTag.Group, "")
		djangotest.AssertEqual(t, orm.DeferredFields(onlyTag), []string{"group"})

		if _, err := orm.RelatedMany(db, a1, artModel.PrimaryKey()); err == nil {
			t.Error("RelatedMany with non-M2M field = nil, want error")
		}
		if _, err := orm.RelatedMany(db, &mqArticle{Title: "Unsaved"}, tagsField); err == nil {
			t.Error("RelatedMany with unsaved owner = nil, want error")
		}
	})
}
