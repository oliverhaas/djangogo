package admin

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// blankActionChoiceLabel is Django 6's blank choice label for the action dropdown.
const blankActionChoiceLabel = "- Select an option -"

// defaultDeleteSelectedAction returns Django's built-in "delete_selected" bulk
// action (mirroring django/contrib/admin/actions.py::delete_selected).
func defaultDeleteSelectedAction() Action {
	return Action{
		Name:        "delete_selected",
		Description: "Delete selected %(verbose_name_plural)s",
		Permissions: []string{"delete"},
		Handler:     handleDeleteSelected,
	}
}

// allowedActions resolves the ordered list of bulk actions available to the
// current user on e, combining site-wide actions with ModelAdmin.Actions,
// filtering by permissions, and invoking ModelAdmin.GetActions when set.
func (s *AdminSite) allowedActions(r *http.Request, e *entry) []Action {
	if e.admin.DisableActions {
		return nil
	}
	if _, isPopup := r.URL.Query()[isPopupVar]; isPopup {
		return nil
	}

	modelActionNames := make(map[string]bool, len(e.admin.Actions))
	for _, a := range e.admin.Actions {
		modelActionNames[a.Name] = true
	}

	// Gather site-wide actions first (unless overridden by ModelAdmin), then
	// ModelAdmin actions, matching Django's ModelAdmin._get_base_actions.
	combined := make([]Action, 0, len(s.actions)+len(e.admin.Actions))
	for _, a := range s.actions {
		if !modelActionNames[a.Name] {
			combined = append(combined, a)
		}
	}
	combined = append(combined, e.admin.Actions...)

	filtered := make([]Action, 0, len(combined))
	for _, a := range combined {
		if s.userCanRunAction(r, e, a) {
			filtered = append(filtered, a)
		}
	}

	if e.admin.GetActions != nil {
		filtered = e.admin.GetActions(r, filtered)
	}
	return filtered
}

// userCanRunAction checks Action.Permissions with OR semantics, matching
// Django's ModelAdmin._filter_actions_by_permissions.
func (s *AdminSite) userCanRunAction(r *http.Request, e *entry, a Action) bool {
	if len(a.Permissions) == 0 {
		return true
	}
	for _, perm := range a.Permissions {
		if e.admin.CustomPermissions != nil {
			if fn, ok := e.admin.CustomPermissions[perm]; ok && fn != nil {
				if fn(r) {
					return true
				}
				continue
			}
		}
		switch perm {
		case "add":
			if e.hasAddPermission(s, r) {
				return true
			}
		case "change":
			if e.hasChangePermission(s, r, nil) {
				return true
			}
		case "delete":
			if e.hasDeletePermission(s, r, nil) {
				return true
			}
		case "view":
			if e.hasViewPermission(s, r, nil) {
				return true
			}
		default:
			permStr := perm
			if !strings.Contains(perm, ".") {
				permStr = permName(e.model, perm)
			}
			if s.userHasPerm(r, permStr) {
				return true
			}
		}
	}
	return false
}

// actionChoices builds the dropdown (value, label) entries for actions,
// formatting "%(verbose_name)s" and "%(verbose_name_plural)s" placeholders.
func (s *AdminSite) actionChoices(e *entry, actions []Action) []map[string]any {
	singular, plural := modelVerboseNames(e.model)
	out := make([]map[string]any, 0, len(actions)+1)
	out = append(out, map[string]any{
		"value":    "",
		"label":    blankActionChoiceLabel,
		"selected": true,
	})
	for _, a := range actions {
		desc := a.Description
		if desc == "" {
			desc = capFirst(strings.ReplaceAll(a.Name, "_", " "))
		}
		desc = strings.ReplaceAll(desc, "%(verbose_name)s", singular)
		desc = strings.ReplaceAll(desc, "%(verbose_name_plural)s", plural)
		out = append(out, map[string]any{
			"value":    a.Name,
			"label":    desc,
			"selected": false,
		})
	}
	return out
}

