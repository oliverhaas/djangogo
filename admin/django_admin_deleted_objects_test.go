package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/admin_views/models.py for
// AdminViewDeletedObjectsTest and AdminConsecutiveWhiteSpaceObjectDisplayTest.

// Villain is Django's admin_views.Villain.
type Villain struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

func (v Villain) String() string { return v.Name }

// Plot is Django's admin_views.Plot without its generic relation.
type Plot struct {
	ID         int64
	Name       string          `orm:"max_length=100"`
	TeamLeader orm.FK[Villain] `orm:"on_delete=cascade;related_name=lead_plots"`
	Contact    orm.FK[Villain] `orm:"on_delete=cascade;related_name=contact_plots"`
}

func (p Plot) String() string { return p.Name }

// PlotDetails is Django's admin_views.PlotDetails.
type PlotDetails struct {
	ID      int64
	Details string        `orm:"max_length=100"`
	Plot    orm.O2O[Plot] `orm:"null;blank;on_delete=cascade"`
}

func (p PlotDetails) String() string { return p.Details }

// SecretHideout is Django's admin_views.SecretHideout. It is not registered
// with the admin.
type SecretHideout struct {
	ID       int64
	Location string          `orm:"max_length=100"`
	Villain  orm.FK[Villain] `orm:"on_delete=cascade"`
}

func (h SecretHideout) String() string { return h.Location }

// CyclicOne is Django's admin_views.CyclicOne. Two is nullable here: the
// foreign keys of the ORM are not deferrable, so the cycle is built in two
// steps.
type CyclicOne struct {
	ID   int64
	Name string            `orm:"max_length=25"`
	Two  orm.FK[CyclicTwo] `orm:"null;on_delete=cascade"`
}

func (c CyclicOne) String() string { return c.Name }

// CyclicTwo is Django's admin_views.CyclicTwo.
type CyclicTwo struct {
	ID   int64
	Name string            `orm:"max_length=25"`
	One  orm.FK[CyclicOne] `orm:"on_delete=cascade"`
}

func (c CyclicTwo) String() string { return c.Name }

// AlbumSong is Django's admin_views.Song, whose name another test model has.
type AlbumSong struct {
	ID    int64
	Name  string        `orm:"max_length=20"`
	Album orm.FK[Album] `orm:"on_delete=restrict"`
}

func (s AlbumSong) String() string { return s.Name }

// Meta names AlbumSong as Django's Song.
func (AlbumSong) Meta() orm.Meta {
	return orm.Meta{VerboseName: "song", VerboseNamePlural: "songs"}
}

// CoverLetter is Django's admin_views.CoverLetter.
type CoverLetter struct {
	ID          int64
	Author      string    `orm:"max_length=30"`
	DateWritten *orm.Date `orm:"null;blank"`
}

func (c CoverLetter) String() string { return c.Author }

// deletedObjectsModels returns the models of these tests.
func deletedObjectsModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		djangotest.App("admin_views",
			&Villain{}, &Plot{}, &PlotDetails{}, &SecretHideout{},
			&ActQuestion{}, &ActAnswer{}, &ActBook{},
			&Album{}, &AlbumSong{}, &CoverLetter{},
		),
	}
}

// deletedObjectsData holds the rows of Django's
// AdminViewDeletedObjectsTest.setUpTestData that the ported tests use.
type deletedObjectsData struct {
	superuser, deleteuser *auth.User
	v1, v2                *Villain
	pl1, pl2, pl3         *Plot
	pd1                   *PlotDetails
}

// mustCreate creates obj or fails the test.
func mustCreate[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("Create %T: %v", obj, err)
	}
	return obj
}

// newPlot returns a plot with its team leader and contact.
func newPlot(name string, leader, contact *Villain) *Plot {
	p := &Plot{Name: name}
	p.TeamLeader.SetPK(leader.ID)
	p.Contact.SetPK(contact.ID)
	return p
}

// newDeletedObjectsData creates the rows of setUpTestData.
func newDeletedObjectsData(t *testing.T, db *orm.DB) *deletedObjectsData {
	t.Helper()
	d := &deletedObjectsData{
		superuser:  createTestUser(t, db, "super", "secret", true, true),
		deleteuser: createTestUser(t, db, "deleteuser", "secret", true, false),
	}
	d.v1 = mustCreate(t, db, &Villain{Name: "Adam"})
	d.v2 = mustCreate(t, db, &Villain{Name: "Sue"})
	d.pl1 = mustCreate(t, db, newPlot("World Domination", d.v1, d.v2))
	d.pl2 = mustCreate(t, db, newPlot("World Peace", d.v2, d.v2))
	d.pl3 = mustCreate(t, db, newPlot("Corn Conspiracy", d.v1, d.v1))
	d.pd1 = &PlotDetails{Details: "almost finished"}
	d.pd1.Plot.SetPK(d.pl1.ID)
	mustCreate(t, db, d.pd1)
	sh1 := &SecretHideout{Location: "underground bunker"}
	sh1.Villain.SetPK(d.v1.ID)
	mustCreate(t, db, sh1)
	return d
}

