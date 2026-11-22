package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/views"
)

// The action flags of a LogEntry, Django's ADDITION, CHANGE and DELETION.
const (
	Addition int16 = 1
	Change   int16 = 2
	Deletion int16 = 3
)

// LogEntry records one addition, change or deletion made through the admin,
// Django's django.contrib.admin.models.LogEntry. The table and the columns
// are Django's. Install App to create the table: without it the admin
// records nothing and shows no history.
type LogEntry struct {
	ID int64
	// ActionTime is the time of the action. LogActions sets it.
	ActionTime time.Time `orm:"editable=false;verbose_name=action time"`
	// User is the user who acted.
	User orm.FK[auth.User] `orm:"on_delete=cascade;verbose_name=user"`
	// ContentType is the model of the object, nil when the content type was
	// deleted.
	ContentType orm.FK[contenttypes.ContentType] `orm:"null;blank;on_delete=set_null;verbose_name=content type"`
	// ObjectID is the primary key of the object as text.
	ObjectID *string `orm:"type=text;null;blank;verbose_name=object id"`
	// ObjectRepr is the label of the object at the time of the action, cut
	// to 200 characters.
	ObjectRepr string `orm:"max_length=200;verbose_name=object repr"`
	// ActionFlag is Addition, Change or Deletion.
	ActionFlag int16 `orm:"type=positive_smallint;verbose_name=action flag"`
	// ChangeMessage is a ChangeMessage as JSON, or plain text.
	ChangeMessage string `orm:"type=text;blank;verbose_name=change message"`
}

// Meta names the table and orders the entries as Django does.
func (LogEntry) Meta() orm.Meta {
	return orm.Meta{
		Table:             "django_admin_log",
		VerboseName:       "log entry",
		VerboseNamePlural: "log entries",
		Ordering:          []string{"-action_time"},
		Choices: map[string][]orm.Choice{
			"action_flag": {
				orm.ChoiceOf(Addition, "Addition"),
				orm.ChoiceOf(Change, "Change"),
				orm.ChoiceOf(Deletion, "Deletion"),
			},
		},
	}
}

// String describes the action, Django's LogEntry.__str__.
func (l LogEntry) String() string {
	switch {
	case l.IsAddition():
		return "Added “" + l.ObjectRepr + "”."
	case l.IsChange():
		return "Changed “" + l.ObjectRepr + "” \u2014 " + l.GetChangeMessage()
	case l.IsDeletion():
		return "Deleted “" + l.ObjectRepr + ".”"
	}
	return "LogEntry Object"
}

// IsAddition reports whether the entry records an addition.
func (l LogEntry) IsAddition() bool { return l.ActionFlag == Addition }

// IsChange reports whether the entry records a change.
func (l LogEntry) IsChange() bool { return l.ActionFlag == Change }

// IsDeletion reports whether the entry records a deletion.
func (l LogEntry) IsDeletion() bool { return l.ActionFlag == Deletion }

// GetChangeMessage returns the change message as text, Django's
// get_change_message: a ChangeMessage stored as JSON becomes sentences such
// as "Changed Title and History.", and other text is returned as stored.
func (l LogEntry) GetChangeMessage() string {
	if !strings.HasPrefix(l.ChangeMessage, "[") {
		return l.ChangeMessage
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(l.ChangeMessage), &items); err != nil {
		return l.ChangeMessage
	}
	var msgs []string
	for _, item := range items {
		if raw, ok := item["added"]; ok {
			c := decodeChange(raw)
			if c.empty {
				msgs = append(msgs, "Added.")
			} else {
				msgs = append(msgs, fmt.Sprintf("Added %s “%s”.", c.Name, c.Object))
			}
		} else if raw, ok := item["changed"]; ok {
			c := decodeChange(raw)
			fields := textList(c.Fields, "and")
			if c.hasName {
				msgs = append(msgs, fmt.Sprintf("Changed %s for %s “%s”.", fields, c.Name, c.Object))
			} else {
				msgs = append(msgs, fmt.Sprintf("Changed %s.", fields))
			}
		} else if raw, ok := item["deleted"]; ok {
			c := decodeChange(raw)
			msgs = append(msgs, fmt.Sprintf("Deleted %s “%s”.", c.Name, c.Object))
		}
	}
	for i, m := range msgs {
		msgs[i] = capFirst(m)
	}
	if len(msgs) == 0 {
		return "No fields changed."
	}
	return strings.Join(msgs, " ")
}

