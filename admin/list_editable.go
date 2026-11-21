package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"sort"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// listEditablePrefix is the prefix of the formset of the list_editable
// fields, Django's BaseFormSet.get_default_prefix.
const listEditablePrefix = "form"

// errListEditableAdd reports a submitted row that is not in the database:
// the changelist only changes rows (Django's "list_editable does not allow
// adding." BadRequest).
var errListEditableAdd = errors.New("admin: list_editable does not allow adding")

// changelistFormSet returns the formset of the editable fields of e, Django's
// get_changelist_formset and _get_formset_with_permissions. Its initial forms
// edit rows, or the rows of spec when spec is not nil. data binds it. A form
// whose row the user may not change loses the editable fields.
func (s *AdminSite) changelistFormSet(r *http.Request, e *entry, editable []string, rows []any, spec *querySpec, data url.Values) (changelistSet, error) {
	built := map[string]bool{}
	callback := func(mf *orm.Field, f forms.Field) forms.Field {
		built[mf.Attr] = true
		return formfieldForDBField(mf, f)
	}
	fields := formNames(e.model, editable)
	opts := slices.Concat([]forms.ModelFormOption{
		forms.WithFields(fields...),
		forms.WithFormFieldCallback(callback),
	}, s.relationWidgets(r, e))
	set := e.ops.changelistSet(opts, rows, spec)
	fs := set.base()
	fs.Prefix = listEditablePrefix
	var choices map[string][][2]string
	fs.ConfigureForm = func(form *forms.Form, _ int) {
		syncAutocompleteRequired(form)
		s.wrapRelatedWidgets(r, e, form, built)
		if choices == nil {
			choices = s.relationChoices(r, e, form, "")
		}
		for name, c := range choices {
			form.SetChoices(name, c)
		}
	}
	if e.admin.ChangelistFormSet != nil {
		e.admin.ChangelistFormSet(r, fs)
	}
	if data != nil {
		set.bind(data)
		fs = set.base()
	}
	fs.SetContext(r.Context())
	objs := set.instances()
	for i, form := range fs.Forms() {
		if i < len(objs) && !e.hasChangePermission(s, r, objs[i]) {
			for _, name := range fields {
				form.RemoveField(name)
			}
		}
		if err := s.loadSelectedChoices(r, e, form); err != nil {
			return nil, err
		}
	}
	return set, fs.Err()
}

// editedObjectPKs returns the primary keys that data submits for the forms of
// the formset with prefix, Django's _get_edited_object_pks.
func editedObjectPKs(data url.Values, prefix, pkName string) []string {
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-\d+-` + regexp.QuoteMeta(pkName) + `$`)
	var out []string
	for key, vals := range data {
		if len(vals) > 0 && pattern.MatchString(key) {
			out = append(out, vals[len(vals)-1])
		}
	}
	sort.Strings(out)
	return out
}

// listEditableSpec returns base limited to the rows with the primary keys
// pks, Django's _get_list_editable_queryset. A key that is not a valid primary
// key leaves base whole.
func listEditableSpec(e *entry, base querySpec, pks []string) querySpec {
	pkField := e.model.PrimaryKey()
	parsed := make([]any, 0, len(pks))
	for _, raw := range pks {
		v, ok := parseFieldValue(pkField, raw, false)
		if !ok {
			return base
		}
		parsed = append(parsed, v)
	}
	base.filters = append(slices.Clone(base.filters), orm.Q("pk__in", parsed))
	return base
}

// saveChangelistFormSet saves the changed rows of the valid formset set in one
// transaction, Django's _save_formset, and returns how many it saved. Each
// saved row goes through the SaveModel and SaveRelated hooks and gets a log
// entry.
func (s *AdminSite) saveChangelistFormSet(r *http.Request, e *entry, set changelistSet) (int, error) {
	count := 0
	err := s.db.Atomic(r.Context(), func(ctx context.Context) error {
		rc := r.WithContext(ctx)
		objs := set.instances()
		for i, form := range set.base().Forms() {
			obj := objs[i]
			if !e.hasChangePermission(s, rc, obj) || !form.HasChanged() {
				continue
			}
			pk := pkOf(reflect.ValueOf(obj).Elem(), e)
			if pk == 0 {
				return errListEditableAdd
			}
			update := func(ctx context.Context) error { return e.ops.update(ctx, pk, obj) }
			if err := s.saveObject(rc, e, obj, form, nil, true, update); err != nil {
				return err
			}
			if err := s.logFormChange(ctx, r, obj, form, nil, false); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

// saveListEditable handles the Save button of a changelist with
// list_editable, Django's bulk-edit branch of changelist_view: it binds the
// formset over the submitted rows and saves them when it is valid. It reports
// true when it answered the request; otherwise it returns the bound formset
// with its errors for the page.
func (s *AdminSite) saveListEditable(w http.ResponseWriter, r *http.Request, e *entry, cl *changeList) (changelistSet, bool) {
	if !e.hasChangePermission(s, r, nil) {
		forbidden(w)
		return nil, true
	}
	pkName := e.model.PrimaryKey().Attr
	spec := listEditableSpec(e, cl.baseSpec, editedObjectPKs(r.PostForm, listEditablePrefix, pkName))
	set, err := s.changelistFormSet(r, e, cl.listEditable, nil, &spec, r.PostForm)
	if err != nil {
		http.Error(w, "invalid list_editable: "+err.Error(), http.StatusInternalServerError)
		return nil, true
	}
	valid := set.base().IsValid()
	if err := set.base().Err(); err != nil {
		http.Error(w, "failed to validate the forms", http.StatusInternalServerError)
		return nil, true
	}
	if !valid {
		return set, false
	}
	count, err := s.saveChangelistFormSet(r, e, set)
	if errors.Is(err, errListEditableAdd) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return nil, true
	}
	if err != nil {
		http.Error(w, "failed to update rows", http.StatusInternalServerError)
		return nil, true
	}
	if count > 0 {
		verb := "were"
		if count == 1 {
			verb = "was"
		}
		_ = messages.Success(r, fmt.Sprintf("%d %s %s changed successfully.", count, modelNgettext(e.model, count), verb))
	}
	http.Redirect(w, r, r.URL.RequestURI(), http.StatusFound)
	return nil, true
}
