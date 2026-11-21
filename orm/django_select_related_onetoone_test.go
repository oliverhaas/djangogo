package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/select_related_onetoone/models.py.
// Model-inheritance models (AdvancedUserStat, Child1..Child4) are out of scope.

type sroUser struct {
	orm.RelCache
	ID       int64
	Username string `orm:"max_length=100"`
	Email    string `orm:"type=email"`
}

type sroUserProfile struct {
	ID    int64
	User  orm.O2O[sroUser] `orm:"on_delete=cascade;related_name=userprofile"`
	City  string           `orm:"max_length=100"`
	State string           `orm:"max_length=2"`
}

type sroUserStatResult struct {
	ID      int64
	Results string `orm:"max_length=50"`
}

type sroUserStat struct {
	orm.RelCache
	User    orm.O2O[sroUser] `orm:"pk;on_delete=cascade;related_name=userstat"`
	Posts   int64
	Results orm.FK[sroUserStatResult] `orm:"on_delete=cascade"`
}

type sroStatDetails struct {
	ID        int64
	BaseStats orm.O2O[sroUserStat] `orm:"on_delete=cascade;related_name=statdetails"`
	Comments  int64
}

type sroImage struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=100"`
}

type sroProduct struct {
	ID    int64
	Name  string            `orm:"max_length=100"`
	Image orm.O2O[sroImage] `orm:"null;on_delete=set_null;related_name=product"`
}

type sroLinkedList struct {
	orm.RelCache
	ID           int64
	Name         string                 `orm:"max_length=50"`
	PreviousItem orm.O2O[sroLinkedList] `orm:"null;blank;on_delete=cascade;related_name=next_item"`
}

// sroModels returns the model list for the select_related_onetoone test app.
func sroModels() []any {
	return []any{djangotest.App("select_related_onetoone",
		&sroUser{}, &sroUserProfile{}, &sroUserStatResult{}, &sroUserStat{},
		&sroStatDetails{}, &sroImage{}, &sroProduct{}, &sroLinkedList{},
	)}
}

// sroSetUp creates the shared fixtures from ReverseSelectRelatedTestCase.setUpTestData.
func sroSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	user := create(t, db, &sroUser{Username: "test", Email: "test@example.com"})
	create(t, db, &sroUserProfile{User: o2oTo[sroUser](user.ID), State: "KS", City: "Lawrence"})
	results := create(t, db, &sroUserStatResult{Results: "first results"})
	userstat := create(t, db, &sroUserStat{
		User:    o2oTo[sroUser](user.ID),
		Posts:   150,
		Results: fkTo[sroUserStatResult](results.ID),
	})
	create(t, db, &sroStatDetails{BaseStats: o2oTo[sroUserStat](userstat.User.PK()), Comments: 259})

	create(t, db, &sroUser{Username: "bob", Email: "bob@example.com"})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_basic
