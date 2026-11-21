package templates

import (
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oliverhaas/djangogo/orm"
)

// modelLabelKey is the ModelMap key under which the __str__ label is stored. It
// mirrors Python's dunder so it never collides with a real snake_case attribute.
const modelLabelKey = "__str__"

// maxModelDepth bounds how deep loaded relations and nested containers are
// followed, so that objects pointing at each other cannot loop.
const maxModelDepth = 12

// ModelMap is the template-facing view of a model instance: a map keyed by the
// names a Django template uses (see ModelContext). pongo2 resolves map keys by
// exact string lookup, so {{ post.title }} works where a struct field
// (resolved case-sensitively as Title) would not. ModelMap also implements
// fmt.Stringer with the instance label; in a template a bare {{ post }}
// renders that label HTML-escaped.
type ModelMap map[string]any

// String returns the model's label (its __str__), satisfying fmt.Stringer.
func (m ModelMap) String() string {
	if s, ok := m[modelLabelKey].(string); ok {
		return s
	}
	return ""
}

// modelView is the form a model instance has inside a render. It differs from
// ModelMap in one point: its String result is HTML-escaped, because pongo2
// autoescapes strings only and would print the label of a Stringer as is.
type modelView map[string]any

// String returns the HTML-escaped label.
func (m modelView) String() string {
	s, _ := m[modelLabelKey].(string)
	return escapeHTML(s)
}

// label returns the label as stored, for filters that escape their output
// themselves.
func (m modelView) label() string {
	s, _ := m[modelLabelKey].(string)
	return s
}

// ModelLookup finds the model a value is an instance of. *orm.Registry
// implements it.
type ModelLookup interface {
	ModelOf(v any) (*orm.Model, bool)
}

// ContextValuer is implemented by a type that knows how a template should see
// it. The engine replaces such a value by its TemplateValue wherever it sits
// in a context, and then treats the result like any other context value. A
// paginator page, for one, can offer Django's attribute names this way.
type ContextValuer interface {
	TemplateValue() any
}

// modelLookupMu guards the process-global model lookup.
var (
	modelLookupMu sync.RWMutex
	modelLookup   ModelLookup
)

// SetModelLookup sets the process-global lookup engines use to recognize
// model instances in a context when they have none of their own (see
// Engine.SetModels). An application wires its registry in at boot, next to
// SetURLResolver. It is safe for concurrent use.
func SetModelLookup(l ModelLookup) {
	modelLookupMu.Lock()
	defer modelLookupMu.Unlock()
	modelLookup = l
}

// globalModelLookup returns the process-global lookup, or nil.
func globalModelLookup() ModelLookup {
	modelLookupMu.RLock()
	defer modelLookupMu.RUnlock()
	return modelLookup
}

// SetModels sets the lookup this engine uses to recognize model instances in
// a context, so that a view can pass a model struct, a pointer to one or a
// slice of them and the template sees Django's attribute names. Passing nil
// falls back to the process-global lookup.
func (e *Engine) SetModels(l ModelLookup) { e.models = l }

// modelLookup returns the lookup of this engine, else the global one.
func (e *Engine) modelLookup() ModelLookup {
	if e.models != nil {
		return e.models
	}
	return globalModelLookup()
}