// GetEditedObject returns the object of the entry as a pointer to an
// instance of its model, Django's get_edited_object. The error wraps
// orm.ErrDoesNotExist when the object or its content type is gone.
func (l *LogEntry) GetEditedObject(ctx context.Context, db *orm.DB) (any, error) {
	ct, ok, err := l.contentType(ctx, db)
	if err != nil {
		return nil, err
	}
	if !ok || l.ObjectID == nil {
		return nil, fmt.Errorf("admin: object of log entry %d: %w", l.ID, orm.ErrDoesNotExist)
	}
	m, ok := contenttypes.ModelClass(db, ct)
	if !ok {
		return nil, fmt.Errorf("admin: object of log entry %d: model %s is not installed: %w",
			l.ID, ct.AppLabeledName(), orm.ErrDoesNotExist)
	}
	var pk any = *l.ObjectID
	if m.PrimaryKey().Kind != orm.KindChar {
		n, err := strconv.ParseInt(*l.ObjectID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("admin: object of log entry %d: %w", l.ID, orm.ErrDoesNotExist)
		}
		pk = n
	}
	return orm.GetByPK(ctx, db, m, pk)
}

// GetAdminURL returns the URL of the change page of the object on site s,
// Django's get_admin_url. It returns "" when the entry has no content type
// or object, or when s does not register the model.
func (l *LogEntry) GetAdminURL(ctx context.Context, s *AdminSite) (string, error) {
	if l.ObjectID == nil || *l.ObjectID == "" {
		return "", nil
	}
	ct, ok, err := l.contentType(ctx, s.db)
	if err != nil || !ok {
		return "", err
	}
	e := s.byKey[entryKey{app: ct.AppLabel, slug: ct.Model}]
	if e == nil {
		return "", nil
	}
	pk, err := strconv.ParseInt(*l.ObjectID, 10, 64)
	if err != nil {
		return "", nil //nolint:nilerr // Django answers None for an id that does not reverse.
	}
	return s.changeURL(e, pk), nil
}

// contentType returns the content type of the entry: the loaded one, or the
// one its key names. ok is false when the entry has none.
func (l *LogEntry) contentType(ctx context.Context, db *orm.DB) (contenttypes.ContentType, bool, error) {
	if ct, loaded := l.ContentType.Object(); loaded {
		if ct == nil {
			return contenttypes.ContentType{}, false, nil
		}
		return *ct, true, nil
	}
	if l.ContentType.PK() == 0 {
		return contenttypes.ContentType{}, false, nil
	}
	ct, err := contenttypes.GetForID(ctx, db, l.ContentType.PK())
	if err != nil {
		return contenttypes.ContentType{}, false, err
	}
	return ct, true, nil
}

// decodedChange is one decoded item of a stored ChangeMessage.
type decodedChange struct {
	Name, Object string
	Fields       []string
	hasName      bool
	empty        bool
}

// decodeChange decodes the object of one change message item.
func decodeChange(raw json.RawMessage) decodedChange {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	c := decodedChange{empty: len(m) == 0}
	if v, ok := m["name"]; ok {
		c.hasName = true
		c.Name = fmt.Sprint(v)
	}
	if v, ok := m["object"]; ok {
		c.Object = fmt.Sprint(v)
	}
	if fs, ok := m["fields"].([]any); ok {
		for _, f := range fs {
			c.Fields = append(c.Fields, fmt.Sprint(f))
		}
	}
	return c
}

