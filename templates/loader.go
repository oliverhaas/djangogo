package templates

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// ErrTemplateDoesNotExist is the sentinel every *TemplateDoesNotExist matches
// with errors.Is.
var ErrTemplateDoesNotExist = errors.New("template does not exist")

// sourceMissing is the reason Django reports for a source without the template.
const sourceMissing = "Source does not exist"

// TriedSource is one place a template was looked for, and why it was not
// taken from there.
type TriedSource struct {
	// Origin locates the candidate: a file path for a directory source,
	// "label:name" for the others.
	Origin string
	// Reason says why the candidate was skipped, Django's "Source does not
	// exist" in the usual case.
	Reason string
}

// TemplateDoesNotExist reports that no source of the engine has a template,
// Django's exception of the same name. Tried lists every candidate in search
// order, the material of Django's template-loader postmortem.
type TemplateDoesNotExist struct {
	// Name is the template name asked for. A lookup of several names
	// (SelectTemplate) joins them with ", ", as Django does.
	Name string
	// Tried lists the candidates in the order they were searched.
	Tried []TriedSource
}

// Error returns the name and the candidates tried.
func (e *TemplateDoesNotExist) Error() string {
	var b strings.Builder
	b.WriteString("TemplateDoesNotExist: ")
	b.WriteString(e.Name)
	for i, t := range e.Tried {
		if i == 0 {
			b.WriteString(" (tried: ")
		} else {
			b.WriteString(", ")
		}
		b.WriteString(t.Origin)
		b.WriteString(" (")
		b.WriteString(t.Reason)
		b.WriteString(")")
	}
	if len(e.Tried) > 0 {
		b.WriteString(")")
	}
	return b.String()
}

// Is reports whether target is ErrTemplateDoesNotExist.
func (e *TemplateDoesNotExist) Is(target error) bool { return target == ErrTemplateDoesNotExist }

// Source is one place an Engine looks for templates: an OS directory, an
// fs.FS (the embedded templates of an app) or a set of templates held in
// memory. An engine searches its sources in order and takes the first one
// that has the name, so a project directory listed first overrides the
// templates an app ships.
//
// A template name is a slash-separated path relative to the source, the same
// in every source ("blog/post_detail.html"). A name that is absolute or leaves
// the source through ".." is in no source.
type Source struct {
	// label names the source in origins.
	label string
	// read returns the content of a template; an error matching
	// fs.ErrNotExist means the source does not have it.
	read func(name string) ([]byte, error)
	// origin locates a template of this source for a TriedSource.
	origin func(name string) string
}

// DirSource returns the source for the templates below an OS directory,
// Django's filesystem loader. The directory is not checked here; a source
// whose directory is missing has no templates.
func DirSource(dir string) Source {
	return Source{
		label: dir,
		read: func(name string) ([]byte, error) {
			// The name was cleaned by the loader and cannot leave dir.
			return os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		},
		origin: func(name string) string { return filepath.Join(dir, filepath.FromSlash(name)) },
	}
}

// FSSource returns the source for the templates in fsys, typically the
// embedded templates directory of an app (Django's app directories loader).
// label names the source in a TemplateDoesNotExist, for example the app name.
func FSSource(label string, fsys fs.FS) Source {
	return Source{
		label:  label,
		read:   func(name string) ([]byte, error) { return fs.ReadFile(fsys, name) },
		origin: func(name string) string { return label + ":" + name },
	}
}

// MapSource returns a source of templates held in memory, keyed by name,
// Django's locmem loader. It suits tests and templates built at startup. The
// map is read on every load and must not change while the engine renders.
func MapSource(label string, templates map[string]string) Source {
	return Source{
		label: label,
		read: func(name string) ([]byte, error) {
			src, ok := templates[name]
			if !ok {
				return nil, fs.ErrNotExist
			}
			return []byte(src), nil
		},
		origin: func(name string) string { return label + ":" + name },
	}
}

// sourceLoader is the pongo2 loader of an Engine. It searches the sources in
// order and remembers which template each one extends, which the render
// recorder needs and pongo2 does not expose.
type sourceLoader struct {
	mu      sync.RWMutex
	sources []Source
	// parents maps the served name of a template to the served name of its
	// parent, see served.
	parents map[string]string
}

// add appends sources to the search order.
func (l *sourceLoader) add(sources ...Source) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sources = append(l.sources, sources...)
}

// labels returns the label of every source in search order.
func (l *sourceLoader) labels() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, len(l.sources))
	for i, s := range l.sources {
		out[i] = s.label
	}
	return out
}

// cleanName normalizes a template name and reports whether it stays inside a
// source.
func cleanName(name string) (string, bool) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsRune(name, 0) {
		return "", false
	}
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// relativeName returns the template name that relative stands for, Django's
// construct_relative_path. relative is the name as a tag has it, with the
// quotes of a string literal. A name that does not start with "./" or "../"
// comes back unchanged. current is the template of the tag, "" for a
// template given as a string. allowSelf lets the result be current, as for a
// literal in {% include %}.
func relativeName(current, relative string, allowSelf bool) (string, error) {
	name := strings.Trim(relative, `'"`)
	if !strings.HasPrefix(name, "./") && !strings.HasPrefix(name, "../") {
		return relative, nil
	}
	if current == "" {
		msg := fmt.Sprintf("The relative path %s cannot be evaluated due to an unknown template origin.", relative)
		return "", errors.New(msg)
	}
	self := strings.TrimLeft(current, "/")
	name = path.Join(path.Dir(self), name)
	if strings.HasPrefix(name, "../") {
		msg := fmt.Sprintf("The relative path '%s' points outside the file hierarchy that template '%s' is in.",
			relative, current)
		return "", errors.New(msg)
	}
	if !allowSelf && name == self {
		msg := fmt.Sprintf("The relative path '%s' was translated to template name '%s', the same template in which the tag appears.",
			relative, current)
		return "", errors.New(msg)
	}
	if len(relative) > 1 && (relative[0] == '"' || relative[0] == '\'') && relative[0] == relative[len(relative)-1] {
		return quoteName(name), nil
	}
	return name, nil
}

