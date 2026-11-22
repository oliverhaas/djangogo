package migrations

import (
	"slices"

	"github.com/oliverhaas/djangogo/orm"
)

// Optimize shortens a list of operations of one app without changing its
// effect, the counterpart of Django's MigrationOptimizer. A CreateModel
// followed by an AddField on the same model becomes one CreateModel, a
// CreateModel followed by its DeleteModel disappears, and so on. Two
// operations are merged across the operations between them only when those
// do not touch the same model or field.
func Optimize(ops []Operation, app string) []Operation {
	for {
		next, changed := optimizeInner(ops, app)
		if !changed {
			return next
		}
		ops = next
	}
}

// optimizeInner makes one pass and applies the first reduction it finds.
func optimizeInner(ops []Operation, app string) ([]Operation, bool) {
	out := make([]Operation, 0, len(ops))
	for i, op := range ops {
		// right tells whether the merged operation may take the place of the
		// later one. It cannot once an operation in between depends on op.
		right := true
		merged := false
	scan:
		for j, other := range ops[i+1:] {
			result, outcome := reduce(op, other, app)
			switch outcome {
			case reduceReplaced:
				between := ops[i+1 : i+j+1]
				switch {
				case right:
					out = append(out, between...)
					out = append(out, result...)
				case allThrough(between, other, app):
					out = append(out, result...)
					out = append(out, between...)
				default:
					break scan
				}
				out = append(out, ops[i+j+2:]...)
				merged = true
				break scan
			case reduceBlocked:
				right = false
			}
		}
		if merged {
			return out, true
		}
		out = append(out, op)
	}
	return out, false
}

// allThrough reports whether other can be moved in front of every operation
// in between.
func allThrough(between []Operation, other Operation, app string) bool {
	for _, op := range between {
		if _, outcome := reduce(op, other, app); outcome != reduceThrough {
			return false
		}
	}
	return true
}

// The outcomes of reduce.
const (
	// reduceBlocked: the two operations cannot be merged or reordered.
	reduceBlocked = iota
	// reduceThrough: the operations are independent and may swap places.
	reduceThrough
	// reduceReplaced: the pair is replaced by the returned operations.
	reduceReplaced
)

// through maps "independent" to an outcome.
func through(independent bool) ([]Operation, int) {
	if independent {
		return nil, reduceThrough
	}
	return nil, reduceBlocked
}

// reduce tries to merge op with the later operation other.
func reduce(op, other Operation, app string) ([]Operation, int) {
	switch o := op.(type) {
	case CreateModel:
		if result, ok := reduceCreateModel(o, other, app); ok {
			return result, reduceReplaced
		}
		return through(!referencesModel(other, o.Name, app))
	case DeleteModel:
		return through(!referencesModel(other, o.Name, app))
	case RenameModel:
		if x, ok := other.(RenameModel); ok && o.NewName == x.OldName {
			return []Operation{RenameModel{OldName: o.OldName, NewName: x.NewName}}, reduceReplaced
		}
		return through(!referencesModel(other, o.NewName, app))
	case AlterModelTable:
		// A later rename of the table, or the deletion of the model, makes
		// this one pointless.
		if x, ok := other.(AlterModelTable); (ok && x.Name == o.Name) || deletesModel(other, o.Name) {
			return []Operation{other}, reduceReplaced
		}
		return through(!referencesModel(other, o.Name, app))
	case AlterUniqueTogether:
		if x, ok := other.(AlterUniqueTogether); (ok && x.Name == o.Name) || deletesModel(other, o.Name) {
			return []Operation{other}, reduceReplaced
		}
		return through(!referencesModel(other, o.Name, app))
	case AddField:
		return reduceAddField(o, other, app)
	case RemoveField:
		if x, ok := other.(DeleteModel); ok && x.Name == o.Model {
			return []Operation{other}, reduceReplaced
		}
		return through(!referencesField(other, o.Model, o.Field, app))
	case AlterField:
		return reduceAlterField(o, other, app)
	case RenameField:
		if x, ok := other.(RenameField); ok && x.Model == o.Model && o.NewName == x.OldName &&
			o.Column == "" && x.Column == "" {
			return []Operation{RenameField{Model: o.Model, OldName: o.OldName, NewName: x.NewName}}, reduceReplaced
		}
		return through(!referencesField(other, o.Model, o.OldName, app) &&
			!referencesField(other, o.Model, o.NewName, app))
	case AddIndex:
		if x, ok := other.(RemoveIndex); ok && x.Model == o.Model && x.Name == o.Index.Name {
			return nil, reduceReplaced
		}
	case AddConstraint:
		switch x := other.(type) {
		case RemoveConstraint:
			if x.Model == o.Model && x.Name == constraintName(o.Constraint) {
				return nil, reduceReplaced
			}
		case AlterConstraint:
			if x.Model == o.Model && x.Name == constraintName(o.Constraint) {
				return []Operation{AddConstraint{Model: o.Model, Constraint: x.Constraint}}, reduceReplaced
			}
		}
	case AlterConstraint:
		switch x := other.(type) {
		case AlterConstraint:
			if x.Model == o.Model && x.Name == o.Name {
				return []Operation{other}, reduceReplaced
			}
		case RemoveConstraint:
			if x.Model == o.Model && x.Name == o.Name {
				return []Operation{other}, reduceReplaced
			}
		}
	}
	return nil, reduceBlocked
}

