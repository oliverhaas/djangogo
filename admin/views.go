package admin

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"

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
	if s.logEnabled() {
		recent, err := s.recentActions(r)
		if err != nil {
			http.Error(w, "failed to load the recent actions", http.StatusInternalServerError)
			return
		}
		ctx["log_enabled"] = true
		ctx["admin_log"] = recent
	}
	maps.Copy(ctx, s.extraContext(r, "index"))
	s.render(w, r, cmp.Or(s.IndexTemplate, "admin/index.html"), ctx)
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
	maps.Copy(ctx, s.extraContext(r, "app_index"))
	names := []string{"admin/" + appLabel + "/app_index.html", "admin/app_index.html"}
	if s.AppIndexTemplate != "" {
		names = []string{s.AppIndexTemplate}
	}
	s.renderFirst(w, r, names, ctx)
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
	excluded, own := s.excluded(r, e, obj)
	exclude := slices.Concat(excluded, readonly)
	if !canChange {
		exclude = append(exclude, fields...)
		readonly = fields
	}
	built := map[string]bool{}
	adminOpts := e.formOptions(r, obj)
	opts := slices.Concat([]forms.ModelFormOption{
		forms.WithFields(fields...),
		forms.WithFormFieldCallback(e.formfieldCallback(r, built)),
	}, s.relationWidgets(r, e), adminOpts, []forms.ModelFormOption{excludeOption(e.model, exclude, own)})
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
// and obj, GetExclude, else the configured ExcludeFields (Django's
// get_exclude), and whether the admin defines them, a list that is not nil.
func (s *AdminSite) excluded(r *http.Request, e *entry, obj any) ([]string, bool) {
	names := e.admin.ExcludeFields
	if e.admin.GetExclude != nil {
		names = e.admin.GetExclude(r, obj)
	}
	return formNames(e.model, names), names != nil
}

