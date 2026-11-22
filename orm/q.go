package orm

import (
	"errors"
	"fmt"
	"strings"
)

// errFilterArgs is reported when the arguments of Filter, Exclude, Get, or Q
// are not conditions and alternating string keys and values.
var errFilterArgs = errors.New("orm: Filter requires alternating string keys and values")

// connector is how the children of a condition are combined.
type connector uint8

const (
	// connAnd requires every child to hold.
	connAnd connector = iota
	// connOr requires at least one child to hold.
	connOr
)

// String returns the SQL keyword of the connector.
func (c connector) String() string {
	if c == connOr {
		return "OR"
	}
	return "AND"
}

// Cond is a tree of filter conditions, the counterpart of Django's Q object.
// Build one with Q and combine conditions with And, Or, and Not:
//
//	Q(name="x") | Q(age__gt=3)     orm.Q("name", "x").Or(orm.Q("age__gt", 3))
//	Q(a=1) & Q(b=2)                orm.Q("a", 1).And(orm.Q("b", 2)) or orm.Q("a", 1, "b", 2)
//	~Q(name="x")                   orm.Not(orm.Q("name", "x")) or orm.Q("name", "x").Not()
//
// A Cond is immutable: every method returns a new value. Pass it to Filter,
// Exclude, or Get, alone or mixed with key/value pairs. The zero Cond is empty
// and matches every row.
type Cond struct {
	conn     connector
	negated  bool
	children []condChild
	err      error
}

// condChild is one child of a Cond: a key/value leaf, or a nested condition
// when sub is set.
type condChild struct {
	key   string
	value any
	sub   *Cond
}

// Q builds a condition from alternating keys and values, combined with AND,
// like Django's Q(**kwargs). A key is a field name with an optional lookup
// ("name", "age__gt", "author__name__icontains"). An argument may also be
// another Cond, which is ANDed in. A malformed argument list is reported when
// the queryset runs.
func Q(args ...any) Cond {
	c, err := parseConds(args)
	if err != nil {
		c.err = err
	}
	return c
}

// Not returns the negation of c, like Django's ~Q.
func Not(c Cond) Cond { return c.Not() }

// And returns the conjunction of c and the given conditions (Django's &).
func (c Cond) And(others ...Cond) Cond {
	out := c
	for _, o := range others {
		out = combine(connAnd, out, o)
	}
	return out
}

// Or returns the disjunction of c and the given conditions (Django's |).
func (c Cond) Or(others ...Cond) Cond {
	out := c
	for _, o := range others {
		out = combine(connOr, out, o)
	}
	return out
}

// Not returns the negation of c (Django's ~).
func (c Cond) Not() Cond {
	out := c.copy()
	out.negated = !out.negated
	return out
}

// String renders the condition tree in the form of Django's Q repr, for
// example "(OR: ('a', 1), (NOT (AND: ('b', 2))))".
func (c Cond) String() string {
	var b strings.Builder
	if c.negated {
		b.WriteString("(NOT ")
	}
	b.WriteString("(" + c.conn.String() + ": ")
	for i, ch := range c.children {
		if i > 0 {
			b.WriteString(", ")
		}
		if ch.sub != nil {
			b.WriteString(ch.sub.String())
			continue
		}
		fmt.Fprintf(&b, "('%s', %v)", ch.key, formatCondValue(ch.value))
	}
	b.WriteString(")")
	if c.negated {
		b.WriteString(")")
	}
	return b.String()
}

// formatCondValue renders a leaf value for Cond.String: strings are quoted, as
// Python's repr quotes them, and other values use their default format.
func formatCondValue(v any) string {
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	}
	return fmt.Sprint(v)
}

// Pairs returns the alternating keys and values of c when c is a conjunction
// of key/value pairs, such as orm.Q("warm", true, "age__gt", 3): the
// counterpart of a Django lookup dict. It reports false for a negated, OR or
// nested condition and for one with an error.
func (c Cond) Pairs() ([]any, bool) {
	if c.err != nil || c.negated || c.conn != connAnd {
		return nil, false
	}
	out := make([]any, 0, 2*len(c.children))
	for _, ch := range c.children {
		if ch.sub != nil {
			return nil, false
		}
		out = append(out, ch.key, ch.value)
	}
	return out, true
}

// empty reports whether the condition has no children, so it restricts nothing.
func (c Cond) empty() bool { return len(c.children) == 0 }

// copy returns a Cond whose children slice is not shared with c.
func (c Cond) copy() Cond {
	out := c
	out.children = append([]condChild(nil), c.children...)
	return out
}

// combine joins a and b under conn the way Django's Q._combine does: an empty
// side yields the other side unchanged, and children that use the same
// connector are merged into one level.
func combine(conn connector, a, b Cond) Cond {
	switch {
	case a.err != nil:
		return a
	case b.err != nil:
		return b
	case b.empty():
		return a.copy()
	case a.empty():
		return b.copy()
	}
	out := Cond{conn: conn}
	out.add(a)
	out.add(b)
	return out
}

// add appends child to c. A child that is not negated and either uses c's
// connector or has a single child is merged into c instead of nested, which
// keeps the tree, and so the generated SQL, flat.
func (c *Cond) add(child Cond) {
	if child.err != nil && c.err == nil {
		c.err = child.err
	}
	if child.empty() {
		return
	}
	if !child.negated && (child.conn == c.conn || len(child.children) == 1) {
		c.children = append(c.children, child.children...)
		return
	}
	sub := child.copy()
	c.children = append(c.children, condChild{sub: &sub})
}

// parseConds turns the arguments of Filter, Exclude, Get, or Q into one AND
// condition. Each argument is a Cond, or a string key followed by its value.
func parseConds(args []any) (Cond, error) {
	var out Cond
	for i := 0; i < len(args); i++ {
		switch a := args[i].(type) {
		case Cond:
			out.add(a)
		case *Cond:
			if a == nil {
				return Cond{}, errFilterArgs
			}
			out.add(*a)
		case string:
			if i+1 >= len(args) {
				return Cond{}, errFilterArgs
			}
			i++
			out.children = append(out.children, condChild{key: a, value: args[i]})
		default:
			return Cond{}, errFilterArgs
		}
	}
	return out, out.err
}
