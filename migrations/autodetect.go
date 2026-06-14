package migrations

// Diff computes the ordered operations that transform the old project state into
// the new one. The order is deterministic: model creations first, then per-model
// field changes, then model deletions.
func Diff(oldState, newState *ProjectState) []Operation {
	var ops []Operation

	// 1. Created models: names in newState.Order not present in oldState.Models.
	for _, name := range newState.Order {
		if _, exists := oldState.Models[name]; !exists {
			nm := newState.Models[name]
			fields := make([]FieldState, len(nm.Fields))
			copy(fields, nm.Fields)
			ops = append(ops, CreateModel{
				Name:   name,
				Table:  nm.Table,
				Fields: fields,
			})
		}
	}

	// 2. Field changes on models present in both states.
	for _, name := range newState.Order {
		om, inOld := oldState.Models[name]
		if !inOld {
			continue
		}
		nm := newState.Models[name]

		// AddField: fields in new not present in old.
		for _, f := range nm.Fields {
			if _, ok := om.FieldByName(f.Name); !ok {
				ops = append(ops, AddField{Model: name, Field: f})
			}
		}

		// AlterField: fields present in both but not equal.
		for _, nf := range nm.Fields {
			of, ok := om.FieldByName(nf.Name)
			if !ok {
				continue
			}
			if !of.Equal(nf) {
				ops = append(ops, AlterField{Model: name, Field: nf})
			}
		}

		// RemoveField: fields in old not present in new.
		for _, f := range om.Fields {
			if _, ok := nm.FieldByName(f.Name); !ok {
				ops = append(ops, RemoveField{Model: name, Field: f.Name})
			}
		}
	}

	// 3. Deleted models: names in oldState.Order not present in newState.Models.
	for _, name := range oldState.Order {
		if _, exists := newState.Models[name]; !exists {
			ops = append(ops, DeleteModel{Name: name})
		}
	}

	if ops == nil {
		return []Operation{}
	}
	return ops
}

// PotentialRename flags a removed field and an added field on the same model
// whose column types are identical -- the signature of a renamed field. The
// autodetector emits such a pair as RemoveField + AddField, which DROPs the old
// column and ADDs a new empty one, discarding the column's data on apply. Django
// resolves the same ambiguity by prompting at makemigrations time; Djan-Go-Go
// has no interactive prompt, so it surfaces the pair as a warning instead.
type PotentialRename struct {
	Model string // model (Go struct) name
	From  string // removed field name; its column is dropped
	To    string // added field name; a new empty column
}

// String renders the rename as "Model.From -> Model.To".
func (r PotentialRename) String() string {
	return r.Model + "." + r.From + " -> " + r.Model + "." + r.To
}

// DetectPotentialRenames reports field pairs that look like renames: within a
// model present in both states, a removed field whose type matches an added one.
// Each removed and added field is paired at most once, scanning in field order,
// so a model that renames several same-typed columns yields one pair per column.
// The result is advisory; Diff still emits the underlying RemoveField/AddField,
// so callers should surface these as warnings rather than silently dropping data.
func DetectPotentialRenames(oldState, newState *ProjectState) []PotentialRename {
	var out []PotentialRename
	for _, name := range newState.Order {
		om, inOld := oldState.Models[name]
		if !inOld {
			continue // brand-new model: every field is a genuine addition.
		}
		nm := newState.Models[name]

		// Added fields: present in new, absent from old.
		var added []FieldState
		for _, nf := range nm.Fields {
			if _, ok := om.FieldByName(nf.Name); !ok {
				added = append(added, nf)
			}
		}

		// Pair each removed field with the first unclaimed added field of the
		// same type.
		claimed := make([]bool, len(added))
		for _, of := range om.Fields {
			if _, ok := nm.FieldByName(of.Name); ok {
				continue // still present: not removed.
			}
			for i := range added {
				if claimed[i] || !of.sameType(added[i]) {
					continue
				}
				claimed[i] = true
				out = append(out, PotentialRename{Model: name, From: of.Name, To: added[i].Name})
				break
			}
		}
	}
	return out
}