// ModelContext builds the view a template has of a model instance, with the
// names Django gives it:
//
//   - every field under its attribute name (created_at for CreatedAt),
//   - pk and id for the primary key (including O2O primary keys),
//   - for a forward foreign key or one-to-one relation (e.g. Author or User):
//     <attr>_id with the related primary key (nil when unset), and <attr> with
//     the related object, itself a ModelMap, when loaded via SelectRelated,
//     PrefetchRelated, or SetObject. An unloaded relation is nil and renders
//     empty: a template never runs a query,
//   - for a reverse one-to-one relation (e.g. user.profile): the related
//     object when loaded via SelectRelated, PrefetchRelated, or RelatedOne on
//     a model embedding orm.RelCache; otherwise nil (renders empty),
//   - for a reverse foreign-key set or many-to-many set (e.g.
//     post.comment_set, article.publications): when prefetched into
//     orm.RelCache, a map exposing all (the slice of related objects), count,
//     exists, first, and last, so {{ post.comment_set.count }} and
//     {% for c in post.comment_set.all %} work without queries. An unloaded
//     set is nil and renders empty, matching unloaded forward relations,
//   - any custom PrefetchSpec.ToAttr cached on orm.RelCache, exposed directly
//     under its ToAttr name (a ModelMap for single objects or a slice of
//     ModelMaps for multi-valued prefetches),
//   - get_<field>_display with the label of the current choice, for a field
//     that declares choices,
//   - every exported method without parameters under its snake_case name
//     (get_absolute_url for GetAbsoluteURL), called when the template uses it,
//   - the instance label for a bare {{ object }}.
//
// The instance may be a value or a pointer. An engine with a ModelLookup does
// this conversion by itself for the instances it finds in a context.
func ModelContext(m *orm.Model, instance any) ModelMap {
	c := converter{lookup: globalModelLookup(), public: true}
	return ModelMap(c.instance(m, reflect.ValueOf(instance), 0))
}

// modelVisitKey identifies one persisted model instance on the active
// conversion stack so circular back-references (such as post -> comment_set ->
// comment.post) stop expanding nested relations instead of growing
// exponentially.
type modelVisitKey struct {
	model *orm.Model
	pk    any
}

// converter rewrites context values for the template.
type converter struct {
	// lookup recognizes model instances; nil leaves them as they are.
	lookup ModelLookup
	// public makes nested objects ModelMap values, the type callers of
	// ModelContext see, in place of the internal modelView.
	public bool
	// ancestors tracks model instances currently being converted on this call
	// stack to break circular relation back-references.
	ancestors map[modelVisitKey]bool
}

// value returns v as the template should see it.
func (c converter) value(v any, depth int) any {
	out, _ := c.convert(v, depth)
	return out
}

// wrap gives a built attribute map the type this converter hands out.
func (c converter) wrap(m map[string]any) any {
	if c.public {
		return ModelMap(m)
	}
	return modelView(m)
}

// convert returns v as the template should see it and whether that differs
// from v. Containers are copied only when something inside them changed.
func (c converter) convert(v any, depth int) (any, bool) {
	switch x := v.(type) {
	case nil, string, bool, int, int64, float64, modelView,
		templateTime, templateDate, templateTimeOfDay, templateDuration,
		orm.Decimal, orm.UUID:
		return v, false
	case time.Time:
		return templateTime{x}, true
	case *time.Time:
		if x == nil {
			return nil, true
		}
		return templateTime{*x}, true
	case orm.Date:
		return templateDate{x}, true
	case *orm.Date:
		if x == nil {
			return nil, true
		}
		return templateDate{*x}, true
	case orm.Time:
		return templateTimeOfDay{x}, true
	case *orm.Time:
		if x == nil {
			return nil, true
		}
		return templateTimeOfDay{*x}, true
	case time.Duration:
		return templateDuration{x}, true
	case *time.Duration:
		if x == nil {
			return nil, true
		}
		return templateDuration{*x}, true
	case *orm.Decimal:
		if x == nil {
			return nil, true
		}
		return *x, true
	case *orm.UUID:
		if x == nil {
			return nil, true
		}
		return *x, true
	case netip.Addr:
		if !x.IsValid() {
			return "", true
		}
		return x, false
	case *netip.Addr:
		if x == nil || !x.IsValid() {
			return nil, true
		}
		return *x, true
	case ModelMap:
		if c.public {
			return v, false
		}
		return c.fromModelMap(x, depth), true
	case ContextValuer:
		return c.value(x.TemplateValue(), depth+1), true
	case map[string]any:
		return c.convertMap(x, depth)
	}
	if depth > maxModelDepth {
		return v, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, true
		}
		if rv.Elem().Kind() != reflect.Struct {
			return v, false
		}
		if m, ok := c.modelOf(v); ok {
			return c.wrap(c.instance(m, rv, depth)), true
		}
	case reflect.Struct:
		if m, ok := c.modelOf(v); ok {
			return c.wrap(c.instance(m, rv, depth)), true
		}
	case reflect.Slice, reflect.Array:
		return c.convertSlice(v, rv, depth)
	}
	return v, false
}

