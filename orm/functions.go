package orm

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// function builds a call of the SQL function fn of one argument. arg is a
// field key or an Expr: like Django's functions, a string names a field. Pass
// Value("text") for a literal. output is the kind of the result when typed
// is set; otherwise the result has the kind of the argument.
func function(fn string, arg any, output Kind, typed bool) Expr {
	a := toExpr(arg)
	if key, ok := arg.(string); ok {
		a = F(key)
	}
	return Expr{kind: exprFunc, fn: fn, arg: &a, output: output, typed: typed}
}

// Lower returns arg in lower case, Django's Lower. arg is a field key such as
// "name", or an Expr.
func Lower(arg any) Expr { return function("LOWER", arg, 0, false) }

// Upper returns arg in upper case, Django's Upper.
func Upper(arg any) Expr { return function("UPPER", arg, 0, false) }

// Length returns the number of characters of arg, Django's Length. The value
// is an int64, and nil when arg is NULL.
func Length(arg any) Expr { return function("LENGTH", arg, KindInt, true) }

// Trim returns arg without leading and trailing spaces, Django's Trim.
func Trim(arg any) Expr { return function("TRIM", arg, 0, false) }

// LTrim returns arg without leading spaces, Django's LTrim.
func LTrim(arg any) Expr { return function("LTRIM", arg, 0, false) }

// RTrim returns arg without trailing spaces, Django's RTrim.
func RTrim(arg any) Expr { return function("RTRIM", arg, 0, false) }

// outputKind returns the kind of the value of e when the expression it
// transforms has kind input: the result kind of a function such as Length,
// and input for the others.
func (e Expr) outputKind(input Kind) Kind {
	if e.kind == exprFunc {
		if e.typed {
			return e.output
		}
		return e.arg.outputKind(input)
	}
	return input
}

// registeredTransform is a transform of RegisterTransform.
type registeredTransform struct {
	name string
	fn   func(arg any) Expr
}

// transformRegistry holds the transforms of RegisterTransform by field kind.
var transformRegistry struct {
	sync.RWMutex
	byKind map[Kind][]*registeredTransform
}

// RegisterTransform makes fn the transform name of the fields of kind k,
// Django's Field.register_lookup with a Transform such as Lower. After
//
//	orm.RegisterTransform(orm.KindChar, "lower", orm.Lower)
//
// the key "name__lower" is the lowercased name in Filter ("name__lower",
// "john smith"), Values, OrderBy and F, and the lookups that follow it
// compare the lowercased value ("name__lower__startswith"). fn gets the
// transformed value and returns the expression of the transform. The value
// of the transform has the kind of the result of fn: an integer for Length,
// and kind k for the other text functions.
//
// The transform applies to kind k only, where Django's registration on
// CharField also covers its subclasses: register it for KindSlug, KindEmail
// and KindURL as well where they need it. A later registration of a name
// takes the place of an earlier one. RegisterTransform returns the function
// that removes the transform again. It panics on an empty name, a name with
// "__" or a nil fn.
func RegisterTransform(k Kind, name string, fn func(arg any) Expr) (unregister func()) {
	if name == "" || strings.Contains(name, lookupSep) || fn == nil {
		panic(fmt.Sprintf("orm: RegisterTransform needs a name without %q and a function", lookupSep))
	}
	rt := &registeredTransform{name: name, fn: fn}
	transformRegistry.Lock()
	defer transformRegistry.Unlock()
	if transformRegistry.byKind == nil {
		transformRegistry.byKind = map[Kind][]*registeredTransform{}
	}
	transformRegistry.byKind[k] = append(transformRegistry.byKind[k], rt)
	return func() {
		transformRegistry.Lock()
		defer transformRegistry.Unlock()
		transformRegistry.byKind[k] = slices.DeleteFunc(transformRegistry.byKind[k], func(x *registeredTransform) bool { return x == rt })
	}
}

// registered returns the function of the transform name of kind k that
// RegisterTransform registered last.
func registered(k Kind, name string) (func(arg any) Expr, bool) {
	transformRegistry.RLock()
	defer transformRegistry.RUnlock()
	list := transformRegistry.byKind[k]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].name == name {
			return list[i].fn, true
		}
	}
	return nil, false
}

// registeredNames returns the names of the transforms of kind k that
// RegisterTransform registered, each once.
func registeredNames(k Kind) []string {
	transformRegistry.RLock()
	defer transformRegistry.RUnlock()
	var names []string
	for _, rt := range transformRegistry.byKind[k] {
		if !slices.Contains(names, rt.name) {
			names = append(names, rt.name)
		}
	}
	return names
}
