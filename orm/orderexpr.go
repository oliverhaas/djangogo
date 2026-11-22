package orm

import (
	"errors"
	"fmt"
)

// errNullsExclusive is the error of an OrderExpr with both NullsFirst and
// NullsLast, as Django's OrderBy raises a ValueError.
var errNullsExclusive = errors.New("orm: NullsFirst and NullsLast are mutually exclusive")

// OrderExpr is an expression with a direction, Django's OrderBy: a term of
// OrderBy, which Expr.Asc and Expr.Desc return. NullsFirst and NullsLast
// place the rows whose expression is NULL:
//
//	OrderBy(orm.F("author").Desc().NullsLast(), "headline")
//	OrderBy(orm.Lower("name").Asc())
type OrderExpr struct {
	expr       Expr
	desc       bool
	nullsFirst bool
	nullsLast  bool
	err        error
}

// Asc orders by e ascending, Django's F("x").asc().
func (e Expr) Asc() OrderExpr { return OrderExpr{expr: e} }

// Desc orders by e descending, Django's F("x").desc().
func (e Expr) Desc() OrderExpr { return OrderExpr{expr: e, desc: true} }

// NullsFirst puts the rows whose expression is NULL first, Django's
// nulls_first=True. A query ordered by a term with both NullsFirst and
// NullsLast fails with an error.
func (o OrderExpr) NullsFirst() OrderExpr {
	if o.nullsLast {
		o.err = errNullsExclusive
	}
	o.nullsFirst = true
	return o
}

// NullsLast puts the rows whose expression is NULL last, Django's
// nulls_last=True.
func (o OrderExpr) NullsLast() OrderExpr {
	if o.nullsFirst {
		o.err = errNullsExclusive
	}
	o.nullsLast = true
	return o
}

// Reverse returns o with the opposite direction, and with NULL rows on the
// other end when NullsFirst or NullsLast is set, Django's
// OrderBy.reverse_ordering.
func (o OrderExpr) Reverse() OrderExpr {
	o.desc = !o.desc
	o.nullsFirst, o.nullsLast = o.nullsLast, o.nullsFirst
	return o
}

// Expr returns the expression that o orders by, Django's OrderBy.expression.
func (o OrderExpr) Expr() Expr { return o.expr }

// Descending reports whether o orders descending, Django's
// OrderBy.descending.
func (o OrderExpr) Descending() bool { return o.desc }

// FieldName returns the key of an F expression, Django's F.name, and false
// for any other expression.
func (e Expr) FieldName() (string, bool) {
	if e.kind != exprField {
		return "", false
	}
	return e.key, true
}

// errOrderAggregate is the error of an aggregate in OrderBy, which Django
// raises as a FieldError.
var errOrderAggregate = errors.New("orm: using an aggregate in OrderBy without also including it in Annotate is not allowed")

// parseOrderArgs turns the arguments of OrderBy into orderTerms: a key in the
// form parseOrderTerms takes, an Expr, which is ordered ascending, or an
// OrderExpr.
func parseOrderArgs(args []any) ([]orderTerm, error) {
	out := make([]orderTerm, 0, len(args))
	for _, a := range args {
		switch v := a.(type) {
		case string:
			out = append(out, parseOrderTerms([]string{v})...)
		case Expr:
			if v.kind == 0 {
				return nil, errors.New("orm: OrderBy got the zero Expr")
			}
			if v.containsAggregate() {
				return nil, errOrderAggregate
			}
			out = append(out, orderTerm{expr: &v})
		case OrderExpr:
			if v.err != nil {
				return nil, v.err
			}
			if v.expr.kind == 0 {
				return nil, errors.New("orm: OrderBy got the zero OrderExpr")
			}
			if v.expr.containsAggregate() {
				return nil, errOrderAggregate
			}
			out = append(out, orderTerm{
				expr:       &v.expr,
				desc:       v.desc,
				nullsFirst: v.nullsFirst,
				nullsLast:  v.nullsLast,
				directed:   true,
			})
		default:
			return nil, fmt.Errorf("orm: OrderBy takes keys and expressions, not %T", a)
		}
	}
	return out, nil
}

// orderArgs returns terms in the form OrderBy takes them: a key, "-" and a
// key, "?" or an OrderExpr.
func orderArgs(terms []orderTerm) []any {
	out := make([]any, len(terms))
	for i, t := range terms {
		switch {
		case t.random:
			out[i] = "?"
		case t.expr != nil:
			out[i] = OrderExpr{expr: *t.expr, desc: t.desc, nullsFirst: t.nullsFirst, nullsLast: t.nullsLast}
		case t.desc:
			out[i] = "-" + t.key
		default:
			out[i] = t.key
		}
	}
	return out
}

// prefixed returns e with prefix in front of every field key it reads,
// Django's prefix_references: the expression of the default ordering of a
// related model, read from the model that orders by the relation.
func (e Expr) prefixed(prefix string) Expr {
	out := e
	if e.kind == exprField {
		out.key = prefix + e.key
	}
	out.left = prefixedPtr(e.left, prefix)
	out.right = prefixedPtr(e.right, prefix)
	out.arg = prefixedPtr(e.arg, prefix)
	out.def = prefixedPtr(e.def, prefix)
	if e.filter != nil {
		f := e.filter.prefixed(prefix)
		out.filter = &f
	}
	if e.args != nil {
		out.args = make([]Expr, len(e.args))
		for i, a := range e.args {
			out.args[i] = a.prefixed(prefix)
		}
	}
	if e.whens != nil {
		out.whens = make([]When, len(e.whens))
		for i, w := range e.whens {
			out.whens[i] = When{Cond: w.Cond.prefixed(prefix), Then: prefixedValue(w.Then, prefix)}
		}
	}
	return out
}

// prefixedPtr is Expr.prefixed for an optional operand.
func prefixedPtr(e *Expr, prefix string) *Expr {
	if e == nil {
		return nil
	}
	p := e.prefixed(prefix)
	return &p
}

// prefixedValue is Expr.prefixed for the value of a condition or a When,
// which is an expression or a plain value.
func prefixedValue(v any, prefix string) any {
	switch e := v.(type) {
	case Expr:
		return e.prefixed(prefix)
	case *Expr:
		return prefixedPtr(e, prefix)
	}
	return v
}

// prefixed returns c with prefix in front of every key and of the field keys
// of its expression values.
func (c Cond) prefixed(prefix string) Cond {
	out := c
	if c.children == nil {
		return out
	}
	out.children = make([]condChild, len(c.children))
	for i, ch := range c.children {
		if ch.sub != nil {
			sub := ch.sub.prefixed(prefix)
			ch.sub = &sub
		} else {
			ch.key = prefix + ch.key
			ch.value = prefixedValue(ch.value, prefix)
		}
		out.children[i] = ch
	}
	return out
}