// LogActions records flag for each of objs by the user with userID, Django's
// LogEntry.objects.log_actions. Each object is a pointer to an instance of a
// registered model. One object is saved with Create, which fires the save
// signals; several are inserted with one BulkCreate, which fires none, as in
// Django.
func LogActions(ctx context.Context, db *orm.DB, userID int64, objs []any, flag int16, changeMessage string) ([]*LogEntry, error) {
	now := time.Now
	if db.Now != nil {
		now = db.Now
	}
	entries := make([]*LogEntry, 0, len(objs))
	for _, obj := range objs {
		m, ok := db.Registry().ModelOf(obj)
		if !ok {
			return nil, fmt.Errorf("admin: log %T: the model is not registered", obj)
		}
		ct, err := contenttypes.GetForModel(ctx, db, m)
		if err != nil {
			return nil, err
		}
		label, err := orm.LabelContext(ctx, db, m, obj)
		if err != nil {
			return nil, err
		}
		id := objectID(m, obj)
		entry := &LogEntry{
			ActionTime:    now().UTC(),
			ObjectID:      &id,
			ObjectRepr:    truncateChars(label, 200),
			ActionFlag:    flag,
			ChangeMessage: changeMessage,
		}
		entry.User.SetPK(userID)
		entry.ContentType.SetPK(ct.ID)
		entries = append(entries, entry)
	}
	switch len(entries) {
	case 0:
		return entries, nil
	case 1:
		if err := orm.Query[LogEntry](db).Create(ctx, entries[0]); err != nil {
			return nil, err
		}
	default:
		if err := orm.Query[LogEntry](db).BulkCreate(ctx, entries); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// objectID returns the primary key of obj, a *T of m, as text, Django's
// str(obj.pk).
func objectID(m *orm.Model, obj any) string {
	pk := m.PrimaryKey()
	v := reflect.ValueOf(obj).Elem().Field(pk.Index).Interface()
	return fmt.Sprint(v)
}

// truncateChars returns the first n characters of s, Python's s[:n].
func truncateChars(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// ChangeMessage is the structured change message of a LogEntry, Django's
// construct_change_message: one item per change of the object and of the
// rows of its inlines. LogEntry.GetChangeMessage turns it into text.
type ChangeMessage []ChangeMessageItem

// ChangeMessageItem is one change. Exactly one of Added, Changed and Deleted
// is set.
type ChangeMessageItem struct {
	// Added is an added object. A zero ChangeObject is the object itself.
	Added *ChangeObject
	// Changed lists the changed fields of the object or of an inline row.
	Changed *ChangeObject
	// Deleted is a deleted inline row.
	Deleted *ChangeObject
}

// ChangeObject describes the object of a change. Name and Object are empty
// for the edited object itself and set for a row of an inline.
type ChangeObject struct {
	// Name is the verbose name of the model of the row.
	Name string
	// Object is the label of the row.
	Object string
	// Fields are the labels of the changed fields.
	Fields []string
}

// String returns the message as JSON in the format of Python's json.dumps,
// as Django stores it.
func (cm ChangeMessage) String() string {
	var b strings.Builder
	b.WriteByte('[')
	for i, item := range cm {
		if i > 0 {
			b.WriteString(", ")
		}
		switch {
		case item.Added != nil:
			b.WriteString(`{"added": `)
			writeChangeObject(&b, item.Added, false)
		case item.Changed != nil:
			b.WriteString(`{"changed": `)
			writeChangeObject(&b, item.Changed, true)
		case item.Deleted != nil:
			b.WriteString(`{"deleted": `)
			writeChangeObject(&b, item.Deleted, false)
		default:
			b.WriteString("{")
			b.WriteString("}")
			continue
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.String()
}

// writeChangeObject writes c as a JSON object with Django's keys in Django's
// order: name and object when set, then fields when withFields is true.
func writeChangeObject(b *strings.Builder, c *ChangeObject, withFields bool) {
	b.WriteByte('{')
	sep := ""
	if c.Name != "" || c.Object != "" {
		b.WriteString(`"name": `)
		pyjson.WriteString(b, c.Name)
		b.WriteString(`, "object": `)
		pyjson.WriteString(b, c.Object)
		sep = ", "
	}
	if withFields {
		b.WriteString(sep + `"fields": [`)
		for i, f := range c.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			pyjson.WriteString(b, f)
		}
		b.WriteByte(']')
	}
	b.WriteByte('}')
}

// App is the app config of the admin app, Django's django.contrib.admin in
// INSTALLED_APPS. It installs LogEntry, the history of the admin. It needs
// auth.App and contenttypes.App.
type App struct{}

// Name returns the app label "admin".
func (App) Name() string { return "admin" }

// VerboseName returns Django's verbose name of the admin app.
func (App) VerboseName() string { return "Administration" }

// Models returns the LogEntry model.
func (App) Models() []any { return []any{&LogEntry{}} }

// logEnabled reports whether the database has LogEntry, that is whether App
// is installed. Without it the admin records no history.
func (s *AdminSite) logEnabled() bool {
	_, ok := s.db.Registry().ModelOf(&LogEntry{})
	return ok
}

// logActions records flag for objs by the user of r, Django's log_addition,
// log_change and log_deletions. It records nothing without App.
func (s *AdminSite) logActions(ctx context.Context, r *http.Request, objs []any, flag int16, message string) error {
	if !s.logEnabled() {
		return nil
	}
	u, err := auth.GetUser(r)
	if err != nil {
		return err
	}
	_, err = LogActions(ctx, s.db, u.ID, objs, flag, message)
	return err
}

// logFormChange records the addition or the change of obj through form and
// the formsets of its inlines, Django's construct_change_message followed by
// log_addition or log_change.
func (s *AdminSite) logFormChange(ctx context.Context, r *http.Request, obj any, form *forms.Form, inlines []*inlineFormSet, add bool) error {
	if !s.logEnabled() {
		return nil
	}
	cm, err := constructChangeMessage(ctx, s.db, form, form.ChangedData(), inlines, add)
	if err != nil {
		return err
	}
	flag := Change
	if add {
		flag = Addition
	}
	return s.logActions(ctx, r, []any{obj}, flag, cm.String())
}

// constructChangeMessage returns the change message of a saved form and the
// formsets of its inlines, Django's construct_change_message. changed is the
// changed data of the form.
func constructChangeMessage(ctx context.Context, db *orm.DB, form *forms.Form, changed []string, inlines []*inlineFormSet, add bool) (ChangeMessage, error) {
	cm := ChangeMessage{}
	if add {
		cm = append(cm, ChangeMessageItem{Added: &ChangeObject{}})
	} else if len(changed) > 0 {
		cm = append(cm, ChangeMessageItem{Changed: &ChangeObject{Fields: changedFieldLabels(form, changed)}})
	}
	for _, ifs := range inlines {
		m := ifs.in.entry.model
		name, _ := modelVerboseNames(m)
		label := func(obj any) (string, error) { return orm.LabelContext(ctx, db, m, obj) }
		added, changedRows, deleted := ifs.set.changes()
		for _, obj := range added {
			l, err := label(obj)
			if err != nil {
				return nil, err
			}
			cm = append(cm, ChangeMessageItem{Added: &ChangeObject{Name: name, Object: l}})
		}
		var first *forms.Form
		if fs := ifs.formSet().Forms(); len(fs) > 0 {
			first = fs[0]
		}
		for _, row := range changedRows {
			l, err := label(row.obj)
			if err != nil {
				return nil, err
			}
			cm = append(cm, ChangeMessageItem{Changed: &ChangeObject{
				Name: name, Object: l, Fields: changedFieldLabels(first, row.fields),
			}})
		}
		for _, obj := range deleted {
			l, err := label(obj)
			if err != nil {
				return nil, err
			}
			cm = append(cm, ChangeMessageItem{Deleted: &ChangeObject{Name: name, Object: l}})
		}
	}
	return cm, nil
}

// changedFieldLabels returns the label of each named field of form, or the
// name for a field without a label or outside the form, Django's
// _get_changed_field_labels_from_form.
func changedFieldLabels(form *forms.Form, names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = name
		if form == nil {
			continue
		}
		if f, ok := form.FieldDef(name); ok {
			if l := f.Base().Label; l != nil && *l != "" {
				out[i] = *l
			}
		}
	}
	return out
}

// historyPerPage is the number of log entries on a page of the history,
// Django's history_view paginator.
const historyPerPage = 100

// historyContext adds the log entries of the object with objectID of e to
// ctx, a page of 100 oldest first, as Django's history_view does:
// action_list, page_links, pagination_required and entry_count.
func (s *AdminSite) historyContext(r *http.Request, e *entry, objectID string, ctx map[string]any) error {
	ctx["action_list"] = []map[string]any(nil)
	if !s.logEnabled() {
		return nil
	}
	rctx := r.Context()
	ct, err := contenttypes.GetForModel(rctx, s.db, e.model)
	if err != nil {
		return err
	}
	qs := orm.Query[LogEntry](s.db).
		Filter("object_id", objectID).
		Filter("content_type", ct.ID).
		SelectRelated("user", "content_type").
		OrderBy("action_time")
	pag := views.NewPaginator(views.QuerySetList(qs), historyPerPage)
	number := r.URL.Query().Get(pageVar)
	if number == "" {
		number = "1"
	}
	page, err := pag.GetPage(rctx, number)
	if err != nil {
		return err
	}
	count, err := pag.Count(rctx)
	if err != nil {
		return err
	}
	pageRange, err := pag.GetElidedPageRange(rctx, page.Number, views.DefaultOnEachSide, views.DefaultOnEnds)
	if err != nil {
		return err
	}
	actions := make([]map[string]any, len(page.ObjectList))
	for i := range page.ObjectList {
		l := &page.ObjectList[i]
		username, fullName := "", ""
		if u, ok := l.User.Object(); ok && u != nil {
			username, fullName = u.GetUsername(), u.GetFullName()
		}
		actions[i] = map[string]any{
			"action_time":    l.ActionTime,
			"username":       username,
			"full_name":      fullName,
			"change_message": l.GetChangeMessage(),
		}
	}
	links := make([]map[string]any, 0, len(pageRange))
	for _, item := range pageRange {
		switch v := item.(type) {
		case string:
			links = append(links, map[string]any{"is_ellipsis": true, "text": v})
		case int:
			links = append(links, map[string]any{
				"is_current": v == page.Number,
				"text":       strconv.Itoa(v),
				"href":       "?" + pageVar + "=" + strconv.Itoa(v),
			})
		}
	}
	ctx["action_list"] = actions
	ctx["page_links"] = links
	ctx["pagination_required"] = count > historyPerPage
	ctx["entry_count"] = count
	return nil
}

// LogEntries returns the log entries that the index shows, with their
// content types and users, Django's AdminSite.get_log_entries: all of them,
// or the ones that GetLogEntries returns.
func (s *AdminSite) LogEntries(r *http.Request) *orm.QuerySet[LogEntry] {
	qs := orm.Query[LogEntry](s.db).SelectRelated("content_type", "user")
	if s.GetLogEntries != nil {
		return s.GetLogEntries(r, qs)
	}
	return qs
}

// recentActionsLimit is the number of entries of "Recent actions", the
// limit of Django's {% get_admin_log 10 as admin_log for_user user %}.
const recentActionsLimit = 10

// recentActions returns the newest log entries of the user of r for the
// sidebar of the index, as Django's index.html renders them.
func (s *AdminSite) recentActions(r *http.Request) ([]map[string]any, error) {
	ctx := r.Context()
	u, err := auth.GetUser(r)
	if err != nil {
		return nil, err
	}
	entries, err := s.LogEntries(r).Filter("user", u.ID).Limit(recentActionsLimit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(entries))
	for i := range entries {
		l := &entries[i]
		url := ""
		if !l.IsDeletion() {
			if url, err = l.GetAdminURL(ctx, s); err != nil {
				return nil, err
			}
		}
		ct, ok, err := l.contentType(ctx, s.db)
		if err != nil {
			return nil, err
		}
		ctName := ""
		if ok {
			ctName = ct.Model
			if m, found := contenttypes.ModelClass(s.db, ct); found {
				ctName, _ = modelVerboseNames(m)
			}
		}
		repr := strings.TrimSpace(l.ObjectRepr)
		if repr == "" {
			repr = emptyValueDisplay
		}
		out[i] = map[string]any{
			"is_addition":       l.IsAddition(),
			"is_change":         l.IsChange(),
			"is_deletion":       l.IsDeletion(),
			"object_repr":       repr,
			"admin_url":         url,
			"has_content_type":  ok,
			"content_type_name": capFirst(ctName),
		}
	}
	return out, nil
}
