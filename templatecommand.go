package djangogo

import (
	"context"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/oliverhaas/djangogo/internal/archive"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/scaffold"
)

// urlSchemes are the schemes of a --template URL, Django's url_schemes. Go's
// HTTP client cannot fetch ftp URLs and reports them as unsupported.
var urlSchemes = []string{"http", "https", "ftp"}

// templateFlags are the options startproject and startapp share, those of
// Django's TemplateCommand.
type templateFlags struct {
	verbosity  verbosityValue
	template   string
	extensions listFlag
	files      listFlag
	exclude    excludeFlag
}

// declare defines the options on fs with their defaults: the Go files and
// go.mod are rendered.
func (f *templateFlags) declare(fs *flag.FlagSet) {
	f.verbosity = 1
	f.verbosity.declare(fs)
	f.template = ""
	f.extensions = listFlag{"go"}
	f.files = listFlag{"go.mod"}
	f.exclude = excludeFlag{}
	fs.StringVar(&f.template, "template", "", "The path or URL to load the template from.")
	const extensionHelp = "The file extension(s) to render. Separate multiple extensions with commas, " +
		"or use -e multiple times."
	fs.Var(&f.extensions, "extension", extensionHelp)
	fs.Var(&f.extensions, "e", extensionHelp)
	const nameHelp = "The file name(s) to render. Separate multiple file names with commas, or use " +
		"-n multiple times."
	fs.Var(&f.files, "name", nameHelp)
	fs.Var(&f.files, "n", nameHelp)
	const excludeHelp = "The directory name(s) to exclude, in addition to .git and __pycache__. Can " +
		"be used multiple times."
	fs.Var(&f.exclude, "exclude", excludeHelp)
	fs.Var(&f.exclude, "x", excludeHelp)
}

// listFlag is an option given any number of times, argparse's
// action="append": every value is added to the default.
type listFlag []string

// String returns the values separated by commas.
func (l *listFlag) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

// Set adds a value.
func (l *listFlag) Set(s string) error {
	*l = append(*l, s)
	return nil
}

// excludeFlag is --exclude, which may be given without a directory name:
// argparse's action="append" with nargs="?". Without the option, hidden
// directories are left out too.
type excludeFlag struct {
	names []string
	given bool
}

// String returns the names separated by commas.
func (e *excludeFlag) String() string {
	if e == nil {
		return ""
	}
	return strings.Join(e.names, ",")
}

// Set adds a directory name.
func (e *excludeFlag) Set(s string) error {
	e.names = append(e.names, s)
	e.given = true
	return nil
}

// NoValue returns the empty name, argparse's const.
func (*excludeFlag) NoValue() string { return "" }

// skipDir reports whether a directory of the template tree is left out:
// .git, __pycache__ and the names given with --exclude, or without the
// option every hidden directory and __pycache__.
func (e *excludeFlag) skipDir(name string) bool {
	if !e.given {
		return strings.HasPrefix(name, ".") || name == "__pycache__"
	}
	if name == ".git" || name == "__pycache__" {
		return true
	}
	return slices.ContainsFunc(e.names, func(n string) bool { return strings.TrimSpace(n) == name })
}

// generate writes the project or app named name into top, from the built-in
// tree or from the tree --template names, the rest of Django's
// TemplateCommand.handle. builtin writes the built-in tree, which is
// rendered as a whole: -e and -n select the files of a custom tree only.
func (t *templateCommand) generate(top, name string, builtin func() error) error {
	if t.template == "" {
		return t.result(builtin())
	}
	extensions := handleExtensions(t.extensions)
	var files []string
	for _, f := range t.files {
		for n := range strings.SplitSeq(f, ",") {
			files = append(files, strings.TrimSpace(n))
		}
	}
	// The built-in trees are rendered as a whole, so only a custom tree has
	// these lines.
	if t.verbosity >= 2 {
		out := t.app.out()
		_, _ = fmt.Fprintf(out, "Rendering %s template files with extensions: %s\n", t.kind, strings.Join(extensions, ", "))
		_, _ = fmt.Fprintf(out, "Rendering %s template files with filenames: %s\n", t.kind, strings.Join(files, ", "))
	}

	ctx := map[string]any{
		"template":                       t.template,
		"extensions":                     extensions,
		"files":                          files,
		"verbosity":                      int(t.verbosity),
		t.kind + "_name":                 name,
		t.kind + "_directory":            top,
		"camel_case_" + t.kind + "_name": strings.ReplaceAll(titleCase(name), "_", ""),
		"djangogo_version":               Version,
		"go_version":                     scaffold.GoVersion,
	}
	if t.exclude.given {
		ctx["exclude"] = t.exclude.names
	}
	if t.kind == "project" {
		key, err := scaffold.SecretKey()
		if err != nil {
			return manage.Errorf("%w", err)
		}
		ctx["secret_key"] = key
	}

	defer t.cleanup()
	src, err := t.handleTemplate(t.template)
	if err != nil {
		return err
	}
	return t.result(scaffold.Tree(top, src, scaffold.TreeOptions{
		Placeholder: t.kind + "_name",
		Name:        name,
		Context:     ctx,
		Extensions:  extensions,
		Files:       files,
		SkipDir:     t.exclude.skipDir,
		Created:     t.created,
	}))
}