// originName returns the template name of a pongo2 template name: "" for a
// template given as a string, the template of a partial or of a variant.
func originName(filename string) string {
	if filename == stringName {
		return ""
	}
	base, _, _ := cutPartial(parseServed(filename).name)
	return base
}

// skipped is the reason Django reports for a template that a chain has
// already passed through.
const skipped = "Skipped to avoid recursion"

// find reads the first source that has name and whose template is not in
// skip, and returns the index of that source. When there is none, at is -1,
// tried lists the candidates and err is nil; err reports a source that has
// the template but failed to deliver it (permissions, a directory in place
// of a file).
func (l *sourceLoader) find(name string, skip []origin) (data []byte, at int, tried []TriedSource, err error) {
	clean, ok := cleanName(name)
	if !ok {
		return nil, -1, nil, nil
	}
	l.mu.RLock()
	sources := l.sources
	l.mu.RUnlock()
	for i, s := range sources {
		if slices.Contains(skip, origin{name: clean, source: i}) {
			tried = append(tried, TriedSource{Origin: s.origin(clean), Reason: skipped})
			continue
		}
		data, err := s.read(clean)
		if err == nil {
			return data, i, tried, nil
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			return nil, -1, tried, fmt.Errorf("templates: read %s: %w", s.origin(clean), err)
		}
		tried = append(tried, TriedSource{Origin: s.origin(clean), Reason: sourceMissing})
	}
	return nil, -1, tried, nil
}

// missing explains why pongo2 could not load name: the TemplateDoesNotExist
// of a failed search or the read error of a source. It returns nil when the
// template loads fine and the failure lies elsewhere.
func (l *sourceLoader) missing(name string) error {
	s := parseServed(name)
	base, _, _ := cutPartial(s.name)
	if base == "" {
		return &TemplateDoesNotExist{Name: displayName(name)}
	}
	_, at, tried, err := l.find(base, s.history)
	switch {
	case err != nil:
		return err
	case at < 0:
		return &TemplateDoesNotExist{Name: base, Tried: tried}
	}
	return nil
}

// cutPartial splits "page.html#card" into the template name and the partial
// name. The template name is empty for a name no template has, such as
// "#card" or a partial name with a line break, which no tag can define.
func cutPartial(name string) (base, partial string, isPartial bool) {
	base, partial, isPartial = strings.Cut(name, "#")
	if strings.ContainsAny(partial, "\r\n") {
		base = ""
	}
	return base, partial, isPartial
}

// Abs returns name unchanged: as in Django a name is relative to the
// sources. A name in a tag that starts with "./" or "../" was made relative
// to the template of the tag before, see relativeName.
func (l *sourceLoader) Abs(_, name string) string { return name }

// Get returns the content of the named template for pongo2. pongo2 drops the
// error, so the engine asks missing for the reason afterwards.
//
// A name such as "page.html#card" stands for the partial card of
// page.html, Django's template partials: Get returns a source that renders
// only that partial, see partialSource. A served name stands for a template
// in a chain of {% extends %}, see served.
func (l *sourceLoader) Get(name string) (io.Reader, error) {
	s := parseServed(name)
	base, partial, isPartial := cutPartial(s.name)
	if base == "" {
		return nil, &TemplateDoesNotExist{Name: displayName(name)}
	}
	data, at, _, err := l.find(base, s.history)
	if err != nil {
		return nil, err
	}
	if at < 0 {
		return nil, &TemplateDoesNotExist{Name: base}
	}
	if isPartial {
		return bytes.NewReader(partialSource(data, base, partial)), nil
	}
	clean, _ := cleanName(base)
	return bytes.NewReader(l.serve(name, data, base, s, origin{name: clean, source: at})), nil
}

// serve returns the source of the template s for pongo2: src in Django's
// syntax rewritten, see rewrite, with its {% extends %} naming the
// parent as a served name that carries the history of s and self, the
// origin of s, on up the chain. name is the template name for messages, ""
// for a template given as a string. key, unless empty, is the served name
// of s, under which the parent is recorded for chain.
func (l *sourceLoader) serve(key string, src []byte, name string, s served, self origin) []byte {
	depth := len(s.history)
	next := served{history: append(slices.Clip(s.history), self), overrides: s.overrides}
	parent := ""
	out := rewrite(src, name, func(p string, literal bool) (string, bool) {
		if to, ok := s.override(depth); ok {
			p = to
		} else if !literal {
			return "", false
		}
		next.name = p
		parent = next.String()
		return parent, true
	})
	if key != "" {
		l.noteParent(key, parent)
	}
	return out
}

// noteParent records parent, a served name or "" for none, as the parent of
// the template with the served name key.
func (l *sourceLoader) noteParent(key, parent string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if parent == "" {
		delete(l.parents, key)
		return
	}
	if l.parents == nil {
		l.parents = make(map[string]string)
	}
	l.parents[key] = parent
}

// chain returns the names of the templates that the template with the served
// name key extends, nearest parent first. The parent of a served name has a
// longer history, so the chain ends.
func (l *sourceLoader) chain(key string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []string
	for {
		parent, ok := l.parents[key]
		if !ok {
			return out
		}
		out = append(out, parseServed(parent).name)
		key = parent
	}
}