// excludeOption returns the option that drops exclude from the model form of
// m, the exclude argument of modelform_factory in Django's get_form and
// get_formset. When the admin defines its own exclusions (own) and exclude is
// not empty, it replaces the exclusions of the form options, else it adds to
// them. It also drops the many-to-many fields with an explicit through
// model, which Django's formfield_for_manytomany leaves out. Pass it after
// the form options.
func excludeOption(m *orm.Model, exclude []string, own bool) forms.ModelFormOption {
	out := slices.Clone(exclude)
	for _, mf := range m.ManyToMany() {
		if mf.Rel != nil && !mf.Rel.AutoCreated {
			out = append(out, mf.Attr)
		}
	}
	if own && len(exclude) > 0 {
		return forms.ReplaceExclude(out...)
	}
	return forms.WithExclude(out...)
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

// saveObject saves obj with save, then the many-to-many fields of form and
// the inlines, through the SaveModel and SaveRelated hooks of e. r carries
// the transaction of the save. change is false for a new object.
func (s *AdminSite) saveObject(r *http.Request, e *entry, obj any, form *forms.Form, inlines []*inlineFormSet, change bool, save func(ctx context.Context) error) error {
	sc := &SaveContext{
		Site: s, DB: s.db, Model: e.model, Obj: obj, Form: form, Change: change,
		Save: save,
		SaveRelated: func(ctx context.Context) error {
			if err := s.saveManyToMany(ctx, e, form, obj); err != nil {
				return err
			}
			return saveInlines(ctx, inlines)
		},
	}
	ctx := r.Context()
	var err error
	if e.admin.SaveModel != nil {
		err = e.admin.SaveModel(r, sc)
	} else {
		err = sc.Save(ctx)
	}
	if err != nil {
		return err
	}
	if e.admin.SaveRelated != nil {
		return e.admin.SaveRelated(r, sc)
	}
	return sc.SaveRelated(ctx)
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
	s.addForm(w, r, e)
}

// saveAsNewVar is the name of the "Save as new" button.
const saveAsNewVar = "_saveasnew"

// saveAsContinue returns e's SaveAsContinue (default true).
func (e *entry) saveAsContinue() bool {
	if e.admin.SaveAsContinue != nil {
		return *e.admin.SaveAsContinue
	}
	return true
}

// addForm shows and saves the add form of e for a user with the add
// permission. A submission of "Save as new", from the add or the change
// page, saves the inlines as new rows too, and a failed one shows the form
// as a change form with only the "Save as new" button, as Django's
// _changeform_view does.
func (s *AdminSite) addForm(w http.ResponseWriter, r *http.Request, e *entry) {
	hooks := e.admin.hooks
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
	saveAsNew := data.Has(saveAsNewVar)
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
				create := func(ctx context.Context) error { return e.ops.create(ctx, obj) }
				if err := s.saveObject(r.WithContext(ctx), e, obj, form, inlines, false, create); err != nil {
					return err
				}
				return s.logFormChange(ctx, r, obj, form, inlines, true)
			})
			if err != nil {
				http.Error(w, "failed to create row", http.StatusInternalServerError)
				return
			}
			if r.PostForm.Has(isPopupVar) {
				s.checkSourceModel(r)
				pk := pkOf(reflect.ValueOf(obj).Elem(), e)
				s.popupResponse(w, r, e, "value", strconv.FormatInt(pk, 10), "obj", orm.Label(e.model, obj))
				return
			}
			// Link the object when the user can change it.
			pk := pkOf(reflect.ValueOf(obj).Elem(), e)
			objURL := ""
			if e.hasChangePermission(s, r, obj) {
				objURL = s.changeURL(e, pk)
			}
			msg := fmt.Sprintf("The %s “%s” was added successfully.",
				forms.Escape(singular), objectHTML(displayLabel(e.model, obj), objURL))
			continueEditing := r.PostForm.Has("_continue") ||
				saveAsNew && e.saveAsContinue() && objURL != "" ||
				hooks != nil && hooks.continueAfterAdd && !r.PostForm.Has("_addanother")
			// The next page keeps the query string of this one.
			preserved := s.preservedFilters(r, e)
			qsl := preservedQSL(r, preserved)
			if continueEditing && e.hasViewOrChangePermission(s, r, obj) {
				if objURL != "" {
					msg += " You may edit it again below."
				}
				_ = messages.AddSafe(r, messages.LevelSuccess, msg)
				http.Redirect(w, r, s.addPreservedFilters(e, s.changeURL(e, pk), preserved, qsl, false, ""), http.StatusFound)
				return
			}
			if r.PostForm.Has("_addanother") {
				_ = messages.AddSafe(r, messages.LevelSuccess,
					fmt.Sprintf("%s You may add another %s below.", msg, forms.Escape(singular)))
				http.Redirect(w, r, s.addPreservedFilters(e, r.URL.Path, preserved, qsl, false, ""), http.StatusFound)
				return
			}
			_ = messages.AddSafe(r, messages.LevelSuccess, msg)
			s.redirectPostSave(w, r, e)
			return
		}
	}
	if saveAsNew {
		// Show the change form with only "Save as new", so that the page
		// does not offer to save the original row.
		s.renderChangeForm(w, r, e, title, nil, af, inlines, formSaveAsNew)
		return
	}
	s.renderChangeForm(w, r, e, title, nil, af, inlines, formAdd)
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
	if r.Method == http.MethodPost {
		data, ok := postData(w, r)
		if !ok {
			return
		}
		// "Save as new" adds a row, as Django's _changeform_view drops the
		// object ID: it needs the add permission, not the change permission.
		if data.Has(saveAsNewVar) {
			if !e.hasAddPermission(s, r) {
				forbidden(w)
				return
			}
			s.addForm(w, r, e)
			return
		}
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
				update := func(ctx context.Context) error { return e.ops.update(ctx, pk, obj) }
				if err := s.saveObject(r.WithContext(ctx), e, obj, form, inlines, true, update); err != nil {
					return err
				}
				return s.logFormChange(ctx, r, obj, form, inlines, false)
			})
			if err != nil {
				http.Error(w, "failed to update row", http.StatusInternalServerError)
				return
			}
			if r.PostForm.Has(isPopupVar) {
				newPK := pkOf(reflect.ValueOf(obj).Elem(), e)
				s.popupResponse(w, r, e, "action", "change", "value", objectID,
					"obj", orm.Label(e.model, obj), "new_value", strconv.FormatInt(newPK, 10))
				return
			}
			msg := fmt.Sprintf("The %s “%s” was changed successfully.",
				forms.Escape(singular), objectHTML(displayLabel(e.model, obj), r.URL.Path))
			// The next page keeps the query string of this one.
			preserved := s.preservedFilters(r, e)
			qsl := preservedQSL(r, preserved)
			if r.PostForm.Has("_continue") {
				_ = messages.AddSafe(r, messages.LevelSuccess, msg+" You may edit it again below.")
				http.Redirect(w, r, s.addPreservedFilters(e, r.URL.Path, preserved, qsl, false, ""), http.StatusFound)
				return
			}
			if r.PostForm.Has("_addanother") && e.hasAddPermission(s, r) {
				_ = messages.AddSafe(r, messages.LevelSuccess,
					fmt.Sprintf("%s You may add another %s below.", msg, forms.Escape(singular)))
				http.Redirect(w, r, s.addPreservedFilters(e, s.addURL(e), preserved, qsl, false, ""), http.StatusFound)
				return
			}
			_ = messages.AddSafe(r, messages.LevelSuccess, msg)
			s.redirectPostSave(w, r, e)
			return
		}
		s.renderChangeForm(w, r, e, "Change "+singular, obj, af, inlines, formChange)
		return
	}

	if !canChange {
		s.renderChangeForm(w, r, e, "View "+singular, obj, af, inlines, formView)
		return
	}
	if err := s.loadSelectedChoices(r, e, af.form); err != nil {
		http.Error(w, "failed to load related rows", http.StatusInternalServerError)
		return
	}
	s.renderChangeForm(w, r, e, "Change "+singular, obj, af, inlines, formChange)
}

