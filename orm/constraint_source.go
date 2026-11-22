package orm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// binaryMethods maps the operator of an arithmetic or bitwise Expr to the
// method that builds it.
var binaryMethods = map[string]string{
	"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Mod", opPow: "Pow",
	"&": "BitAnd", "|": "BitOr", opBitXor: "BitXor",
	opShiftLeft: "BitLeftShift", opShiftRight: "BitRightShift",
}

// functionNames maps the SQL function of a function Expr to the function that
// builds it.
var functionNames = map[string]string{
	"LOWER": "Lower", "UPPER": "Upper", "LENGTH": "Length",
	"TRIM": "Trim", "LTRIM": "LTrim", "RTRIM": "RTrim",
	"COALESCE": "Coalesce", fnConcat: "Concat", "ABS": "Abs",
}

// ConstraintSource returns Go source code that builds c, such as
//
//	orm.CheckConstraint{Name: "age_gte_18", Condition: orm.Q("age__gte", int64(18))}
//
// for the migration writer (Django's deconstruct). literal renders a value of
// a condition or expression and reports an error for a value it cannot
// write. Building the source and rendering it again gives the same text, so
// two constraints are the same when their sources are.
func ConstraintSource(c Constraint, literal func(any) (string, error)) (string, error) {
	g := sourceWriter{literal: literal}
	switch x := c.(type) {
	case CheckConstraint:
		return g.check(x)
	case *CheckConstraint:
		if x != nil {
			return g.check(*x)
		}
	case UniqueConstraint:
		return g.unique(x)
	case *UniqueConstraint:
		if x != nil {
			return g.unique(*x)
		}
	}
	return "", fmt.Errorf("orm: no Go source for the constraint %#v", c)
}

// sourceWriter renders constraints, conditions and expressions as Go source.
type sourceWriter struct {
	literal func(any) (string, error)
}

// check renders a CheckConstraint literal.
func (g sourceWriter) check(c CheckConstraint) (string, error) {
	cond, err := g.cond(c.Condition)
	if err != nil {
		return "", fmt.Errorf("orm: constraint %q: %w", c.Name, err)
	}
	parts := []string{"Name: " + strconv.Quote(c.Name), "Condition: " + cond}
	parts = appendViolation(parts, c.ViolationErrorCode, c.ViolationErrorMessage)
	return "orm.CheckConstraint{" + strings.Join(parts, ", ") + "}", nil
}

// unique renders a UniqueConstraint literal.
func (g sourceWriter) unique(c UniqueConstraint) (string, error) {
	parts := []string{"Name: " + strconv.Quote(c.Name)}
	if len(c.Fields) > 0 {
		quoted := make([]string, len(c.Fields))
		for i, f := range c.Fields {
			quoted[i] = strconv.Quote(f)
		}
		parts = append(parts, "Fields: []string{"+strings.Join(quoted, ", ")+"}")
	}
	if len(c.Expressions) > 0 {
		exprs := make([]string, len(c.Expressions))
		for i, e := range c.Expressions {
			s, err := g.expr(e)
			if err != nil {
				return "", fmt.Errorf("orm: constraint %q: %w", c.Name, err)
			}
			exprs[i] = s
		}
		parts = append(parts, "Expressions: []orm.Expr{"+strings.Join(exprs, ", ")+"}")
	}
	if !c.Condition.empty() || c.Condition.err != nil {
		cond, err := g.cond(c.Condition)
		if err != nil {
			return "", fmt.Errorf("orm: constraint %q: %w", c.Name, err)
		}
		parts = append(parts, "Condition: "+cond)
	}
	if c.NullsNotDistinct {
		parts = append(parts, "NullsNotDistinct: true")
	}
	parts = appendViolation(parts, c.ViolationErrorCode, c.ViolationErrorMessage)
	return "orm.UniqueConstraint{" + strings.Join(parts, ", ") + "}", nil
}

// appendViolation adds the violation code and message fields that are set.
func appendViolation(parts []string, code, message string) []string {
	if code != "" {
		parts = append(parts, "ViolationErrorCode: "+strconv.Quote(code))
	}
	if message != "" {
		parts = append(parts, "ViolationErrorMessage: "+strconv.Quote(message))
	}
	return parts
}

// cond renders a condition with the calls that build the same tree: Q for a
// conjunction, Or for a disjunction, and Not for a negation. Q and Or merge
// and nest children the way the tree was built, so the source builds an
// identical tree.
func (g sourceWriter) cond(c Cond) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	inner, err := g.condInner(c)
	if err != nil {
		return "", err
	}
	if c.negated {
		return "orm.Not(" + inner + ")", nil
	}
	return inner, nil
}

