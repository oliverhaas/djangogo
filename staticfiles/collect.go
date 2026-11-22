package staticfiles

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// CollectOptions configures Collect (Django's collectstatic).
type CollectOptions struct {
	// Root is STATIC_ROOT, the destination directory.
	Root string
	// Clear deletes the existing files of Root first (--clear).
	Clear bool
	// DryRun reports without changing anything (--dry-run).
	DryRun bool
	// Link creates a symbolic link to each file instead of copying it
	// (--link). A file of a source without a directory on disk, such as an
	// embed.FS, has nothing to link to and is copied.
	Link bool
	// IgnorePatterns are extra ignore patterns (--ignore); DefaultIgnorePatterns
	// apply as well unless NoDefaultIgnore is set. Finder.List describes their
	// syntax.
	IgnorePatterns []string
	// NoDefaultIgnore turns DefaultIgnorePatterns off (--no-default-ignore).
	NoDefaultIgnore bool
	// Verbosity selects the log lines, as in Django: 0 prints nothing, 1 only
	// the "Pretending to copy" lines of a dry run, 2 and above a line per
	// file. The zero value is silent, while Django's commands default to 1.
	Verbosity int
	// Out receives the log lines; nil discards them.
	Out io.Writer
}

// CollectResult counts what Collect did. The paths are relative to the static
// root and slash-separated.
type CollectResult struct {
	// Copied lists the paths copied, in order. A dry run lists the paths it
	// would copy.
	Copied []string
	// Linked lists the paths linked with CollectOptions.Link, in order. A dry
	// run lists the paths it would link.
	Linked []string
	// Unmodified lists the paths skipped because the destination is not older
	// than the source.
	Unmodified []string
	// Deleted lists the paths removed by Clear. A dry run lists the paths it
	// would remove.
	Deleted []string
	// Skipped lists the paths that a later source holds as well: only the
	// file of the first source is collected. A path is listed once per
	// ignored file.
	Skipped []string
	// link records CollectOptions.Link for the wording of Summary.
	link bool
}

// configError is an error whose text is a message of Django, kept verbatim
// because it reaches the user of a command.
type configError string

// Error returns Django's message.
func (e configError) Error() string { return string(e) }

var (
	// ErrStaticRootNotSet is returned by Collect when CollectOptions.Root is
	// empty. Its text is Django's ImproperlyConfigured message.
	ErrStaticRootNotSet error = configError(
		"You're using the staticfiles app without having set the STATIC_ROOT setting to a filesystem path.",
	)
	// ErrStaticRootInSources is returned by Collect when the static root is
	// also the directory of a source: collecting would copy the files onto
	// themselves, and Clear would delete them. Its text is Django's system
	// check staticfiles.E002.
	ErrStaticRootInSources error = configError(
		"The STATICFILES_DIRS setting should not contain the STATIC_ROOT setting.",
	)
)

// Collect copies or links every static file into opts.Root (Django's
// collectstatic without post-processing). Of the files that several sources hold under one
// path, the file of the first source is copied and the others are counted as
// skipped. A destination file that is not older than its source is left alone
// and counted as unmodified; a source without modification times, such as an
// embed.FS, is always copied.
//
// Collect writes Django's log lines to opts.Out but not the closing summary:
// a command prints CollectResult.Summary after a blank line when the verbosity
// is 1 or more. On an error the result holds what was done up to it.
func Collect(finder *Finder, opts CollectOptions) (CollectResult, error) {
	if opts.Root == "" {
		return CollectResult{}, ErrStaticRootNotSet
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return CollectResult{}, fmt.Errorf("staticfiles: static root %s: %w", opts.Root, err)
	}
	for _, src := range finder.sources {
		if src.Root == "" {
			continue
		}
		if dir, err := filepath.Abs(src.Root); err == nil && dir == root {
			return CollectResult{}, ErrStaticRootInSources
		}
	}
	c := &collector{root: root, opts: opts, found: map[string]bool{}}
	c.result.link = opts.Link
	if opts.Clear {
		if err := c.clearDir(""); err != nil {
			return c.result, err
		}
	}
	err = finder.walk(opts.patterns(), c.collect)
	return c.result, err
}

