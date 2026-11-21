package orm

import (
	"sort"
	"strings"
)

// resolveSelectRelated resolves the SelectRelated names of the query into the
// relations to load, parents before the relations below them. It follows
// forward FK, forward O2O, and reverse O2O relations. The joins are shared
// with the filters and the ordering: a relation that a filter already joins is
// not joined again.
func (c *compiler) resolveSelectRelated() ([]relPlan, error) {
	var plans []relPlan
	for _, name := range c.q.selectRelated {
		model := c.q.model
		parent := -1
		var hops []hop
		for _, part := range strings.Split(name, lookupSep) {
			f, rev, rel := selectRelatedStep(model, part)
			if rel == nil {
				// Django names the part that failed, and tells a field that
				// is not a relation from a name that is no field at all.
				problem := "invalid field name(s)"
				if f != nil {
					problem = "non-relational field"
				}
				return nil, &FieldError{
					Model: c.q.model.Name(),
					Message: problem + " given in select_related: '" + part +
						"'. Choices are: " + relationChoices(model),
				}
			}
			hops = append(hops, rel.hops...)
			joins := c.joins.setup(hops, nil)
			j := joins[len(joins)-1]
			idx := -1
			for i, p := range plans {
				if p.join == j {
					idx = i
					break
				}
			}
			if idx < 0 {
				plans = append(plans, relPlan{field: f, rev: rev, join: j, parent: parent})
				idx = len(plans) - 1
			}
			parent = idx
			model = rel.target
		}
	}
	return plans, nil
}

// selectRelatedStep finds the forward FK/O2O or reverse O2O relation of m
// named by name. A reverse FK is multi-valued and is rejected (returning
// nil, nil, nil). When name addresses a plain non-relational field of m, f is
// returned with a nil path so the caller distinguishes non-relational from
// invalid field names.
func selectRelatedStep(m *Model, name string) (*Field, *ReverseRelation, *relationPath) {
	f, rel, ok := lookupName(m, name)
	if !ok {
		if byName, found := m.byName[name]; found {
			f, rel, ok = lookupName(m, byName.Attr)
		}
	}
	if ok {
		if rel != nil && rel.local != nil {
			return f, nil, rel
		}
		if rel != nil && rel.reverse != nil {
			if rel.reverse.Kind == RelO2O && !rel.reverse.Hidden {
				return nil, rel.reverse, rel
			}
			return nil, nil, nil
		}
		return f, nil, nil
	}
	// Also accept reverse O2O accessor Name when it differs from QueryName.
	if rev, found, _ := m.lookupReverseAccessor(name); found && rev.Kind == RelO2O && !rev.Hidden &&
		m.PrimaryKey() != nil && rev.Source.PrimaryKey() != nil {
		return nil, rev, &relationPath{
			hops: []hop{{
				fromColumn: m.PrimaryKey().Column,
				target:     rev.Source,
				toColumn:   rev.Field.Column,
				nullable:   true,
			}},
			target:  rev.Source,
			reverse: rev,
		}
	}
	return nil, nil, nil
}

// relationChoices lists the forward FK/O2O fields and reverse O2O relations of
// m for a select_related error message, or "(none)" as Django writes it.
func relationChoices(m *Model) string {
	seen := make(map[string]bool)
	for _, f := range m.fields {
		if f.Rel != nil && isForwardToOne(f.Rel.Kind) {
			seen[f.Attr] = true
		}
	}
	for _, rev := range m.reverseRels {
		if rev.Kind == RelO2O && !rev.Hidden {
			seen[rev.Name] = true
		}
	}
	if len(seen) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// rowsStmt returns the SELECT of model rows for the query and the plan to
// scan its result. The annotations are selected after the model's columns.
func (q *query) rowsStmt() (*selectStmt, *rowPlan, error) {
	c, err := q.compile()
	if err != nil {
		return nil, nil, err
	}
	plan := &rowPlan{dialect: c.d, model: q.model}
	items := make([]selectItem, 0, len(q.model.fields))
	for _, f := range q.model.fields {
		items = append(items, selectItem{node: c.baseColumn(f)})
	}
	plan.rels, err = c.resolveSelectRelated()
	if err != nil {
		return nil, nil, err
	}
	for _, r := range plan.rels {
		for _, f := range r.join.model.fields {
			items = append(items, selectItem{node: colNode{join: r.join, field: f}})
		}
	}
	for _, name := range c.annotNames {
		n := c.annots[name]
		items = append(items, selectItem{node: n, alias: name})
		plan.annots = append(plan.annots, name)
		plan.annotTypes = append(plan.annotTypes, n.vtype())
	}
	stmt, err := c.build(items, true)
	if err != nil {
		return nil, nil, err
	}
	if c.empty {
		return nil, nil, errEmptyResultSet
	}
	plan.ncols = len(stmt.items)
	return stmt, plan, nil
}

// selectSQL returns the SELECT of model rows as text and arguments.
func (q *query) selectSQL() (string, []any, *rowPlan, error) {
	stmt, plan, err := q.rowsStmt()
	if err != nil {
		return "", nil, nil, err
	}
	w := newSQLWriter(q.db.Dialect())
	stmt.render(w)
	return w.String(), w.args, plan, nil
}
