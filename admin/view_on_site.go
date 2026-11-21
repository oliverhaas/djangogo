package admin

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/views"
)

// viewOnSite serves the view_on_site URL of the site with the shortcut view
// of contenttypes, which redirects to the URL of the object.
func (s *AdminSite) viewOnSite(w http.ResponseWriter, r *http.Request) {
	err := views.ContentTypeShortcut(s.db)(w, r)
	switch {
	case err == nil:
	case errors.Is(err, views.ErrNotFound):
		s.notFound(w, r, err.Error())
	default:
		http.Error(w, "failed to load the object", http.StatusInternalServerError)
	}
}

// viewOnSiteURL returns the URL of the "View on site" link of obj, a row of
// the model of e, Django's get_view_on_site_url. show and custom are the
// ViewOnSite and ViewOnSiteURL of the admin. It returns "" for no link: on
// the add page, when show is false, and when obj has no GetAbsoluteURL and
// custom is nil.
func (s *AdminSite) viewOnSiteURL(ctx context.Context, e *entry, show *bool, custom func(obj any) string, obj any) string {
	if obj == nil || show != nil && !*show {
		return ""
	}
	if custom != nil {
		return custom(obj)
	}
	if _, ok := obj.(views.AbsoluteURLer); !ok {
		return ""
	}
	ct, err := contenttypes.GetForModel(ctx, s.db, e.model)
	if err != nil {
		// Leave the link out, as reverse does for a URL it cannot build.
		// The same database error fails the rest of the page.
		return ""
	}
	return s.reverse("view_on_site", ct.ID, pkOf(reflect.ValueOf(obj).Elem(), e))
}
