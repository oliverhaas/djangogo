package migrations

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// fieldRename is a confirmed field rename.
type fieldRename struct {
	key      fieldKey
	oldName  string
	oldField FieldState
	newField FieldState
}

// indexChange lists the indexes a kept model gains and loses.
type indexChange struct {
	key     modelKey
	added   []Index
	removed []Index
}

// explicitTable returns the table of ms when it is not the default one, and
// "" otherwise. Operations name a table only when the model sets one.
func explicitTable(ms *ModelState) string {
	if ms.Table == defaultTable(ms.App, ms.Name) {
		return ""
	}
	return ms.Table
}

// oldModel returns the from-state model that the kept model key stands for.
func (a *Autodetector) oldModel(key modelKey) *ModelState {
	name := key.name
	if old, ok := a.renamedModels[key]; ok {
		name = old
	}
	return a.from.Models[ModelLabel(key.app, name)]
}

// foreignKeyDeps returns the dependency of a relation field: the model it
// points to exists.
func foreignKeyDeps(f FieldState) []opDep {
	app, name := splitLabel(f.RelTo)
	return []opDep{{app: app, model: name, kind: depCreate}}
}

// modelDeps returns the foreign-key dependencies of all fields of ms.
func modelDeps(ms *ModelState) []opDep {
	var deps []opDep
	for _, f := range ms.Fields {
		if f.IsRelation() {
			deps = append(deps, foreignKeyDeps(f)...)
		}
	}
	return deps
}

// relationAgnostic returns the definitions of the fields in the order of their
// names, without the names, the columns that follow from the names and the
// targets of the relations. Two models with the same result can be one model
// under two names, also when fields were renamed along with it.
func relationAgnostic(fields []FieldState) []FieldState {
	out := slices.Clone(fields)
	slices.SortFunc(out, func(x, y FieldState) int { return cmp.Compare(x.Name, y.Name) })
	for i := range out {
		if out[i].Column == defaultColumn(out[i].Name, out[i].RelKind) {
			out[i].Column = ""
		}
		out[i].Name = ""
		out[i].RelTo = ""
	}
	return out
}

// generateRenamedModels finds an added and a removed model of one app with
// the same fields, asks whether it is a rename, and generates RenameModel. It
// runs first: everything else treats a renamed model as kept.
func (a *Autodetector) generateRenamedModels() error {
	a.renamedModels = map[modelKey]string{}
	a.renamedModelsRel = map[string]string{}
	for _, key := range sortedDiff(a.newModels, a.oldModels, compareModelKeys) {
		ms := a.to.Models[key.label()]
		fields := relationAgnostic(ms.Fields)
		for _, rem := range sortedDiff(a.oldModels, a.newModels, compareModelKeys) {
			if rem.app != key.app {
				continue
			}
			remMS := a.from.Models[rem.label()]
			if !slices.Equal(fields, relationAgnostic(remMS.Fields)) {
				continue
			}
			yes, err := a.questioner.AskRenameModel(remMS, ms)
			if err != nil {
				return err
			}
			if !yes {
				continue
			}
			// The rename repoints the foreign keys to the model, so it waits
			// for the models on both ends of its relations.
			deps := modelDeps(ms)
			for _, ref := range a.to.referencing(key.label()) {
				deps = append(deps, opDep{app: ref.model.App, model: ref.model.Name, kind: depCreate})
			}
			a.addOperation(key.app, RenameModel{OldName: rem.name, NewName: key.name}, deps, false)
			a.renamedModels[key] = rem.name
			a.renamedModelsRel[rem.label()] = key.label()
			delete(a.oldModels, rem)
			a.oldModels[key] = true
			break
		}
	}
	return nil
}

// prepareFieldLists collects the fields of the kept models on both sides. The
// old fields are filed under the new model name.
func (a *Autodetector) prepareFieldLists() {
	a.keptModels = sortedCommon(a.oldModels, a.newModels, compareModelKeys)
	a.oldFields = map[fieldKey]bool{}
	a.newFields = map[fieldKey]bool{}
	for _, key := range a.keptModels {
		for _, f := range a.oldModel(key).Fields {
			a.oldFields[fieldKey{key.app, key.name, f.Name}] = true
		}
		for _, f := range a.to.Models[key.label()].Fields {
			a.newFields[fieldKey{key.app, key.name, f.Name}] = true
		}
	}
}

