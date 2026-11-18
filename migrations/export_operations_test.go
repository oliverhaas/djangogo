package migrations

// ReferencesModel exposes referencesModel to the external test package, which
// ports Django's FieldOperationTests.test_references_model.
func ReferencesModel(op Operation, name, app string) bool { return referencesModel(op, name, app) }

// ReferencesField exposes referencesField to the external test package, which
// ports Django's FieldOperationTests.test_references_field_by_name.
func ReferencesField(op Operation, model, field, app string) bool {
	return referencesField(op, model, field, app)
}