// responseAction validates and executes the submitted bulk action (Django's
// ModelAdmin.response_action). It returns true when a response has already been
// written, and false when the caller should redirect back to the changelist.
func (s *AdminSite) responseAction(
	w http.ResponseWriter,
	r *http.Request,
	e *entry,
	cl *changeList,
	actions []Action,
) bool {
	actionIdx := 0
	if rawIdx := r.PostForm.Get("index"); rawIdx != "" {
		if n, err := strconv.Atoi(rawIdx); err == nil && n >= 0 {
			actionIdx = n
		}
	}

	actionVals := r.PostForm["action"]
	chosenName := ""
	if actionIdx < len(actionVals) {
		chosenName = actionVals[actionIdx]
	}

	var chosen *Action
	for i := range actions {
		if actions[i].Name == chosenName && chosenName != "" {
			chosen = &actions[i]
			break
		}
	}
	if chosen == nil {
		_ = messages.Warning(r, "No action selected.")
		return false
	}

	selectAcross := isTruthyParam(r.PostForm.Get("select_across"))
	selected := r.PostForm[ActionCheckboxName]
	if len(selected) == 0 && !selectAcross {
		_ = messages.Warning(r, "Items must be selected in order to perform actions on them. No items have been changed.")
		return false
	}

	spec := cl.filteredSpec
	if !selectAcross {
		pkField := e.model.PrimaryKey()
		parsedPKs := make([]any, 0, len(selected))
		for _, raw := range selected {
			if v, ok := parseFieldValue(pkField, raw, false); ok {
				parsedPKs = append(parsedPKs, v)
			}
		}
		spec.filters = append(append([]orm.Cond(nil), spec.filters...), orm.Q("pk__in", parsedPKs))
	}

	actCtx := &ActionContext{
		Site:         s,
		DB:           s.db,
		Model:        e.model,
		SelectAcross: selectAcross,
		Objects: func(ctx context.Context) ([]any, error) {
			return e.ops.query(ctx, spec)
		},
		Count: func(ctx context.Context) (int64, error) {
			return e.ops.count(ctx, spec)
		},
		PKs: func(ctx context.Context) ([]any, error) {
			return e.ops.pks(ctx, spec)
		},
		Delete: func(ctx context.Context) (int64, error) {
			return e.ops.deleteSpec(ctx, spec)
		},
	}

	handled, err := chosen.Handler(w, r, actCtx)
	if err != nil {
		http.Error(w, "failed to run action", http.StatusInternalServerError)
		return true
	}
	return handled
}

