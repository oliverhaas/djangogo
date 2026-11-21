package orm

// Index declares a model-level database index in Meta.Indexes, the analog of
// Django's models.Index. Name is optional: when omitted, the registry derives
// Django's default 30-character name from the table and columns.
type Index struct {
	// Name is the database index name. Leave empty for Django's default name.
	Name string
	// Fields lists the fields the index covers, in order. Entries accept Go
	// struct names, attribute names, or column names, with an optional leading
	// "-" for descending order.
	Fields []string
}

// Meta is the optional per-model configuration hook (the analog of Django's Meta).
type Meta struct {
	// Table overrides the default table name (Django's db_table).
	Table string
	// Ordering sets the default QuerySet ordering (Django's Meta.ordering).
	Ordering []string
	// UniqueTogether declares sets of fields that must be unique together
	// (Django's Meta.unique_together).
	UniqueTogether [][]string
	// Indexes declares model-level database indexes (Django's Meta.indexes).
	Indexes []Index
	// VerboseName overrides the human-readable singular name (Django's
	// Meta.verbose_name).
	VerboseName string
	// VerboseNamePlural overrides the human-readable plural name (Django's
	// Meta.verbose_name_plural).
	VerboseNamePlural string
	// Choices declares per-field choices keyed by Go struct field name,
	// attribute name, or column name (Django's Field(choices=...)).
	Choices map[string][]Choice
	// ChoicesFunc declares per-field dynamic choice providers evaluated on
	// demand (Django's Field(choices=callable)).
	ChoicesFunc map[string]func() []Choice
	// Validators declares per-field custom validators keyed by Go struct field
	// name, attribute name, or column name (Django's Field(validators=[...])).
	Validators map[string][]Validator
	// ErrorMessages declares per-field error message overrides keyed by field
	// name then error code (Django's Field(error_messages={...})).
	ErrorMessages map[string]map[string]string
}

// withMeta is implemented by models that customize their Meta.
type withMeta interface {
	Meta() Meta
}

// Choice is one (value, label) option or optgroup for a choices-constrained
// field, the analog of one entry in Django's choices=[(value, label), ...] or
// choices=[("Group", [(value, label), ...])].
type Choice struct {
	Value string
	Label string
	// Group holds nested choices when this entry is an optgroup heading.
	// When non-empty, Label is the group name and Value is ignored.
	Group []Choice
	// Raw holds the typed choice value (such as int, Decimal, Date, bool, or
	// UUID) when constructed via ChoiceOf. When nil, Value is used.
	Raw any
}

// withChoices is implemented by models that declare per-field choices. The map
// key is the Go struct field name (e.g. "Status"); the value is that field's
// allowed options. It is the analog of declaring choices= on a Django field.
type withChoices interface {
	Choices() map[string][]Choice
}