// modelOf looks v up when the converter has a lookup.
func (c converter) modelOf(v any) (*orm.Model, bool) {
	if c.lookup == nil {
		return nil, false
	}
	return c.lookup.ModelOf(v)
}

// fromModelMap turns a ModelMap built by a caller into the render form,
// converting the objects nested in it.
func (c converter) fromModelMap(m ModelMap, depth int) modelView {
	out := make(modelView, len(m))
	for k, v := range m {
		out[k] = c.value(v, depth+1)
	}
	return out
}

// convertMap converts the values of a nested context map.
func (c converter) convertMap(m map[string]any, depth int) (any, bool) {
	var out map[string]any
	for k, v := range m {
		nv, changed := c.convert(v, depth+1)
		if !changed {
			continue
		}
		if out == nil {
			out = make(map[string]any, len(m))
			for k2, v2 := range m {
				out[k2] = v2
			}
		}
		out[k] = nv
	}
	if out == nil {
		return m, false
	}
	return out, true
}

// convertSlice converts the elements of a slice that may hold model
// instances. A slice of plain values (strings, numbers) is returned as is.
func (c converter) convertSlice(v any, rv reflect.Value, depth int) (any, bool) {
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return v, false
	}
	switch rv.Type().Elem().Kind() {
	case reflect.Struct, reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
	default:
		return v, false
	}
	n := rv.Len()
	var out []any
	for i := range n {
		nv, changed := c.convert(rv.Index(i).Interface(), depth+1)
		if !changed {
			if out != nil {
				out[i] = nv
			}
			continue
		}
		if out == nil {
			out = make([]any, n)
			for j := range i {
				out[j] = rv.Index(j).Interface()
			}
		}
		out[i] = nv
	}
	if out == nil {
		return v, false
	}
	return out, true
}

// instance builds the attribute map of one model instance. rv is the struct
// or a pointer to it.
func (c converter) instance(m *orm.Model, rv reflect.Value, depth int) map[string]any {
	fields := m.Fields()
	out := make(map[string]any, len(fields)+4)
	if !rv.IsValid() {
		out[modelLabelKey] = ""
		return out
	}
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		out[modelLabelKey] = orm.Label(m, rv.Interface())
		return out
	}
	// Methods may have pointer receivers, so they are bound to a pointer; a
	// struct passed by value is copied to get one.
	ptr := rv
	if rv.Kind() != reflect.Pointer {
		ptr = reflect.New(rv.Type())
		ptr.Elem().Set(rv)
	}
	sv := ptr.Elem()
	if sv.Kind() != reflect.Struct {
		out[modelLabelKey] = orm.Label(m, rv.Interface())
		return out
	}

	var pk any
	hasPK := false
	// Resolve primary key first so ancestor cycle detection knows this
	// instance before expanding nested relations.
	if pkField := m.PrimaryKey(); pkField != nil {
		pfv := sv.Field(pkField.Index)
		if pkField.Rel != nil {
			if pfv.CanInterface() {
				if keyed, ok := pfv.Interface().(interface{ PK() int64 }); ok {
					if id := keyed.PK(); id != 0 {
						pk, hasPK = id, true
					}
				}
			}
		} else {
			pk, hasPK = pfv.Interface(), true
		}
	}

	childDepth := depth
	var visitKey modelVisitKey
	trackVisit := false
	if hasPK && isTrackablePK(pk) {
		visitKey = modelVisitKey{model: m, pk: pk}
		if c.ancestors != nil && c.ancestors[visitKey] {
			// Already on the active conversion stack: keep scalar fields,
			// foreign-key IDs, methods, and label, but stop recursing into
			// further related objects.
			childDepth = maxModelDepth
		} else {
			if c.ancestors == nil {
				c.ancestors = make(map[modelVisitKey]bool)
			}
			c.ancestors[visitKey] = true
			trackVisit = true
		}
	}
	if trackVisit {
		defer delete(c.ancestors, visitKey)
	}

	for _, f := range fields {
		fv := sv.Field(f.Index)
		if f.Rel != nil {
			c.relation(out, f, fv, childDepth)
			if f.PrimaryKey {
				pk, hasPK = out[f.Attr+"_id"], true
			}
			continue
		}
		val := fv.Interface()
		out[f.Attr] = val
		if !c.public {
			// A time takes the form that prints like Django's datetime.
			out[f.Attr] = c.value(val, maxModelDepth+1)
		}
		if f.PrimaryKey {
			pk, hasPK = val, true
		}
		if f.HasChoices {
			disp := orm.GetDisplay(m, ptr.Interface(), f.Attr)
			if !c.public {
				disp = c.value(disp, maxModelDepth+1)
			}
			out["get_"+f.Attr+"_display"] = disp
		}
	}
	if hasPK {
		out["pk"] = pk
		if _, taken := out["id"]; !taken {
			out["id"] = pk
		}
	}

	c.cachedRelations(out, m, ptr.Interface(), childDepth)

	for _, meth := range methodsOf(ptr.Type()) {
		if _, taken := out[meth.key]; !taken {
			out[meth.key] = c.method(ptr, meth)
		}
	}
	out[modelLabelKey] = orm.Label(m, ptr.Interface())
	return out
}