// delete handles the delete page, Django's ModelAdmin._delete_view: it
// checks delete permission on the loaded object, redirects with a warning if
// it does not exist, lists the objects that the deletion cascades to, and on
// a confirmed POST deletes the object unless protected objects block it or
// the user may not delete one of the cascaded models.
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

	deleted, err := s.getDeletedObjects(r, e, []any{obj})
	if err != nil {
		http.Error(w, "failed to collect related rows", http.StatusInternalServerError)
		return
	}

	singular, plural := modelVerboseNames(e.model)
	objDisplay := displayLabel(e.model, obj)

	// Django tests request.POST, which is false for a POST without data.
	if r.Method == http.MethodPost && len(r.PostForm) > 0 && len(deleted.Protected) == 0 {
		if len(deleted.PermsNeeded) > 0 {
			forbidden(w)
			return
		}
		// Log first, as Django's log_deletions must run before the deletion.
		err := s.db.Atomic(r.Context(), func(ctx context.Context) error {
			if err := s.logActions(ctx, r, []any{obj}, Deletion, ""); err != nil {
				return err
			}
			dc := &DeleteContext{
				Site: s, DB: s.db, Model: e.model, Obj: obj,
				Delete: func(ctx context.Context) error { return e.ops.del(ctx, pk) },
			}
			if e.admin.DeleteModel != nil {
				return e.admin.DeleteModel(r.WithContext(ctx), dc)
			}
			return dc.Delete(ctx)
		})
		if err != nil {
			http.Error(w, "failed to delete row", http.StatusInternalServerError)
			return
		}
		if r.PostForm.Has(isPopupVar) {
			s.popupResponse(w, r, e, "action", "delete", "value", strconv.FormatInt(pk, 10))
			return
		}
		_ = messages.Success(r, fmt.Sprintf("The %s “%s” was deleted successfully.", singular, objDisplay))
		if e.hasChangePermission(s, r, nil) {
			u := s.addPreservedFilters(e, s.changelistURL(e), s.preservedFilters(r, e), nil, false, "")
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
		http.Redirect(w, r, s.indexURL(), http.StatusFound)
		return
	}

	title := "Delete"
	if len(deleted.PermsNeeded) > 0 || len(deleted.Protected) > 0 {
		title = "Cannot delete " + singular
	}
	maxDisplay := e.admin.DeleteConfirmationMaxDisplay
	modelCounts := make([]map[string]any, len(deleted.ModelCount))
	for i, mc := range deleted.ModelCount {
		modelCounts[i] = map[string]any{"name": capFirst(mc.Name), "count": mc.Count}
	}

	ctx := s.EachContext(r)
	ctx["title"] = title
	ctx["subtitle"] = ""
	ctx["object"] = objDisplay
	ctx["object_name"] = singular
	ctx["app_label"] = e.app
	ctx["model_name"] = e.slug
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["has_view_permission"] = e.hasViewOrChangePermission(s, r, nil)
	ctx["changelist_url"] = s.changelistURL(e)
	ctx["change_url"] = s.changeURL(e, pk)
	ctx["model_count"] = modelCounts
	ctx["has_perms_lacking"] = len(deleted.PermsNeeded) > 0
	ctx["has_protected"] = len(deleted.Protected) > 0
	ctx["hide_objects"] = maxDisplay != nil && *maxDisplay == 0
	ctx["perms_lacking_html"] = forms.SafeHTML(truncatedUnorderedList(textItems(deleted.PermsNeeded), maxDisplay))
	ctx["protected_html"] = forms.SafeHTML(truncatedUnorderedList(htmlItems(deleted.Protected), maxDisplay))
	ctx["deleted_objects_html"] = forms.SafeHTML(truncatedUnorderedList(deleted.Items, maxDisplay))
	ctx["media"] = e.media(s)
	ctx["is_popup"] = isPopup(r)
	ctx["is_popup_var"] = isPopupVar
	ctx["to_field"] = r.FormValue(toFieldVar)
	ctx["to_field_var"] = toFieldVar
	maps.Copy(ctx, e.extraContext(r, "delete"))
	s.renderPage(w, r, e, e.admin.DeleteConfirmationTemplate, "delete_confirmation.html", ctx)
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
	ctx["module_name"] = capFirst(plural)
	ctx["opts_verbose_name"], _ = modelVerboseNames(e.model)
	ctx["page_var"] = pageVar
	if err := s.historyContext(r, e, objectID, ctx); err != nil {
		http.Error(w, "failed to load the history", http.StatusInternalServerError)
		return
	}
	maps.Copy(ctx, e.extraContext(r, "history"))
	s.renderPage(w, r, e, e.admin.ObjectHistoryTemplate, "object_history.html", ctx)
}

