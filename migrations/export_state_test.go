package migrations

// Referencing exposes referencing to the external test package, which ports
// the tests of Django's ProjectState.relations. Each entry reads
// "app.Model.Field", the model and field whose foreign key points to label.
func (ps *ProjectState) Referencing(label string) []string {
	refs := ps.referencing(label)
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.model.Label()+"."+ref.field.Name)
	}
	return out
}