// registerDeletedObjectsModels registers the models of Django's admin_views
// site that these tests reach. SecretHideout stays unregistered.
func registerDeletedObjectsModels(site *admin.AdminSite) {
	admin.Register[Villain](site, admin.ModelAdmin{})
	admin.Register[Plot](site, admin.ModelAdmin{})
	admin.Register[PlotDetails](site, admin.ModelAdmin{})
	admin.Register[ActQuestion](site, admin.ModelAdmin{})
	admin.Register[ActAnswer](site, admin.ModelAdmin{})
	admin.Register[Album](site, admin.ModelAdmin{})
	admin.Register[AlbumSong](site, admin.ModelAdmin{})
}

// deletedObjectsEnv returns the admin site of AdminViewDeletedObjectsTest with
// its data, logged in as the superuser.
func deletedObjectsEnv(t *testing.T, db *orm.DB) (*adminTestEnv, *deletedObjectsData) {
	t.Helper()
	d := newDeletedObjectsData(t, db)
	env := newAdminTestEnv(t, db, registerDeletedObjectsModels)
	env.client.ForceLogin(d.superuser)
	return env, d
}

// createQuestion creates a question with answers that protect it.
func createQuestion(t *testing.T, db *orm.DB, question string, answers ...string) (*ActQuestion, []*ActAnswer) {
	t.Helper()
	q := mustCreate(t, db, &ActQuestion{Question: question})
	out := make([]*ActAnswer, 0, len(answers))
	for _, a := range answers {
		ans := &ActAnswer{Answer: a}
		ans.Question.SetPK(q.ID)
		out = append(out, mustCreate(t, db, ans))
	}
	return q, out
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_nesting
func TestDeletedObjectsNesting(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, d := deletedObjectsEnv(t, db)
		pattern := regexp.MustCompile(fmt.Sprintf(
			`<li>Plot: <a href="%s">World Domination</a>\s*<ul>\s*`+
				`<li>Plot details: <a href="%s">almost finished</a>`,
			regexp.QuoteMeta(mustReverse(t, env.router, "admin:admin_views_plot_change", d.pl1.ID)),
			regexp.QuoteMeta(mustReverse(t, env.router, "admin:admin_views_plotdetails_change", d.pd1.ID)),
		))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_villain_delete", d.v1.ID))
		if !pattern.Match(resp.Body) {
			t.Errorf("the delete page does not match %s:\n%s", pattern, resp.Body)
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_cyclic
//
// FIXME-PORT: the test runs on SQLite only. djangotest creates each table
// with its foreign key constraints, so Postgres cannot create CyclicOne and
// CyclicTwo, which reference each other. Django adds the constraints after
// the tables, DEFERRABLE INITIALLY DEFERRED.
func TestDeletedObjectsCyclic(t *testing.T) {
	db := djangotest.NewDB(t, auth.App{}, contenttypes.App{}, djangotest.App("admin_views", &CyclicOne{}, &CyclicTwo{}))
	superuser := createTestUser(t, db, "super", "secret", true, true)
	cy1 := mustCreate(t, db, &CyclicOne{Name: "I am recursive"})
	cy2 := &CyclicTwo{Name: "I am recursive too"}
	cy2.One.SetPK(cy1.ID)
	mustCreate(t, db, cy2)
	cy1.Two.SetPK(cy2.ID)
	if err := orm.Query[CyclicOne](db).Save(context.Background(), cy1); err != nil {
		t.Fatalf("Save cy1: %v", err)
	}
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[CyclicOne](site, admin.ModelAdmin{})
		admin.Register[CyclicTwo](site, admin.ModelAdmin{})
	})
	env.client.ForceLogin(superuser)

	one := fmt.Sprintf(`<li>Cyclic one: <a href="%s">I am recursive</a>`,
		mustReverse(t, env.router, "admin:admin_views_cyclicone_change", cy1.ID))
	two := fmt.Sprintf(`<li>Cyclic two: <a href="%s">I am recursive too</a>`,
		mustReverse(t, env.router, "admin:admin_views_cyclictwo_change", cy2.ID))
	resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_cyclicone_delete", cy1.ID))
	djangotest.AssertContains(t, resp, one, djangotest.Count(1))
	djangotest.AssertContains(t, resp, two, djangotest.Count(1))
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_perms_needed
func TestDeletedObjectsPermsNeeded(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, d := deletedObjectsEnv(t, db)
		env.client.Logout()
		grantPerm(t, db, d.deleteuser, "admin_views", "delete_plot")
		env.client.ForceLogin(d.deleteuser)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_plot_delete", d.pl1.ID))
		djangotest.AssertContains(t, resp,
			"your account doesn't have permission to delete the following types of objects")
		djangotest.AssertContains(t, resp, "<li>plot details</li>")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_protected
func TestDeletedObjectsProtected(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := deletedObjectsEnv(t, db)
		q, answers := createQuestion(t, db, "Why?", "Because.", "Yes.")
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actquestion_delete", q.ID))
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<li>Answer: <a href="%s">Because.</a></li>`,
			mustReverse(t, env.router, "admin:admin_views_actanswer_change", answers[0].ID)))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<li>Answer: <a href="%s">Yes.</a></li>`,
			mustReverse(t, env.router, "admin:admin_views_actanswer_change", answers[1].ID)))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_protected_zero_display
func TestDeletedObjectsProtectedZeroDisplay(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		d := newDeletedObjectsData(t, db)
		// Django's zero_display_site with QuestionAdminZeroDisplay.
		zero := 0
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActQuestion](site, admin.ModelAdmin{DeleteConfirmationMaxDisplay: &zero})
		})
		env.client.ForceLogin(d.superuser)
		q, _ := createQuestion(t, db, "Why?", "Because.")
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actquestion_delete", q.ID))
		djangotest.AssertContains(t, resp, "would require deleting some protected related objects.")
		djangotest.AssertNotContains(t, resp, `id="deleted-objects"`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_post_delete_protected
func TestDeletedObjectsPostDeleteProtected(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := deletedObjectsEnv(t, db)
		q, _ := createQuestion(t, db, "Why?", "Because.")
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_actquestion_delete", q.ID),
			url.Values{"post": {"yes"}})
		n, err := orm.Query[ActQuestion](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
	})
}

