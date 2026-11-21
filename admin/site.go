package admin

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// Default values for AdminSite metadata and URLs.
const (
	defaultPrefix     = "/admin"
	defaultLoginURL   = "/admin/login/"
	defaultSiteName   = "admin"
	defaultSiteTitle  = "Django site admin"
	defaultSiteHeader = "Django administration"
	defaultIndexTitle = "Site administration"
	defaultSiteURL    = "/"
	emptyValueDisplay = "-"
)

// templateFS holds the admin's embedded pongo2 templates under templates/admin
// and templates/registration.
//
//go:embed templates/admin/*.html templates/admin/includes/*.html templates/registration/*.html
var templateFS embed.FS

// entryKey uniquely identifies a registered model within an AdminSite by app
// label and lowercased model name.
type entryKey struct {
	app  string
	slug string
}

// entry is one model registered with the admin: its app label, URL slug, ORM
// model descriptor, ModelAdmin customization, and type-erased operations.
type entry struct {
	app   string // app_label (may be "" when registered without an app).
	slug  string // URL segment: lowercase model name.
	model *orm.Model
	admin ModelAdmin
	ops   modelOps
}

// urlName returns the unqualified route name for action ("changelist", "add",
// "change", "delete", "history"), e.g. "admin_views_article_changelist" or
// "article_changelist" when e has no app label.
func (e *entry) urlName(action string) string {
	if e.app == "" {
		return e.slug + "_" + action
	}
	return e.app + "_" + e.slug + "_" + action
}

// routePrefix returns the path prefix of e's views relative to the admin mount
// point: "<app>/<model>/" for models with an app label, or "<model>/" when
// registered without one.
func (e *entry) routePrefix() string {
	if e.app == "" {
		return e.slug + "/"
	}
	return e.app + "/" + e.slug + "/"
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
	byKey   map[entryKey]*entry

	// Name is the instance URL namespace (Django's AdminSite.name, default
	// "admin"). The application namespace is always "admin".
	Name string
	// Prefix is the URL prefix the admin is mounted under (default "/admin").
	Prefix string
	// LoginURL is where non-staff requests are redirected (default
	// "<Prefix>/login/").
	LoginURL string
	// SiteTitle is appended to every page's <title> (default "Django site admin").
	SiteTitle string
	// SiteHeader is rendered in the header branding block (default
	// "Django administration").
	SiteHeader string
	// IndexTitle is the heading of the main admin index (default
	// "Site administration").
	IndexTitle string
	// SiteURL is the target of the "View site" link (default "/").
	SiteURL string
	// EmptyValueDisplay is the placeholder rendered for empty field values
	// across the site (Django's AdminSite.empty_value_display, default "-").
	EmptyValueDisplay string
	// Apps supplies app verbose names when set. When nil, app labels are
	// title-cased as Django does by default.
	Apps *apps.Registry
	// FinalCatchAllView mounts a catch-all pattern at the end of the admin URLs
	// that redirects unauthenticated requests to login and handles trailing
	// slashes before 404ing (Django's AdminSite.final_catch_all_view, default
	// true).
	FinalCatchAllView bool
	// HasPermissionFunc overrides AdminSite.HasPermission when non-nil.
	HasPermissionFunc func(r *http.Request) bool
	// LoginConfirmAllowed overrides the staff/active check during login POST
	// when non-nil. Return a non-nil error to reject the login with that error
	// message.
	LoginConfirmAllowed func(r *http.Request, u *auth.User) error

	actions   []Action
	revMu     sync.RWMutex
	revRouter *urls.Router
}