// isTrackablePK reports whether pk is a comparable non-zero primary key value
// suitable as a map key in converter.ancestors.
func isTrackablePK(pk any) bool {
	if pk == nil {
		return false
	}
	rv := reflect.ValueOf(pk)
	if !rv.Type().Comparable() || rv.IsZero() {
		return false
	}
	return true
}

// cachedRelations exposes reverse O2O, reverse FK sets, M2M sets (forward and
// reverse), and custom PrefetchSpec.ToAttr entries from orm.RelCache without
// issuing any database queries. Unloaded relations remain nil and render empty.
func (c converter) cachedRelations(out map[string]any, m *orm.Model, inst any, depth int) {
	for _, rev := range m.ReverseRelations() {
		if _, taken := out[rev.Name]; taken {
			continue
		}
		out[rev.Name] = nil
		if depth >= maxModelDepth {
			continue
		}
		switch rev.Kind {
		case orm.RelO2O:
			obj, loaded := orm.CachedOne(m, inst, rev.Name)
			if loaded && obj != nil && rev.Source != nil {
				out[rev.Name] = c.wrap(c.instance(rev.Source, reflect.ValueOf(obj), depth+1))
			}
		case orm.RelFK, orm.RelM2M:
			items, target, loaded := orm.CachedList(m, inst, rev.Name)
			if loaded {
				if target == nil {
					target = rev.Source
				}
				out[rev.Name] = c.relatedSet(target, items, depth)
			}
		}
	}
	for _, mf := range m.ManyToMany() {
		if _, taken := out[mf.Attr]; taken {
			continue
		}
		out[mf.Attr] = nil
		if depth >= maxModelDepth {
			continue
		}
		items, target, loaded := orm.CachedList(m, inst, mf.Attr)
		if loaded {
			if target == nil && mf.Rel != nil {
				target = mf.Rel.Target
			}
			out[mf.Attr] = c.relatedSet(target, items, depth)
		}
	}
	for _, attr := range orm.CachedAttrs(m, inst) {
		if _, taken := out[attr.Name]; taken {
			continue
		}
		out[attr.Name] = nil
		if depth >= maxModelDepth {
			continue
		}
		if attr.Single {
			if attr.Obj != nil && attr.Model != nil {
				out[attr.Name] = c.wrap(c.instance(attr.Model, reflect.ValueOf(attr.Obj), depth+1))
			}
		} else {
			out[attr.Name] = c.relatedSlice(attr.Model, attr.Items, depth)
		}
	}
}

