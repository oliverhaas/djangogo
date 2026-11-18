package migrations

// GeneratePlan exposes generatePlan to the external test package, which ports
// a Django test of the private _generate_plan.
func (g *Graph) GeneratePlan(targets []Key, atEnd bool) ([]Key, error) {
	return g.generatePlan(targets, atEnd)
}

// SetNameTimestamp replaces the timestamp of fallback migration names and
// returns a function that restores the clock-based one.
func SetNameTimestamp(stamp string) (restore func()) {
	old := nameTimestamp
	nameTimestamp = func() string { return stamp }
	return func() { nameTimestamp = old }
}

// DetectChanges exposes the migrations before they are named for a graph,
// Django's _detect_changes. Their names are "auto_1", "auto_2" and so on.
func (a *Autodetector) DetectChanges(graph *Graph) (map[string][]*Migration, error) {
	if graph == nil {
		graph = NewGraph()
	}
	if err := a.detectChanges(graph); err != nil {
		return nil, err
	}
	return a.migrations, nil
}

// TrimToApps exposes trimChanges, Django's _trim_to_apps.
func TrimToApps(changes map[string][]*Migration, apps []string) map[string][]*Migration {
	return trimChanges(changes, apps)
}

// ParseNumber exposes parseNumber, Django's parse_number.
func ParseNumber(name string) int { return parseNumber(name) }

// BoolInput exposes boolInput, Django's _boolean_input.
func (q *InteractiveQuestioner) BoolInput(question string, def bool) (bool, error) {
	return q.boolInput(question, def)
}

// ChoiceInput exposes choiceInput, Django's _choice_input.
func (q *InteractiveQuestioner) ChoiceInput(question string, choices []string) (int, error) {
	return q.choiceInput(question, choices)
}

// AskDefault exposes askDefault, Django's _ask_default.
func (q *InteractiveQuestioner) AskDefault(field FieldState) (any, error) {
	return q.askDefault(field)
}