func TestSelectRelatedO2OBasic(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userprofile").Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			prof, err := orm.RelatedOne[sroUserProfile](ctx, db, &u, "userprofile")
			if err != nil {
				t.Fatalf("RelatedOne userprofile: %v", err)
			}
			djangotest.AssertEqual(t, prof.State, "KS")
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_follow_next_level
func TestSelectRelatedO2OFollowNextLevel(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userstat__results").Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			stat, err := orm.RelatedOne[sroUserStat](ctx, db, &u, "userstat")
			if err != nil {
				t.Fatalf("RelatedOne userstat: %v", err)
			}
			djangotest.AssertEqual(t, stat.Posts, int64(150))
			res, err := stat.Results.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Results.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, res.Results, "first results")
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_follow_two
func TestSelectRelatedO2OFollowTwo(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userprofile", "userstat").Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			prof, err := orm.RelatedOne[sroUserProfile](ctx, db, &u, "userprofile")
			if err != nil {
				t.Fatalf("userprofile: %v", err)
			}
			stat, err := orm.RelatedOne[sroUserStat](ctx, db, &u, "userstat")
			if err != nil {
				t.Fatalf("userstat: %v", err)
			}
			djangotest.AssertEqual(t, prof.State, "KS")
			djangotest.AssertEqual(t, stat.Posts, int64(150))
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_follow_two_next_level
func TestSelectRelatedO2OFollowTwoNextLevel(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userstat__results", "userstat__statdetails").Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			stat, err := orm.RelatedOne[sroUserStat](ctx, db, &u, "userstat")
			if err != nil {
				t.Fatalf("userstat: %v", err)
			}
			res, err := stat.Results.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("results: %v", err)
			}
			details, err := orm.RelatedOne[sroStatDetails](ctx, db, stat, "statdetails")
			if err != nil {
				t.Fatalf("statdetails: %v", err)
			}
			djangotest.AssertEqual(t, res.Results, "first results")
			djangotest.AssertEqual(t, details.Comments, int64(259))
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_forward_and_back
func TestSelectRelatedO2OForwardAndBack(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			stat, err := orm.Query[sroUserStat](db).SelectRelated("user__userprofile").Get(ctx, "user__username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			u, err := stat.User.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("stat.User.Fetch: %v", err)
			}
			prof, err := orm.RelatedOne[sroUserProfile](ctx, db, u, "userprofile")
			if err != nil {
				t.Fatalf("u.userprofile: %v", err)
			}
			backStat, err := orm.RelatedOne[sroUserStat](ctx, db, u, "userstat")
			if err != nil {
				t.Fatalf("u.userstat: %v", err)
			}
			djangotest.AssertEqual(t, prof.State, "KS")
			djangotest.AssertEqual(t, backStat.Posts, int64(150))
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_back_and_forward
func TestSelectRelatedO2OBackAndForward(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userstat").Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			stat, err := orm.RelatedOne[sroUserStat](ctx, db, &u, "userstat")
			if err != nil {
				t.Fatalf("u.userstat: %v", err)
			}
			backUser, err := stat.User.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("stat.User.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, backUser.Username, "test")
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_not_followed_by_default
func TestSelectRelatedO2ONotFollowedByDefault(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 2, func() {
			u, err := orm.Query[sroUser](db).SelectRelated().Get(ctx, "username", "test")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			stat, err := orm.RelatedOne[sroUserStat](ctx, db, &u, "userstat")
			if err != nil {
				t.Fatalf("userstat: %v", err)
			}
			djangotest.AssertEqual(t, stat.Posts, int64(150))
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_nullable_relation
func TestSelectRelatedO2ONullableRelation(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		im := create(t, db, &sroImage{Name: "imag1"})
		create(t, db, &sroProduct{Name: "Django Plushie", Image: o2oTo[sroImage](im.ID)})
		create(t, db, &sroProduct{Name: "Talking Django Plushie"})

		djangotest.AssertNumQueries(t, db, 1, func() {
			rows := mustAll(t, orm.Query[sroProduct](db).SelectRelated("image").OrderBy("name"))
			djangotest.AssertEqual(t, len(rows), 2)
			djangotest.AssertEqual(t, rows[0].Name, "Django Plushie")
			djangotest.AssertEqual(t, rows[1].Name, "Talking Django Plushie")

			img0, err := rows[0].Image.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("rows[0].Image.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, img0.Name, "imag1")

			img1, loaded := rows[1].Image.Object()
			djangotest.AssertEqual(t, loaded, false)
			djangotest.AssertEqual(t, img1, nil)
			djangotest.AssertEqual(t, rows[1].Image.PK(), int64(0))
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_missing_reverse
func TestSelectRelatedO2OMissingReverse(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		sroSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			u, err := orm.Query[sroUser](db).SelectRelated("userprofile").Get(ctx, "username", "bob")
			if err != nil {
				t.Fatalf("Get bob: %v", err)
			}
			_, err = orm.RelatedOne[sroUserProfile](ctx, db, &u, "userprofile")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_nullable_missing_reverse
func TestSelectRelatedO2ONullableMissingReverse(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &sroImage{Name: "imag1"})
		djangotest.AssertNumQueries(t, db, 1, func() {
			img, err := orm.Query[sroImage](db).SelectRelated("product").Get(ctx)
			if err != nil {
				t.Fatalf("Get image: %v", err)
			}
			_, err = orm.RelatedOne[sroProduct](ctx, db, &img, "product")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedTestCase.test_self_relation
func TestSelectRelatedO2OSelfRelation(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		item1 := create(t, db, &sroLinkedList{Name: "item1"})
		create(t, db, &sroLinkedList{Name: "item2", PreviousItem: o2oTo[sroLinkedList](item1.ID)})

		djangotest.AssertNumQueries(t, db, 1, func() {
			item1DB, err := orm.Query[sroLinkedList](db).SelectRelated("next_item").Get(ctx, "name", "item1")
			if err != nil {
				t.Fatalf("Get item1: %v", err)
			}
			next, err := orm.RelatedOne[sroLinkedList](ctx, db, &item1DB, "next_item")
			if err != nil {
				t.Fatalf("RelatedOne next_item: %v", err)
			}
			djangotest.AssertEqual(t, next.Name, "item2")
		})
	})
}

// Ported from Django: tests/select_related_onetoone/tests.py::ReverseSelectRelatedValidationTests.test_reverse_related_validation
func TestSelectRelatedO2OValidation(t *testing.T) {
	djangotest.EachDB(t, sroModels(), func(t *testing.T, db *orm.DB) {
		selAssertSelectRelatedError[sroUser](t, db, "foobar", selInv("foobar", "userprofile, userstat"))
		selAssertSelectRelatedError[sroUser](t, db, "username", selNonRel("username", "userprofile, userstat"))
	})
}
