package orm

// Meta is the optional per-model configuration hook (the analog of Django's Meta).
type Meta struct {
	Table string // overrides the default table name
}

// withMeta is implemented by models that customize their Meta.
type withMeta interface {
	Meta() Meta
}