// patterns returns the ignore patterns of a run, normalized as collectstatic
// normalizes them (os.path.normpath), so "vendor/" ignores like "vendor".
func (o CollectOptions) patterns() []string {
	patterns := slices.Clone(o.IgnorePatterns)
	if !o.NoDefaultIgnore {
		patterns = append(patterns, DefaultIgnorePatterns...)
	}
	for i, pattern := range patterns {
		patterns[i] = path.Clean(pattern)
	}
	return patterns
}

// collector holds the state of one Collect run.
type collector struct {
	// root is the absolute path of the static root.
	root string
	opts CollectOptions
	// found holds the destination paths that a source has already provided.
	found  map[string]bool
	result CollectResult
}

// log writes a line when the verbosity reaches level (the log helper of
// Django's command).
func (c *collector) log(level int, format string, args ...any) {
	if c.opts.Out == nil || c.opts.Verbosity < level {
		return
	}
	_, _ = fmt.Fprintf(c.opts.Out, format+"\n", args...)
}

// clearDir deletes the files below a directory of the static root, given
// relative to the root (Django's clear_dir): the files of a directory, then
// those of its subdirectories. The directories stay. A symbolic link is
// removed and never followed, so that clearing cannot reach outside the root.
func (c *collector) clearDir(rel string) error {
	dir := filepath.Join(c.root, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("staticfiles: clear %s: %w", dir, err)
	}
	var subdirs []string
	for _, entry := range entries {
		name := path.Join(rel, entry.Name())
		if entry.IsDir() {
			subdirs = append(subdirs, name)
			continue
		}
		c.result.Deleted = append(c.result.Deleted, name)
		if c.opts.DryRun {
			c.log(2, "Pretending to delete '%s'", name)
			continue
		}
		c.log(2, "Deleting '%s'", name)
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("staticfiles: clear: %w", err)
		}
	}
	for _, sub := range subdirs {
		if err := c.clearDir(sub); err != nil {
			return err
		}
	}
	return nil
}

// collect handles one file of a source (Django's copy_file and the loop
// around it in collect).
func (c *collector) collect(m Match) error {
	if c.found[m.Path] {
		c.result.Skipped = append(c.result.Skipped, m.Path)
		c.log(2, "Found another file with the destination path '%s'. It will be ignored since only the "+
			"first encountered file is collected. If this is not what you want, make sure every static "+
			"file has a unique path.", m.Path)
		return nil
	}
	c.found[m.Path] = true
	dest, err := c.destination(m)
	if err != nil {
		return err
	}
	link := c.opts.Link && m.Source.Root != ""
	stale, err := c.removeStale(m, dest, link)
	if err != nil || !stale {
		return err
	}
	if link {
		return c.link(m, dest)
	}
	if c.opts.DryRun {
		c.log(1, "Pretending to copy '%s'", m.Location)
	} else {
		c.log(2, "Copying '%s'", m.Location)
		if err := copyFile(m, dest); err != nil {
			return err
		}
	}
	c.result.Copied = append(c.result.Copied, m.Path)
	return nil
}

// link creates the symbolic link from dest to the file of a match, which
// lies in a directory on disk (Django's link_file). The link holds the
// absolute path of the file.
func (c *collector) link(m Match, dest string) error {
	if c.opts.DryRun {
		c.log(1, "Pretending to link '%s'", m.Location)
	} else {
		c.log(2, "Linking '%s'", m.Location)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("staticfiles: link %s: %w", m.Location, err)
		}
		if err := os.Symlink(m.Location, dest); err != nil {
			return fmt.Errorf("staticfiles: link %s: %w", m.Location, err)
		}
	}
	c.result.Linked = append(c.result.Linked, m.Path)
	return nil
}

// destination returns the file in the static root that a match is copied to.
// It refuses a path that is not a plain path below the root, which only the
// prefix of a source can cause, for example one with a ".." element.
func (c *collector) destination(m Match) (string, error) {
	local, err := filepath.Localize(m.Path)
	if err != nil {
		return "", fmt.Errorf("staticfiles: collect %s: destination %q is not a path below the static root",
			m.Location, m.Path)
	}
	return filepath.Join(c.root, local), nil
}

