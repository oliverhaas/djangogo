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