// extraContext returns the AdminSite.ExtraContext values of view for r.
func (s *AdminSite) extraContext(r *http.Request, view string) map[string]any {
	if s.ExtraContext == nil {
		return nil
	}
	return s.ExtraContext(r, view)
}

// extraContext returns the ModelAdmin.ExtraContext values of view for r.
func (e *entry) extraContext(r *http.Request, view string) map[string]any {
	if e.admin.ExtraContext == nil {
		return nil
	}
	return e.admin.ExtraContext(r, view)
}

// changeFormOwnKeys are the keys of the add and change pages that the extra
// context does not replace: Django's render_change_form sets them after it
// merges extra_context, and the submit_row tag computes the buttons.
var changeFormOwnKeys = map[string]bool{
	"add": true, "change": true, "app_label": true, "form_url": true,
	"has_view_permission": true, "has_add_permission": true,
	"has_change_permission": true, "has_delete_permission": true,
	"has_absolute_url": true, "absolute_url": true, "save_on_top": true,
	"to_field_var": true, "is_popup_var": true, "can_change": true,
	"show_save": true, "show_save_as_new": true, "show_save_and_add_another": true,
	"show_save_and_continue": true, "show_close": true, "show_delete_link": true,
}

// formMode is the kind of page renderChangeForm draws.
type formMode int

const (
	// formAdd is the add page.
	formAdd formMode = iota
	// formChange is the change page of a user who may change the object.
	formChange
	// formView is the change page of a user who may only view the object.
	formView
	// formSaveAsNew is a failed "Save as new": a change page without an
	// object that offers only "Save as new".
	formSaveAsNew
)