// relationFields returns the non-primary-key relation fields of ms, sorted by
// their snake_case name.
func relationFields(ms *ModelState) []FieldState {
	var out []FieldState
	for _, f := range ms.Fields {
		if f.IsRelation() && !f.PrimaryKey {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(x, y FieldState) int { return cmp.Compare(x.Attr(), y.Attr()) })
	return out
}

// generateDeletedModels generates, for every removed model, the removal of
// its unique-together sets, indexes and relation fields, and then DeleteModel.
// The optimizer folds them back into one DeleteModel where nothing is in
// between.
func (a *Autodetector) generateDeletedModels() {
	for _, key := range sortedDiff(a.oldModels, a.newModels, compareModelKeys) {
		ms := a.from.Models[key.label()]
		related := relationFields(ms)
		if len(ms.UniqueTogether) > 0 {
			a.addOperation(key.app, AlterUniqueTogether{Name: key.name}, nil, false)
		}
		for _, idx := range ms.Indexes {
			a.addOperation(key.app, RemoveIndex{Model: key.name, Name: idx.Name}, nil, false)
		}
		for _, f := range related {
			a.addOperation(key.app, RemoveField{Model: key.name, Field: f.Name},
				[]opDep{{app: key.app, model: key.name, field: f.Name, kind: depRemoveIndex}}, false)
		}
		// The model goes after the foreign keys that point to it are removed
		// or repointed, and after its own relation fields.
		refs := a.from.referencing(key.label())
		deps := make([]opDep, 0, 2*len(refs)+len(related))
		for _, ref := range refs {
			deps = append(deps,
				opDep{app: ref.model.App, model: ref.model.Name, field: ref.field.Name, kind: depRemove},
				opDep{app: ref.model.App, model: ref.model.Name, field: ref.field.Name, kind: depAlter})
		}
		for _, f := range related {
			deps = append(deps, opDep{app: key.app, model: key.name, field: f.Name, kind: depRemove})
		}
		a.addOperation(key.app, DeleteModel{Name: key.name}, deps, false)
	}
}

// generateCreatedModels generates, for every new model, a CreateModel
// without its relation fields and one AddField per relation, so the models
// of a cycle can be created first and linked afterwards. Indexes and
// unique-together sets come last, because they may cover relation fields.
// The optimizer folds everything back into the CreateModel where it can.
func (a *Autodetector) generateCreatedModels() {
	added := sortedDiff(a.newModels, a.oldModels, compareModelKeys)
	slices.Reverse(added)
	for _, key := range added {
		ms := a.to.Models[key.label()]
		related := relationFields(ms)
		var plain []FieldState
		for _, f := range ms.Fields {
			if !f.IsRelation() || f.PrimaryKey {
				plain = append(plain, f)
			}
		}
		a.addOperation(key.app,
			CreateModel{Name: key.name, Table: explicitTable(ms), Fields: plain},
			[]opDep{{app: key.app, model: key.name, kind: depRemove}}, true)

		created := opDep{app: key.app, model: key.name, kind: depCreate}
		for _, f := range related {
			a.addOperation(key.app, AddField{Model: key.name, Field: f}, append(foreignKeyDeps(f), created), false)
		}
		relatedDeps := make([]opDep, 0, len(related)+1)
		for _, f := range related {
			relatedDeps = append(relatedDeps, opDep{app: key.app, model: key.name, field: f.Name, kind: depCreate})
		}
		relatedDeps = append(relatedDeps, created)
		for _, idx := range ms.Indexes {
			a.addOperation(key.app, AddIndex{Model: key.name, Index: idx}, relatedDeps, false)
		}
		if len(ms.UniqueTogether) > 0 {
			a.addOperation(key.app,
				AlterUniqueTogether{Name: key.name, UniqueTogether: cloneFieldSets(ms.UniqueTogether)}, relatedDeps, false)
		}
	}
}

// looksRenamed reports whether newField can be oldField under a new name:
// the same definition, and a column that either follows the name or stays.
func looksRenamed(oldField, newField FieldState) bool {
	if !oldField.sameDefinition(newField) {
		return false
	}
	if oldField.Column == newField.Column {
		return true
	}
	return oldField.Column == defaultColumn(oldField.Name, oldField.RelKind) &&
		newField.Column == defaultColumn(newField.Name, newField.RelKind)
}

// createRenamedFields pairs every added field with a removed field of the
// same model that has the same definition, and asks whether it is a rename.
func (a *Autodetector) createRenamedFields() error {
	oldFields := maps.Clone(a.oldFields)
	for _, key := range sortedDiff(a.newFields, oldFields, compareFieldKeys) {
		mk := modelKey{key.app, key.model}
		oldMS := a.oldModel(mk)
		newMS := a.to.Models[mk.label()]
		field, _ := newMS.FieldByName(key.field)
		for _, rem := range sortedDiff(oldFields, a.newFields, compareFieldKeys) {
			if rem.app != key.app || rem.model != key.model {
				continue
			}
			old, _ := oldMS.FieldByName(rem.field)
			oldField := *old
			// A relation to a renamed model still names the old model.
			if to, ok := a.renamedModelsRel[oldField.RelTo]; ok && oldField.IsRelation() {
				oldField.RelTo = to
			}
			if !looksRenamed(oldField, *field) {
				continue
			}
			yes, err := a.questioner.AskRename(newMS, rem.field, key.field, *field)
			if err != nil {
				return err
			}
			if !yes {
				continue
			}
			a.renames = append(a.renames, fieldRename{key: key, oldName: rem.field, oldField: oldField, newField: *field})
			delete(oldFields, rem)
			oldFields[key] = true
			a.renamedFields[key] = rem.field
			break
		}
	}
	return nil
}

// generateRenamedFields generates RenameField for the confirmed renames. The
// operation names the column only when the default rule would not yield it.
func (a *Autodetector) generateRenamedFields() {
	for _, r := range a.renames {
		op := RenameField{Model: r.key.model, OldName: r.oldName, NewName: r.key.field}
		if renamedField(r.oldField, op).Column != r.newField.Column {
			op.Column = r.newField.Column
		}
		a.addOperation(r.key.app, op, nil, false)
		delete(a.oldFields, fieldKey{r.key.app, r.key.model, r.oldName})
		a.oldFields[r.key] = true
	}
}

// newFieldName returns the current name of an old field of a kept model.
func (a *Autodetector) newFieldName(key modelKey, oldName string) string {
	for fk, old := range a.renamedFields {
		if fk.app == key.app && fk.model == key.name && old == oldName {
			return fk.field
		}
	}
	return oldName
}

// renameFields maps a list of old field names to their current names,
// preserving a leading "-" on descending index fields.
func (a *Autodetector) renameFields(key modelKey, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		if bare, desc := strings.CutPrefix(n, "-"); desc {
			out[i] = "-" + a.newFieldName(key, bare)
			continue
		}
		out[i] = a.newFieldName(key, n)
	}
	return out
}

