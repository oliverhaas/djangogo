package orm

// Meta is the optional per-model configuration hook (the analog of Django's Meta).
type Meta struct {
	Table string // overrides the default table name
}

// withMeta is implemented by models that customize their Meta.
type withMeta interface {
	Meta() Meta
}

// Choice is one (value, label) option for a choices-constrained field, the analog
// of one entry in Django's choices=[(value, label), ...].
type Choice struct {
	Value string
	Label string
}

// withChoices is implemented by models that declare per-field choices. The map
// key is the Go struct field name (e.g. "Status"); the value is that field's
// allowed options. It is the analog of declaring choices= on a Django field.
type withChoices interface {
	Choices() map[string][]Choice
}
