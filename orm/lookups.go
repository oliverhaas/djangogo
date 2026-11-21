package orm

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// The lookups a filter key may end with.
const (
	lookupExact       = "exact"
	lookupIExact      = "iexact"
	lookupGt          = "gt"
	lookupGte         = "gte"
	lookupLt          = "lt"
	lookupLte         = "lte"
	lookupContains    = "contains"
	lookupIContains   = "icontains"
	lookupStartsWith  = "startswith"
	lookupIStartsWith = "istartswith"
	lookupEndsWith    = "endswith"
	lookupIEndsWith   = "iendswith"
	lookupIn          = "in"
	lookupRange       = "range"
	lookupIsNull      = "isnull"
	lookupRegex       = "regex"
	lookupIRegex      = "iregex"
)

// comparisonOps maps the comparison lookups to their SQL operator.
var comparisonOps = map[string]string{
	lookupExact: "=",
	lookupGt:    ">",
	lookupGte:   ">=",
	lookupLt:    "<",
	lookupLte:   "<=",
}

// patternLookups holds the lookups that match text against a pattern. Their
// SQL differs between dialects; see LookupDialect.
var patternLookups = map[string]bool{
	lookupIExact:      true,
	lookupContains:    true,
	lookupIContains:   true,
	lookupStartsWith:  true,
	lookupIStartsWith: true,
	lookupEndsWith:    true,
	lookupIEndsWith:   true,
	lookupRegex:       true,
	lookupIRegex:      true,
}

// lookupNames lists every lookup the ORM implements.
var lookupNames = []string{
	lookupExact, lookupIExact, lookupGt, lookupGte, lookupLt, lookupLte,
	lookupContains, lookupIContains, lookupStartsWith, lookupIStartsWith,
	lookupEndsWith, lookupIEndsWith, lookupIn, lookupRange, lookupIsNull,
	lookupRegex, lookupIRegex,
}

// relationLookupNames lists the lookups of a key that ends on a relation,
// which compares the related primary key. They are the lookups Django
// registers on a ForeignKey.
var relationLookupNames = []string{
	lookupExact, lookupGt, lookupGte, lookupLt, lookupLte, lookupIn, lookupIsNull,
}

// knownLookup reports whether name is a lookup the ORM implements.
func knownLookup(name string) bool { return slices.Contains(lookupNames, name) }

// closeMatches returns up to three of the candidates that resemble word, best
// first, for a "perhaps you meant" hint. It ports Python's
// difflib.get_close_matches, which Django uses, so the hints are the same.
func closeMatches(word string, candidates []string) []string {
	const (
		cutoff = 0.6
		limit  = 3
	)
	type scored struct {
		ratio float64
		name  string
	}
	var found []scored
	for _, cand := range candidates {
		total := len(word) + len(cand)
		if total == 0 {
			continue
		}
		ratio := 2 * float64(matchingChars(cand, word)) / float64(total)
		if ratio >= cutoff {
			found = append(found, scored{ratio: ratio, name: cand})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].ratio != found[j].ratio {
			return found[i].ratio > found[j].ratio
		}
		return found[i].name > found[j].name
	})
	out := make([]string, 0, limit)
	for i := 0; i < len(found) && i < limit; i++ {
		out = append(out, found[i].name)
	}
	return out
}

// matchingChars counts the characters of a and b that lie in matching blocks,
// as difflib.SequenceMatcher finds them: the longest common substring first
// (the earliest in a, then in b), then the same on both sides of it.
func matchingChars(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	bestA, bestB, bestLen := 0, 0, 0
	prev := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		for j := 1; j <= len(b); j++ {
			if a[i-1] != b[j-1] {
				continue
			}
			cur[j] = prev[j-1] + 1
			if cur[j] > bestLen {
				bestLen, bestA, bestB = cur[j], i-cur[j], j-cur[j]
			}
		}
		prev = cur
	}
	if bestLen == 0 {
		return 0
	}
	return bestLen + matchingChars(a[:bestA], b[:bestB]) + matchingChars(a[bestA+bestLen:], b[bestB+bestLen:])
}