// createAlbumWithSong creates an album with a song that restricts its
// deletion.
func createAlbumWithSong(t *testing.T, db *orm.DB) (*Album, *AlbumSong) {
	t.Helper()
	album := mustCreate(t, db, &Album{Title: "Amaryllis"})
	song := &AlbumSong{Name: "Unity"}
	song.Album.SetPK(album.ID)
	return album, mustCreate(t, db, song)
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_restricted
func TestDeletedObjectsRestricted(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := deletedObjectsEnv(t, db)
		album, song := createAlbumWithSong(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_album_delete", album.ID))
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<li>Song: <a href="%s">Unity</a></li>`,
			mustReverse(t, env.router, "admin:admin_views_albumsong_change", song.ID)))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_post_delete_restricted
func TestDeletedObjectsPostDeleteRestricted(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, _ := deletedObjectsEnv(t, db)
		album, _ := createAlbumWithSong(t, db)
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_album_delete", album.ID),
			url.Values{"post": {"yes"}})
		n, err := orm.Query[Album](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_not_registered
func TestDeletedObjectsNotRegistered(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, d := deletedObjectsEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_villain_delete", d.v1.ID))
		djangotest.AssertContains(t, resp, "<li>Secret hideout: underground bunker", djangotest.Count(1))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_multiple_fkeys_to_same_model
func TestDeletedObjectsMultipleFKeysToSameModel(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, d := deletedObjectsEnv(t, db)
		shouldContain := fmt.Sprintf(`<li>Plot: <a href="%s">World Domination</a>`,
			mustReverse(t, env.router, "admin:admin_views_plot_change", d.pl1.ID))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_villain_delete", d.v1.ID))
		djangotest.AssertContains(t, resp, shouldContain)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_villain_delete", d.v2.ID))
		djangotest.AssertContains(t, resp, shouldContain)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_multiple_fkeys_to_same_instance
func TestDeletedObjectsMultipleFKeysToSameInstance(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env, d := deletedObjectsEnv(t, db)
		shouldContain := fmt.Sprintf(`<li>Plot: <a href="%s">World Peace</a></li>`,
			mustReverse(t, env.router, "admin:admin_views_plot_change", d.pl2.ID))
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_villain_delete", d.v2.ID))
		djangotest.AssertContains(t, resp, shouldContain, djangotest.Count(1))
	})
}

// FIXME-PORT: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_inheritance
// needs multi-table inheritance (SuperVillain), which the ORM does not support.

// FIXME-PORT: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_generic_relations
// and test_generic_relations_with_related_query_name need GenericForeignKey,
// which contenttypes does not have.

// bookAdminDeletedObjects is Django's BookAdmin.get_deleted_objects.
func bookAdminDeletedObjects(*http.Request, []any) (*admin.DeletedObjects, error) {
	return &admin.DeletedObjects{
		Items: []admin.DeletedObject{
			{HTML: "a deletable object"}, {HTML: "another object"}, {HTML: "last object"},
		},
		ModelCount: []admin.DeletedModelCount{{Name: "books", Count: 1}},
	}, nil
}

// bookAdminEnv returns a site with ActBook registered as Django's BookAdmin
// with delete_confirmation_max_display set to maxDisplay, logged in as a
// superuser.
func bookAdminEnv(t *testing.T, db *orm.DB, maxDisplay int) *adminTestEnv {
	t.Helper()
	super := createTestUser(t, db, "super", "secret", true, true)
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[ActBook](site, admin.ModelAdmin{
			DeleteConfirmationMaxDisplay: &maxDisplay,
			GetDeletedObjects:            bookAdminDeletedObjects,
		})
	})
	env.client.ForceLogin(super)
	return env
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_delete_view_uses_get_deleted_objects
// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_delete_view_uses_delete_confirmation_max_display
func TestDeleteViewUsesGetDeletedObjects(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := bookAdminEnv(t, db, 1)
		book := mustCreate(t, db, &ActBook{Name: "Test Book"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actbook_delete", book.ID))
		// BookAdmin.get_deleted_objects() returns custom text.
		djangotest.AssertContains(t, resp, "a deletable object")
		djangotest.AssertContains(t, resp, "…and 2 more objects.")
		djangotest.AssertNotContains(t, resp, "another object")
		djangotest.AssertNotContains(t, resp, "last object")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewDeletedObjectsTest.test_delete_view_hides_objects_when_delete_confirmation_max_display_is_zero
func TestDeleteViewHidesObjectsWhenMaxDisplayIsZero(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := bookAdminEnv(t, db, 0)
		book := mustCreate(t, db, &ActBook{Name: "Test Book"})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actbook_delete", book.ID))
		djangotest.AssertNotContains(t, resp, "<h2>Objects</h2>")
		djangotest.AssertNotContains(t, resp, `id="deleted-objects"`)
	})
}

// whitespaceEnv holds the data of Django's
// AdminConsecutiveWhiteSpaceObjectDisplayTest.
type whitespaceEnv struct {
	*adminTestEnv
	user       *auth.User
	obj        *CoverLetter
	changeLink string
}

// newWhitespaceEnv creates a superuser and a cover letter whose labels are
// only whitespace, and logs the superuser in.
func newWhitespaceEnv(t *testing.T, db *orm.DB) *whitespaceEnv {
	t.Helper()
	user := createTestUser(t, db, "   ", "secret", true, true)
	obj := mustCreate(t, db, &CoverLetter{Author: "             "})
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[auth.User](site, admin.UserAdmin(db))
		admin.Register[ActQuestion](site, admin.ModelAdmin{})
		admin.Register[ActAnswer](site, admin.ModelAdmin{})
		// Django's CoverLetterAdmin: formfield_overrides with strip=False.
		admin.Register[CoverLetter](site, admin.ModelAdmin{
			FormOptions: []forms.ModelFormOption{
				forms.WithFormFieldCallback(func(_ *orm.Field, f forms.Field) forms.Field {
					if cf, ok := f.(*forms.CharField); ok {
						cf.Strip = false
					}
					return f
				}),
			},
		})
	})
	env.client.ForceLogin(user)
	return &whitespaceEnv{
		adminTestEnv: env,
		user:         user,
		obj:          obj,
		changeLink:   mustReverse(t, env.router, "admin:admin_views_coverletter_change", obj.ID),
	}
}

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_breadcrumbs
func TestDisplayConsecutiveWhitespaceObjectInBreadcrumbs(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		userChangeLink := mustReverse(t, env.router, "admin:auth_user_change", env.user.ID)
		cases := []struct{ url, expectedBreadcrumbs string }{
			{
				env.changeLink,
				fmt.Sprintf(`<li><a href="%s">Cover letters</a></li><li aria-current="page">-</li>`,
					mustReverse(t, env.router, "admin:admin_views_coverletter_changelist")),
			},
			{
				mustReverse(t, env.router, "admin:admin_views_coverletter_delete", env.obj.ID),
				fmt.Sprintf(`<li><a href="%s">-</a></li><li aria-current="page">Delete</li>`, env.changeLink),
			},
			{
				mustReverse(t, env.router, "admin:admin_views_coverletter_history", env.obj.ID),
				fmt.Sprintf(`<li><a href="%s">-</a></li><li aria-current="page">History</li>`, env.changeLink),
			},
			{
				mustReverse(t, env.router, "admin:auth_user_password_change", env.user.ID),
				fmt.Sprintf(`<li><a href="%s">-</a></li><li aria-current="page">Change password</li>`, userChangeLink),
			},
		}
		for _, tc := range cases {
			t.Run(tc.url, func(t *testing.T) {
				resp := env.client.Get(tc.url)
				djangotest.AssertContains(t, resp, tc.expectedBreadcrumbs, djangotest.HTML())
			})
		}
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_delete_confirmation_page
func TestDisplayConsecutiveWhitespaceObjectInDeleteConfirmationPage(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_coverletter_delete", env.obj.ID))
		djangotest.AssertContains(t, resp, "Are you sure you want to delete the cover letter “-”?")

		// delete protected case
		q, _ := createQuestion(t, db, "    ", "Because.")
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_actquestion_delete", q.ID))
		djangotest.AssertContains(t, resp,
			"Deleting the question “-” would require deleting the following protected related objects")

		// delete forbidden case
		noPermsUser := createTestUser(t, db, "no-perm", "secret", true, false)
		grantPerm(t, db, noPermsUser, "admin_views", "view_actquestion")
		grantPerm(t, db, noPermsUser, "admin_views", "delete_actquestion")
		env.client.ForceLogin(noPermsUser)
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_actquestion_delete", q.ID))
		djangotest.AssertContains(t, resp,
			"Deleting the question “-” would result in deleting related objects, "+
				"but your account doesn't have permission to delete the following types of objects")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_changelist
func TestDisplayConsecutiveWhitespaceObjectInChangelist(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_coverletter_changelist"))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<a href="%s">-</a>`, env.changeLink))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_deleted_object
func TestDisplayConsecutiveWhitespaceObjectInDeletedObject(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_coverletter_delete", env.obj.ID))
		djangotest.AssertContains(t, resp, fmt.Sprintf(
			`<ul id="deleted-objects"><li>Cover letter: <a href="%s">-</a></li></ul>`, env.changeLink),
			djangotest.HTML())
	})
}

