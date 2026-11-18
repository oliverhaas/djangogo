package orm

import (
	"errors"
	"fmt"
	"strings"
)

// query is the type-independent description of a queryset: what to filter,
// join, order, group, and project. QuerySet[T], ValuesQuerySet, and
// ValuesListQuerySet are thin shells around it, and the compiler turns it into
// SQL. A query is treated as immutable: every builder works on a clone.
type query struct {
	db    *DB
	model *Model
	// filters holds one condition per Filter or Exclude call, in call order.
	// The order matters: each call may change the type of the joins it uses.
	filters       []Cond
	orders        []orderTerm
	selectRelated []string
	annotations   []annotation
	values        *valuesSpec
	limit         int // -1 means no limit
	offset        int // 0 means no offset
	distinct      bool
	forUpdate     bool
	err           error
}

// orderTerm is one ORDER BY term: a field key or annotation name, descending
// when desc is set, or a random order when random is set.
type orderTerm struct {
	key    string
	desc   bool
	random bool
}

// annotation is a named expression added by Annotate.
type annotation struct {
	name string
	expr Expr
}

// valuesSpec records a Values or ValuesList call.
type valuesSpec struct {
	// keys are the requested keys; empty means every field of the model.
	keys []string
	// annotated is the number of annotations that existed when Values was
	// called. Annotations added later are appended to the output.
	annotated int
	// groups reports that Values was called before any aggregate annotation, so
	// the keys form the GROUP BY (Django's values().annotate() idiom).
	groups bool
}

// newQuery returns the empty query for model T on db.
func newQuery(db *DB, m *Model) *query {
	return &query{db: db, model: m, limit: -1}
}

// clone returns a copy of q whose slices are not shared with q, so that a
// change to the clone never reaches the receiver.
func (q *query) clone() *query {
	cp := *q
	cp.filters = append([]Cond(nil), q.filters...)
	cp.orders = append([]orderTerm(nil), q.orders...)
	cp.selectRelated = append([]string(nil), q.selectRelated...)
	cp.annotations = append([]annotation(nil), q.annotations...)
	if q.values != nil {
		v := *q.values
		v.keys = append([]string(nil), q.values.keys...)
		cp.values = &v
	}
	return &cp
}

// setErr records err unless an error is already recorded: the first error wins.
func (q *query) setErr(err error) {
	if q.err == nil {
		q.err = err
	}
}

// filter returns a clone restricted by the conditions in args, negated as a
// whole for Exclude, as Django negates exclude(a=1, b=2) to NOT (a AND b).
func (q *query) filter(args []any, negate bool) *query {
	cp := q.clone()
	cond, err := parseConds(args)
	if err != nil {
		cp.setErr(err)
		return cp
	}
	if cond.empty() {
		return cp
	}
	if negate {
		cond = cond.Not()
	}
	cp.filters = append(cp.filters, cond)
	return cp
}

// orderBy returns a clone with the given terms appended to the ordering. No
// terms clears the ordering.
func (q *query) orderBy(fields []string) *query {
	cp := q.clone()
	if len(fields) == 0 {
		cp.orders = nil
		return cp
	}
	for _, field := range fields {
		if field == "?" {
			cp.orders = append(cp.orders, orderTerm{random: true})
			continue
		}
		desc := strings.HasPrefix(field, "-")
		key := strings.TrimPrefix(strings.TrimPrefix(field, "-"), "+")
		cp.orders = append(cp.orders, orderTerm{key: key, desc: desc})
	}
	return cp
}

// reversed returns a clone with every ordering term flipped. An unordered
// query is ordered by descending primary key, as Django's reverse of the
// default "pk" ordering used by first() and last().
func (q *query) reversed() *query {
	cp := q.clone()
	if len(cp.orders) == 0 {
		cp.orders = []orderTerm{{key: "pk", desc: true}}
		return cp
	}
	for i := range cp.orders {
		cp.orders[i].desc = !cp.orders[i].desc
	}
	return cp
}

// annotate returns a clone with the named expressions in args added. args
// holds alternating names and expressions, or bare aggregates, which take
// their default alias.
func (q *query) annotate(args []any) *query {
	cp := q.clone()
	annots, err := parseAnnotations(args)
	if err != nil {
		cp.setErr(err)
		return cp
	}
	for _, a := range annots {
		if cp.model != nil {
			if _, _, ok := lookupName(cp.model, a.name); ok {
				cp.setErr(fmt.Errorf("orm: the annotation %q conflicts with a field on the model", a.name))
				return cp
			}
		}
		for _, existing := range cp.annotations {
			if existing.name == a.name {
				cp.setErr(fmt.Errorf("orm: the annotation %q is already defined", a.name))
				return cp
			}
		}
		cp.annotations = append(cp.annotations, a)
	}
	return cp
}

// hasAggregates reports whether any annotation holds an aggregate, which makes
// the query a GROUP BY query.
func (q *query) hasAggregates() bool {
	for _, a := range q.annotations {
		if a.expr.containsAggregate() {
			return true
		}
	}
	return false
}

// withValues returns a clone that projects the given keys instead of model
// rows.
func (q *query) withValues(keys []string) *query {
	cp := q.clone()
	cp.values = &valuesSpec{
		keys:      append([]string(nil), keys...),
		annotated: len(cp.annotations),
		groups:    !cp.hasAggregates(),
	}
	return cp
}

// parseAnnotations reads the arguments of Annotate and Aggregate: a string
// name followed by its Expr, or a bare aggregate that takes Django's default
// alias "<key>__<function>".
func parseAnnotations(args []any) ([]annotation, error) {
	var out []annotation
	for i := 0; i < len(args); i++ {
		switch a := args[i].(type) {
		case string:
			if i+1 >= len(args) {
				return nil, fmt.Errorf("orm: annotation %q has no expression", a)
			}
			i++
			e, ok := args[i].(Expr)
			if !ok {
				return nil, fmt.Errorf("orm: annotation %q requires an orm.Expr, got %T", a, args[i])
			}
			if a == "" {
				return nil, errors.New("orm: an annotation name must not be empty")
			}
			out = append(out, annotation{name: a, expr: e})
		case Expr:
			name, ok := a.defaultAlias()
			if !ok {
				return nil, errors.New("orm: complex annotations require an alias")
			}
			out = append(out, annotation{name: name, expr: a})
		default:
			return nil, fmt.Errorf("orm: annotations are names followed by an orm.Expr, got %T", args[i])
		}
	}
	return out, nil
}
