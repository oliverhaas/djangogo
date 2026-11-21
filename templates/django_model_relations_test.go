package templates_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// Models for template relation context tests covering forward FK, forward O2O,
// reverse O2O, reverse FK sets, M2M sets (both sides), chained prefetches, and
// custom to_attr prefetches.

type tmplRelUser struct {
	orm.RelCache
	ID       int64
	Username string `orm:"max_length=50"`
}

func (u tmplRelUser) String() string { return u.Username }

type tmplRelProfile struct {
	orm.RelCache
	ID   int64
	User orm.O2O[tmplRelUser] `orm:"on_delete=cascade;related_name=profile"`
	Bio  string               `orm:"max_length=200"`
}

func (p tmplRelProfile) String() string { return "Profile:" + p.Bio }

type tmplRelBadge struct {
	orm.RelCache
	User  orm.O2O[tmplRelUser] `orm:"pk;on_delete=cascade;related_name=badge"`
	Label string               `orm:"max_length=50"`
}

func (b tmplRelBadge) String() string { return b.Label }

type tmplRelTag struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=50"`
}

func (tmplRelTag) Meta() orm.Meta    { return orm.Meta{Ordering: []string{"id"}} }
func (tg tmplRelTag) String() string { return tg.Name }

type tmplRelPost struct {
	orm.RelCache
	ID     int64
	Title  string              `orm:"max_length=100"`
	Author orm.FK[tmplRelUser] `orm:"on_delete=cascade;related_name=posts"`
	Tags   orm.M2M[tmplRelTag] `orm:"related_name=posts"`
}

func (tmplRelPost) Meta() orm.Meta   { return orm.Meta{Ordering: []string{"id"}} }
func (p tmplRelPost) String() string { return p.Title }

type tmplRelComment struct {
	orm.RelCache
	ID   int64
	Post orm.FK[tmplRelPost] `orm:"on_delete=cascade;related_name=comment_set"`
	Body string              `orm:"max_length=200"`
}

func (tmplRelComment) Meta() orm.Meta   { return orm.Meta{Ordering: []string{"id"}} }
func (c tmplRelComment) String() string { return c.Body }

func tmplRelModels() []any {
	return []any{djangotest.App("tmplrel",
		&tmplRelUser{}, &tmplRelProfile{}, &tmplRelBadge{},
		&tmplRelTag{}, &tmplRelPost{}, &tmplRelComment{},
	)}
}