// LookupDialect is an optional capability of a Dialect: the SQL of the text
// matching lookups (iexact, contains, icontains, startswith, istartswith,
// endswith, iendswith, regex, iregex), which is where SQLite and PostgreSQL
// differ. lhs is the rendered column or expression and rhs the placeholder of
// the pattern. The pattern of every lookup but regex and iregex is a LIKE
// pattern in which the ORM has already escaped "%", "_", and "\" with a
// backslash.
//
// A dialect that does not implement the interface, or returns ok false, gets
// the portable form `lhs LIKE rhs ESCAPE '\'`. The regex lookups have no
// portable form and are an error on such a dialect.
type LookupDialect interface {
	LookupSQL(lookup, lhs, rhs string) (sql string, ok bool)
}

// parseLookup splits a filter key into the field path and the lookup. A key
// whose last part is not a lookup uses "exact".
func parseLookup(key string) (path, lookup string) {
	idx := strings.LastIndex(key, lookupSep)
	if idx < 0 || !knownLookup(key[idx+len(lookupSep):]) {
		return key, lookupExact
	}
	return key[:idx], key[idx+len(lookupSep):]
}

// escapeLike escapes the LIKE wildcards in s, so that the text matches
// literally under ESCAPE '\'. The backslash is escaped first.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// predicate is one resolved comparison of a WHERE or HAVING clause.
type predicate struct {
	lookup string
	lhs    sqlNode
	// rhs holds the right-hand values: one for a comparison or pattern, two
	// for range, any number for in, and none for isnull.
	rhs []sqlNode
	// sub is the subquery of an "in" or "exact" lookup whose value is a
	// queryset.
	sub *selectStmt
	// flag is the operand of isnull: true tests IS NULL.
	flag bool
	// empty reports that the predicate can match no row: an "in" lookup with
	// no values, or a lookup against a queryset that is known to be empty.
	empty bool
	// null reports that the values of an "in" lookup held a nil. The list
	// leaves it out, because NULL equals nothing.
	null bool
}

// containsAggregate reports whether either side references an aggregate.
func (p *predicate) containsAggregate() bool {
	if p.lhs.aggregate() {
		return true
	}
	for _, r := range p.rhs {
		if r.aggregate() {
			return true
		}
	}
	return false
}

// render writes the predicate.
func (p *predicate) render(w *sqlWriter) {
	if p.empty {
		w.write("0 = 1")
		return
	}
	if op, ok := comparisonOps[p.lookup]; ok {
		p.lhs.render(w)
		w.write(" " + op + " ")
		if p.sub != nil {
			w.write("(")
			p.sub.render(w)
			w.write(")")
			return
		}
		p.rhs[0].render(w)
		return
	}
	switch p.lookup {
	case lookupIsNull:
		p.lhs.render(w)
		if p.flag {
			w.write(" IS NULL")
		} else {
			w.write(" IS NOT NULL")
		}
	case lookupIn:
		p.renderIn(w)
	case lookupRange:
		p.lhs.render(w)
		w.write(" BETWEEN ")
		p.rhs[0].render(w)
		w.write(" AND ")
		p.rhs[1].render(w)
	default:
		lhs := w.capture(func() { p.lhs.render(w) })
		rhs := w.capture(func() { p.rhs[0].render(w) })
		w.write(patternSQL(w.d, p.lookup, lhs, rhs))
	}
}

// renderIn writes an IN predicate.
func (p *predicate) renderIn(w *sqlWriter) {
	if p.sub != nil {
		p.lhs.render(w)
		w.write(" IN (")
		p.sub.render(w)
		w.write(")")
		return
	}
	p.lhs.render(w)
	w.write(" IN (")
	for i, r := range p.rhs {
		if i > 0 {
			w.write(", ")
		}
		r.render(w)
	}
	w.write(")")
}

// patternSQL returns the SQL of a pattern lookup for dialect d, falling back
// to the portable LIKE form.
func patternSQL(d Dialect, lookup, lhs, rhs string) string {
	if ld, ok := d.(LookupDialect); ok {
		if s, ok := ld.LookupSQL(lookup, lhs, rhs); ok {
			return s
		}
	}
	return lhs + ` LIKE ` + rhs + ` ESCAPE '\'`
}

// supportsRegex reports whether dialect d can render the regex lookups.
func supportsRegex(d Dialect) bool {
	ld, ok := d.(LookupDialect)
	if !ok {
		return false
	}
	_, ok = ld.LookupSQL(lookupRegex, "x", "y")
	return ok
}