// createAlteredIndexes compares the model-level indexes of the kept models. An
// index over a renamed field is the same index.
func (a *Autodetector) createAlteredIndexes() {
	same := func(x, y Index) bool { return x.Name == y.Name && slices.Equal(x.Fields, y.Fields) }
	for _, key := range a.keptModels {
		newIndexes := a.to.Models[key.label()].Indexes
		declared := a.oldModel(key).Indexes
		oldIndexes := make([]Index, 0, len(declared))
		for _, idx := range declared {
			oldIndexes = append(oldIndexes, Index{Name: idx.Name, Fields: a.renameFields(key, idx.Fields)})
		}
		change := indexChange{key: key}
		for _, idx := range newIndexes {
			if !slices.ContainsFunc(oldIndexes, func(o Index) bool { return same(o, idx) }) {
				change.added = append(change.added, idx)
			}
		}
		for _, idx := range oldIndexes {
			if !slices.ContainsFunc(newIndexes, func(n Index) bool { return same(n, idx) }) {
				change.removed = append(change.removed, idx)
			}
		}
		if len(change.added) > 0 || len(change.removed) > 0 {
			a.indexChanges = append(a.indexChanges, change)
		}
	}
}

// generateRemovedIndexes generates RemoveIndex. It runs before the fields
// are removed.
func (a *Autodetector) generateRemovedIndexes() {
	for _, change := range a.indexChanges {
		for _, idx := range change.removed {
			a.addOperation(change.key.app, RemoveIndex{Model: change.key.name, Name: idx.Name}, nil, false)
		}
	}
}

// generateAddedIndexes generates AddIndex. It runs after the fields are added.
func (a *Autodetector) generateAddedIndexes() {
	for _, change := range a.indexChanges {
		deps := modelDeps(a.to.Models[change.key.label()])
		for _, idx := range change.added {
			a.addOperation(change.key.app, AddIndex{Model: change.key.name, Index: idx}, deps, false)
		}
	}
}

// setKey returns a comparable form of a unique-together set.
func setKey(fields []string) string { return strings.Join(fields, "\x00") }

