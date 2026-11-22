package orm

import (
	"slices"
	"testing"
)

// orderAuthor and orderBook are a referenced model and the model that
// references it.
type orderAuthor struct {
	ID int64
}

type orderBook struct {
	ID     int64
	Author FK[orderAuthor]
}

// orderSelf references itself, which no creation order can satisfy.
type orderSelf struct {
	ID     int64
	Parent FK[orderSelf] `orm:"null"`
}

// orderNames registers models in the given order and returns the model names
// in creation order.
func orderNames(t *testing.T, models ...any) []string {
	t.Helper()
	reg := NewRegistry()
	for _, m := range models {
		if _, err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Resolve(); err != nil {
		t.Fatal(err)
	}
	order := creationOrder(reg.Models())
	names := make([]string, 0, len(order))
	for _, m := range order {
		names = append(names, m.Name())
	}
	return names
}

// No Django test exists: Django's test databases are built by migrations.
func TestCreationOrderPutsReferencedModelsFirst(t *testing.T) {
	got := orderNames(t, &orderBook{}, &orderAuthor{})
	if want := []string{"orderAuthor", "orderBook"}; !slices.Equal(got, want) {
		t.Errorf("creation order %v, want %v", got, want)
	}
}

// No Django test exists: Django's test databases are built by migrations.
func TestCreationOrderKeepsSelfReference(t *testing.T) {
	got := orderNames(t, &orderSelf{}, &orderAuthor{})
	if want := []string{"orderSelf", "orderAuthor"}; !slices.Equal(got, want) {
		t.Errorf("creation order %v, want %v", got, want)
	}
}