// handleTemplate returns the directory of the template tree: the directory
// template names, or a temporary directory with the archive it names on disk
// or at a URL extracted, Django's handle_template.
func (t *templateCommand) handleTemplate(template string) (string, error) {
	template = strings.TrimPrefix(template, "file://")
	expanded := filepath.Clean(expandUser(template))
	if info, err := os.Stat(expanded); err == nil && info.IsDir() {
		return expanded, nil
	}
	var absolute string
	if isURL(template) {
		var err error
		if absolute, err = t.download(template); err != nil {
			return "", err
		}
	} else {
		var err error
		if absolute, err = filepath.Abs(expanded); err != nil {
			return "", manage.Errorf("%w", err)
		}
	}
	if _, err := os.Stat(absolute); err == nil {
		return t.extract(absolute)
	}
	return "", manage.Errorf("couldn't handle %s template %s.", t.kind, template)
}

// download fetches url into a temporary directory and returns the path of
// the file, named after the URL, the Content-Disposition header or the
// Content-Type header, so its format is known. The request names
// Djan-Go-Go as the user agent.
func (t *templateCommand) download(url string) (string, error) {
	trimmed := strings.TrimRight(url, "/")
	filename := trimmed[strings.LastIndex(trimmed, "/")+1:]
	displayURL := url
	if strings.HasSuffix(url, "/") {
		displayURL = trimmed + "/"
	}
	dir, err := t.tempDir("download")
	if err != nil {
		return "", err
	}
	if t.verbosity >= 2 {
		_, _ = fmt.Fprintf(t.app.out(), "Downloading %s\n", displayURL)
	}

	thePath := filepath.Join(dir, filename)
	header, err := fetch(url, thePath)
	if err != nil {
		return "", manage.Errorf("couldn't download URL %s to %s: %s", url, filename, err)
	}

	guessed := filename
	if _, params, err := mime.ParseMediaType(header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		// The base name keeps the file in the temporary directory.
		guessed = path.Base(filepath.ToSlash(params["filename"]))
	}
	if _, ext := splitext(guessed); ext == "" {
		if contentType := header.Get("Content-Type"); contentType != "" {
			if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
				guessed += exts[0]
			}
		}
	}
	if guessed == filename {
		return thePath, nil
	}
	guessedPath := filepath.Join(dir, guessed)
	if err := os.Rename(thePath, guessedPath); err != nil {
		return "", manage.Errorf("%w", err)
	}
	return guessedPath, nil
}

// fetch writes the body of a GET of url into the file dst and returns the
// response header. A status of 400 or more is an error, as for Python's
// urlopen.
func fetch(url, dst string) (http.Header, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Djan-Go-Go/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	f, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return nil, err
	}
	return resp.Header, f.Close()
}

// extract unpacks the archive filename into a temporary directory and
// returns the directory.
func (t *templateCommand) extract(filename string) (string, error) {
	dir, err := t.tempDir("extract")
	if err != nil {
		return "", err
	}
	if t.verbosity >= 2 {
		_, _ = fmt.Fprintf(t.app.out(), "Extracting %s\n", filename)
	}
	if err := archive.Extract(filename, dir); err != nil {
		return "", manage.Errorf("couldn't extract file %s to %s: %s", filename, dir, err)
	}
	return dir, nil
}

// tempDir creates a temporary directory that cleanup removes.
func (t *templateCommand) tempDir(suffix string) (string, error) {
	dir, err := os.MkdirTemp("", "djangogo_"+t.kind+"_template_*_"+suffix)
	if err != nil {
		return "", manage.Errorf("%w", err)
	}
	t.pathsToRemove = append(t.pathsToRemove, dir)
	return dir, nil
}

// cleanup removes the temporary directories.
func (t *templateCommand) cleanup() {
	if len(t.pathsToRemove) == 0 {
		return
	}
	if t.verbosity >= 2 {
		_, _ = fmt.Fprintln(t.app.out(), "Cleaning up temporary files.")
	}
	for _, p := range t.pathsToRemove {
		_ = os.RemoveAll(p)
	}
	t.pathsToRemove = nil
}

// isURL reports whether template looks like a URL of a supported scheme.
func isURL(template string) bool {
	scheme, _, found := strings.Cut(template, ":")
	return found && slices.Contains(urlSchemes, strings.ToLower(scheme))
}

// splitext splits the extension off name, ".tar" included, as Django's
// TemplateCommand.splitext does.
func splitext(name string) (base, ext string) {
	ext = path.Ext(name)
	base = strings.TrimSuffix(name, ext)
	if strings.HasSuffix(strings.ToLower(base), ".tar") {
		ext = base[len(base)-4:] + ext
		base = base[:len(base)-4]
	}
	return base, ext
}

// expandUser replaces a leading "~" or "~user" with the home directory, as
// Python's os.path.expanduser does. The path stays as it is when the home
// directory is unknown.
func expandUser(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	name, rest, _ := strings.Cut(p[1:], string(filepath.Separator))
	var home string
	if name == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		home = h
	} else {
		u, err := user.Lookup(name)
		if err != nil {
			return p
		}
		home = u.HomeDir
	}
	return filepath.Join(home, rest)
}

// handleExtensions returns the extensions given with -e, split at commas,
// without spaces and with a leading dot, without duplicates and sorted,
// Django's handle_extensions.
func handleExtensions(extensions []string) []string {
	var exts []string
	for _, e := range extensions {
		for ext := range strings.SplitSeq(strings.ReplaceAll(e, " ", ""), ",") {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			exts = append(exts, ext)
		}
	}
	slices.Sort(exts)
	return slices.Compact(exts)
}

// titleCase returns s with the first letter of every word in title case and
// the other letters in lower case, Python's str.title: a word is a run of
// cased letters.
func titleCase(s string) string {
	var b strings.Builder
	previousIsCased := false
	for _, r := range s {
		if previousIsCased {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(unicode.ToTitle(r))
		}
		previousIsCased = unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
	}
	return b.String()
}