// deletesModel reports whether op deletes the model name.
func deletesModel(op Operation, name string) bool {
	x, ok := op.(DeleteModel)
	return ok && x.Name == name
}

// reduceCreateModel folds a later operation on the same model into its
// CreateModel.
func reduceCreateModel(o CreateModel, other Operation, app string) ([]Operation, bool) {
	one := func(c CreateModel) ([]Operation, bool) { return []Operation{c}, true }
	switch x := other.(type) {
	case DeleteModel:
		if x.Name == o.Name {
			return nil, true
		}
	case RenameModel:
		if x.OldName == o.Name {
			c := o
			c.Name = x.NewName
			if c.Table == defaultTable(app, o.Name) {
				c.Table = ""
			}
			return one(c)
		}
	case AlterModelTable:
		if x.Name == o.Name {
			c := o
			c.Table = x.Table
			return one(c)
		}
	case AlterUniqueTogether:
		if x.Name == o.Name {
			c := o
			c.UniqueTogether = cloneFieldSets(x.UniqueTogether)
			return one(c)
		}
	case AddField:
		if x.Model == o.Name {
			c := o
			c.Fields = append(slices.Clone(o.Fields), x.Field)
			return one(c)
		}
	case AlterField:
		if x.Model == o.Name {
			c := o
			c.Fields = slices.Clone(o.Fields)
			for i := range c.Fields {
				if c.Fields[i].Name == x.Field.Name {
					c.Fields[i] = x.Field
				}
			}
			return one(c)
		}
	case RemoveField:
		if x.Model == o.Name {
			c := o
			c.Fields = slices.DeleteFunc(slices.Clone(o.Fields), func(f FieldState) bool { return f.Name == x.Field })
			c.UniqueTogether = nil
			for _, set := range o.UniqueTogether {
				rest := slices.DeleteFunc(slices.Clone(set), func(n string) bool { return n == x.Field })
				if len(rest) > 0 {
					c.UniqueTogether = append(c.UniqueTogether, rest)
				}
			}
			return one(c)
		}
	case RenameField:
		if x.Model == o.Name {
			c := o
			c.Fields = slices.Clone(o.Fields)
			for i := range c.Fields {
				if c.Fields[i].Name == x.OldName {
					c.Fields[i] = renamedField(c.Fields[i], x)
				}
			}
			c.UniqueTogether = cloneFieldSets(o.UniqueTogether)
			for _, set := range c.UniqueTogether {
				renameIn(set, x.OldName, x.NewName)
			}
			c.Indexes = cloneIndexes(o.Indexes)
			for i := range c.Indexes {
				renameIn(c.Indexes[i].Fields, x.OldName, x.NewName)
			}
			return one(c)
		}
	case AddIndex:
		if x.Model == o.Name {
			c := o
			c.Indexes = append(cloneIndexes(o.Indexes), x.Index)
			return one(c)
		}
	case RemoveIndex:
		if x.Model == o.Name {
			c := o
			c.Indexes = slices.DeleteFunc(cloneIndexes(o.Indexes), func(idx Index) bool { return idx.Name == x.Name })
			return one(c)
		}
	case AddConstraint:
		if x.Model == o.Name {
			c := o
			c.Constraints = append(slices.Clone(o.Constraints), x.Constraint)
			return one(c)
		}
	case AlterConstraint:
		if x.Model == o.Name {
			c := o
			c.Constraints = append(withoutConstraint(o.Constraints, x.Name), x.Constraint)
			return one(c)
		}
	case RemoveConstraint:
		if x.Model == o.Name {
			c := o
			c.Constraints = withoutConstraint(o.Constraints, x.Name)
			return one(c)
		}
	}
	return nil, false
}

