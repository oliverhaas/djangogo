package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// postDetailView renders a single post together with its comments and a comment
// form, and handles comment submissions. It mirrors a Django function view that
// dispatches on the request method: GET renders the page; POST binds the comment
// ModelForm, saves a valid Comment, then redirects to the same URL so a browser
// refresh does not resubmit (the Post/Redirect/Get pattern). An invalid POST
// re-renders the page with the bound form so its field errors are shown.
type postDetailView struct {
	db     *orm.DB
	engine *templates.Engine
}

// ServeHTTP implements http.Handler.
func (v postDetailView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pk, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	post, err := orm.Query[Post](v.db).Get(r.Context(), "id", pk)
	if err != nil {
		if errors.Is(err, orm.ErrDoesNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// The comment form exposes only the reader-supplied fields; Post is taken
	// from the URL and CreatedAt is stamped by auto_now_add.
	commentModel, ok := v.db.Registry().ModelOf(Comment{})
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	form := forms.FromModel(commentModel, forms.WithFields("Name", "Body"))

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		form = form.Bind(r.PostForm)
		if form.IsValid() {
			comment := &Comment{}
			if err := forms.PopulateStruct(commentModel, form.Cleaned(), comment); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			comment.Post.SetPK(pk)
			if err := orm.Query[Comment](v.db).Create(r.Context(), comment); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			views.Redirect(w, r, r.URL.Path, http.StatusFound)
			return
		}
		// Fall through: re-render with the bound (invalid) form.
	}

	comments, err := orm.Query[Comment](v.db).
		Filter("post_id", pk).
		OrderBy("created_at").
		All(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	postModel, _ := v.db.Registry().ModelOf(post)
	ctx := map[string]any{
		"post":       templates.ModelContext(postModel, post),
		"comments":   wrapComments(commentModel, comments),
		"form_html":  form.Render(),
		"csrf_token": csrf.Token(r.Context()),
	}
	_ = views.Render(w, v.engine, "post_detail.html", ctx)
}

// wrapComments turns each comment into a templates.ModelMap so the detail
// template can use snake_case access ({{ comment.name }}, {{ comment.body }})
// and render {{ comment }} as the comment's __str__ label.
func wrapComments(m *orm.Model, comments []Comment) []any {
	out := make([]any, len(comments))
	for i := range comments {
		out[i] = templates.ModelContext(m, comments[i])
	}
	return out
}