// NewAdminSite returns an admin site bound to db with Django's default titles,
// namespace ("admin"), prefix ("/admin"), and catch-all view enabled.
func NewAdminSite(db *orm.DB) (*AdminSite, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("admin: open embedded templates: %w", err)
	}
	engine, err := templates.NewEngineFS(sub)
	if err != nil {
		return nil, fmt.Errorf("admin: build template engine: %w", err)
	}
	engine.SetModels(db.Registry())
	engine.AddContextProcessor(
		csrf.ContextProcessor,
		auth.ContextProcessor,
		messages.ContextProcessor,
		templates.RequestProcessor,
	)
	site := &AdminSite{
		db:                db,
		engine:            engine,
		byKey:             make(map[entryKey]*entry),
		Name:              defaultSiteName,
		Prefix:            defaultPrefix,
		LoginURL:          defaultLoginURL,
		SiteTitle:         defaultSiteTitle,
		SiteHeader:        defaultSiteHeader,
		IndexTitle:        defaultIndexTitle,
		SiteURL:           defaultSiteURL,
		EmptyValueDisplay: emptyValueDisplay,
		FinalCatchAllView: true,
		actions:           []Action{defaultDeleteSelectedAction()},
	}
	engine.SetResolver(site.resolveURL)
	return site, nil
}

// AddAction registers a site-wide bulk action available on every registered
// ModelAdmin unless disabled (Django's AdminSite.add_action). If an action
// with the same Name is already registered, it is replaced in place.
func (s *AdminSite) AddAction(a Action) {
	for i, existing := range s.actions {
		if existing.Name == a.Name {
			s.actions[i] = a
			return
		}
	}
	s.actions = append(s.actions, a)
}

// DisableAction removes a site-wide bulk action by name (Django's
// AdminSite.disable_action), for example site.DisableAction("delete_selected").
func (s *AdminSite) DisableAction(name string) {
	for i, existing := range s.actions {
		if existing.Name == name {
			s.actions = append(s.actions[:i], s.actions[i+1:]...)
			return
		}
	}
}

// entryByModel returns the registered entry for ORM model m, or nil if m is
// not registered on s.
func (s *AdminSite) entryByModel(m *orm.Model) *entry {
	if m == nil {
		return nil
	}
	return s.byKey[entryKey{app: m.App(), slug: strings.ToLower(m.Name())}]
}

// emptyValue returns the site-wide empty-value display string, defaulting to "-".
func (s *AdminSite) emptyValue() string {
	if s.EmptyValueDisplay != "" {
		return s.EmptyValueDisplay
	}
	return emptyValueDisplay
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
		app:   m.App(),
		slug:  slug,
		model: m,
		admin: ma,
		ops:   buildOps[T](site.db, m),
	}
	key := entryKey{app: e.app, slug: slug}
	if _, exists := site.byKey[key]; exists {
		for i, existing := range site.entries {
			if existing.app == e.app && existing.slug == slug {
				site.entries[i] = e
				break
			}
		}
	} else {
		site.entries = append(site.entries, e)
	}
	site.byKey[key] = e
	site.invalidateReverseRouter()
}

// HasPermission reports whether the request's user has access to the admin
// site (Django's AdminSite.has_permission). By default the user must be
// authenticated, active, and staff.
func (s *AdminSite) HasPermission(r *http.Request) bool {
	if s.HasPermissionFunc != nil {
		return s.HasPermissionFunc(r)
	}
	u, err := auth.GetUser(r)
	return err == nil && u.IsAuthenticated() && u.IsActive && u.IsStaff
}

// AdminView wraps h with the admin site's permission gate and never-cache
// headers (Django's AdminSite.admin_view).
func (s *AdminSite) AdminView(h http.Handler) http.Handler {
	return s.adminView(h.ServeHTTP, false)
}

// adminView wraps fn with HasPermission gating and optional never-cache
// headers. Requests that fail HasPermission are redirected to login (or to
// admin:index when hitting the logout URL, matching Django).
func (s *AdminSite) adminView(fn http.HandlerFunc, cacheable bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cacheable {
			neverCache(w)
		}
		if !s.HasPermission(r) {
			if r.URL.Path == s.logoutURL() {
				http.Redirect(w, r, s.indexURL(), http.StatusFound)
				return
			}
			auth.RedirectToLogin(w, r.URL.RequestURI(), s.loginURL())
			return
		}
		fn(w, r)
	})
}