// relatedSet builds the template-facing manager map for a prefetched reverse
// FK or M2M set, exposing all, count, exists, first, and last.
func (c converter) relatedSet(target *orm.Model, items any, depth int) map[string]any {
	list := c.relatedSlice(target, items, depth)
	var first, last any
	if len(list) > 0 {
		first = list[0]
		last = list[len(list)-1]
	}
	return map[string]any{
		"all":    list,
		"count":  len(list),
		"exists": len(list) > 0,
		"first":  first,
		"last":   last,
	}
}

// relatedSlice converts a prefetched typed slice []T into a []any of wrapped
// model maps.
func (c converter) relatedSlice(target *orm.Model, items any, depth int) []any {
	if items == nil {
		return []any{}
	}
	rv := reflect.ValueOf(items)
	if rv.Kind() != reflect.Slice {
		return []any{}
	}
	n := rv.Len()
	out := make([]any, n)
	for i := range n {
		elem := rv.Index(i)
		if target != nil {
			out[i] = c.wrap(c.instance(target, elem, depth+1))
		} else {
			out[i] = c.value(elem.Interface(), depth+1)
		}
	}
	return out
}

// relation adds the two attributes of a forward FK or O2O relation: <attr>_id
// with the related primary key and <attr> with the related object when loaded.
func (c converter) relation(out map[string]any, f *orm.Field, fv reflect.Value, depth int) {
	out[f.Attr] = nil
	out[f.Attr+"_id"] = nil
	if !fv.CanInterface() {
		return
	}
	if keyed, ok := fv.Interface().(interface{ PK() int64 }); ok {
		if pk := keyed.PK(); pk != 0 {
			out[f.Attr+"_id"] = pk
		}
	}
	object := fv.MethodByName("Object")
	if !object.IsValid() || object.Type().NumIn() != 0 || object.Type().NumOut() != 2 {
		return
	}
	res := object.Call(nil)
	if res[1].Kind() != reflect.Bool || !res[1].Bool() {
		return
	}
	obj := res[0]
	if obj.Kind() != reflect.Pointer || obj.IsNil() || f.Rel.Target == nil || depth >= maxModelDepth {
		return
	}
	out[f.Attr] = c.wrap(c.instance(f.Rel.Target, obj, depth+1))
}

// modelMethod is one method a template can call on a model instance.
type modelMethod struct {
	// key is the snake_case name the template uses.
	key string
	// index is the method's index in the method set of the pointer type.
	index int
	// withErr is set for a method with a second, error result.
	withErr bool
}

// methodCache maps a pointer type to its []modelMethod.
var methodCache sync.Map

// errorType is the reflect.Type of error.
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// methodsOf lists the methods of t a template can call: exported, without
// parameters, with one result or a result and an error. String is left out;
// it is the label.
func methodsOf(t reflect.Type) []modelMethod {
	if cached, ok := methodCache.Load(t); ok {
		return cached.([]modelMethod)
	}
	var out []modelMethod
	for i := range t.NumMethod() {
		meth := t.Method(i)
		mt := meth.Type
		// The receiver is the first input of a method taken from a type.
		if mt.NumIn() != 1 || meth.Name == "String" {
			continue
		}
		switch {
		case mt.NumOut() == 1:
			out = append(out, modelMethod{key: snakeCase(meth.Name), index: i})
		case mt.NumOut() == 2 && mt.Out(1) == errorType:
			out = append(out, modelMethod{key: snakeCase(meth.Name), index: i, withErr: true})
		}
	}
	methodCache.Store(t, out)
	return out
}

// method returns the function a template calls for one model method. The
// result goes through a converter, so a method may return model instances;
// they take the render form, because only a template sees them.
func (c converter) method(ptr reflect.Value, meth modelMethod) func() (any, error) {
	inner := converter{lookup: c.lookup}
	return func() (any, error) {
		res := ptr.Method(meth.index).Call(nil)
		if meth.withErr && !res[1].IsNil() {
			err, _ := res[1].Interface().(error)
			return nil, err
		}
		return inner.value(res[0].Interface(), 1), nil
	}
}

// snakeCase converts a Go identifier to snake_case the way the orm names
// attributes: GetAbsoluteURL becomes get_absolute_url.
func snakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if !unicode.IsUpper(r) {
			b.WriteRune(r)
			continue
		}
		if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
			(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