// renderChangeForm renders admin/change_form.html for the page of mode.
// errors in the context is the number of fields with errors, counting the
// errors of the whole form as one, as the length of Django's AdminErrorList.
func (s *AdminSite) renderChangeForm(
	w http.ResponseWriter,
	r *http.Request,
	e *entry,
	title string,
	obj any,
	af *adminForm,
	inlines []*inlineFormSet,
	mode formMode,
) {
	isAdd := mode == formAdd
	view := "add"
	if r.PathValue("object_id") != "" {
		view = "change"
	}
	extra := e.extraContext(r, view)
	canChange := mode == formAdd || mode == formChange ||
		mode == formSaveAsNew && e.hasChangePermission(s, r, nil)
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
	// The form posts back to the page; with changelist filters it posts to
	// "?_changelist_filters=...", as Django's form_url.
	preserved := s.preservedFilters(r, e)
	ctx["form_url"] = s.addPreservedFilters(e, "", preserved, nil, false, "")
	ctx["close_url"] = s.addPreservedFilters(e, s.changelistURL(e), preserved, nil, false, "")
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
	// The object tools link the history; a failed "Save as new" has no
	// object, and its link is empty, as Django's url tag leaves it.
	ctx["history_url"] = ""
	absoluteURL := s.viewOnSiteURL(r.Context(), e, e.admin.ViewOnSite, e.admin.ViewOnSiteURL, obj)
	ctx["has_absolute_url"] = absoluteURL != ""
	ctx["absolute_url"] = absoluteURL
	if obj != nil {
		pk := pkOf(reflect.ValueOf(obj).Elem(), e)
		ctx["history_url"] = s.addPreservedFilters(e, s.historyURL(e, pk), preserved, nil, false, "")
		if hasObjDelete {
			ctx["delete_url"] = s.addPreservedFilters(e, s.deleteURL(e, pk), preserved, nil, false, "")
		}
	}

	// Submit-row button visibility mirrors Django's submit_row template tag,
	// which reads the show_* flags of the extra context. A failed "Save as
	// new" hides the buttons that save the original row.
	flag := func(key string, def bool) bool {
		if v, ok := extra[key]; ok {
			return pongo2.AsValue(v).IsTrue()
		}
		return def
	}
	canSave := isAdd || canChange
	showSave := canSave && flag("show_save", mode != formSaveAsNew)
	ctx["save_on_top"] = e.admin.SaveOnTop
	ctx["show_save"] = showSave
	ctx["show_save_as_new"] = !popup && hasAdd && !isAdd && e.admin.SaveAs
	ctx["show_save_and_add_another"] = !popup && hasAdd && canSave && (!e.admin.SaveAs || isAdd) &&
		flag("show_save_and_add_another", true)
	ctx["show_save_and_continue"] = !popup && canSave && flag("show_save_and_continue", mode != formSaveAsNew) &&
		(hasObjChange || hasObjView)
	ctx["show_close"] = !showSave
	ctx["show_delete_link"] = !popup && hasObjDelete && flag("show_delete", true)

	ctx["prepopulated_fields_json"] = prepopulatedJSON(r, e, obj, af, inlines, mode)

	ctx["includes"] = s.includes(e, "submit_line", "change_form_object_tools", "prepopulated_fields_js")
	for k, v := range extra {
		if !changeFormOwnKeys[k] {
			ctx[k] = v
		}
	}
	custom := e.admin.ChangeFormTemplate
	if isAdd && e.admin.AddFormTemplate != "" {
		custom = e.admin.AddFormTemplate
	}
	s.renderPage(w, r, e, custom, "change_form.html", ctx)
}

// prepopulatedJSON returns the data of the script that fills fields from
// other fields as the user types, Django's prepopulated_fields_js tag: the
// fields of the form, unless the user may only view it, then those of the
// inline forms that add a row.
func prepopulatedJSON(r *http.Request, e *entry, obj any, af *adminForm, inlines []*inlineFormSet, mode formMode) string {
	var items []string
	if mode != formView {
		items = appendPrepopulated(items, e.model, af.form, e.prepopulatedFields(r, obj))
	}
	for _, ifs := range inlines {
		ie := ifs.in.entry
		fields := ie.prepopulatedFields(r, obj)
		if len(fields) == 0 {
			continue
		}
		for _, form := range ifs.addForms() {
			items = appendPrepopulated(items, ie.model, form, fields)
		}
	}
	return "[" + strings.Join(items, ", ") + "]"
}