// FIXME-PORT: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_recent_action
// needs LogEntry, which the admin does not have yet.

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_messages
func TestDisplayConsecutiveWhitespaceObjectInMessages(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		ctx := context.Background()
		for _, button := range []string{"_save", "_continue", "_addanother"} {
			body := url.Values{"author": {env.obj.Author}, button: {"1"}}
			t.Run(button, func(t *testing.T) {
				resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_coverletter_add"),
					body, djangotest.Follow())
				latest, err := orm.Query[CoverLetter](db).OrderBy("-id").First(ctx)
				if err != nil {
					t.Fatalf("latest cover letter: %v", err)
				}
				changeLink := mustReverse(t, env.router, "admin:admin_views_coverletter_change", latest.ID)
				djangotest.AssertContains(t, resp, fmt.Sprintf(
					`The cover letter “<a href="%s">-</a>” was added successfully.`, changeLink))
				resp = env.client.Post(changeLink,
					url.Values{"author": {"             "}, button: {"1"}}, djangotest.Follow())
				djangotest.AssertContains(t, resp, fmt.Sprintf(
					`The cover letter “<a href="%s">-</a>” was changed successfully.`, changeLink))
			})
		}

		newObj := mustCreate(t, db, &CoverLetter{Author: env.obj.Author})
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_coverletter_delete", newObj.ID),
			url.Values{"post": {"yes"}}, djangotest.Follow())
		djangotest.AssertContains(t, resp, "The cover letter “-” was deleted successfully.")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminConsecutiveWhiteSpaceObjectDisplayTest.test_display_consecutive_whitespace_object_in_sub_title
func TestDisplayConsecutiveWhitespaceObjectInSubTitle(t *testing.T) {
	djangotest.EachDB(t, deletedObjectsModels(), func(t *testing.T, db *orm.DB) {
		env := newWhitespaceEnv(t, db)
		resp := env.client.Get(env.changeLink)
		djangotest.AssertContains(t, resp, "<h2>-</h2>")
		resp = env.client.Get(mustReverse(t, env.router, "admin:admin_views_coverletter_history", env.obj.ID))
		djangotest.AssertContains(t, resp, "<h1>Change history: -</h1>")
	})
}
