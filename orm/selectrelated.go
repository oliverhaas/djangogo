package orm

import (
	"sort"
	"strings"
)

// resolveSelectRelated resolves the SelectRelated names of the query into the
// relations to load, parents before the relations below them. The joins are
// shared with the filters and the ordering: a relation that a filter already
// joins is not joined again.
func (c *compiler) resolveSelectRelated() ([]relPlan, error) {
	var plans []relPlan
	for _, name := range c.q.selectRelated {
		model := c.q.model
		parent := -1
		var hops []hop
		for _, part := range strings.Split(name, lookupSep) {
			f, rel := forwardRelation(model, part)
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
			joins := c.joins.setup(hops)
			j := joins[len(joins)-1]
			idx := -1
			for i, p := range plans {
				if p.join == j {
					idx = i
					break
				}
			}
			if idx < 0 {
				plans = append(plans, relPlan{field: f, join: j, parent: parent})
				idx = len(plans) - 1
			}
			parent = idx
			model = rel.target
		}
	}
	return plans, nil
}

// forwardRelation finds the forward foreign key of m named by name: a key as
// in a filter, or the Go field name. It returns a nil path when name is not a
// relation whose object can be stored on a field of m; the field is then the
// field that name addresses, or nil when it addresses none.
func forwardRelation(m *Model, name string) (*Field, *relationPath) {
	f, rel, ok := lookupName(m, name)
	if !ok {
		byName, found := m.byName[name]
		if !found {
			return nil, nil
		}
		f, rel, ok = lookupName(m, byName.Attr)
	}
	if !ok {
		return nil, nil
	}
	if rel == nil || rel.local == nil {
		return f, nil
	}
	return f, rel
}

// relationChoices lists the forward foreign keys of m for an error message,
// or "(none)" as Django writes it.
func relationChoices(m *Model) string {
	var names []string
	for _, f := range m.fields {
		if f.Rel != nil {
			names = append(names, f.Attr)
		}
	}
	if len(names) == 0 {
		return "(none)"
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
