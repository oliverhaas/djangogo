package views_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// withMessages serves h behind messages.Middleware with the cookie storage,
// in a client that records the messages of every request.
func withMessages(t *testing.T, path string, h http.Handler) *djangotest.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(path, h)
	mw := messages.MiddlewareWithOptions(messages.Options{
		Storage: messages.StorageCookie,
		Cookie:  messages.CookieOptions{Secret: []byte("views-tests-secret")},
	})
	return djangotest.NewClient(t, mw(mux), djangotest.WithMessages())
}

// TestCreateViewSuccessMessage adds the success message with the
// placeholders filled from the cleaned data after saving. No Django test
// exists: Django tests SuccessMessageMixin with FormView and DeleteView only.
func TestCreateViewSuccessMessage(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		view := authorCreate(t, db)
		view.SuccessMessage = "%(name)s was created successfully"
		c := withMessages(t, "/edit/authors/create/", view)

		resp := c.Post("/edit/authors/create/", map[string]string{"name": "Randall Munroe", "slug": "randall-munroe"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "Randall Munroe was created successfully"},
		})
		assertAuthorCount(t, db, 1)
	})
}

// TestUpdateViewGetSuccessMessage builds the message from the saved object,
// as the get_success_message of Django's documentation reads self.object. No
// Django test exists.
func TestUpdateViewGetSuccessMessage(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		author := Author{Name: "Randall Munroe", Slug: "randall-munroe"}
		create(t, db, &author)
		view := authorUpdate(t, db, nil)
		view.SuccessMessage = "ignored"
		view.GetSuccessMessage = func(_ *http.Request, obj Author, cleanedData map[string]any) (string, error) {
			return fmt.Sprintf("%s is now %d (%s)", cleanedData["name"], obj.ID, obj.Slug), nil
		}
		c := withMessages(t, "/edit/author/{pk}/update/", view)

		path := fmt.Sprintf("/edit/author/%d/update/", author.ID)
		resp := c.Post(path, map[string]string{"name": "Randall", "slug": "randall"})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: fmt.Sprintf("Randall is now %d (randall)", author.ID)},
		})
	})
}

// TestFormViewEmptySuccessMessage adds no message for an empty message, so
// the view needs no message middleware. No Django test exists.
func TestFormViewEmptySuccessMessage(t *testing.T) {
	view := views.FormView{
		Engine:     genericEngine(t),
		Form:       func(*http.Request) (*forms.Form, error) { return forms.New(forms.F("name", forms.NewCharField())), nil },
		SuccessURL: "/done/",
		GetSuccessMessage: func(*http.Request, map[string]any) (string, error) {
			return "", nil
		},
	}
	resp := mount(t, "/contact/", view).Post("/contact/", map[string]string{"name": "Ann"})
	djangotest.AssertStatus(t, resp, http.StatusFound)
}

// TestFormViewSuccessMessageWithoutMiddleware answers with an error when the
// message cannot be added, as messages.success raises MessageFailure. No
// Django test exists.
func TestFormViewSuccessMessageWithoutMiddleware(t *testing.T) {
	view := views.FormView{
		Engine:         genericEngine(t),
		Form:           func(*http.Request) (*forms.Form, error) { return forms.New(forms.F("name", forms.NewCharField())), nil },
		SuccessURL:     "/done/",
		SuccessMessage: "Thanks, %(name)s",
	}
	resp := mount(t, "/contact/", view).Post("/contact/", map[string]string{"name": "Ann"})
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
}

// TestDeleteViewSuccessMessageNotOnDelete adds the message after a POST, but
// not after a DELETE, which Django's DeletionMixin.delete answers without
// form_valid. No Django test exists.
func TestDeleteViewSuccessMessageNotOnDelete(t *testing.T) {
	eachDeleteDB(t, func(t *testing.T, db *orm.DB, author Author) {
		view := authorDelete(t, db)
		view.SuccessMessage = "Object was deleted successfully"
		c := withMessages(t, "/edit/author/{pk}/delete/", view)

		resp := c.Delete(fmt.Sprintf("/edit/author/%d/delete/", author.ID), nil)
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertMessages(t, resp, nil)
		assertAuthorCount(t, db, 0)
	})
}