// Routes returns the admin's URL routes mounted under Prefix with application
// namespace "admin" and instance namespace Name. The route names follow
// Django's convention: "admin:index", "admin:login", "admin:logout",
// "admin:password_change", "admin:password_change_done", "admin:app_list", and
// per-model "admin:<app>_<model>_{changelist,add,change,delete,history}".
func (s *AdminSite) Routes() []urls.Route {
	routes := s.buildRoutes()
	rt := urls.NewRouter(routes...)
	s.revMu.Lock()
	s.revRouter = rt
	s.revMu.Unlock()
	return routes
}

// buildRoutes assembles the namespaced route slice for the current site state.
func (s *AdminSite) buildRoutes() []urls.Route {
	inner := []urls.Route{
		urls.Path("", s.adminView(s.index, false), "index"),
		urls.Path("login/", http.HandlerFunc(s.login), "login"),
		urls.Path("logout/", s.adminView(s.logout, false), "logout"),
		urls.Path("password_change/", s.adminView(s.passwordChange, true), "password_change"),
		urls.Path("password_change/done/", s.adminView(s.passwordChangeDone, true), "password_change_done"),
	}

	var validApps []string
	seenApp := make(map[string]bool)
	for _, e := range s.entries {
		mp := e.routePrefix()
		inner = append(inner,
			urls.Path(mp, s.adminView(s.makeChangelistHandler(e), false), e.urlName("changelist")),
			urls.Path(mp+"add/", s.adminView(s.makeAddHandler(e), false), e.urlName("add")),
			urls.Path(mp+"<path:object_id>/history/", s.adminView(s.makeHistoryHandler(e), false), e.urlName("history")),
			urls.Path(mp+"<path:object_id>/delete/", s.adminView(s.makeDeleteHandler(e), false), e.urlName("delete")),
			urls.Path(mp+"<path:object_id>/change/", s.adminView(s.makeChangeHandler(e), false), e.urlName("change")),
			urls.Path(mp+"<path:object_id>/", s.adminView(s.makeOldChangeRedirect(e), false), ""),
		)
		if e.app != "" && !seenApp[e.app] {
			seenApp[e.app] = true
			validApps = append(validApps, regexp.QuoteMeta(e.app))
		}
	}

	if len(validApps) > 0 {
		regex := "^(?P<app_label>" + strings.Join(validApps, "|") + ")/$"
		inner = append(inner, urls.RePath(regex, s.adminView(s.appIndex, false), "app_list"))
	}

	if s.FinalCatchAllView {
		inner = append(inner, urls.RePath("^(?P<url>.*)$", urls.NoAppendSlash(s.adminView(s.catchAllView, false)), ""))
	}

	return urls.IncludeApp(s.prefix(), defaultSiteName, s.siteName(), inner...)
}

// invalidateReverseRouter clears the cached reverse router after a model
// registration so the next reverse rebuilds it.
func (s *AdminSite) invalidateReverseRouter() {
	s.revMu.Lock()
	s.revRouter = nil
	s.revMu.Unlock()
}

// reverseRouter returns the cached internal router for this site, building it
// lazily when Routes has not been called yet.
func (s *AdminSite) reverseRouter() *urls.Router {
	s.revMu.RLock()
	rt := s.revRouter
	s.revMu.RUnlock()
	if rt != nil {
		return rt
	}
	rt = urls.NewRouter(s.buildRoutes()...)
	s.revMu.Lock()
	s.revRouter = rt
	s.revMu.Unlock()
	return rt
}