// appendPrepopulated appends to items the script data of each field of
// form that fields fills, Django's AdminForm.prepopulated_fields, in the
// order of the sorted names and as json.dumps writes it. It skips the
// names that form lacks, such as read-only fields, where Django fails.
// maxLength and allowUnicode come from the model field, else 50 and false.
func appendPrepopulated(items []string, m *orm.Model, form *forms.Form, fields map[string][]string) []string {
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		bf := form.Field(formName(m, name))
		if bf == nil {
			continue
		}
		var ids, names []string
		for _, dep := range fields[name] {
			if d := form.Field(formName(m, dep)); d != nil {
				ids = append(ids, templates.QuoteJSON("#"+d.AutoID()))
				names = append(names, templates.QuoteJSON(d.Name))
			}
		}
		maxLength, allowUnicode := 50, false
		if f, _ := relationOrField(m, name); f != nil {
			maxLength = cmp.Or(f.MaxLength, 50)
			allowUnicode = f.AllowUnicode
		}
		items = append(items, fmt.Sprintf(
			`{"id": %s, "name": %s, "dependency_ids": [%s], "dependency_list": [%s], "maxLength": %d, "allowUnicode": %t}`,
			templates.QuoteJSON("#"+bf.AutoID()), templates.QuoteJSON(bf.Name),
			strings.Join(ids, ", "), strings.Join(names, ", "), maxLength, allowUnicode))
	}
	return items
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

// objectHTML returns the label of an object for the HTML of a success
// message, Django's obj_repr: a link to u, or the bare label when u is empty,
// escaped as Django's format_html('<a href="{}">{}</a>', urlquote(u), label)
// does.
func objectHTML(label, u string) string {
	if u == "" {
		return forms.Escape(label)
	}
	return `<a href="` + forms.Escape(pyQuote(u, "/")) + `">` + forms.Escape(label) + `</a>`
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
func (s *AdminSite) popupResponse(w http.ResponseWriter, r *http.Request, e *entry, pairs ...string) {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(templates.QuoteJSON(pairs[i]) + ": " + templates.QuoteJSON(pairs[i+1]))
	}
	b.WriteByte('}')
	s.renderPage(w, r, e, e.admin.PopupResponseTemplate, "popup_response.html",
		map[string]any{"popup_response_data": b.String()})
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
		u := s.addPreservedFilters(e, s.changelistURL(e), s.preservedFilters(r, e), nil, false, "")
		http.Redirect(w, r, u, http.StatusFound)
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

// displayLabel returns orm.Label(m, obj) without surrounding whitespace, or
// emptyValueDisplay ("-") when nothing is left, Django's
// display_for_value(str(obj), EMPTY_VALUE_STRING).
func displayLabel(m *orm.Model, obj any) string {
	if label := strings.TrimSpace(orm.Label(m, obj)); label != "" {
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

// templateNames returns the names of the template page of e, Django's search
// list: the model's, the app's, then the admin's own.
func (e *entry) templateNames(page string) []string {
	if e.app == "" {
		return []string{"admin/" + page}
	}
	return []string{"admin/" + e.app + "/" + e.slug + "/" + page, "admin/" + e.app + "/" + page, "admin/" + page}
}

// renderPage renders the template custom, or the first template of the
// search list of page for e that exists when custom is empty.
func (s *AdminSite) renderPage(w http.ResponseWriter, r *http.Request, e *entry, custom, page string, ctx map[string]any) {
	if custom != "" {
		s.render(w, r, custom, ctx)
		return
	}
	s.renderFirst(w, r, e.templateNames(page), ctx)
}

// renderFirst renders the first of names that exists, as Django's
// TemplateResponse with a list of names.
func (s *AdminSite) renderFirst(w http.ResponseWriter, r *http.Request, names []string, ctx map[string]any) {
	name, err := s.engine.SelectTemplate(names...)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, name, ctx)
}

// includes returns the templates that a page of e includes, keyed by their
// page names without ".html": the first that exists of the search list of
// each, as Django's InclusionAdminNode selects them.
func (s *AdminSite) includes(e *entry, pages ...string) map[string]any {
	out := make(map[string]any, len(pages))
	for _, page := range pages {
		names := e.templateNames(page + ".html")
		name, err := s.engine.SelectTemplate(names...)
		if err != nil {
			// Render the admin's own, which reports the error.
			name = names[len(names)-1]
		}
		out[page] = name
	}
	return out
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
