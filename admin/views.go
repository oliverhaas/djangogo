package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// index renders the admin landing page grouped by app, listing only apps and
// models the current user has permission to access.
func (s *AdminSite) index(w http.ResponseWriter, r *http.Request) {
	ctx := s.EachContext(r)
	ctx["title"] = s.indexTitle()
	ctx["subtitle"] = ""
	ctx["app_list"] = ctx["available_apps"]
	s.render(w, r, "admin/index.html", ctx)
}

// appIndex renders the per-app admin index page, or 404 when the user has no
// visible models in the requested app.
func (s *AdminSite) appIndex(w http.ResponseWriter, r *http.Request) {
	appLabel := r.PathValue("app_label")
	appList := s.GetAppList(r, appLabel)
	if len(appList) == 0 {
		http.NotFound(w, r)
		return
	}
	appName, _ := appList[0]["name"].(string)
	ctx := s.EachContext(r)
	ctx["title"] = appName + " administration"
	ctx["subtitle"] = ""
	ctx["app_label"] = appLabel
	ctx["app_list"] = appList
	s.render(w, r, "admin/app_index.html", ctx)
}

// catchAllView runs for unmatched URLs under the admin prefix after
// HasPermission passes. If appending a trailing slash matches a real admin
// route, it redirects permanently; otherwise it responds 404.
func (s *AdminSite) catchAllView(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasSuffix(path, "/") {
		if m, err := s.reverseRouter().Resolve(path + "/"); err == nil {
			if opt, ok := m.Handler.(urls.SlashOptOut); !ok || opt.ShouldAppendSlash() {
				target := path + "/"
				if r.URL.RawQuery != "" {
					target += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, target, http.StatusMovedPermanently)
				return
			}
		}
	}
	http.NotFound(w, r)
}

// makeChangelistHandler binds e to changelist.
func (s *AdminSite) makeChangelistHandler(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.changelist(w, r, e) }
}

// makeAddHandler binds e to add.
func (s *AdminSite) makeAddHandler(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.add(w, r, e) }
}

// makeChangeHandler binds e to change.
func (s *AdminSite) makeChangeHandler(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.change(w, r, e) }
}

// makeDeleteHandler binds e to delete.
func (s *AdminSite) makeDeleteHandler(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.delete(w, r, e) }
}

// makeHistoryHandler binds e to history.
func (s *AdminSite) makeHistoryHandler(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.history(w, r, e) }
}