// withoutConstraint returns a copy of constraints without the one named name.
func withoutConstraint(constraints []orm.Constraint, name string) []orm.Constraint {
	out := slices.DeleteFunc(slices.Clone(constraints), func(c orm.Constraint) bool { return c.ConstraintName() == name })
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneIndexes deep-copies a list of indexes.
func cloneIndexes(indexes []Index) []Index {
	if len(indexes) == 0 {
		return nil
	}
	out := make([]Index, len(indexes))
	for i, idx := range indexes {
		out[i] = Index{Name: idx.Name, Fields: slices.Clone(idx.Fields)}
	}
	return out
}

// renamedField returns f as it is after the rename r, with the column rule of
// RenameField.StateForwards.
func renamedField(f FieldState, r RenameField) FieldState {
	switch {
	case r.Column != "":
		f.Column = r.Column
	case f.Column == defaultColumn(r.OldName, f.RelKind):
		f.Column = defaultColumn(r.NewName, f.RelKind)
	}
	f.Name = r.NewName
	return f
}

// reduceAddField merges an AddField with a later operation on the same field.
func reduceAddField(o AddField, other Operation, app string) ([]Operation, int) {
	switch x := other.(type) {
	case AlterField:
		if x.Model == o.Model && x.Field.Name == o.Field.Name {
			oneOff := o.Default
			if x.Default != nil {
				oneOff = x.Default
			}
			return []Operation{AddField{Model: o.Model, Field: x.Field, Default: oneOff}}, reduceReplaced
		}
	case RemoveField:
		if x.Model == o.Model && x.Field == o.Field.Name {
			return nil, reduceReplaced
		}
	case RenameField:
		if x.Model == o.Model && x.OldName == o.Field.Name {
			return []Operation{AddField{Model: o.Model, Field: renamedField(o.Field, x), Default: o.Default}}, reduceReplaced
		}
	}
	return through(!referencesField(other, o.Model, o.Field.Name, app))
}

// reduceAlterField merges an AlterField with a later operation on the same field.
func reduceAlterField(o AlterField, other Operation, app string) ([]Operation, int) {
	switch x := other.(type) {
	case AlterField:
		if x.Model == o.Model && x.Field.Name == o.Field.Name {
			return []Operation{other}, reduceReplaced
		}
	case RemoveField:
		if x.Model == o.Model && x.Field == o.Field.Name {
			return []Operation{other}, reduceReplaced
		}
	case RenameField:
		// With an explicit column the alteration cannot simply move behind
		// the rename.
		if x.Model == o.Model && x.OldName == o.Field.Name && x.Column == "" &&
			o.Field.Column == defaultColumn(o.Field.Name, o.Field.RelKind) {
			return []Operation{other, AlterField{Model: o.Model, Field: renamedField(o.Field, x), Default: o.Default}}, reduceReplaced
		}
	}
	return through(!referencesField(other, o.Model, o.Field.Name, app))
}

// referencesModel reports whether op touches the model name of app. An
// operation of unknown effect counts as touching every model.
func referencesModel(op Operation, name, app string) bool {
	label := ModelLabel(app, name)
	pointsTo := func(f FieldState) bool { return f.IsRelation() && (f.RelTo == label || f.RelThrough == label) }
	switch o := op.(type) {
	case CreateModel:
		return o.Name == name || slices.ContainsFunc(o.Fields, pointsTo)
	case DeleteModel:
		// The deleted model may point to the model through a relation field,
		// which the operation does not carry.
		return true
	case RenameModel:
		return o.OldName == name || o.NewName == name
	case AlterModelTable:
		return o.Name == name
	case AlterUniqueTogether:
		return o.Name == name
	case AddField:
		return o.Model == name || pointsTo(o.Field)
	case AlterField:
		return o.Model == name || pointsTo(o.Field)
	case RemoveField:
		return o.Model == name
	case RenameField:
		return o.Model == name
	}
	return true
}

// referencesField reports whether op touches the field of the model in app. A
// foreign key to the model counts as a reference to each of its fields,
// because the referenced column is not known here.
func referencesField(op Operation, model, field, app string) bool {
	label := ModelLabel(app, model)
	pointsTo := func(f FieldState) bool { return f.IsRelation() && (f.RelTo == label || f.RelThrough == label) }
	switch o := op.(type) {
	case AddField:
		return (o.Model == model && o.Field.Name == field) || pointsTo(o.Field)
	case AlterField:
		return (o.Model == model && o.Field.Name == field) || pointsTo(o.Field)
	case RemoveField:
		return o.Model == model && o.Field == field
	case RenameField:
		return o.Model == model && (o.OldName == field || o.NewName == field)
	case AlterUniqueTogether:
		if o.Name != model {
			return false
		}
		return len(o.UniqueTogether) == 0 ||
			slices.ContainsFunc(o.UniqueTogether, func(set []string) bool { return slices.Contains(set, field) })
	}
	return referencesModel(op, model, app)
}
