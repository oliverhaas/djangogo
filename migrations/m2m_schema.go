package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// fkKindForPK returns the column Kind for an auto-created through table FK
// pointing at primary-key field pk.
func fkKindForPK(pk *FieldState) orm.Kind {
	if pk == nil || pk.Kind == orm.KindAuto {
		return orm.KindInt
	}
	return pk.Kind
}

// autoThroughModelState synthesizes the ModelState for an auto-created M2M
// join table on owner for field f, matching Django's naming rules.
func autoThroughModelState(state *ProjectState, owner *ModelState, f FieldState) (*ModelState, error) {
	target, err := state.relatedTo(f)
	if err != nil {
		return nil, err
	}
	ownerPK := owner.PrimaryKey()
	if ownerPK == nil {
		return nil, fmt.Errorf("migrations: model %s has no primary key for M2M field %s", owner.Label(), f.Name)
	}
	targetPK := target.PrimaryKey()
	if targetPK == nil {
		return nil, fmt.Errorf("migrations: model %s has no primary key for M2M field %s", target.Label(), f.Name)
	}

	table := f.RelDBTable
	if table == "" {
		table = owner.Table + "_" + f.Attr()
	}

	ownerLower := strings.ToLower(owner.Name)
	targetLower := strings.ToLower(target.Name)
	var fromName, fromCol, toName, toCol string
	if ownerLower == targetLower {
		fromName = "From" + owner.Name
		fromCol = "from_" + ownerLower + "_id"
		toName = "To" + target.Name
		toCol = "to_" + targetLower + "_id"
	} else {
		fromName = owner.Name
		fromCol = ownerLower + "_id"
		toName = target.Name
		toCol = targetLower + "_id"
	}

	return &ModelState{
		App:   owner.App,
		Name:  owner.Name + "_" + f.Attr(),
		Table: table,
		Fields: []FieldState{
			{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
			{
				Name:          fromName,
				Column:        fromCol,
				Kind:          fkKindForPK(ownerPK),
				MaxLength:     ownerPK.MaxLength,
				MaxDigits:     ownerPK.MaxDigits,
				DecimalPlaces: ownerPK.DecimalPlaces,
				RelKind:       orm.RelFK,
				RelTo:         owner.Label(),
				RelOnDelete:   orm.OnDeleteCascade,
			},
			{
				Name:          toName,
				Column:        toCol,
				Kind:          fkKindForPK(targetPK),
				MaxLength:     targetPK.MaxLength,
				MaxDigits:     targetPK.MaxDigits,
				DecimalPlaces: targetPK.DecimalPlaces,
				RelKind:       orm.RelFK,
				RelTo:         target.Label(),
				RelOnDelete:   orm.OnDeleteCascade,
			},
		},
		UniqueTogether: [][]string{{fromName, toName}},
	}, nil
}

// createAutoThroughTables creates the auto-created M2M join tables declared on ms.
func (e *SchemaEditor) createAutoThroughTables(ctx context.Context, state *ProjectState, ms *ModelState) error {
	for _, f := range ms.Fields {
		if !f.HasAutoThrough() {
			continue
		}
		th, err := autoThroughModelState(state, ms, f)
		if err != nil {
			return err
		}
		if err := e.createModel(ctx, state, th); err != nil {
			return err
		}
	}
	return nil
}

// dropAutoThroughTables drops every auto-created M2M join table declared on ms.
func (e *SchemaEditor) dropAutoThroughTables(ctx context.Context, state *ProjectState, ms *ModelState) error {
	for _, f := range ms.Fields {
		if !f.HasAutoThrough() {
			continue
		}
		th, err := autoThroughModelState(state, ms, f)
		if err != nil {
			return err
		}
		if err := e.deleteTable(ctx, th.Table); err != nil {
			return err
		}
	}
	return nil
}

// alterM2MField applies database changes for an AlterField or RenameField on an
// M2M field (renaming or updating its auto-created join table when needed).
func (e *SchemaEditor) alterM2MField(ctx context.Context, from, to *ProjectState, oldMS, newMS *ModelState, oldF, newF FieldState) error {
	switch {
	case !oldF.HasAutoThrough() && !newF.HasAutoThrough():
		return nil
	case oldF.HasAutoThrough() && !newF.HasAutoThrough():
		oldTh, err := autoThroughModelState(from, oldMS, oldF)
		if err != nil {
			return err
		}
		return e.deleteTable(ctx, oldTh.Table)
	case !oldF.HasAutoThrough() && newF.HasAutoThrough():
		newTh, err := autoThroughModelState(to, newMS, newF)
		if err != nil {
			return err
		}
		return e.createModel(ctx, to, newTh)
	}

	oldTh, err := autoThroughModelState(from, oldMS, oldF)
	if err != nil {
		return err
	}
	newTh, err := autoThroughModelState(to, newMS, newF)
	if err != nil {
		return err
	}
	return e.syncThroughTable(ctx, to, oldTh, newTh)
}

// syncThroughTable renames an auto-created through table and updates its FK
// columns and unique-together constraint when its definition changed.
func (e *SchemaEditor) syncThroughTable(ctx context.Context, state *ProjectState, oldTh, newTh *ModelState) error {
	cur := oldTh.clone()
	if cur.Table != newTh.Table {
		if err := e.renameTableAndDerived(ctx, cur, newTh.Table); err != nil {
			return err
		}
		cur.Table = newTh.Table
	}

	if len(cur.Fields) != 3 || len(newTh.Fields) != 3 {
		return nil
	}
	f1Same := schemaEqual(cur.Fields[1], newTh.Fields[1]) && cur.Fields[1].Column == newTh.Fields[1].Column
	f2Same := schemaEqual(cur.Fields[2], newTh.Fields[2]) && cur.Fields[2].Column == newTh.Fields[2].Column
	if f1Same && f2Same {
		return nil
	}

	if e.isSQLite() {
		exprs := map[string]string{
			newTh.Fields[0].Column: e.q(cur.Fields[0].Column),
			newTh.Fields[1].Column: e.q(cur.Fields[1].Column),
			newTh.Fields[2].Column: e.q(cur.Fields[2].Column),
		}
		return e.remakeTable(ctx, state, cur, newTh, exprs)
	}

	// PostgreSQL: drop the old unique_together, alter both FK columns, then add
	// the new unique_together constraint.
	noUT := cur.clone()
	noUT.UniqueTogether = nil
	if err := e.alterUniqueTogether(ctx, cur, noUT); err != nil {
		return err
	}
	mid := noUT.clone()
	mid.Fields[1] = newTh.Fields[1]
	if !f1Same {
		if err := e.alterFieldPostgres(ctx, state, noUT, mid, noUT.Fields[1], mid.Fields[1], nil); err != nil {
			return err
		}
	}
	afterF2 := mid.clone()
	afterF2.Fields[2] = newTh.Fields[2]
	if !f2Same {
		if err := e.alterFieldPostgres(ctx, state, mid, afterF2, mid.Fields[2], afterF2.Fields[2], nil); err != nil {
			return err
		}
	}
	return e.alterUniqueTogether(ctx, afterF2, newTh)
}

// renameTableAndDerived renames oldMS.Table to newTable and updates derived
// index and unique-together constraint names on the table.
func (e *SchemaEditor) renameTableAndDerived(ctx context.Context, oldMS *ModelState, newTable string) error {
	if err := e.Execute(ctx, "ALTER TABLE "+e.q(oldMS.Table)+" RENAME TO "+e.q(newTable)); err != nil {
		return err
	}
	renamed := oldMS.clone()
	renamed.Table = newTable
	return e.renameDerived(ctx, oldMS, renamed)
}

// alterTableM2MTables updates auto-created M2M join tables affected when oldMS
// is renamed (RenameModel) or its table changes (AlterModelTable).
func (e *SchemaEditor) alterTableM2MTables(ctx context.Context, from, to *ProjectState, oldMS, newMS *ModelState) error {
	// Update M2M fields declared directly on this model.
	for _, oldF := range oldMS.Fields {
		if !oldF.HasAutoThrough() {
			continue
		}
		newF, ok := newMS.FieldByName(oldF.Name)
		if !ok || !newF.HasAutoThrough() {
			continue
		}
		oldTh, err := autoThroughModelState(from, oldMS, oldF)
		if err != nil {
			return err
		}
		newTh, err := autoThroughModelState(to, newMS, *newF)
		if err != nil {
			return err
		}
		if err := e.syncThroughTable(ctx, to, oldTh, newTh); err != nil {
			return err
		}
	}

	// When the model name itself changed (RenameModel), also update auto-created
	// M2M join tables on other models that point to oldMS.
	if oldMS.Name == newMS.Name {
		return nil
	}
	oldLabel := oldMS.Label()
	for _, label := range from.Order {
		otherOld := from.Models[label]
		if otherOld == oldMS {
			continue
		}
		otherNew, ok := to.Models[label]
		if !ok {
			continue
		}
		for _, oldF := range otherOld.Fields {
			if !oldF.HasAutoThrough() || oldF.RelTo != oldLabel {
				continue
			}
			newF, ok := otherNew.FieldByName(oldF.Name)
			if !ok || !newF.HasAutoThrough() {
				continue
			}
			oldTh, err := autoThroughModelState(from, otherOld, oldF)
			if err != nil {
				return err
			}
			newTh, err := autoThroughModelState(to, otherNew, *newF)
			if err != nil {
				return err
			}
			if err := e.syncThroughTable(ctx, to, oldTh, newTh); err != nil {
				return err
			}
		}
	}
	return nil
}
