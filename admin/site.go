package admin

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// templateFS holds the admin's embedded pongo2 templates (base, index,
// changelist). It is loaded into a dedicated engine by NewAdminSite.
//
//go:embed templates/*.html
var templateFS embed.FS

// entry is one model registered with the admin: its URL slug, ORM model
// descriptor, ModelAdmin customization, and type-erased operations.
type entry struct {
	slug  string // URL segment: lowercase model name.
	model *orm.Model
	admin ModelAdmin
	ops   modelOps
}

// AdminSite is the registry and HTTP surface for the admin. It owns a template
// engine built from the package's embedded templates and the set of registered
// model entries. Mount its routes with Routes.
//
// The name mirrors Django's AdminSite and is part of this package's public API
// (NewAdminSite returns it); the revive stutter warning is suppressed for it.
//
//nolint:revive // AdminSite is the intended public name, matching Django.
type AdminSite struct {
	db      *orm.DB
	engine  *templates.Engine
	entries []*entry
	bySlug  map[string]*entry
	// Prefix is the URL prefix the admin is mounted under (default "/admin").
	Prefix string
	// LoginURL is where non-staff requests are redirected (default
	// "/admin/login/").
	LoginURL string
}

// NewAdminSite returns an admin site bound to db. It builds its own template
// engine from the package's embedded templates. Prefix defaults to "/admin" and
// LoginURL to "/admin/login/".
func NewAdminSite(db *orm.DB) (*AdminSite, error) {
	// Root the FS at the templates directory so templates resolve by bare name
	// (e.g. {% extends "base.html" %}).
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("admin: open embedded templates: %w", err)
	}
	engine, err := templates.NewEngineFS(sub)
	if err != nil {
		return nil, fmt.Errorf("admin: build template engine: %w", err)
	}
	return &AdminSite{
		db:       db,
		engine:   engine,
		bySlug:   make(map[string]*entry),
		Prefix:   "/admin",
		LoginURL: "/admin/login/",
	}, nil
}

// Register adds model T (which must already be registered in db's ORM registry)
// to site with the given ModelAdmin. It panics when T has no registered model,
// since that is a boot-time programmer error.
func Register[T any](site *AdminSite, ma ModelAdmin) {
	var zero T
	m, ok := site.db.Registry().ModelOf(zero)
	if !ok {
		panic(fmt.Sprintf("admin: Register: no ORM model registered for %T", zero))
	}
	slug := strings.ToLower(m.Name())
	e := &entry{
		slug:  slug,
		model: m,
		admin: ma,
		ops:   buildOps[T](site.db, m),
	}
	site.entries = append(site.entries, e)
	site.bySlug[slug] = e
}