// makeOldChangeRedirect redirects Django's legacy "<pk>/" URL to "<pk>/change/".
func (s *AdminSite) makeOldChangeRedirect(e *entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		objectID := r.PathValue("object_id")
		target := s.reverse(e.urlName("change"), objectID)
		if target == "" {
			target = s.prefix() + "/" + e.routePrefix() + objectID + "/change/"
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
}

// changeForm returns the admin form of the add and change views for r and
// obj, Django's ModelAdmin.get_form and helpers.AdminForm: the fieldsets, the
// read-only fields, and the model form of the other fields of the layout with
// the widgets and help texts of formfield_for_manytomany and the options of
// its relations. obj is nil on the add page. instance is the object the form
// edits: obj on the change page, a new object with the field defaults on the
// add page. The validation of the bound form fills it and runs the model
// validation and the unique checks, as Django's ModelForm does, and the
// read-only fields show it. A user who cannot change obj sees every field of
// the layout read-only. An unknown or non-editable name in the layout is the
// Err of the form.
func (s *AdminSite) changeForm(r *http.Request, e *entry, obj, instance any, canChange bool) *adminForm {
	sets := s.fieldsets(r, e, obj)
	fields := formNames(e.model, flattenFieldsets(sets))
	readonly := e.readonlyFields(r, obj)
	exclude := slices.Concat(s.excluded(r, e, obj), readonly)
	if !canChange {
		exclude = append(exclude, fields...)
		readonly = fields
	}
	built := map[string]bool{}
	callback := func(mf *orm.Field, f forms.Field) forms.Field {
		built[mf.Attr] = true
		return formfieldForDBField(mf, f)
	}
	adminOpts := e.formOptions(r, obj)
	opts := slices.Concat([]forms.ModelFormOption{
		forms.WithFields(fields...),
		forms.WithExclude(exclude...),
		forms.WithFormFieldCallback(callback),
	}, s.relationWidgets(r, e), adminOpts)
	form := forms.FromInstance(s.db, e.model, instance, opts...)
	syncAutocompleteRequired(form)
	s.wrapRelatedWidgets(r, e, form, built)
	s.populateChoices(r, form, e)
	return &adminForm{form: form, sets: sets, readonly: readonly, obj: instance, opts: adminOpts}
}

// multipleSelectHint is the help text Django's admin adds to a <select
// multiple> of a many-to-many field.
const multipleSelectHint = "Hold down “Control”, or “Command” on a Mac, to select more than one."

// namesField reports whether names holds the Go name or the attribute name of
// f.
func namesField(names []string, f *orm.Field) bool {
	return slices.Contains(names, f.Name) || slices.Contains(names, f.Attr)
}

// excluded returns the field names the admin omits from a model's form for r
// and obj: GetExclude, else the configured ExcludeFields, Django's
// get_exclude, and the many-to-many fields with an explicit through model,
// which Django's formfield_for_manytomany leaves out.
func (s *AdminSite) excluded(r *http.Request, e *entry, obj any) []string {
	names := e.admin.ExcludeFields
	if e.admin.GetExclude != nil {
		names = e.admin.GetExclude(r, obj)
	}
	out := formNames(e.model, names)
	for _, mf := range e.model.ManyToMany() {
		if mf.Rel != nil && !mf.Rel.AutoCreated {
			out = append(out, mf.Attr)
		}
	}
	return out
}

// formFieldNames returns the set of the names of the fields of form.
func formFieldNames(form *forms.Form) map[string]bool {
	present := make(map[string]bool, len(form.Fields()))
	for _, fld := range form.Fields() {
		present[fld.Name] = true
	}
	return present
}

// populateChoices fills the list of every foreign-key and many-to-many field
// of the form with the related model's (pk, label) rows, in the ordering of
// the related model's admin (Django's get_field_queryset), else the related
// model's Meta.Ordering. The raw-id and autocomplete fields are left out:
// loadSelectedChoices loads only the rows they show.
func (s *AdminSite) populateChoices(r *http.Request, form *forms.Form, e *entry) {
	for name, opts := range s.relationChoices(r, e, form, "") {
		form.SetChoices(name, opts)
	}
}

// relationChoices returns the options of the foreign-key and many-to-many
// fields of form, without the field named skip and the fields that load only
// the rows they show.
func (s *AdminSite) relationChoices(r *http.Request, e *entry, form *forms.Form, skip string) map[string][][2]string {
	out := map[string][][2]string{}
	present := formFieldNames(form)
	limited := e.limitedFields(r)
	for _, mf := range slices.Concat(e.model.Relations(), e.model.ManyToMany()) {
		if mf.Rel == nil || mf.Rel.Target == nil || !present[mf.Attr] || mf.Attr == skip || slices.Contains(limited, mf) {
			continue
		}
		opts, err := orm.LabeledRowsOrdered(r.Context(), s.db, mf.Rel.Target, s.relatedOrdering(r, mf.Rel.Target), nil)
		if err != nil {
			continue
		}
		out[mf.Attr] = opts
	}
	return out
}

// relatedOrdering returns the ordering of the admin of the related model
// target: its GetOrdering or Ordering, with the names translated to the
// model's attribute paths. It is nil when target has no admin or the admin no
// ordering.
func (s *AdminSite) relatedOrdering(r *http.Request, target *orm.Model) []string {
	relEntry := s.entryByModel(target)
	if relEntry == nil {
		return nil
	}
	ordering := relEntry.admin.Ordering
	if relEntry.admin.GetOrdering != nil {
		ordering = relEntry.admin.GetOrdering(r)
	}
	if len(ordering) == 0 {
		return nil
	}
	norm := make([]string, len(ordering))
	for i, o := range ordering {
		norm[i] = normalizeFieldPath(target, o)
	}
	return norm
}

// manyInitial sets the initial value of every many-to-many field of form to
// the primary keys of the rows obj is related to.
func (s *AdminSite) manyInitial(ctx context.Context, form *forms.Form, e *entry, obj any) error {
	present := formFieldNames(form)
	for _, mf := range e.model.ManyToMany() {
		if !present[mf.Attr] {
			continue
		}
		pks, err := orm.ManyValues(ctx, s.db, obj, mf)
		if err != nil {
			return err
		}
		form.Initial[mf.Attr] = pks
	}
	return nil
}

// saveManyToMany writes the cleaned many-to-many fields of form for obj,
// Django's ModelForm._save_m2m.
func (s *AdminSite) saveManyToMany(ctx context.Context, e *entry, form *forms.Form, obj any) error {
	cleaned := form.Cleaned()
	for _, mf := range e.model.ManyToMany() {
		value, ok := cleaned[mf.Attr]
		if !ok {
			continue
		}
		pks, _ := value.([]any)
		if err := orm.SetManyPKs(ctx, s.db, obj, mf, pks); err != nil {
			return err
		}
	}
	return nil
}

// add handles the add form: requires add permission, validates POST data,
// creates a row, sets a success message, and redirects per Django's
// response_add rules.
func (s *AdminSite) add(w http.ResponseWriter, r *http.Request, e *entry) {
	if e.rejectToField(w, r.FormValue(toFieldVar)) {
		return
	}
	if !e.hasAddPermission(s, r) {
		forbidden(w)
		return
	}
	hooks := e.admin.hooks
	if hooks != nil && hooks.allowAdd != nil && !hooks.allowAdd(s, w, r, e) {
		return
	}
	singular, _ := modelVerboseNames(e.model)
	title := "Add " + singular
	// The form fills the new row and the inlines point at it; the save gives
	// it its primary key before the rows of the inlines are saved.
	obj := e.ops.newPtr()
	e.model.ApplyDefaults(obj)
	af := s.changeForm(r, e, nil, obj, true)
	if err := af.form.Err(); err != nil {
		http.Error(w, "invalid admin form: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, ok := postData(w, r)
	if !ok {
		return
	}
	inlines, err := s.inlineFormSets(r, e, obj, nil, data)
	if err != nil {
		http.Error(w, "invalid admin inline: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if data != nil {
		af.form = af.form.Bind(data)
		form := af.form
		if err := s.loadSelectedChoices(r, e, form); err != nil {
			http.Error(w, "failed to load related rows", http.StatusInternalServerError)
			return
		}
		valid, err := validChangeForm(form, inlines)
		if err != nil {
			http.Error(w, "failed to validate the form", http.StatusInternalServerError)
			return
		}
		if valid {
			err := s.db.Atomic(r.Context(), func(ctx context.Context) error {
				if hooks != nil && hooks.beforeCreate != nil {
					if err := hooks.beforeCreate(form, obj); err != nil {
						return err
					}
				}
				if err := e.ops.create(ctx, obj); err != nil {
					return err
				}
				if err := s.saveManyToMany(ctx, e, form, obj); err != nil {
					return err
				}
				return saveInlines(ctx, inlines)
			})
			if err != nil {
				http.Error(w, "failed to create row", http.StatusInternalServerError)
				return
			}
			if r.PostForm.Has(isPopupVar) {
				s.checkSourceModel(r)
				pk := pkOf(reflect.ValueOf(obj).Elem(), e)
				s.popupResponse(w, r, "value", strconv.FormatInt(pk, 10), "obj", orm.Label(e.model, obj))
				return
			}
			objDisplay := displayLabel(e.model, obj)
			msg := fmt.Sprintf("The %s “%s” was added successfully.", singular, objDisplay)
			continueEditing := r.PostForm.Has("_continue") ||
				hooks != nil && hooks.continueAfterAdd && !r.PostForm.Has("_addanother")
			if continueEditing && e.hasViewOrChangePermission(s, r, obj) {
				if e.hasChangePermission(s, r, obj) {
					msg += " You may edit it again below."
				}
				_ = messages.Success(r, msg)
				pk := pkOf(reflect.ValueOf(obj).Elem(), e)
				http.Redirect(w, r, s.changeURL(e, pk), http.StatusFound)
				return
			}
			if r.PostForm.Has("_addanother") {
				_ = messages.Success(r, fmt.Sprintf("%s You may add another %s below.", msg, singular))
				http.Redirect(w, r, r.URL.Path, http.StatusFound)
				return
			}
			_ = messages.Success(r, msg)
			s.redirectPostSave(w, r, e)
			return
		}
	}
	s.renderChangeForm(w, r, e, title, nil, af, inlines, true, true)
}

// postData parses the body of a POST request and returns its form data, nil
// for other methods. It answers a body that does not parse with 400 and
// returns false.
func postData(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if r.Method != http.MethodPost {
		return nil, true
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return nil, false
	}
	return r.PostForm, true
}

// validChangeForm validates the bound form and the formsets of the inlines,
// all of them as Django's _changeform_view does, and reports whether all are
// valid. The form validates first: it fills its instance, valid or not, as
// Django's ModelForm._post_clean does, and the formsets of the inlines see
// the parent with the posted values. The error is a failure of the form
// validation that is not a validation error.
func validChangeForm(form *forms.Form, inlines []*inlineFormSet) (bool, error) {
	formValid := form.IsValid()
	if err := form.Err(); err != nil {
		return false, err
	}
	return inlinesValid(inlines) && formValid, nil
}

// change handles the change/view form: checks view or change permission on the
// loaded object, rejects POST from view-only users with 403, renders a
// read-only form for view-only users, and updates through orm.Save for users
// with change permission.
func (s *AdminSite) change(w http.ResponseWriter, r *http.Request, e *entry) {
	if e.rejectToField(w, r.FormValue(toFieldVar)) {
		return
	}
	objectID := r.PathValue("object_id")
	obj, pk, err := s.loadObject(r.Context(), e, objectID)
	if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	if !e.hasViewOrChangePermission(s, r, obj) {
		forbidden(w)
		return
	}
	if obj == nil {
		s.objDoesNotExistRedirect(w, r, e, objectID)
		return
	}

	canChange := e.hasChangePermission(s, r, obj)
	singular, _ := modelVerboseNames(e.model)
	if r.Method == http.MethodPost && !canChange {
		forbidden(w)
		return
	}
	af := s.changeForm(r, e, obj, obj, canChange)
	if err := af.form.Err(); err != nil {
		http.Error(w, "invalid admin form: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// The initial values are the stored ones, Django's model_to_dict of the
	// instance: a disabled field, such as the password of a user, cleans to
	// its initial value and keeps the stored one.
	af.form.Initial = forms.InitialFromStruct(e.model, obj)
	if err := s.manyInitial(r.Context(), af.form, e, obj); err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	data, ok := postData(w, r)
	if !ok {
		return
	}
	inlines, err := s.inlineFormSets(r, e, obj, obj, data)
	if err != nil {
		http.Error(w, "invalid admin inline: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if data != nil {
		af.form = af.form.Bind(data)
		form := af.form
		if err := s.loadSelectedChoices(r, e, form); err != nil {
			http.Error(w, "failed to load related rows", http.StatusInternalServerError)
			return
		}
		valid, err := validChangeForm(form, inlines)
		if err != nil {
			http.Error(w, "failed to validate the form", http.StatusInternalServerError)
			return
		}
		if valid {
			err := s.db.Atomic(r.Context(), func(ctx context.Context) error {
				if err := e.ops.update(ctx, pk, obj); err != nil {
					return err
				}
				if err := s.saveManyToMany(ctx, e, form, obj); err != nil {
					return err
				}
				return saveInlines(ctx, inlines)
			})
			if err != nil {
				http.Error(w, "failed to update row", http.StatusInternalServerError)
				return
			}
			if r.PostForm.Has(isPopupVar) {
				newPK := pkOf(reflect.ValueOf(obj).Elem(), e)
				s.popupResponse(w, r, "action", "change", "value", objectID,
					"obj", orm.Label(e.model, obj), "new_value", strconv.FormatInt(newPK, 10))
				return
			}
			objDisplay := displayLabel(e.model, obj)
			msg := fmt.Sprintf("The %s “%s” was changed successfully.", singular, objDisplay)
			if r.PostForm.Has("_continue") {
				_ = messages.Success(r, msg+" You may edit it again below.")
				http.Redirect(w, r, r.URL.Path, http.StatusFound)
				return
			}
			if r.PostForm.Has("_addanother") && e.hasAddPermission(s, r) {
				_ = messages.Success(r, fmt.Sprintf("%s You may add another %s below.", msg, singular))
				http.Redirect(w, r, s.addURL(e), http.StatusFound)
				return
			}
			_ = messages.Success(r, msg)
			s.redirectPostSave(w, r, e)
			return
		}
		s.renderChangeForm(w, r, e, "Change "+singular, obj, af, inlines, false, true)
		return
	}

	if !canChange {
		s.renderChangeForm(w, r, e, "View "+singular, obj, af, inlines, false, false)
		return
	}
	if err := s.loadSelectedChoices(r, e, af.form); err != nil {
		http.Error(w, "failed to load related rows", http.StatusInternalServerError)
		return
	}
	s.renderChangeForm(w, r, e, "Change "+singular, obj, af, inlines, false, true)
}

// delete handles the delete confirmation: checks delete permission on the
// loaded object, redirects with a warning if nonexistent, and on POST deletes
// the row and redirects to changelist (with change perm) or admin index.
func (s *AdminSite) delete(w http.ResponseWriter, r *http.Request, e *entry) {
	if e.rejectToField(w, r.FormValue(toFieldVar)) {
		return
	}
	objectID := r.PathValue("object_id")
	obj, pk, err := s.loadObject(r.Context(), e, objectID)
	if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	if !e.hasDeletePermission(s, r, obj) {
		forbidden(w)
		return
	}
	if obj == nil {
		s.objDoesNotExistRedirect(w, r, e, objectID)
		return
	}

	singular, plural := modelVerboseNames(e.model)
	objDisplay := displayLabel(e.model, obj)

	if r.Method == http.MethodPost {
		if err := e.ops.del(r.Context(), pk); err != nil {
			http.Error(w, "failed to delete row", http.StatusInternalServerError)
			return
		}
		if r.PostForm.Has(isPopupVar) {
			s.popupResponse(w, r, "action", "delete", "value", strconv.FormatInt(pk, 10))
			return
		}
		_ = messages.Success(r, fmt.Sprintf("The %s “%s” was deleted successfully.", singular, objDisplay))
		if e.hasChangePermission(s, r, nil) {
			http.Redirect(w, r, s.changelistURL(e), http.StatusFound)
			return
		}
		http.Redirect(w, r, s.indexURL(), http.StatusFound)
		return
	}

	ctx := s.EachContext(r)
	ctx["title"] = "Delete"
	ctx["subtitle"] = ""
	ctx["object"] = objDisplay
	ctx["object_name"] = singular
	ctx["app_label"] = e.app
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["has_view_permission"] = e.hasViewOrChangePermission(s, r, nil)
	ctx["changelist_url"] = s.changelistURL(e)
	ctx["change_url"] = s.changeURL(e, pk)
	ctx["action"] = r.URL.Path
	ctx["media"] = e.media(s)
	ctx["is_popup"] = isPopup(r)
	ctx["is_popup_var"] = isPopupVar
	ctx["to_field"] = r.FormValue(toFieldVar)
	ctx["to_field_var"] = toFieldVar
	s.render(w, r, "admin/delete_confirmation.html", ctx)
}

// history handles the per-object change history view: requires view or change
// permission on the object and redirects to admin:index when nonexistent.
func (s *AdminSite) history(w http.ResponseWriter, r *http.Request, e *entry) {
	objectID := r.PathValue("object_id")
	obj, pk, err := s.loadObject(r.Context(), e, objectID)
	if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	if !e.hasViewOrChangePermission(s, r, obj) {
		forbidden(w)
		return
	}
	if obj == nil {
		s.objDoesNotExistRedirect(w, r, e, objectID)
		return
	}

	_, plural := modelVerboseNames(e.model)
	objDisplay := displayLabel(e.model, obj)
	ctx := s.EachContext(r)
	ctx["title"] = "Change history: " + objDisplay
	ctx["subtitle"] = ""
	ctx["object"] = objDisplay
	ctx["app_label"] = e.app
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["has_view_permission"] = e.hasViewOrChangePermission(s, r, nil)
	ctx["changelist_url"] = s.changelistURL(e)
	ctx["change_url"] = s.changeURL(e, pk)
	s.render(w, r, "admin/object_history.html", ctx)
}

// renderChangeForm renders admin/change_form.html for add, change, or
// view-only change requests. errors in the context is the number of fields
// with errors, counting the errors of the whole form as one, as the length of
// Django's AdminErrorList.
func (s *AdminSite) renderChangeForm(
	w http.ResponseWriter,
	r *http.Request,
	e *entry,
	title string,
	obj any,
	af *adminForm,
	inlines []*inlineFormSet,
	isAdd bool,
	canChange bool,
) {
	singular, plural := modelVerboseNames(e.model)
	ctx := s.EachContext(r)
	ctx["title"] = title
	if obj != nil {
		ctx["subtitle"] = displayLabel(e.model, obj)
		ctx["original"] = obj
	} else {
		ctx["subtitle"] = ""
	}
	ctx["add"] = isAdd
	ctx["change"] = !isAdd
	ctx["can_change"] = canChange
	ctx["app_label"] = e.app
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["model_name"] = e.slug
	ctx["verbose_name"] = singular
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["changelist_url"] = s.changelistURL(e)
	ctx["action"] = r.URL.Path
	popup := isPopup(r)
	ctx["is_popup"] = popup
	ctx["is_popup_var"] = isPopupVar
	ctx["to_field"] = r.FormValue(toFieldVar)
	ctx["to_field_var"] = toFieldVar
	ctx["source_model"] = r.URL.Query().Get(sourceModelVar)
	ctx["source_model_var"] = sourceModelVar

	hasAdd := e.hasAddPermission(s, r)
	hasModelView := e.hasViewOrChangePermission(s, r, nil)
	hasObjView := e.hasViewPermission(s, r, obj)
	hasObjChange := e.hasChangePermission(s, r, obj)
	hasObjDelete := !isAdd && obj != nil && e.hasDeletePermission(s, r, obj)

	ctx["has_add_permission"] = hasAdd
	ctx["has_view_permission"] = hasModelView
	ctx["has_change_permission"] = hasObjChange
	ctx["has_delete_permission"] = hasObjDelete

	ctx["form"] = af.form
	ctx["adminform"] = s.fieldsetContext(r.Context(), e, af)
	ctx["inline_admin_formsets"] = s.inlineContexts(r.Context(), inlines)
	ctx["media"] = e.media(s).Add(af.form.Media()).Add(s.inlinesMedia(inlines))
	ctx["jsi18n_url"] = s.jsi18nURL()
	ctx["errors"] = 0
	if af.form.IsBound() {
		ctx["errors"] = af.form.Errors().Len() + inlineErrorCount(inlines)
		ctx["non_field_errors"] = templates.MarkSafe(af.form.NonFieldErrors().AsUL())
	}
	if hasObjDelete {
		pk := pkOf(reflect.ValueOf(obj).Elem(), e)
		ctx["delete_url"] = s.deleteURL(e, pk)
	}

	// Submit-row button visibility mirrors Django's submit_row template tag.
	showSave := isAdd || canChange
	ctx["show_save"] = showSave
	ctx["show_save_and_add_another"] = !popup && hasAdd && showSave
	ctx["show_save_and_continue"] = !popup && showSave && (hasObjChange || hasObjView)
	ctx["show_close"] = !showSave && hasModelView
	ctx["show_delete_link"] = !popup && hasObjDelete

	name := "admin/change_form.html"
	if isAdd && e.admin.hooks != nil && e.admin.hooks.addFormTemplate != "" {
		name = e.admin.hooks.addFormTemplate
	}
	s.render(w, r, name, ctx)
}

// manyLabels returns the labels of the rows obj is related to through the
// many-to-many field f, separated by ", ", or "" when there are none or they
// cannot be read.
func (s *AdminSite) manyLabels(ctx context.Context, obj any, f *orm.Field) string {
	pks, err := orm.ManyValues(ctx, s.db, obj, f)
	if err != nil || len(pks) == 0 {
		return ""
	}
	rows, err := orm.LabeledRowsOrdered(ctx, s.db, f.Rel.Target, nil, pks)
	if err != nil {
		return ""
	}
	labels := make([]string, len(rows))
	for i, row := range rows {
		labels[i] = row[1]
	}
	return strings.Join(labels, ", ")
}

// isPopup reports whether r comes from a popup window, Django's IS_POPUP_VAR
// in request.POST or request.GET.
func isPopup(r *http.Request) bool {
	_ = r.ParseForm()
	return r.Form.Has(isPopupVar)
}

// popupResponse writes the page that closes a popup window and hands data to
// the window that opened it, Django's admin/popup_response.html. pairs are
// the keys and values of the data, written in order as Python's json.dumps
// writes an object.
func (s *AdminSite) popupResponse(w http.ResponseWriter, r *http.Request, pairs ...string) {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(templates.QuoteJSON(pairs[i]) + ": " + templates.QuoteJSON(pairs[i+1]))
	}
	b.WriteByte('}')
	s.render(w, r, "admin/popup_response.html", map[string]any{"popup_response_data": b.String()})
}

// checkSourceModel adds Django's error message to the request when the
// _source_model of a popup add names no model. Django then looks for the
// choice of the new object in the option groups of the source model's form;
// Go choices have no option groups, so a model that exists needs no lookup.
func (s *AdminSite) checkSourceModel(r *http.Request) {
	name := r.PostForm.Get(sourceModelVar)
	if name == "" {
		return
	}
	if app, model, ok := strings.Cut(name, "."); ok {
		for _, m := range s.db.Registry().AppModels(app) {
			if strings.EqualFold(m.Name(), model) {
				return
			}
		}
	}
	_ = messages.Error(r, fmt.Sprintf(`The app "%s" could not be found.`, name))
}

// redirectPostSave redirects after a successful add or change save: to the
// changelist when the user has model-level view or change permission, and to
// the admin index otherwise (Django's ModelAdmin._response_post_save).
func (s *AdminSite) redirectPostSave(w http.ResponseWriter, r *http.Request, e *entry) {
	if e.hasViewOrChangePermission(s, r, nil) {
		http.Redirect(w, r, s.changelistURL(e), http.StatusFound)
		return
	}
	http.Redirect(w, r, s.indexURL(), http.StatusFound)
}

// parseObjectPK parses objectID as a base-10 int64 primary key.
func parseObjectPK(objectID string) (int64, bool) {
	pk, err := strconv.ParseInt(objectID, 10, 64)
	return pk, err == nil
}

// loadObject parses objectID as an int64 primary key and loads the matching row
// of e. It returns (nil, 0, nil) when objectID is non-numeric or no row exists.
func (s *AdminSite) loadObject(ctx context.Context, e *entry, objectID string) (any, int64, error) {
	pk, ok := parseObjectPK(objectID)
	if !ok {
		return nil, 0, nil
	}
	obj, err := e.ops.get(ctx, pk)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, pk, nil
	}
	if err != nil {
		return nil, pk, err
	}
	return obj, pk, nil
}

// objDoesNotExistRedirect adds Django's missing-object warning message and
// redirects to admin:index.
func (s *AdminSite) objDoesNotExistRedirect(w http.ResponseWriter, r *http.Request, e *entry, objectID string) {
	singular, _ := modelVerboseNames(e.model)
	msg := fmt.Sprintf("%s with ID “%s” doesn’t exist. Perhaps it was deleted?", singular, objectID)
	_ = messages.Warning(r, msg)
	http.Redirect(w, r, s.indexURL(), http.StatusFound)
}

// appURLOrEmpty returns the app index URL for appLabel, or "" when empty.
func (s *AdminSite) appURLOrEmpty(appLabel string) string {
	if appLabel == "" {
		return ""
	}
	return s.appIndexURL(appLabel)
}

// displayLabel returns orm.Label(m, obj), or emptyValueDisplay ("-") when the
// object's label is empty (Django's display_for_value(str(obj), "-")).
func displayLabel(m *orm.Model, obj any) string {
	if label := orm.Label(m, obj); label != "" {
		return label
	}
	return emptyValueDisplay
}

// forbidden writes Django's 403 Forbidden response.
func forbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("<h1>403 Forbidden</h1>"))
}

// neverCache sets response headers that prevent caching of admin pages,
// matching Django's never_cache decorator.
func neverCache(w http.ResponseWriter) {
	h := w.Header()
	if h.Get("Expires") == "" {
		h.Set("Expires", time.Now().UTC().Format(http.TimeFormat))
	}
	h.Set("Cache-Control", "max-age=0, no-cache, no-store, must-revalidate, private")
}

// render writes the named template with ctx, reporting a 500 on failure. It
// renders through the request with this site's instance namespace as the
// current app so {% url 'admin:...' %} resolves to this site.
func (s *AdminSite) render(w http.ResponseWriter, r *http.Request, name string, ctx map[string]any) {
	req := r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName()))
	out, err := s.engine.RenderRequest(req, name, ctx)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(out))
}
