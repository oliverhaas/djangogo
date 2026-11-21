package admin

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/oliverhaas/djangogo/forms"
)

// staticFiles holds Django's admin stylesheets, scripts and images under
// static/admin.
//
//go:embed static
var staticFiles embed.FS

// jsi18n is the JavaScript catalog of the admin's scripts, what Django's
// JavaScriptCatalog view writes for English: the gettext functions, which
// return their message unchanged, and the date and time formats.
//
//go:embed jsi18n.js
var jsi18n []byte

// StaticFS returns the admin's static files, Django's
// django/contrib/admin/static: the stylesheets, scripts and images that the
// admin's pages load from "admin/..." below the static URL. The application
// adds them to its static files when an app registers models with the admin.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("admin: open embedded static files: " + err.Error())
	}
	return sub
}

// jsi18nView serves the JavaScript catalog of the admin's scripts, Django's
// AdminSite.i18n_javascript.
func (s *AdminSite) jsi18nView(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", `text/javascript; charset="utf-8"`)
	_, _ = w.Write(jsi18n)
}

// jsi18nURL returns the URL of the site's JavaScript catalog.
func (s *AdminSite) jsi18nURL() string {
	if u := s.reverse("jsi18n"); u != "" {
		return u
	}
	return s.prefix() + "/jsi18n/"
}

// minified returns ".min" unless the site is in Debug mode, the suffix of the
// vendored scripts Django loads.
func (s *AdminSite) minified() string {
	if s.Debug {
		return ""
	}
	return ".min"
}

// adminJS returns the given scripts below admin/js/ as media.
func adminJS(names ...string) forms.Media {
	js := make([]string, len(names))
	for i, name := range names {
		js[i] = "admin/js/" + name
	}
	return forms.NewMedia(nil, js)
}

// media returns the scripts every page of the model admin loads, Django's
// ModelAdmin.media: jQuery, the admin's core script, the related-object
// popups, the bulk actions, the slug helpers and XRegExp.
func (e *entry) media(s *AdminSite) forms.Media {
	extra := s.minified()
	return adminJS(
		"vendor/jquery/jquery"+extra+".js",
		"jquery.init.js",
		"core.js",
		"admin/RelatedObjectLookups.js",
		"actions.js",
		"urlify.js",
		"prepopulate.js",
		"vendor/xregexp/xregexp"+extra+".js",
	).Add(e.admin.Media)
}

// Media returns the scripts of Django's SelectFilter2.js, which turn the list
// into two filtered lists. They need the site's JavaScript catalog on the
// page.
func (FilteredSelectMultiple) Media() forms.Media {
	return adminJS("core.js", "SelectBox.js", "SelectFilter2.js")
}

// Media returns jQuery, Select2 with its English messages, and Django's
// autocomplete.js, which turn the list into a search box, and the Select2
// stylesheets, Django's AutocompleteMixin.media.
func (a autocomplete) Media() forms.Media {
	extra := ".min"
	if a.Debug {
		extra = ""
	}
	return forms.NewMedia(
		map[string][]string{"screen": {
			"admin/css/vendor/select2/select2" + extra + ".css",
			"admin/css/autocomplete.css",
		}},
		[]string{
			"admin/js/vendor/jquery/jquery" + extra + ".js",
			"admin/js/vendor/select2/select2.full" + extra + ".js",
			"admin/js/vendor/select2/i18n/en.js",
			"admin/js/jquery.init.js",
			"admin/js/autocomplete.js",
		},
	)
}