// TestTemplateRelationContext checks forward FK (SelectRelated and
// PrefetchRelated), forward O2O (including O2O primary key), reverse O2O,
// reverse FK sets, M2M sets (both sides), and unloaded relations in templates,
// asserting that rendering never executes database queries.
func TestTemplateRelationContext(t *testing.T) {
	djangotest.EachDB(t, tmplRelModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		engine := templates.New()
		engine.SetModels(db.Registry())

		alice := &tmplRelUser{Username: "Alice & Co"}
		if err := orm.Query[tmplRelUser](db).Create(ctx, alice); err != nil {
			t.Fatalf("create alice: %v", err)
		}
		bob := &tmplRelUser{Username: "Bob"}
		if err := orm.Query[tmplRelUser](db).Create(ctx, bob); err != nil {
			t.Fatalf("create bob: %v", err)
		}

		prof := &tmplRelProfile{Bio: "Go <dev>"}
		prof.User.SetPK(alice.ID)
		if err := orm.Query[tmplRelProfile](db).Create(ctx, prof); err != nil {
			t.Fatalf("create profile: %v", err)
		}

		badge := &tmplRelBadge{Label: "Core"}
		badge.User.SetPK(alice.ID)
		if err := orm.Query[tmplRelBadge](db).Create(ctx, badge); err != nil {
			t.Fatalf("create badge: %v", err)
		}

		tagGo := &tmplRelTag{Name: "go"}
		tagWeb := &tmplRelTag{Name: "web"}
		if err := orm.Query[tmplRelTag](db).Create(ctx, tagGo); err != nil {
			t.Fatalf("create tagGo: %v", err)
		}
		if err := orm.Query[tmplRelTag](db).Create(ctx, tagWeb); err != nil {
			t.Fatalf("create tagWeb: %v", err)
		}

		post := &tmplRelPost{Title: "Hello"}
		post.Author.SetPK(alice.ID)
		if err := orm.Query[tmplRelPost](db).Create(ctx, post); err != nil {
			t.Fatalf("create post: %v", err)
		}
		if err := orm.Many[tmplRelTag](db, post, "tags").Add(ctx, tagGo, tagWeb); err != nil {
			t.Fatalf("post.tags.Add: %v", err)
		}

		c1 := &tmplRelComment{Body: "First!"}
		c1.Post.SetPK(post.ID)
		c2 := &tmplRelComment{Body: "Second & nice"}
		c2.Post.SetPK(post.ID)
		if err := orm.Query[tmplRelComment](db).Create(ctx, c1); err != nil {
			t.Fatalf("create c1: %v", err)
		}
		if err := orm.Query[tmplRelComment](db).Create(ctx, c2); err != nil {
			t.Fatalf("create c2: %v", err)
		}

		// 1. Forward O2O (SelectRelated & O2O primary key) and unloaded forward O2O.
		loadedProf, err := orm.Query[tmplRelProfile](db).SelectRelated("user").Get(ctx, "pk", prof.ID)
		if err != nil {
			t.Fatalf("Get profile: %v", err)
		}
		loadedBadge, err := orm.Query[tmplRelBadge](db).PrefetchRelated("user").Get(ctx, "pk", alice.ID)
		if err != nil {
			t.Fatalf("Get badge: %v", err)
		}
		unloadedProf, err := orm.Query[tmplRelProfile](db).Get(ctx, "pk", prof.ID)
		if err != nil {
			t.Fatalf("Get unloaded profile: %v", err)
		}

		djangotest.AssertNumQueries(t, db, 0, func() {
			got := renderStr(t, engine, `{{ p.user_id }}|{{ p.user }}|{{ p.user.username }}`, map[string]any{"p": loadedProf})
			djangotest.AssertEqual(t, got, "1|Alice &amp; Co|Alice &amp; Co")

			got = renderStr(t, engine, `{{ b.pk }}|{{ b.user_id }}|{{ b.user.username }}`, map[string]any{"b": loadedBadge})
			djangotest.AssertEqual(t, got, "1|1|Alice &amp; Co")

			got = renderStr(t, engine, `{{ p.user_id }}[{{ p.user }}][{{ p.user.username }}]`, map[string]any{"p": unloadedProf})
			djangotest.AssertEqual(t, got, "1[][]")
		})

		// 2. Reverse O2O (SelectRelated, PrefetchRelated, missing, and unloaded).
		userSel, err := orm.Query[tmplRelUser](db).SelectRelated("profile").Get(ctx, "pk", alice.ID)
		if err != nil {
			t.Fatalf("Get user SelectRelated: %v", err)
		}
		userPref, err := orm.Query[tmplRelUser](db).PrefetchRelated("profile").Get(ctx, "pk", alice.ID)
		if err != nil {
			t.Fatalf("Get user PrefetchRelated: %v", err)
		}
		bobPref, err := orm.Query[tmplRelUser](db).PrefetchRelated("profile").Get(ctx, "pk", bob.ID)
		if err != nil {
			t.Fatalf("Get bob PrefetchRelated: %v", err)
		}
		userUnloaded, err := orm.Query[tmplRelUser](db).Get(ctx, "pk", alice.ID)
		if err != nil {
			t.Fatalf("Get user unloaded: %v", err)
		}

		djangotest.AssertNumQueries(t, db, 0, func() {
			for _, u := range []tmplRelUser{userSel, userPref} {
				got := renderStr(t, engine, `{{ u.profile }}|{{ u.profile.bio }}|{{ u.profile.user.username }}`, map[string]any{"u": u})
				djangotest.AssertEqual(t, got, "Profile:Go &lt;dev&gt;|Go &lt;dev&gt;|Alice &amp; Co")
			}
			got := renderStr(t, engine, `[{{ u.profile }}]{% if u.profile %}y{% else %}n{% endif %}`, map[string]any{"u": bobPref})
			djangotest.AssertEqual(t, got, "[]n")

			got = renderStr(t, engine, `[{{ u.profile }}]{% if u.profile %}y{% else %}n{% endif %}`, map[string]any{"u": userUnloaded})
			djangotest.AssertEqual(t, got, "[]n")
		})

		// 3. Forward FK loaded by PrefetchRelated + Reverse FK set + Forward M2M set.
		prefPost, err := orm.Query[tmplRelPost](db).PrefetchRelated("author", "comment_set", "tags").Get(ctx, "pk", post.ID)
		if err != nil {
			t.Fatalf("Get prefPost: %v", err)
		}
		unloadedPost, err := orm.Query[tmplRelPost](db).Get(ctx, "pk", post.ID)
		if err != nil {
			t.Fatalf("Get unloadedPost: %v", err)
		}

		djangotest.AssertNumQueries(t, db, 0, func() {
			got := renderStr(t, engine,
				`{{ post.author_id }}|{{ post.author }}|{{ post.author.username }}`,
				map[string]any{"post": prefPost},
			)
			djangotest.AssertEqual(t, got, "1|Alice &amp; Co|Alice &amp; Co")

			got = renderStr(t, engine,
				`{{ post.comment_set.count }}:{% for c in post.comment_set.all %}[{{ c.body }}|{{ c.post.title }}]{% endfor %}`,
				map[string]any{"post": prefPost},
			)
			djangotest.AssertEqual(t, got, "2:[First!|Hello][Second &amp; nice|Hello]")

			got = renderStr(t, engine,
				`{{ post.tags.count }}:{% for tg in post.tags.all %}{{ tg.name }},{% endfor %}|{{ post.tags.all|join:"/" }}|{{ post.tags.first }}`,
				map[string]any{"post": prefPost},
			)
			djangotest.AssertEqual(t, got, "2:go,web,|go/web|go")

			// Unloaded sets render empty and execute no queries.
			got = renderStr(t, engine,
				`[{{ post.comment_set }}][{{ post.comment_set.all }}][{{ post.comment_set.count }}]{% for c in post.comment_set.all %}X{% empty %}E{% endfor %}`,
				map[string]any{"post": unloadedPost},
			)
			djangotest.AssertEqual(t, got, "[][][]E")

			got = renderStr(t, engine,
				`[{{ post.tags }}][{{ post.tags.all }}]{% if post.tags.all %}Y{% else %}N{% endif %}`,
				map[string]any{"post": unloadedPost},
			)
			djangotest.AssertEqual(t, got, "[][]N")
		})

		// 4. Reverse M2M set + chained prefetch + custom to_attr + public ModelContext.
		prefTag, err := orm.Query[tmplRelTag](db).PrefetchRelated("posts__comment_set").Get(ctx, "pk", tagGo.ID)
		if err != nil {
			t.Fatalf("Get prefTag: %v", err)
		}
		prefPostToAttr, err := orm.Query[tmplRelPost](db).PrefetchRelated(orm.PrefetchSpec{
			Lookup:   "comment_set",
			QuerySet: orm.Query[tmplRelComment](db).Filter("body", "First!"),
			ToAttr:   "first_comments",
		}).Get(ctx, "pk", post.ID)
		if err != nil {
			t.Fatalf("Get prefPostToAttr: %v", err)
		}

		djangotest.AssertNumQueries(t, db, 0, func() {
			got := renderStr(t, engine,
				`{{ tag.posts.count }}:{% for p in tag.posts.all %}{{ p.title }}({{ p.comment_set.count }}){% endfor %}`,
				map[string]any{"tag": prefTag},
			)
			djangotest.AssertEqual(t, got, "1:Hello(2)")

			got = renderStr(t, engine,
				`{% for c in post.first_comments %}{{ c.body }}{% endfor %}`,
				map[string]any{"post": prefPostToAttr},
			)
			djangotest.AssertEqual(t, got, "First!")

			// Public ModelContext also exposes prefetched relations and renders cleanly.
			postModel, _ := db.Registry().ModelOf(&tmplRelPost{})
			mm := templates.ModelContext(postModel, prefPost)
			got = renderStr(t, templates.New(),
				`{{ post.author.username }}|{{ post.comment_set.count }}|{% for tg in post.tags.all %}{{ tg }}{% endfor %}`,
				map[string]any{"post": mm},
			)
			djangotest.AssertEqual(t, got, "Alice &amp; Co|2|goweb")
		})

		// 5. Empty prefetched sets, list of prefetched posts, chained author__profile,
		// and single-object to_attr.
		emptyPost := &tmplRelPost{Title: "Quiet"}
		emptyPost.Author.SetPK(bob.ID)
		if err := orm.Query[tmplRelPost](db).Create(ctx, emptyPost); err != nil {
			t.Fatalf("create emptyPost: %v", err)
		}
		allPosts, err := orm.Query[tmplRelPost](db).PrefetchRelated(
			"author__profile",
			"comment_set",
			"tags",
			orm.PrefetchSpec{Lookup: "author", ToAttr: "writer"},
		).All(ctx)
		if err != nil {
			t.Fatalf("All posts: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			got := renderStr(t, engine,
				`{% for p in posts %}{{ p.title }}({{ p.writer.username }}|{{ p.author.profile.bio }}|c={{ p.comment_set.count }}|t={{ p.tags.count }}){% endfor %}`,
				map[string]any{"posts": allPosts},
			)
			djangotest.AssertEqual(t, got, "Hello(Alice &amp; Co|Go &lt;dev&gt;|c=2|t=2)Quiet(Bob||c=0|t=0)")

			got = renderStr(t, engine,
				`{{ p.comment_set.count }}|{% if p.comment_set.exists %}Y{% else %}N{% endif %}|{% for c in p.comment_set.all %}X{% empty %}EMPTY{% endfor %}|[{{ p.comment_set.first }}]`,
				map[string]any{"p": allPosts[1]},
			)
			djangotest.AssertEqual(t, got, "0|N|EMPTY|[]")
		})
	})
}
