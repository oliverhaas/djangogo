package templates

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// querystringTagNode implements Django's {% querystring %}: the query string
// of the current request with parameters set or removed.
//
//	{% querystring page=page.next_page_number %}   ?q=go&page=3
//	{% querystring q=None %}                        removes q
//	{% querystring params tag=tags %}               starts from params, tag repeated
//
// Positional arguments are maps (a url.Values, a map with string keys);
// without one the tag starts from the GET parameters of the request. The
// keyword arguments come last. A later argument replaces the values of a
// key, None removes it, and a list or map gives one parameter per element.
// The result starts with "?" and, with "as name", is stored instead of
// printed.
type querystringTagNode struct {
	start  *pongo2.Token
	args   []pongo2.IEvaluator
	kwargs []querystringKwarg
	asVar  string
}

// querystringKwarg is one keyword argument of {% querystring %}.
type querystringKwarg struct {
	name string
	expr pongo2.IEvaluator
}

// Execute builds the query string and prints or stores it.
func (n *querystringTagNode) Execute(ctx *pongo2.ExecutionContext, w pongo2.TemplateWriter) *pongo2.Error {
	var params queryParams
	if len(n.args) == 0 {
		st := stateFrom(ctx)
		if st == nil || st.request == nil {
			return ctx.OrigError(errors.New("'querystring' without arguments needs the request: render with RenderRequest"), n.start)
		}
		params = parseQueryParams(st.request.URL.RawQuery)
	}
	for _, expr := range n.args {
		val, err := expr.Evaluate(ctx)
		if err != nil {
			return err
		}
		if err := params.update(val.Interface()); err != nil {
			return ctx.OrigError(err, n.start)
		}
	}
	for _, kw := range n.kwargs {
		val, err := kw.expr.Evaluate(ctx)
		if err != nil {
			return err
		}
		params.set(kw.name, val.Interface())
	}
	out := "?" + params.encode()
	if n.asVar != "" {
		ctx.Private[n.asVar] = out
		return nil
	}
	return writeOutput(ctx, w, pongo2.AsValue(out))
}

// queryParams is a Django QueryDict: a list of values per key, the keys in
// the order they were first set.
type queryParams struct {
	keys   []string
	values map[string][]string
}

// parseQueryParams reads the GET parameters of a request as the tag sees
// them through QueryDict.items(): "+" and percent escapes decode, a
// parameter without a value is kept, and a repeated key keeps its last
// value at the place it first appeared.
func parseQueryParams(raw string) queryParams {
	var p queryParams
	for part := range strings.SplitSeq(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		p.setList(unquotePlus(k), []string{unquotePlus(v)})
	}
	return p
}

// unquotePlus decodes s as Python's urllib.parse.unquote_plus; it keeps an
// invalid escape as it is.
func unquotePlus(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return strings.ReplaceAll(s, "+", " ")
}

// setList replaces the values of key, keeping its place.
func (p *queryParams) setList(key string, values []string) {
	if p.values == nil {
		p.values = make(map[string][]string)
	}
	if _, ok := p.values[key]; !ok {
		p.keys = append(p.keys, key)
	}
	p.values[key] = values
}

// remove deletes key.
func (p *queryParams) remove(key string) {
	if _, ok := p.values[key]; !ok {
		return
	}
	delete(p.values, key)
	for i, k := range p.keys {
		if k == key {
			p.keys = append(p.keys[:i], p.keys[i+1:]...)
			break
		}
	}
}

// set applies one argument value to key: None removes the key, a list or a
// map sets one value per element (a map's keys), anything else one value.
func (p *queryParams) set(key string, value any) {
	value = unwrapValue(value)
	if value == nil {
		p.remove(key)
		return
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		values := make([]string, rv.Len())
		for i := range values {
			values[i] = pyStr(rv.Index(i).Interface())
		}
		p.setList(key, values)
	case reflect.Map:
		if _, isModel := value.(modelView); !isModel {
			keys := sortedKeys(rv)
			values := make([]string, len(keys))
			for i, k := range keys {
				values[i] = pyStr(k.Interface())
			}
			p.setList(key, values)
			return
		}
		p.setList(key, []string{pyStr(value)})
	default:
		p.setList(key, []string{pyStr(value)})
	}
}

// update applies the entries of a positional argument, which must be a map
// with string keys, in key order. A url.Values gives the last value of each
// key, as Django's QueryDict.items() does.
func (p *queryParams) update(m any) error {
	m = unwrapValue(m)
	rv := reflect.ValueOf(m)
	if _, isModel := m.(modelView); isModel || rv.Kind() != reflect.Map {
		msg := fmt.Sprintf("querystring requires mappings for positional arguments (got %s instead).", pyRepr(m))
		return errors.New(msg)
	}
	values, isQuery := m.(url.Values)
	for _, k := range sortedKeys(rv) {
		key, ok := k.Interface().(string)
		if !ok {
			msg := fmt.Sprintf("querystring requires strings for mapping keys (got %s instead).", pyRepr(k.Interface()))
			return errors.New(msg)
		}
		if isQuery {
			if list := values[key]; len(list) > 0 {
				p.setList(key, list[len(list)-1:])
			}
			continue
		}
		p.set(key, rv.MapIndex(k).Interface())
	}
	return nil
}

// encode returns the parameters as a query string, Django's
// QueryDict.urlencode.
func (p *queryParams) encode() string {
	var parts []string
	for _, k := range p.keys {
		for _, v := range p.values[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

// querystringTagParser parses {% querystring [mapping ...] [key=value ...]
// [as name] %}.
func querystringTagParser(_ *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	node := &querystringTagNode{start: start}
	seen := map[string]bool{}
	for args.Remaining() > 0 {
		if args.Match(pongo2.TokenKeyword, "as") != nil {
			name := args.MatchType(pongo2.TokenIdentifier)
			if name == nil || args.Remaining() > 0 {
				return nil, args.Error("'querystring' expects a variable name after 'as'", start)
			}
			node.asVar = name.Val
			break
		}
		if key := args.PeekType(pongo2.TokenIdentifier); key != nil && args.PeekN(1, pongo2.TokenSymbol, "=") != nil {
			args.ConsumeN(2)
			if seen[key.Val] {
				return nil, args.Error(fmt.Sprintf("'querystring' received multiple values for keyword argument '%s'", key.Val), start)
			}
			seen[key.Val] = true
			expr, err := args.ParseExpression()
			if err != nil {
				return nil, err
			}
			node.kwargs = append(node.kwargs, querystringKwarg{name: key.Val, expr: expr})
			continue
		}
		if len(node.kwargs) > 0 {
			return nil, args.Error("'querystring' received some positional argument(s) after some keyword argument(s)", start)
		}
		expr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}
		node.args = append(node.args, expr)
	}
	return node, nil
}
