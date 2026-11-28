package serializers

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// NaturalKeyer is implemented by models that define a natural key, mirroring
// Django's Model.natural_key().
type NaturalKeyer interface {
	NaturalKey() []any
}

// NaturalKeyGetter is implemented on a model's pointer receiver to look up an
// instance by its natural key, mirroring Django's Manager.get_by_natural_key().
type NaturalKeyGetter interface {
	GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error)
}

// NaturalKeyDepender is optionally implemented by models whose natural key
// depends on other models, mirroring Django's natural_key.dependencies.
type NaturalKeyDepender interface {
	NaturalKeyDependencies() []string
}

var (
	naturalKeyerType    = reflect.TypeFor[NaturalKeyer]()
	naturalKeyGetterTyp = reflect.TypeFor[NaturalKeyGetter]()
	naturalKeyDepType   = reflect.TypeFor[NaturalKeyDepender]()
)

// modelHasNaturalKey reports whether m's Go struct or pointer type implements
// NaturalKeyer.
func modelHasNaturalKey(m *orm.Model) bool {
	if m == nil || m.GoType() == nil {
		return false
	}
	t := m.GoType()
	return t.Implements(naturalKeyerType) || reflect.PointerTo(t).Implements(naturalKeyerType)
}

// modelHasNaturalKeyGetter reports whether a pointer to m's Go struct type
// implements NaturalKeyGetter.
func modelHasNaturalKeyGetter(m *orm.Model) bool {
	if m == nil || m.GoType() == nil {
		return false
	}
	return reflect.PointerTo(m.GoType()).Implements(naturalKeyGetterTyp)
}

// naturalKeyOf invokes NaturalKey on instance (a struct value or pointer).
func naturalKeyOf(instance any) ([]any, bool) {
	if instance == nil {
		return nil, false
	}
	if nk, ok := instance.(NaturalKeyer); ok {
		return nk.NaturalKey(), true
	}
	rv := reflect.ValueOf(instance)
	if rv.Kind() == reflect.Pointer {
		return nil, false
	}
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	if nk, ok := ptr.Interface().(NaturalKeyer); ok {
		return nk.NaturalKey(), true
	}
	return nil, false
}

// callGetByNaturalKey allocates a zero pointer to m.GoType() and calls its
// GetByNaturalKey method with key.
func callGetByNaturalKey(ctx context.Context, db *orm.DB, m *orm.Model, key []any) (any, error) {
	if !modelHasNaturalKeyGetter(m) {
		return nil, fmt.Errorf("serializers: model %s does not implement NaturalKeyGetter", m.Name())
	}
	zeroPtr := reflect.New(m.GoType()).Interface().(NaturalKeyGetter)
	return zeroPtr.GetByNaturalKey(ctx, db, key...)
}

// naturalKeyDependencies returns the explicit natural key dependencies declared
// on m when m also implements NaturalKeyer.
func naturalKeyDependencies(m *orm.Model) []string {
	if !modelHasNaturalKey(m) {
		return nil
	}
	t := m.GoType()
	if t.Implements(naturalKeyDepType) {
		return reflect.Zero(t).Interface().(NaturalKeyDepender).NaturalKeyDependencies()
	}
	if reflect.PointerTo(t).Implements(naturalKeyDepType) {
		return reflect.New(t).Interface().(NaturalKeyDepender).NaturalKeyDependencies()
	}
	return nil
}

// SortDependencies orders models so that any model with a natural key comes
// before models that reference it via foreign key, auto-created M2M, or
// NaturalKeyDependencies, porting Django's
// django.core.serializers.sort_dependencies. Pass allowCycles true to break
// dependency cycles arbitrarily instead of returning an error.
func SortDependencies(reg *orm.Registry, models []*orm.Model, allowCycles ...bool) ([]*orm.Model, error) {
	allow := len(allowCycles) > 0 && allowCycles[0]
	if models == nil && reg != nil {
		models = reg.Models()
	}

	type modelDeps struct {
		model *orm.Model
		deps  []*orm.Model
	}

	modelSet := make(map[*orm.Model]bool, len(models))
	depsList := make([]modelDeps, 0, len(models))

	for _, m := range models {
		modelSet[m] = true
		var deps []*orm.Model
		for _, depName := range naturalKeyDependencies(m) {
			depModel, err := lookupModelInRegistryOrList(reg, models, depName)
			if err != nil {
				return nil, err
			}
			deps = append(deps, depModel)
		}
		for _, f := range m.Fields() {
			if f.Rel != nil && f.Rel.Target != nil {
				relModel := f.Rel.Target
				if modelHasNaturalKey(relModel) && relModel != m {
					deps = append(deps, relModel)
				}
			}
		}
		for _, f := range m.ManyToMany() {
			if f.Rel != nil && f.Rel.AutoCreated && f.Rel.Target != nil {
				relModel := f.Rel.Target
				if modelHasNaturalKey(relModel) && relModel != m {
					deps = append(deps, relModel)
				}
			}
		}
		depsList = append(depsList, modelDeps{model: m, deps: deps})
	}

	slices.Reverse(depsList)

	var result []*orm.Model
	inResult := make(map[*orm.Model]bool, len(models))
	for len(depsList) > 0 {
		var skipped []modelDeps
		changed := false
		for len(depsList) > 0 {
			item := depsList[len(depsList)-1]
			depsList = depsList[:len(depsList)-1]

			satisfied := true
			for _, d := range item.deps {
				if modelSet[d] && !inResult[d] {
					satisfied = false
					break
				}
			}
			if satisfied {
				result = append(result, item.model)
				inResult[item.model] = true
				changed = true
			} else {
				skipped = append(skipped, item)
			}
		}
		if !changed {
			if allow {
				item := skipped[len(skipped)-1]
				skipped = skipped[:len(skipped)-1]
				result = append(result, item.model)
				inResult[item.model] = true
			} else {
				slices.SortFunc(skipped, func(a, b modelDeps) int {
					return strings.Compare(a.model.Name(), b.model.Name())
				})
				labels := make([]string, len(skipped))
				for i, s := range skipped {
					labels[i] = s.model.Label()
				}
				return nil, &SerializationError{
					Message: fmt.Sprintf("Can't resolve dependencies for %s in serialized app list.", strings.Join(labels, ", ")),
				}
			}
		}
		depsList = skipped
	}
	return result, nil
}

// lookupModelInRegistryOrList resolves an "app.Model" identifier in reg or
// among models using case-sensitive app matching and case-insensitive model
// name matching.
func lookupModelInRegistryOrList(reg *orm.Registry, models []*orm.Model, identifier string) (*orm.Model, error) {
	if reg != nil {
		if m, err := lookupModel(reg, identifier); err == nil {
			return m, nil
		}
	}
	app, name, hasDot := strings.Cut(identifier, ".")
	for _, m := range models {
		if hasDot {
			if m.App() == app && strings.EqualFold(m.Name(), name) {
				return m, nil
			}
		} else if m.App() == "" && strings.EqualFold(m.Name(), identifier) {
			return m, nil
		}
	}
	return nil, NewDeserializationError(fmt.Sprintf("Invalid model identifier: '%s'", identifier))
}