// condInner renders c without its negation.
func (g sourceWriter) condInner(c Cond) (string, error) {
	if c.conn == connAnd || len(c.children) < 2 {
		args := make([]string, 0, 2*len(c.children))
		for _, ch := range c.children {
			if ch.sub != nil {
				s, err := g.cond(*ch.sub)
				if err != nil {
					return "", err
				}
				args = append(args, s)
				continue
			}
			v, err := g.value(ch.value)
			if err != nil {
				return "", fmt.Errorf("the value of %q: %w", ch.key, err)
			}
			args = append(args, strconv.Quote(ch.key), v)
		}
		return "orm.Q(" + strings.Join(args, ", ") + ")", nil
	}
	parts := make([]string, len(c.children))
	for i, ch := range c.children {
		var err error
		if ch.sub != nil {
			parts[i], err = g.cond(*ch.sub)
		} else {
			parts[i], err = g.condInner(Cond{children: []condChild{ch}})
		}
		if err != nil {
			return "", err
		}
	}
	return parts[0] + ".Or(" + strings.Join(parts[1:], ", ") + ")", nil
}

// value renders the value of a condition: an expression, a slice as []any,
// or a literal.
func (g sourceWriter) value(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "nil", nil
	case Expr:
		return g.expr(x)
	case *Expr:
		if x != nil {
			return g.expr(*x)
		}
		return "nil", nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
		items := make([]string, rv.Len())
		for i := range items {
			s, err := g.value(rv.Index(i).Interface())
			if err != nil {
				return "", err
			}
			items[i] = s
		}
		return "[]any{" + strings.Join(items, ", ") + "}", nil
	}
	return g.literal(v)
}

// expr renders an expression with the calls that build it.
func (g sourceWriter) expr(e Expr) (string, error) {
	switch e.kind {
	case exprField:
		return "orm.F(" + strconv.Quote(e.key) + ")", nil
	case exprValue:
		v, err := g.value(e.value)
		if err != nil {
			return "", err
		}
		return "orm.Value(" + v + ")", nil
	case exprBinary:
		method, ok := binaryMethods[e.op]
		if !ok {
			break
		}
		left, err := g.expr(*e.left)
		if err != nil {
			return "", err
		}
		right, err := g.expr(*e.right)
		if err != nil {
			return "", err
		}
		return left + "." + method + "(" + right + ")", nil
	case exprFunc:
		name, ok := functionNames[e.fn]
		if !ok {
			return "", fmt.Errorf("orm: the function %s has no Go source", e.fn)
		}
		arg, err := g.expr(*e.arg)
		if err != nil {
			return "", err
		}
		return "orm." + name + "(" + arg + ")", nil
	case exprCall:
		args := make([]string, len(e.args))
		for i, a := range e.args {
			arg, err := g.expr(a)
			if err != nil {
				return "", err
			}
			args[i] = arg
		}
		return "orm." + functionNames[e.fn] + "(" + strings.Join(args, ", ") + ")", nil
	case exprCast:
		arg, err := g.expr(*e.arg)
		if err != nil {
			return "", err
		}
		return "orm.Cast(" + arg + ", " + castFieldSource(e.castTo) + ")", nil
	case exprCase:
		return g.caseExpr(e)
	case exprNot:
		arg, err := g.expr(*e.arg)
		if err != nil {
			return "", err
		}
		return arg + ".Not()", nil
	}
	return "", fmt.Errorf("orm: an aggregate or bound expression (kind %d) has no Go source", e.kind)
}

// caseExpr renders a Case with its branches and its Default.
func (g sourceWriter) caseExpr(e Expr) (string, error) {
	whens := make([]string, len(e.whens))
	for i, w := range e.whens {
		cond, err := g.cond(w.Cond)
		if err != nil {
			return "", err
		}
		then, err := g.expr(fieldOrExpr(w.Then))
		if err != nil {
			return "", err
		}
		whens[i] = "orm.When{Cond: " + cond + ", Then: " + then + "}"
	}
	out := "orm.Case(" + strings.Join(whens, ", ") + ")"
	if e.def != nil {
		def, err := g.expr(*e.def)
		if err != nil {
			return "", err
		}
		out += ".Default(" + def + ")"
	}
	return out, nil
}

// castFieldSource renders the field of a Cast with the attributes that Cast
// keeps.
func castFieldSource(f *Field) string {
	parts := []string{"Kind: orm.Kind" + f.Kind.String()}
	for _, attr := range []struct {
		name  string
		value int
	}{{"MaxLength", f.MaxLength}, {"MaxDigits", f.MaxDigits}, {"DecimalPlaces", f.DecimalPlaces}} {
		if attr.value != 0 {
			parts = append(parts, attr.name+": "+strconv.Itoa(attr.value))
		}
	}
	return "orm.Field{" + strings.Join(parts, ", ") + "}"
}
