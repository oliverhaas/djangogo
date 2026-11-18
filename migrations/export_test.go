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