// removeStale is Django's delete_file: it reports whether the file has to be
// copied or, with link, linked, and removes what is in the way at the
// destination first. A destination that is not older than the source stays
// and counts as unmodified when it is what the run creates: a symbolic link
// with link, else a regular file. The time of a link is that of the file it
// points to. Whole seconds are compared, as in Django, because not every
// filesystem keeps more.
func (c *collector) removeStale(m Match, dest string, link bool) (bool, error) {
	target, err := os.Lstat(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("staticfiles: collect %s: %w", m.Location, err)
	}
	name := m.name()
	source, err := m.Source.stat(name)
	if err != nil {
		return false, fmt.Errorf("staticfiles: collect %s: %w", m.Location, err)
	}
	// The zero time means that the source keeps no modification times, as an
	// embed.FS does: nothing can be said about such a file, so it is copied.
	isLink := target.Mode()&fs.ModeSymlink != 0
	same := isLink == link && (isLink || target.Mode().IsRegular())
	if same && isLink {
		// A link to nothing has no time and is replaced.
		target, err = os.Stat(dest)
		same = err == nil
	}
	if same && !source.ModTime().IsZero() && target.ModTime().Unix() >= source.ModTime().Unix() {
		c.result.Unmodified = append(c.result.Unmodified, m.Path)
		c.log(2, "Skipping '%s' (not modified)", name)
		return false, nil
	}
	if c.opts.DryRun {
		c.log(2, "Pretending to delete '%s'", name)
		return true, nil
	}
	c.log(2, "Deleting '%s'", name)
	if err := os.Remove(dest); err != nil {
		return false, fmt.Errorf("staticfiles: collect %s: %w", m.Location, err)
	}
	return true, nil
}

// copyFile writes the file of a match to dest, which must not exist. Creating
// the file exclusively keeps the copy from following a symbolic link that
// appeared at the destination.
func copyFile(m Match, dest string) error {
	src, err := m.open()
	if err != nil {
		return fmt.Errorf("staticfiles: copy %s: %w", m.Location, err)
	}
	defer func() { _ = src.Close() }()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("staticfiles: copy %s: %w", m.Location, err)
	}
	dst, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("staticfiles: copy %s: %w", m.Location, err)
	}
	_, err = io.Copy(dst, src)
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		// Leave no half-written file behind: the next run would take it for
		// an unmodified copy.
		_ = os.Remove(dest)
		return fmt.Errorf("staticfiles: copy %s: %w", m.Location, err)
	}
	return nil
}

// Summary is the closing line of Django's collectstatic, without the blank
// line before it: "N static files copied to '/root'." with "M static files
// deleted, " in front after a Clear, and ", M unmodified" and ", M skipped due
// to conflict" appended when M is not zero. A run with Link counts the links:
// "N static files symlinked", followed by ", M copied" when files of an
// embedded source were copied. A dry run has the same wording. The root is
// printed as an absolute path and left out when it is empty.
func (r CollectResult) Summary(root string) string {
	summary := ""
	if n := len(r.Deleted); n > 0 {
		summary = fmt.Sprintf("%d static file%s deleted, ", n, plural(n))
	}
	modified, action := len(r.Copied), "copied"
	if r.link {
		modified, action = len(r.Linked), "symlinked"
	}
	summary += fmt.Sprintf("%d static file%s %s", modified, plural(modified), action)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		summary += fmt.Sprintf(" to '%s'", root)
	}
	if n := len(r.Copied); r.link && n > 0 {
		summary += fmt.Sprintf(", %d copied", n)
	}
	if n := len(r.Unmodified); n > 0 {
		summary += fmt.Sprintf(", %d unmodified", n)
	}
	if n := len(r.Skipped); n > 0 {
		summary += fmt.Sprintf(", %d skipped due to conflict", n)
	}
	return summary + "."
}

// plural returns the plural ending of a count: Django writes "1 static file"
// and "0 static files".
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