// handleDeleteSelected implements Django's built-in delete_selected action:
// collects affected and protected objects, checks delete permissions on every
// registered affected model, renders delete_selected_confirmation.html, and
// performs deletion on confirmed POST (post=yes).
func handleDeleteSelected(w http.ResponseWriter, r *http.Request, act *ActionContext) (bool, error) {
	ctx := r.Context()
	s := act.Site
	e := s.entryByModel(act.Model)
	if e == nil {
		return false, fmt.Errorf("admin: model %s not registered", act.Model.Name())
	}

	objs, err := act.Objects(ctx)
	if err != nil {
		return false, err
	}

	deleted, err := s.getDeletedObjects(r, e, objs)
	if err != nil {
		return false, err
	}

	if r.PostForm.Get("post") != "" && len(deleted.Protected) == 0 {
		if len(deleted.PermsNeeded) > 0 {
			forbidden(w)
			return true, nil
		}
		n := len(objs)
		if n > 0 {
			if err := s.logActions(ctx, r, objs, Deletion, ""); err != nil {
				return false, err
			}
			if e.admin.DeleteQueryset != nil {
				if err := e.admin.DeleteQueryset(r, act); err != nil {
					return false, err
				}
			} else {
				if _, err := act.Delete(ctx); err != nil {
					return false, err
				}
			}
			_ = messages.Success(r, fmt.Sprintf("Successfully deleted %d %s.", n, modelNgettext(e.model, n)))
		}
		return false, nil
	}

	objectsName := modelNgettext(e.model, len(objs))
	title := "Delete multiple objects"
	if len(deleted.PermsNeeded) > 0 || len(deleted.Protected) > 0 {
		title = "Cannot delete " + objectsName
	}

	maxDisplay := e.admin.DeleteConfirmationMaxDisplay
	hideObjects := maxDisplay != nil && *maxDisplay == 0

	modelCounts := make([]map[string]any, len(deleted.ModelCount))
	for i, mc := range deleted.ModelCount {
		modelCounts[i] = map[string]any{
			"name":  capFirst(mc.Name),
			"count": mc.Count,
		}
	}

	selectedPKs := make([]string, len(objs))
	for i, obj := range objs {
		selectedPKs[i] = strconv.FormatInt(pkOf(reflect.ValueOf(obj).Elem(), e), 10)
	}

	_, plural := modelVerboseNames(e.model)
	tplCtx := s.EachContext(r)
	tplCtx["title"] = title
	tplCtx["subtitle"] = ""
	tplCtx["app_label"] = e.app
	tplCtx["app_verbose_name"] = s.appVerboseName(e.app)
	tplCtx["app_url"] = s.appURLOrEmpty(e.app)
	tplCtx["model_name"] = e.slug
	tplCtx["verbose_name_plural"] = capFirst(plural)
	tplCtx["changelist_url"] = s.changelistURL(e)
	tplCtx["objects_name"] = objectsName
	tplCtx["model_count"] = modelCounts
	tplCtx["has_perms_lacking"] = len(deleted.PermsNeeded) > 0
	tplCtx["has_protected"] = len(deleted.Protected) > 0
	tplCtx["hide_objects"] = hideObjects
	tplCtx["perms_lacking_html"] = forms.SafeHTML(truncatedUnorderedList(textItems(deleted.PermsNeeded), maxDisplay))
	tplCtx["protected_html"] = forms.SafeHTML(truncatedUnorderedList(htmlItems(deleted.Protected), maxDisplay))
	tplCtx["deletable_objects_html"] = forms.SafeHTML(truncatedUnorderedList(deleted.Items, maxDisplay))
	tplCtx["selected_pks"] = selectedPKs
	tplCtx["action_checkbox_name"] = ActionCheckboxName
	tplCtx["media"] = e.media(s)

	s.render(w, r, "admin/delete_selected_confirmation.html", tplCtx)
	return true, nil
}

// getDeletedObjects returns the objects that deleting objs deletes or that
// block it, from ModelAdmin.GetDeletedObjects when it is set, Django's
// ModelAdmin.get_deleted_objects.
func (s *AdminSite) getDeletedObjects(r *http.Request, e *entry, objs []any) (*DeletedObjects, error) {
	if e.admin.GetDeletedObjects != nil {
		return e.admin.GetDeletedObjects(r, objs)
	}
	return s.collectDeletedObjects(r.Context(), r, e, objs)
}

