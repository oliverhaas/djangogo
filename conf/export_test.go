package conf

// SwapActive replaces the active settings and returns the former ones; nil
// stands for the state before Configure. It lets a test of the unconfigured
// state restore what it found.
func SwapActive(s *Settings) *Settings {
	return active.Swap(s)
}
