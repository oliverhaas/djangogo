package urls

// UnregisterConverter removes a converter added with RegisterConverter, so
// that a test leaves the process-wide registry as it found it.
func UnregisterConverter(name string) { unregisterConverter(name) }

// NormalizedForm is one reverse form in the shape Django's normalize returns
// it: a %-format string and the list of parameter names.
type NormalizedForm struct {
	Format string
	Params []string
}

// Normalize exposes the port of django.utils.regex_helper.normalize.
func Normalize(pattern string) []NormalizedForm {
	forms := normalize(pattern)
	out := make([]NormalizedForm, len(forms))
	for i, f := range forms {
		for _, p := range f.parts {
			if p.ref == "" {
				out[i].Format += p.lit
				continue
			}
			out[i].Format += "%(" + p.ref + ")s"
		}
		out[i].Params = append([]string{}, f.params...)
	}
	return out
}