// collectDeletedObjects runs orm.CollectDelete for objs and formats the
// result as Django's django.contrib.admin.utils.get_deleted_objects: the
// deleted objects nested under the objects whose deletion cascades to them,
// the count per model, the verbose names of the registered models that the
// user may not delete, and the protected objects.
func (s *AdminSite) collectDeletedObjects(ctx context.Context, r *http.Request, e *entry, objs []any) (*DeletedObjects, error) {
	out := &DeletedObjects{}
	if len(objs) == 0 {
		return out, nil
	}
	pks := make([]any, len(objs))
	for i, obj := range objs {
		pks[i] = pkOf(reflect.ValueOf(obj).Elem(), e)
	}
	plan, err := orm.CollectDelete(ctx, s.db, e.model, pks)
	if err != nil {
		return nil, err
	}

	// formatObj is Django's format_callback: it adds the verbose name of a
	// registered model that the user may not delete to PermsNeeded, protected
	// objects included, and links the object to its change page.
	permSeen := make(map[string]bool)
	formatObj := func(m *orm.Model, obj any) string {
		singular, _ := modelVerboseNames(m)
		noEditLink := forms.Escape(capFirst(singular)) + ": " + forms.Escape(orm.Label(m, obj))
		relEntry := s.entryByModel(m)
		if relEntry == nil {
			return noEditLink
		}
		if !relEntry.hasDeletePermission(s, r, obj) && !permSeen[singular] {
			permSeen[singular] = true
			out.PermsNeeded = append(out.PermsNeeded, singular)
		}
		pk := pkOf(reflect.ValueOf(obj).Elem(), relEntry)
		changeURL := s.changeURL(relEntry, pk)
		if changeURL == "" {
			return noEditLink
		}
		return fmt.Sprintf(`%s: <a href="%s">%s</a>`,
			forms.Escape(capFirst(singular)),
			forms.Escape(changeURL),
			forms.Escape(displayLabel(m, obj)),
		)
	}
	var nest func(nodes []orm.DeleteNode) []DeletedObject
	nest = func(nodes []orm.DeleteNode) []DeletedObject {
		items := make([]DeletedObject, len(nodes))
		for i, n := range nodes {
			items[i] = DeletedObject{HTML: formatObj(n.Model, n.Object), Children: nest(n.Children)}
		}
		return items
	}
	out.Items = nest(plan.Tree)

	for _, grp := range plan.Groups {
		_, plural := modelVerboseNames(grp.Model)
		out.ModelCount = append(out.ModelCount, DeletedModelCount{
			Name:  plural,
			Count: len(grp.Objects),
		})
	}
	for _, grp := range plan.Protected {
		for _, obj := range grp.Objects {
			out.Protected = append(out.Protected, formatObj(grp.Model, obj))
		}
	}
	return out, nil
}

// textItems returns texts as list items, escaped.
func textItems(texts []string) []DeletedObject {
	items := make([]DeletedObject, len(texts))
	for i, t := range texts {
		items[i] = DeletedObject{HTML: forms.Escape(t)}
	}
	return items
}

// htmlItems returns HTML snippets as list items.
func htmlItems(snippets []string) []DeletedObject {
	items := make([]DeletedObject, len(snippets))
	for i, h := range snippets {
		items[i] = DeletedObject{HTML: h}
	}
	return items
}

// truncatedUnorderedList renders items as the <li> elements of Django's
// truncated_unordered_list filter: children in a nested <ul>, at most
// *maxItems items counted through all levels, then Django's "…and N more
// objects." item. A nil maxItems shows every item.
func truncatedUnorderedList(items []DeletedObject, maxItems *int) string {
	if maxItems != nil && *maxItems <= 0 {
		return ""
	}
	count := 0
	var format func(items []DeletedObject, tabs int) string
	format = func(items []DeletedObject, tabs int) string {
		indent := strings.Repeat("\t", tabs)
		out := make([]string, 0, len(items))
		for _, it := range items {
			count++
			show := maxItems == nil || count <= *maxItems
			sublist := ""
			if len(it.Children) > 0 {
				sublist = fmt.Sprintf("\n%s<ul>\n%s\n%s</ul>\n%s", indent, format(it.Children, tabs+1), indent, indent)
			}
			if show {
				out = append(out, indent+"<li>"+it.HTML+sublist+"</li>")
			}
		}
		return strings.Join(out, "\n")
	}
	list := format(items, 1)
	if maxItems != nil && count > *maxItems {
		rem := count - *maxItems
		noun := "objects"
		if rem == 1 {
			noun = "object"
		}
		list += fmt.Sprintf("\n\t<li>…and %d more %s.</li>", rem, noun)
	}
	return list
}
