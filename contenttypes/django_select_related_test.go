package contenttypes_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The generic models of Django's tests/select_related/models.py. The other
// models of that app are in orm/django_select_related_test.go.

// SelectRelatedTaggedItem is tests/select_related/models.py::TaggedItem.
type SelectRelatedTaggedItem struct {
	ID            int64
	Tag           string                           `orm:"max_length=30"`
	ContentType   orm.FK[contenttypes.ContentType] `orm:"related_name=select_related_tagged_items"`
	ObjectID      int64                            `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// SelectRelatedBookmark is tests/select_related/models.py::Bookmark.
type SelectRelatedBookmark struct {
	ID   int64
	URL  string `orm:"type=url"`
	Tags contenttypes.GenericRelation[SelectRelatedTaggedItem]
}

// Ported from Django: tests/select_related/tests.py::SelectRelatedValidationTests.test_generic_relations
func TestSelectRelatedGenericRelations(t *testing.T) {
	models := []any{
		contenttypes.App{},
		djangotest.App("select_related", &SelectRelatedTaggedItem{}, &SelectRelatedBookmark{}),
	}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		tests := []struct {
			err  error
			want string
		}{
			{
				func() error {
					_, err := orm.Query[SelectRelatedBookmark](db).SelectRelated("tags").All(ctx)
					return err
				}(),
				"nvalid field name(s) given in select_related: 'tags'. Choices are: ",
			},
			{
				func() error {
					_, err := orm.Query[SelectRelatedTaggedItem](db).SelectRelated("content_object").All(ctx)
					return err
				}(),
				"nvalid field name(s) given in select_related: 'content_object'. Choices are: content_type",
			},
		}
		for _, tt := range tests {
			if !errors.Is(tt.err, orm.ErrFieldError) || !strings.Contains(tt.err.Error(), tt.want) {
				t.Errorf("error = %v, want a FieldError containing %q", tt.err, tt.want)
			}
		}
	})
}
