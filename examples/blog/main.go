package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
	"github.com/oliverhaas/djangogo/views"
)

// templateFS holds the public list and detail templates so the binary is fully
// self-contained (no template directory needs to ship alongside it).
//
//go:embed templates/*.html
var templateFS embed.FS

// config holds the knobs newServer needs. main fills it with production values;
// the test fills it with an in-memory database and a throwaway secret.
type config struct {
	// DSN is the SQLite connection string (e.g. a file path or an in-memory DSN).
	DSN string
	// SecretKey signs the session cookie.
	SecretKey string
}

// newServer wires the whole application and returns it as an http.Handler.
//
// It opens the database, registers the Post model alongside auth.AppModels,
// resolves and freezes the registry, creates every table, builds the template
// engine and router (public post list and detail plus the staff-gated admin),
// and wraps everything in the sessions -> csrf -> auth middleware chain. The
// returned handler is ready for http.ListenAndServe or httptest.
func newServer(cfg config) (http.Handler, error) {
	db, err := openDB(cfg.DSN)
	if err != nil {
		return nil, err
	}

	engine, err := publicEngine()
	if err != nil {
		return nil, err
	}

	site, err := adminSite(db)
	if err != nil {
		return nil, err
	}

	router := urls.NewRouter(append(publicRoutes(db, engine), site.Routes()...)...)
	// Wire the router's reverse into the engine so {% url %} resolves named
	// routes against it (Django's single root URLconf). This must follow router
	// construction; templates render later, at request time.
	engine.SetResolver(router.Reverse)

	// Middleware chain (outer to inner): sessions -> csrf -> auth -> router.
	// sessions must run first so csrf and auth can read the session; auth then
	// loads the logged-in user the admin's staff gate checks.
	store := sessions.NewSignedCookieStore([]byte(cfg.SecretKey))
	var handler http.Handler = router
	handler = auth.Middleware(db)(handler)
	handler = csrf.Middleware(handler)
	handler = sessions.Middleware(store, "sessionid")(handler)
	return handler, nil
}

// openDB opens the SQLite database, registers Post and Comment plus the auth
// models, resolves relations, freezes the registry, and creates every table.
func openDB(dsn string) (*orm.DB, error) {
	reg := orm.NewRegistry()
	models := append([]any{&Post{}, &Comment{}}, auth.AppModels()...)
	for _, m := range models {
		if _, err := reg.Register(m); err != nil {
			return nil, fmt.Errorf("register %T: %w", m, err)
		}
	}
	if err := reg.Resolve(); err != nil {
		return nil, fmt.Errorf("resolve relations: %w", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db := orm.NewDB(sdb, sqlite.New(), reg)

	ctx := context.Background()
	for _, m := range reg.Models() {
		if err := db.CreateTable(ctx, m); err != nil {
			return nil, fmt.Errorf("create table %s: %w", m.Table(), err)
		}
	}
	return db, nil
}

// publicEngine builds the template engine for the public pages from the embedded
// templates directory, rooted so templates resolve by bare name.
func publicEngine() (*templates.Engine, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("open embedded templates: %w", err)
	}
	engine, err := templates.NewEngineFS(sub)
	if err != nil {
		return nil, fmt.Errorf("build template engine: %w", err)
	}
	return engine, nil
}

// publicRoutes returns the public post list ("/") and detail ("/posts/{pk}/")
// routes. The list uses the generic ListView; the detail uses a custom
// postDetailView that also accepts comment submissions, so its route carries no
// method prefix and matches both GET (render) and POST (submit a comment).
func publicRoutes(db *orm.DB, engine *templates.Engine) []urls.Route {
	list := views.ListView[Post]{
		DB:       db,
		Engine:   engine,
		Template: "post_list.html",
		Query: func(db *orm.DB) *orm.QuerySet[Post] {
			return orm.Query[Post](db).Filter("published", true).OrderBy("-created_at")
		},
	}
	detail := postDetailView{db: db, engine: engine}
	return []urls.Route{
		urls.Path("GET /{$}", list, "post-list"),
		urls.Path("/posts/{pk}/", detail, "post-detail"),
	}
}

// adminSite builds the admin site and registers Post and Comment, each with a
// small ModelAdmin. Comment's changelist shows its FK to Post via the post's
// __str__ label, and its change form renders the FK as a <select> of posts.
func adminSite(db *orm.DB) (*admin.AdminSite, error) {
	site, err := admin.NewAdminSite(db)
	if err != nil {
		return nil, fmt.Errorf("new admin site: %w", err)
	}
	admin.Register[Post](site, admin.ModelAdmin{
		ListDisplay: []string{"ID", "Title", "Published"},
		Ordering:    []string{"-id"},
	})
	admin.Register[Comment](site, admin.ModelAdmin{
		ListDisplay: []string{"ID", "Post", "Name"},
		Ordering:    []string{"-id"},
	})
	return site, nil
}

func main() {
	handler, err := newServer(config{
		DSN:       "blog.sqlite3",
		SecretKey: "dev-insecure-key-change-me",
	})
	if err != nil {
		log.Fatalf("blog: %v", err)
	}

	addr := "127.0.0.1:8000"
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("blog: serving on http://%s/ (admin at /admin/)", addr)
	log.Fatal(srv.ListenAndServe())
}