// resolveURL resolves named URLs for the admin's template engine, preferring
// this site's own route table so isolated sites do not depend on or collide
// through the process-global URL resolver, and falling back to the global
// resolver for non-admin routes.
func (s *AdminSite) resolveURL(name string, args ...any) (string, error) {
	if u, err := s.reverseRouter().Reverse(name, args...); err == nil {
		return u, nil
	}
	if global := templates.URLResolverFunc(); global != nil {
		return global(name, args...)
	}
	return s.reverseRouter().Reverse(name, args...)
}

// reverse reverses an admin-namespaced route name for this site instance.
func (s *AdminSite) reverse(name string, args ...any) string {
	allArgs := make([]any, 0, len(args)+1)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, urls.CurrentApp(s.siteName()))
	u, err := s.reverseRouter().Reverse(defaultSiteName+":"+name, allArgs...)
	if err != nil {
		return ""
	}
	return u
}

// EachContext returns the template context dictionary added to every page of
// the admin site (Django's AdminSite.each_context).
func (s *AdminSite) EachContext(r *http.Request) map[string]any {
	u, _ := auth.GetUser(r)
	return map[string]any{
		"site_title":               s.siteTitle(),
		"site_header":              s.siteHeader(),
		"site_url":                 s.siteURL(),
		"has_permission":           s.HasPermission(r),
		"available_apps":           s.GetAppList(r),
		"is_popup":                 false,
		"prefix":                   s.prefix(),
		"user_has_usable_password": u != nil && u.IsAuthenticated() && u.HasUsablePassword(),
	}
}

// GetAppList returns the sorted list of apps and models visible to the user of
// r (Django's AdminSite.get_app_list). Pass an appLabel to restrict the list to
// one app.
func (s *AdminSite) GetAppList(r *http.Request, appLabel ...string) []map[string]any {
	filterApp := ""
	if len(appLabel) > 0 {
		filterApp = appLabel[0]
	}
	appDict := make(map[string]map[string]any)
	for _, e := range s.entries {
		if filterApp != "" && e.app != filterApp {
			continue
		}
		hasModule := e.hasModulePermission(s, r)
		if !hasModule {
			continue
		}
		perms := e.modelPerms(s, r)
		if !perms["add"] && !perms["change"] && !perms["delete"] && !perms["view"] {
			continue
		}
		_, plural := modelVerboseNames(e.model)
		modelDict := map[string]any{
			"model":       e.model,
			"name":        capFirst(plural),
			"object_name": e.model.Name(),
			"model_name":  e.slug,
			"perms":       perms,
			"admin_url":   "",
			"add_url":     "",
		}
		if perms["change"] || perms["view"] {
			modelDict["view_only"] = !perms["change"]
			modelDict["admin_url"] = s.changelistURL(e)
		}
		if perms["add"] {
			modelDict["add_url"] = s.addURL(e)
		}

		appEntry, ok := appDict[e.app]
		if !ok {
			appURL := ""
			if e.app != "" {
				appURL = s.appIndexURL(e.app)
			}
			appEntry = map[string]any{
				"name":             s.appVerboseName(e.app),
				"app_label":        e.app,
				"app_url":          appURL,
				"has_module_perms": hasModule,
				"models":           []map[string]any{modelDict},
			}
			appDict[e.app] = appEntry
		} else {
			appEntry["models"] = append(appEntry["models"].([]map[string]any), modelDict)
		}
	}

	appList := make([]map[string]any, 0, len(appDict))
	for _, appEntry := range appDict {
		models := appEntry["models"].([]map[string]any)
		sort.Slice(models, func(i, j int) bool {
			return models[i]["name"].(string) < models[j]["name"].(string)
		})
		appEntry["models"] = models
		appList = append(appList, appEntry)
	}
	sort.Slice(appList, func(i, j int) bool {
		return strings.ToLower(appList[i]["name"].(string)) < strings.ToLower(appList[j]["name"].(string))
	})
	return appList
}