// buildPredicate resolves the comparison of lhs with value under lookup. field
// is the model field behind lhs, or nil when lhs is an annotation.
func (c *compiler) buildPredicate(lookup string, lhs sqlNode, field *Field, value any) (*predicate, error) {
	p := &predicate{lookup: lookup, lhs: lhs}
	if lookup == lookupIsNull {
		b, ok := value.(bool)
		if !ok {
			return nil, errors.New("orm: the QuerySet value for an isnull lookup must be True or False")
		}
		p.flag = b
		return p, nil
	}
	if lookup == lookupIn {
		return p, c.buildIn(p, field, value)
	}
	if lookup == lookupRange {
		return p, c.buildRange(p, field, value)
	}
	if sq, ok := value.(subqueryer); ok {
		if lookup != lookupExact {
			return nil, fmt.Errorf("orm: the %s lookup does not take a queryset", lookup)
		}
		if sq.subquery().limit != 1 {
			return nil, errors.New(
				"orm: the QuerySet value for an exact lookup must be limited to one result using slicing")
		}
		return p, c.setSubquery(p, sq.subquery(), "exact")
	}

	if patternLookups[lookup] {
		rhs, err := c.prepPatternValue(field, value)
		if err != nil {
			return nil, err
		}
		if rhs == nil {
			if lookup != lookupIExact {
				return nil, errors.New("orm: cannot use nil as a query value")
			}
			p.lookup, p.flag = lookupIsNull, true
			return p, nil
		}
		if (lookup == lookupRegex || lookup == lookupIRegex) && !supportsRegex(c.d) {
			return nil, fmt.Errorf("orm: the %s dialect does not support the %s lookup", c.d.Name(), lookup)
		}
		param, ok := rhs.(paramNode)
		if !ok {
			p.rhs = []sqlNode{exprPattern(lookup, rhs)}
			return p, nil
		}
		text := fmt.Sprint(param.value)
		if field != nil && field.Kind == KindUUID && !hasNativeUUID(c.d) && lookup != lookupRegex && lookup != lookupIRegex {
			text = strings.ReplaceAll(text, "-", "")
		}
		p.rhs = []sqlNode{paramNode{value: likePattern(lookup, text)}}
		return p, nil
	}

	rhs, err := c.prepValue(field, value)
	if err != nil {
		return nil, err
	}
	if rhs == nil {
		// Comparing with NULL is never true in SQL, so "= nil" means IS NULL, as
		// Django turns exact=None into isnull=True.
		if lookup != lookupExact {
			return nil, errors.New("orm: cannot use nil as a query value")
		}
		p.lookup, p.flag = lookupIsNull, true
		return p, nil
	}
	p.rhs = []sqlNode{rhs}
	return p, nil
}

// exprPattern returns the pattern of a pattern lookup whose value is the
// expression n, such as F("lastname"). It is the SQL counterpart of
// likePattern: the database escapes the LIKE wildcards of the value and adds
// the wildcards of the lookup, as Django's pattern_esc and pattern_ops do.
func exprPattern(lookup string, n sqlNode) sqlNode {
	switch lookup {
	case lookupRegex, lookupIRegex:
		return n
	case lookupContains, lookupIContains:
		return likeExprNode{inner: n, prefix: true, suffix: true}
	case lookupStartsWith, lookupIStartsWith:
		return likeExprNode{inner: n, suffix: true}
	case lookupEndsWith, lookupIEndsWith:
		return likeExprNode{inner: n, prefix: true}
	default:
		return likeExprNode{inner: n}
	}
}

// likePattern returns the bound pattern of a pattern lookup for the text s.
// A regular expression is passed through; every other lookup escapes the LIKE
// wildcards of s and adds its own.
func likePattern(lookup, s string) string {
	switch lookup {
	case lookupRegex, lookupIRegex:
		return s
	case lookupContains, lookupIContains:
		return "%" + escapeLike(s) + "%"
	case lookupStartsWith, lookupIStartsWith:
		return escapeLike(s) + "%"
	case lookupEndsWith, lookupIEndsWith:
		return "%" + escapeLike(s)
	default:
		return escapeLike(s)
	}
}

// buildIn fills p for an "in" lookup. value is a slice or array, whose nil
// elements are dropped since NULL is never equal to anything, or a queryset,
// which becomes a subquery.
func (c *compiler) buildIn(p *predicate, field *Field, value any) error {
	if sq, ok := value.(subqueryer); ok {
		return c.setSubquery(p, sq.subquery(), "'in'")
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return fmt.Errorf("orm: in lookup requires a slice, got %T", value)
	}
	for i := range rv.Len() {
		n, err := c.prepValue(field, rv.Index(i).Interface())
		if err != nil {
			return err
		}
		if n == nil {
			p.null = true
			continue
		}
		p.rhs = append(p.rhs, n)
	}
	p.empty = len(p.rhs) == 0
	return nil
}

