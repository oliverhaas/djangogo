package views

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/orm"
)

// ContentTypeShortcut returns Django's contenttypes shortcut view
// (django.contrib.contenttypes.views.shortcut): it redirects to the page of
// the object with the primary key "object_id" of the model of the content
// type "content_type_id", as the object's GetAbsoluteURL names it. Mount it
// on a route that captures both values, for example
//
//	urls.Path("r/<int:content_type_id>/<path:object_id>/", views.Handler(engine, views.ContentTypeShortcut(db)), "")
//
// A URL that starts with "http://", "https://" or "//" is used as is. The
// view prefixes any other URL with the scheme and the host of the request, the
// domain of Django's RequestSite without the sites framework. For an unknown
// content type or object, or an object without GetAbsoluteURL, the view
// returns a NotFound error, Django's Http404.
func ContentTypeShortcut(db *orm.DB) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		ctID, objectID := r.PathValue("content_type_id"), r.PathValue("object_id")
		missing := NotFound(fmt.Sprintf("Content type %s object %s doesn’t exist", ctID, objectID))
		id, err := strconv.ParseInt(ctID, 10, 64)
		if err != nil {
			return missing
		}
		ct, err := contenttypes.GetForID(r.Context(), db, id)
		if errors.Is(err, orm.ErrDoesNotExist) {
			return missing
		}
		if err != nil {
			return err
		}
		m, ok := contenttypes.ModelClass(db, ct)
		if !ok {
			return NotFound(fmt.Sprintf("Content type %s object has no associated model", ctID))
		}
		pk, ok := parseObjectPK(m, objectID)
		if !ok {
			return missing
		}
		obj, err := orm.GetByPK(r.Context(), db, m, pk)
		if errors.Is(err, orm.ErrDoesNotExist) {
			return missing
		}
		if err != nil {
			return err
		}
		u, ok := obj.(AbsoluteURLer)
		if !ok {
			return NotFound(fmt.Sprintf("%s objects don’t have a get_absolute_url() method", ct.Name()))
		}
		absURL := u.GetAbsoluteURL()
		for _, prefix := range []string{"http://", "https://", "//"} {
			if strings.HasPrefix(absURL, prefix) {
				http.Redirect(w, r, absURL, http.StatusFound)
				return nil
			}
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		http.Redirect(w, r, scheme+"://"+r.Host+absURL, http.StatusFound)
		return nil
	}
}

// parseObjectPK converts the object ID of the URL to the type of the primary key of
// m, as the lookup of Django's get_object_for_this_type does. It reports false
// for an ID that is not a value of that type, Django's ValueError and
// ValidationError.
func parseObjectPK(m *orm.Model, objectID string) (any, bool) {
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, false
	}
	switch pk.Kind {
	case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
		orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
		v, err := strconv.ParseInt(objectID, 10, 64)
		return v, err == nil
	case orm.KindUUID:
		v, err := orm.ParseUUID(objectID)
		return v, err == nil
	default:
		return objectID, true
	}
}
