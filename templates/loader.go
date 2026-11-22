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
	"regexp"
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

// extendsRe finds the parent named by a template's {% extends %} tag.
var extendsRe = regexp.MustCompile(`\{%-?\s*extends\s+(?:"([^"]*)"|'([^']*)')\s*-?%\}`)

// sourceLoader is the pongo2 loader of an Engine. It searches the sources in
// order and remembers which template each one extends, which the render
// recorder needs and pongo2 does not expose.
type sourceLoader struct {
	mu      sync.RWMutex
	sources []Source
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

// find reads the first source that has name. When none has it, tried lists the
// candidates and err is nil; err reports a source that has the template but
// failed to deliver it (permissions, a directory in place of a file).
func (l *sourceLoader) find(name string) (data []byte, found bool, tried []TriedSource, err error) {
	clean, ok := cleanName(name)
	if !ok {
		return nil, false, nil, nil
	}
	l.mu.RLock()
	sources := l.sources
	l.mu.RUnlock()
	for _, s := range sources {
		data, err := s.read(clean)
		if err == nil {
			return data, true, tried, nil
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			return nil, false, tried, fmt.Errorf("templates: read %s: %w", s.origin(clean), err)
		}
		tried = append(tried, TriedSource{Origin: s.origin(clean), Reason: sourceMissing})
	}
	return nil, false, tried, nil
}

// missing explains why pongo2 could not load name: the TemplateDoesNotExist
// of a failed search, the read error of a source, or an inheritance cycle. It
// returns nil when the template loads fine and the failure lies elsewhere.
func (l *sourceLoader) missing(name string) error {
	base, overrides := parseVariant(name)
	base, _, isPartial := cutPartial(base)
	if base == "" {
		return &TemplateDoesNotExist{Name: displayName(name)}
	}
	data, found, tried, err := l.find(base)
	if err != nil {
		return err
	}
	switch {
	case !found:
		return &TemplateDoesNotExist{Name: base, Tried: tried}
	case isPartial:
		return nil
	}
	return l.checkExtends(base, data, overrides)
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

// parentOf returns the template the source src extends, or "".
func parentOf(src []byte) string {
	if m := extendsRe.FindSubmatch(src); m != nil {
		return string(m[1]) + string(m[2])
	}
	return ""
}

// checkExtends follows the {% extends %} chain of a template and reports a
// cycle. pongo2 compiles a parent while it compiles the child, so a template
// that extends itself, directly or through others, would recurse until the
// stack is exhausted. overrides give the parents of the templates whose
// {% extends %} has a variable, see variantName.
func (l *sourceLoader) checkExtends(name string, src []byte, overrides []extendsOverride) error {
	chain := []string{name}
	for parent := parentIn(name, src, overrides); parent != ""; {
		if slices.Contains(chain, parent) {
			return fmt.Errorf("templates: template %q extends itself (%s -> %s)",
				name, strings.Join(chain, " -> "), parent)
		}
		chain = append(chain, parent)
		// A parent that is missing or unreadable ends the chain; pongo2 reports
		// it when it compiles the child.
		data, _, _, _ := l.find(parent)
		parent = parentIn(parent, data, overrides)
	}
	return nil
}

// parentIn returns the parent of the template name with the source src: the
// one overrides give, or else the one its {% extends %} names.
func parentIn(name string, src []byte, overrides []extendsOverride) string {
	if parent, ok := overrideOf(overrides, name); ok {
		return parent
	}
	return parentOf(src)
}

// Abs returns name unchanged: as in Django a name is relative to the sources,
// never to the template that mentions it.
func (l *sourceLoader) Abs(_, name string) string { return name }

// Get returns the content of the named template for pongo2. pongo2 drops the
// error, so the engine asks missing for the reason afterwards.
//
// A name such as "page.html#card" stands for the partial card of
// page.html, Django's template partials: Get returns a source that renders
// only that partial, see partialSource. A name of variantName stands for a
// template with the parents a render gave its {% extends %} tags with a
// variable, see variantSource.
func (l *sourceLoader) Get(name string) (io.Reader, error) {
	base, overrides := parseVariant(name)
	base, partial, isPartial := cutPartial(base)
	if base == "" {
		return nil, &TemplateDoesNotExist{Name: name}
	}
	data, found, _, err := l.find(base)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &TemplateDoesNotExist{Name: base}
	}
	if isPartial {
		return bytes.NewReader(partialSource(data, base, partial)), nil
	}
	if err := l.checkExtends(base, data, overrides); err != nil {
		return nil, err
	}
	l.noteParent(base, data)
	if overrides != nil {
		return bytes.NewReader(variantSource(data, base, overrides)), nil
	}
	return bytes.NewReader(djangoSyntax(data, base)), nil
}

// noteParent records the template that the source of name extends.
func (l *sourceLoader) noteParent(name string, src []byte) {
	parent := parentOf(src)
	l.mu.Lock()
	defer l.mu.Unlock()
	if parent == "" {
		delete(l.parents, name)
		return
	}
	if l.parents == nil {
		l.parents = make(map[string]string)
	}
	l.parents[name] = parent
}

// chain returns the templates name extends, nearest parent first.
func (l *sourceLoader) chain(name string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []string
	seen := map[string]bool{name: true}
	for {
		parent, ok := l.parents[name]
		if !ok || seen[parent] {
			return out
		}
		seen[parent] = true
		out = append(out, parent)
		name = parent
	}
}