// setSubquery makes the single column that sub selects the right-hand side of
// p. lookup names the lookup in the error for a queryset that selects several
// columns. A subquery that can match no row makes the predicate empty.
func (c *compiler) setSubquery(p *predicate, sub *query, lookup string) error {
	if sub.db != c.q.db {
		return errors.New("orm: subqueries aren't allowed across different databases")
	}
	stmt, err := sub.subselect(lookup)
	switch {
	case errors.Is(err, errEmptyResultSet):
		p.empty = true
	case err != nil:
		return err
	default:
		p.sub = stmt
	}
	return nil
}

// buildRange fills p for a "range" lookup. value is a slice or array of the
// two inclusive bounds.
func (c *compiler) buildRange(p *predicate, field *Field, value any) error {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) || rv.Len() != 2 {
		return fmt.Errorf("orm: range lookup requires a slice of two values, got %T", value)
	}
	for i := range 2 {
		n, err := c.prepValue(field, rv.Index(i).Interface())
		if err != nil {
			return err
		}
		if n == nil {
			return errors.New("orm: cannot use nil as a query value")
		}
		p.rhs = append(p.rhs, n)
	}
	return nil
}

// subqueryer is implemented by the queryset types, so that a queryset can be
// the value of an "in" lookup.
type subqueryer interface {
	subquery() *query
}

// prepValue turns a filter value into its SQL node: an Expr is resolved
// against the query, and any other value is bound as a parameter. It returns
// nil for a value that is SQL NULL. field is the field the value is compared
// with, or nil.
//
// A value may be a model instance or a pointer to one, which stands for its
// primary key, and an orm.FK[T] or any other driver.Valuer, which stands for
// the value it yields.
func (c *compiler) prepValue(field *Field, value any) (sqlNode, error) {
	switch e := value.(type) {
	case nil:
		return nil, nil
	case Expr:
		return c.resolveExpr(e)
	case *Expr:
		if e == nil {
			return nil, nil
		}
		return c.resolveExpr(*e)
	}
	v, err := plainValue(c.q.db.Registry(), field, value)
	if err != nil || v == nil {
		return nil, err
	}
	bound, err := bindField(c.d, field, v)
	if err != nil || bound == nil {
		return nil, err
	}
	return paramNode{value: bound}, nil
}

// prepPatternValue resolves a right-hand value for a text pattern lookup
// without parsing fragments as full typed values (e.g. UUID substrings).
func (c *compiler) prepPatternValue(field *Field, value any) (sqlNode, error) {
	switch e := value.(type) {
	case nil:
		return nil, nil
	case Expr:
		return c.resolveExpr(e)
	case *Expr:
		if e == nil {
			return nil, nil
		}
		return c.resolveExpr(*e)
	}
	v, err := plainValue(c.q.db.Registry(), field, value)
	if err != nil || v == nil {
		return nil, err
	}
	return paramNode{value: v}, nil
}

// plainValue reduces a Go value to what the driver can bind: a nil pointer is
// NULL, a driver.Valuer is replaced by its value, and an instance of a
// registered model by its primary key. field is the field the value is meant
// for, or nil; when it is a relation, an instance must be of its target model.
func plainValue(reg *Registry, field *Field, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, nil
	}
	if valuer, ok := value.(driver.Valuer); ok {
		v, err := valuer.Value()
		if err != nil {
			return nil, fmt.Errorf("orm: query value %T: %w", value, err)
		}
		return v, nil
	}
	m, ok := reg.ModelOf(value)
	if !ok {
		return value, nil
	}
	if field != nil && field.Rel != nil && field.Rel.Target != nil && field.Rel.Target != m {
		return nil, fmt.Errorf("orm: cannot query %q: must be %q instance", m.Name(), field.Rel.Target.Name())
	}
	pk := m.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("orm: model %s has no primary key", m.Name())
	}
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	pv := rv.Field(pk.Index)
	if pv.IsZero() {
		return nil, errors.New("orm: model instances passed to related filters must be saved")
	}
	return pv.Interface(), nil
}