// appVerboseName returns the human-readable name for appLabel.
func (s *AdminSite) appVerboseName(appLabel string) string {
	if s.Apps != nil && appLabel != "" {
		if cfg, err := s.Apps.ByLabel(appLabel); err == nil {
			return apps.VerboseName(cfg)
		}
	}
	if appLabel == "" {
		return "Default"
	}
	return titleCase(appLabel)
}

// prefix returns the normalized mount prefix without a trailing slash.
func (s *AdminSite) prefix() string {
	p := strings.TrimRight(s.Prefix, "/")
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// siteName returns the instance namespace, defaulting to "admin".
func (s *AdminSite) siteName() string {
	if s.Name != "" {
		return s.Name
	}
	return defaultSiteName
}

// siteTitle returns SiteTitle or the Django default.
func (s *AdminSite) siteTitle() string {
	if s.SiteTitle != "" {
		return s.SiteTitle
	}
	return defaultSiteTitle
}

// siteHeader returns SiteHeader or the Django default.
func (s *AdminSite) siteHeader() string {
	if s.SiteHeader != "" {
		return s.SiteHeader
	}
	return defaultSiteHeader
}

// indexTitle returns IndexTitle or the Django default.
func (s *AdminSite) indexTitle() string {
	if s.IndexTitle != "" {
		return s.IndexTitle
	}
	return defaultIndexTitle
}

// siteURL returns SiteURL (empty string disables the "View site" link).
func (s *AdminSite) siteURL() string { return s.SiteURL }

// indexURL returns the URL of the admin index.
func (s *AdminSite) indexURL() string {
	if u := s.reverse("index"); u != "" {
		return u
	}
	return s.prefix() + "/"
}

// loginURL returns the login URL, honoring an explicit custom LoginURL while
// adapting to a customized Prefix when LoginURL was left at its default.
func (s *AdminSite) loginURL() string {
	if s.LoginURL != "" && (s.LoginURL != defaultLoginURL || s.prefix() == defaultPrefix) {
		return s.LoginURL
	}
	if u := s.reverse("login"); u != "" {
		return u
	}
	return s.prefix() + "/login/"
}

// logoutURL returns the URL of the admin logout view.
func (s *AdminSite) logoutURL() string {
	if u := s.reverse("logout"); u != "" {
		return u
	}
	return s.prefix() + "/logout/"
}

// passwordChangeDoneURL returns the URL of the password-change-done view.
func (s *AdminSite) passwordChangeDoneURL() string {
	if u := s.reverse("password_change_done"); u != "" {
		return u
	}
	return s.prefix() + "/password_change/done/"
}

// appIndexURL returns the URL of the app index for appLabel.
func (s *AdminSite) appIndexURL(appLabel string) string {
	if u := s.reverse("app_list", urls.Kwargs{"app_label": appLabel}); u != "" {
		return u
	}
	return s.prefix() + "/" + appLabel + "/"
}

// changelistURL returns the URL of e's changelist.
func (s *AdminSite) changelistURL(e *entry) string {
	if u := s.reverse(e.urlName("changelist")); u != "" {
		return u
	}
	return s.prefix() + "/" + e.routePrefix()
}

// addURL returns the URL of e's add view.
func (s *AdminSite) addURL(e *entry) string {
	if u := s.reverse(e.urlName("add")); u != "" {
		return u
	}
	return s.prefix() + "/" + e.routePrefix() + "add/"
}

// changeURL returns the URL of e's change view for pk.
func (s *AdminSite) changeURL(e *entry, pk int64) string {
	if u := s.reverse(e.urlName("change"), pk); u != "" {
		return u
	}
	return fmt.Sprintf("%s/%s%d/change/", s.prefix(), e.routePrefix(), pk)
}

// deleteURL returns the URL of e's delete view for pk.
func (s *AdminSite) deleteURL(e *entry, pk int64) string {
	if u := s.reverse(e.urlName("delete"), pk); u != "" {
		return u
	}
	return fmt.Sprintf("%s/%s%d/delete/", s.prefix(), e.routePrefix(), pk)
}