// generateAlteredUniqueTogether generates AlterUniqueTogether for the kept
// models whose sets changed. Django does it in two steps, and so does this:
// with removalOnly, before the fields change, the sets are reduced to those
// that stay; afterwards they are set to the new value. The optimizer merges
// the two when nothing is in between.
func (a *Autodetector) generateAlteredUniqueTogether(removalOnly bool) {
	for _, key := range a.keptModels {
		newMS := a.to.Models[key.label()]
		oldSets := map[string]bool{}
		for _, set := range a.oldModel(key).UniqueTogether {
			oldSets[setKey(a.renameFields(key, set))] = true
		}
		newSets := map[string]bool{}
		var kept [][]string
		for _, set := range newMS.UniqueTogether {
			newSets[setKey(set)] = true
			if oldSets[setKey(set)] {
				kept = append(kept, slices.Clone(set))
			}
		}
		if maps.Equal(oldSets, newSets) {
			continue
		}
		var deps []opDep
		for _, set := range newMS.UniqueTogether {
			for _, name := range set {
				if f, ok := newMS.FieldByName(name); ok && f.IsRelation() {
					deps = append(deps, foreignKeyDeps(*f)...)
				}
			}
		}
		switch {
		case removalOnly && len(oldSets) > 0:
			a.addOperation(key.app, AlterUniqueTogether{Name: key.name, UniqueTogether: kept}, deps, false)
		case !removalOnly && len(kept) != len(newSets):
			a.addOperation(key.app,
				AlterUniqueTogether{Name: key.name, UniqueTogether: cloneFieldSets(newMS.UniqueTogether)}, deps, false)
		}
	}
}

// generateRemovedFields generates RemoveField. A field goes after the
// unique-together sets and indexes that may cover it.
func (a *Autodetector) generateRemovedFields() {
	for _, key := range sortedDiff(a.oldFields, a.newFields, compareFieldKeys) {
		a.addOperation(key.app, RemoveField{Model: key.model, Field: key.field}, []opDep{
			{app: key.app, model: key.model, field: key.field, kind: depAlterTogether},
			{app: key.app, model: key.model, field: key.field, kind: depRemoveIndex},
		}, false)
	}
}

// fillsExistingRows reports whether existing rows get a value for the new
// column f without a one-off default.
func fillsExistingRows(f FieldState) bool {
	return f.Null || f.PrimaryKey || f.Kind == orm.KindAuto || effectiveDefault(f, nil) != nil
}

// generateAddedFields generates AddField. A NOT NULL field that has nothing
// to fill existing rows with needs a one-off default from the questioner.
func (a *Autodetector) generateAddedFields() error {
	for _, key := range sortedDiff(a.newFields, a.oldFields, compareFieldKeys) {
		ms := a.to.Models[ModelLabel(key.app, key.model)]
		field, _ := ms.FieldByName(key.field)
		deps := []opDep{{app: key.app, model: key.model, field: key.field, kind: depRemove}}
		if field.IsRelation() {
			deps = append(deps, foreignKeyDeps(*field)...)
		}
		op := AddField{Model: key.model, Field: *field}
		if !fillsExistingRows(*field) {
			oneOff, err := a.questioner.AskNotNullAddition(ms, *field)
			if err != nil {
				return err
			}
			op.Default = oneOff
		}
		a.addOperation(key.app, op, deps, false)
	}
	return nil
}

// generateAlteredFields generates AlterField for the fields whose definition
// changed. A renamed field is left alone: RenameField covers its name and
// column, and nothing else may differ for the rename to be offered.
func (a *Autodetector) generateAlteredFields() error {
	for _, key := range sortedCommon(a.oldFields, a.newFields, compareFieldKeys) {
		mk := modelKey{key.app, key.model}
		if _, renamed := a.renamedFields[key]; renamed {
			continue
		}
		old, _ := a.oldModel(mk).FieldByName(key.field)
		newMS := a.to.Models[mk.label()]
		field, _ := newMS.FieldByName(key.field)

		// RenameModel repoints the relations to a renamed model, so a target
		// that changed only through a rename is no alteration.
		compared := *field
		var deps []opDep
		if field.IsRelation() {
			tApp, tName := splitLabel(field.RelTo)
			if _, ok := a.renamedModels[modelKey{tApp, tName}]; ok {
				compared.RelTo = old.RelTo
			}
			deps = foreignKeyDeps(*field)
		}
		if *old == compared {
			continue
		}
		op := AlterField{Model: key.model, Field: *field}
		if old.Null && !field.Null && effectiveDefault(*field, nil) == nil {
			oneOff, err := a.questioner.AskNotNullAlteration(newMS, *field)
			if err != nil {
				return err
			}
			op.Default = oneOff
		}
		a.addOperation(key.app, op, deps, false)
	}
	return nil
}

// generateAlteredTables generates AlterModelTable for the kept models whose
// explicit table name changed.
func (a *Autodetector) generateAlteredTables() {
	for _, key := range a.keptModels {
		oldTable := explicitTable(a.oldModel(key))
		newTable := explicitTable(a.to.Models[key.label()])
		if oldTable != newTable {
			a.addOperation(key.app, AlterModelTable{Name: key.name, Table: newTable}, nil, false)
		}
	}
}
