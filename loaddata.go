package djangogo

import (
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
	"github.com/oliverhaas/djangogo/validators"
)

// readStdin is the fixture label that reads serialized data from standard input.
const readStdin = "-"

// loaddataCompressionFormats lists the compression extensions loaddata accepts.
var loaddataCompressionFormats = []string{"", "gz", "zip", "bz2", "stdin"}

// loaddataCommand installs the named fixtures in the database (Django's
// loaddata).
type loaddataCommand struct {
	app       *Application
	database  string
	appLabel  string
	ignore    bool
	exclude   stringList
	format    string
	verbosity verbosityValue
}

// Name returns "loaddata".
func (*loaddataCommand) Name() string { return "loaddata" }

// Help returns Django's help text of the command.
func (*loaddataCommand) Help() string {
	return "Installs the named fixture(s) in the database."
}

// Usage names the positional arguments.
func (*loaddataCommand) Usage() string { return "fixture [fixture ...]" }

// Flags declares Django's options of loaddata. It resets the values of an
// earlier run first.
func (c *loaddataCommand) Flags(fs *flag.FlagSet) {
	*c = loaddataCommand{app: c.app, verbosity: 1}
	fs.StringVar(&c.database, "database", conf.DefaultDatabaseAlias,
		"Nominates a specific database to load fixtures into. Defaults to the \"default\" database.")
	fs.StringVar(&c.appLabel, "app", "", "Only look for fixtures in the specified app.")
	const ignoreHelp = "Ignores entries in the serialized data for fields that do not currently exist on the model."
	fs.BoolVar(&c.ignore, "ignorenonexistent", false, ignoreHelp)
	fs.BoolVar(&c.ignore, "i", false, ignoreHelp)
	const excludeHelp = "An app_label or app_label.ModelName to exclude. Can be used multiple times."
	fs.Var(&c.exclude, "exclude", excludeHelp)
	fs.Var(&c.exclude, "e", excludeHelp)
	fs.StringVar(&c.format, "format", "", "Format of serialized data when reading from stdin.")
	c.verbosity.declare(fs)
}

// Run loads every named fixture inside one database transaction.
func (c *loaddataCommand) Run(fixtureLabels []string) error {
	if len(fixtureLabels) == 0 {
		return &manage.UsageError{
			Msg: "No database fixture specified. Please provide the path of at least one fixture in the command line.",
		}
	}
	a := c.app
	if err := checkDatabaseAliases(a.Settings, []string{c.database}); err != nil {
		return err
	}
	if a.DB == nil {
		return errNoDatabase
	}
	excludedModels, excludedApps, err := parseAppsAndModelLabels(a, c.exclude)
	if err != nil {
		return err
	}

	state := &loaddataState{
		cmd:                  c,
		excludedModels:       excludedModels,
		excludedApps:         excludedApps,
		serializationFormats: serializers.PublicFormats(),
		foundCache:           make(map[string][]fixtureMatch),
		loadedModelSet:       make(map[*orm.Model]bool),
	}

	hasFixtures := false
	for _, label := range fixtureLabels {
		matches, err := state.findFixtures(label)
		if err != nil {
			return err
		}
		if len(matches) > 0 {
			hasFixtures = true
			break
		}
	}
	if !hasFixtures {
		return nil
	}

	ctx := context.Background()
	err = a.DB.Atomic(ctx, func(txCtx context.Context) error {
		for _, label := range fixtureLabels {
			if err := state.loadLabel(txCtx, label); err != nil {
				return err
			}
		}
		for _, obj := range state.objsWithDeferredFields {
			if err := obj.SaveDeferredFields(txCtx, a.DB); err != nil {
				return err
			}
		}

		tableNames := make([]string, len(state.loadedModels))
		for i, m := range state.loadedModels {
			tableNames[i] = m.Table()
		}
		if err := orm.CheckForeignKeys(txCtx, a.DB, tableNames...); err != nil {
			return manage.Errorf("Problem installing fixtures: %w", err)
		}

		if state.loadedObjectCount > 0 {
			if err := state.resetSequences(txCtx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if c.verbosity >= 1 {
		if state.fixtureObjectCount == state.loadedObjectCount {
			_, _ = fmt.Fprintf(a.out(), "Installed %d object(s) from %d fixture(s)\n",
				state.loadedObjectCount, state.fixtureCount)
		} else {
			_, _ = fmt.Fprintf(a.out(), "Installed %d object(s) (of %d) from %d fixture(s)\n",
				state.loadedObjectCount, state.fixtureObjectCount, state.fixtureCount)
		}
	}
	return nil
}

// fixtureSource represents one directory or filesystem searched for fixtures.
type fixtureSource struct {
	fsys fs.FS
	dir  string
}

// fixtureMatch represents one discovered fixture file.
type fixtureMatch struct {
	fsys        fs.FS
	file        string
	displayFile string
	dir         string
	name        string
}

// loaddataState holds the per-run state of a loaddata execution.
type loaddataState struct {
	cmd                    *loaddataCommand
	excludedModels         map[*orm.Model]bool
	excludedApps           map[string]bool
	serializationFormats   []string
	sources                []fixtureSource
	sourcesLoaded          bool
	foundCache             map[string][]fixtureMatch
	fixtureCount           int
	loadedObjectCount      int
	fixtureObjectCount     int
	loadedModels           []*orm.Model
	loadedModelSet         map[*orm.Model]bool
	objsWithDeferredFields []*serializers.DeserializedObject
}

// resetSequences resets database sequences for the loaded models.
func (s *loaddataState) resetSequences(ctx context.Context) error {
	db := s.cmd.app.DB
	if db.Dialect().Name() == "postgres" && s.cmd.verbosity >= 2 {
		hasSeq := false
		for _, m := range s.loadedModels {
			if pk := m.PrimaryKey(); pk != nil && pk.Kind == orm.KindAuto {
				hasSeq = true
				break
			}
		}
		if hasSeq {
			_, _ = fmt.Fprintln(s.cmd.app.out(), "Resetting sequences")
		}
	}
	return orm.ResetSequences(ctx, db, s.loadedModels...)
}

// loadLabel loads all fixture files matching fixtureLabel.
func (s *loaddataState) loadLabel(ctx context.Context, fixtureLabel string) error {
	a := s.cmd.app
	showProgress := s.cmd.verbosity >= 3
	matches, err := s.findFixtures(fixtureLabel)
	if err != nil {
		return err
	}
	for _, match := range matches {
		baseName := match.file
		if match.file != readStdin {
			baseName = filepath.Base(match.file)
		}
		_, serFmt, cmpFmt, err := s.parseName(baseName)
		if err != nil {
			return err
		}
		rc, err := s.openFixture(match, cmpFmt)
		if err != nil {
			return err
		}
		s.fixtureCount++
		objectsInFixture := 0
		loadedObjectsInFixture := 0
		if s.cmd.verbosity >= 2 {
			_, _ = fmt.Fprintf(a.out(), "Installing %s fixture '%s' from %s.\n",
				serFmt, match.name, humanizeFixtureDir(match.dir))
		}

		opts := []serializers.Option{serializers.HandleForwardReferences()}
		if s.cmd.ignore {
			opts = append(opts, serializers.IgnoreNonexistent())
		}

		var loadErr error
		for obj, err := range serializers.Deserialize(ctx, a.DB, serFmt, rc, opts...) {
			if err != nil {
				loadErr = err
				break
			}
			objectsInFixture++
			saved, err := s.saveObj(ctx, obj)
			if err != nil {
				loadErr = err
				break
			}
			if saved {
				loadedObjectsInFixture++
				if showProgress {
					_, _ = fmt.Fprintf(a.out(), "\rProcessed %d object(s).", loadedObjectsInFixture)
				}
			}
		}
		_ = rc.Close()
		if loadErr != nil {
			var cmdErr *manage.CommandError
			if errors.As(loadErr, &cmdErr) {
				return loadErr
			}
			return manage.Errorf("Problem installing fixture '%s': %w", match.displayFile, loadErr)
		}

		if objectsInFixture > 0 && showProgress {
			_, _ = fmt.Fprintln(a.out())
		}
		s.loadedObjectCount += loadedObjectsInFixture
		s.fixtureObjectCount += objectsInFixture
		if objectsInFixture == 0 {
			_, _ = fmt.Fprintf(a.errOut(),
				"No fixture data found for '%s'. (File format may be invalid.)\n", match.name)
		}
	}
	return nil
}

// saveObj saves obj unless its app or model is excluded.
func (s *loaddataState) saveObj(ctx context.Context, obj *serializers.DeserializedObject) (bool, error) {
	if obj == nil || obj.Model == nil {
		return false, nil
	}
	if s.excludedApps[obj.Model.App()] || s.excludedModels[obj.Model] {
		return false, nil
	}
	if !s.loadedModelSet[obj.Model] {
		s.loadedModelSet[obj.Model] = true
		s.loadedModels = append(s.loadedModels, obj.Model)
	}
	if err := obj.Save(ctx, s.cmd.app.DB); err != nil {
		return false, manage.Errorf("Could not load %s(pk=%s): %w",
			obj.Model.Label(), formatDeserializedPK(obj), err)
	}
	if len(obj.DeferredFields) > 0 {
		s.objsWithDeferredFields = append(s.objsWithDeferredFields, obj)
	}
	return true, nil
}

// formatDeserializedPK formats the primary key of obj the way Python's str(pk)
// does.
func formatDeserializedPK(obj *serializers.DeserializedObject) string {
	if obj == nil || obj.Object == nil || obj.Model == nil {
		return "None"
	}
	pkField := obj.Model.PrimaryKey()
	if pkField == nil {
		return "None"
	}
	sv := reflect.Indirect(reflect.ValueOf(obj.Object))
	if !sv.IsValid() || sv.Kind() != reflect.Struct {
		return "None"
	}
	fv := sv.Field(pkField.Index)
	if pkField.Kind == orm.KindAuto && fv.IsZero() {
		return "None"
	}
	return validators.PyString(fv.Interface())
}

// openFixture opens a fixture stream using cmpFmt.
func (s *loaddataState) openFixture(match fixtureMatch, cmpFmt string) (io.ReadCloser, error) {
	if cmpFmt == "stdin" {
		return io.NopCloser(s.cmd.app.in()), nil
	}
	var raw io.ReadCloser
	var err error
	if match.fsys != nil {
		raw, err = match.fsys.Open(match.file)
	} else {
		raw, err = os.Open(match.file)
	}
	if err != nil {
		return nil, err
	}

	switch cmpFmt {
	case "":
		return raw, nil
	case "gz":
		gz, err := gzip.NewReader(raw)
		if err != nil {
			_ = raw.Close()
			return nil, manage.Errorf("Problem installing fixture '%s': %w", match.displayFile, err)
		}
		return &chainedReadCloser{Reader: gz, closers: []io.Closer{gz, raw}}, nil
	case "bz2":
		return &chainedReadCloser{Reader: bzip2.NewReader(raw), closers: []io.Closer{raw}}, nil
	case "zip":
		data, err := io.ReadAll(raw)
		_ = raw.Close()
		if err != nil {
			return nil, manage.Errorf("Problem installing fixture '%s': %w", match.displayFile, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, manage.Errorf("Problem installing fixture '%s': %w", match.displayFile, err)
		}
		if len(zr.File) != 1 {
			return nil, manage.Errorf("Problem installing fixture '%s': Zip-compressed fixtures must contain one file.", match.displayFile)
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			return nil, manage.Errorf("Problem installing fixture '%s': %w", match.displayFile, err)
		}
		return rc, nil
	default:
		_ = raw.Close()
		return nil, manage.Errorf("Problem installing fixture '%s': %s is not a known compression format.", match.displayFile, cmpFmt)
	}
}

// chainedReadCloser closes a chain of underlying closers when Close is called.
type chainedReadCloser struct {
	io.Reader
	closers []io.Closer
}

// Close closes every underlying closer in order and returns the first error.
func (c *chainedReadCloser) Close() error {
	var firstErr error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// findFixtures discovers fixture files for fixtureLabel, caching the result per
// label.
func (s *loaddataState) findFixtures(fixtureLabel string) ([]fixtureMatch, error) {
	if cached, ok := s.foundCache[fixtureLabel]; ok {
		return cached, nil
	}
	if fixtureLabel == readStdin {
		if _, _, _, err := s.parseName(readStdin); err != nil {
			return nil, err
		}
		res := []fixtureMatch{{file: readStdin, displayFile: readStdin, name: readStdin}}
		s.foundCache[fixtureLabel] = res
		return res, nil
	}

	fixtureName, serFmt, cmpFmt, err := s.parseName(fixtureLabel)
	if err != nil {
		return nil, err
	}
	a := s.cmd.app
	if s.cmd.verbosity >= 2 {
		_, _ = fmt.Fprintf(a.out(), "Loading '%s' fixtures...\n", fixtureName)
	}

	baseName, dirs, err := s.getFixtureNameAndDirs(fixtureName)
	if err != nil {
		return nil, err
	}
	targets := s.getTargets(baseName, serFmt, cmpFmt)

	var fixtureFiles []fixtureMatch
	for _, src := range dirs {
		if s.cmd.verbosity >= 2 {
			_, _ = fmt.Fprintf(a.out(), "Checking %s for fixtures...\n", humanizeFixtureDir(src.dir))
		}
		inDir := findFixtureFilesInSource(src, baseName, targets)
		if s.cmd.verbosity >= 2 && len(inDir) == 0 {
			_, _ = fmt.Fprintf(a.out(), "No fixture '%s' in %s.\n", baseName, humanizeFixtureDir(src.dir))
		}
		if len(inDir) > 1 {
			return nil, manage.Errorf("Multiple fixtures named '%s' in %s. Aborting.",
				baseName, humanizeFixtureDir(src.dir))
		}
		fixtureFiles = append(fixtureFiles, inDir...)
	}

	if len(fixtureFiles) == 0 {
		return nil, manage.Errorf("No fixture named '%s' found.", baseName)
	}
	s.foundCache[fixtureLabel] = fixtureFiles
	return fixtureFiles, nil
}

// findFixtureFilesInSource lists files in src matching targets.
func findFixtureFilesInSource(src fixtureSource, fixtureName string, targets map[string]bool) []fixtureMatch {
	var matches []fixtureMatch
	if src.fsys != nil {
		entries, err := fs.ReadDir(src.fsys, ".")
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if targets[name] {
				matches = append(matches, fixtureMatch{
					fsys:        src.fsys,
					file:        name,
					displayFile: path.Join(src.dir, name),
					dir:         src.dir,
					name:        fixtureName,
				})
			}
		}
		return matches
	}

	entries, err := os.ReadDir(src.dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if targets[name] {
			full := filepath.Join(src.dir, name)
			matches = append(matches, fixtureMatch{
				file:        full,
				displayFile: full,
				dir:         src.dir,
				name:        fixtureName,
			})
		}
	}
	return matches
}

// getFixtureNameAndDirs splits fixtureName into its base name and the sources
// to search.
func (s *loaddataState) getFixtureNameAndDirs(fixtureName string) (string, []fixtureSource, error) {
	if filepath.IsAbs(fixtureName) {
		return filepath.Base(fixtureName), []fixtureSource{{dir: filepath.Dir(fixtureName)}}, nil
	}
	sources, err := s.fixtureDirs()
	if err != nil {
		return "", nil, err
	}
	dirName := filepath.Dir(fixtureName)
	baseName := filepath.Base(fixtureName)
	if dirName != "." && dirName != "" {
		subSources := make([]fixtureSource, 0, len(sources))
		slashDir := path.Clean(filepath.ToSlash(dirName))
		for _, src := range sources {
			if src.fsys != nil {
				subFS, err := fs.Sub(src.fsys, slashDir)
				if err != nil {
					subFS = emptyFS{}
				}
				subSources = append(subSources, fixtureSource{
					fsys: subFS,
					dir:  filepath.Join(src.dir, dirName),
				})
			} else {
				subSources = append(subSources, fixtureSource{
					dir: filepath.Join(src.dir, dirName),
				})
			}
		}
		sources = subSources
	}
	return baseName, sources, nil
}

// emptyFS is an fs.FS that contains no files.
type emptyFS struct{}

// Open always returns fs.ErrNotExist.
func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

// fixtureDirs returns the list of fixture sources: each installed app's
// Fixtures() fs.FS, then Settings.FixtureDirs, then the current directory.
func (s *loaddataState) fixtureDirs() ([]fixtureSource, error) {
	if s.sourcesLoaded {
		return s.sources, nil
	}
	a := s.cmd.app
	seen := make(map[string]bool, len(a.Settings.FixtureDirs))
	for _, d := range a.Settings.FixtureDirs {
		if seen[d] {
			return nil, &conf.ImproperlyConfigured{Msg: "settings.FIXTURE_DIRS contains duplicates."}
		}
		seen[d] = true
	}

	var sources []fixtureSource
	for _, cfg := range a.Apps.Configs() {
		appLabel := apps.Label(cfg)
		if s.cmd.appLabel != "" && appLabel != s.cmd.appLabel {
			continue
		}
		fp, ok := cfg.(apps.FixturesProvider)
		if !ok {
			continue
		}
		if fsys := fp.Fixtures(); fsys != nil {
			sources = append(sources, fixtureSource{
				fsys: fsys,
				dir:  filepath.Join(appLabel, "fixtures"),
			})
		}
	}
	for _, d := range a.Settings.FixtureDirs {
		sources = append(sources, fixtureSource{dir: resolveRealPath(d)})
	}
	sources = append(sources, fixtureSource{dir: resolveRealPath("")})
	s.sources = sources
	s.sourcesLoaded = true
	return sources, nil
}

// resolveRealPath resolves dir to an absolute, symlink-evaluated path like
// Python's os.path.realpath.
func resolveRealPath(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// getTargets builds the set of candidate file names for fixtureName, serFmt,
// and cmpFmt.
func (s *loaddataState) getTargets(fixtureName, serFmt, cmpFmt string) map[string]bool {
	databases := []string{s.cmd.database, ""}
	serFmts := s.serializationFormats
	if serFmt != "" {
		serFmts = []string{serFmt}
	}
	cmpFmts := loaddataCompressionFormats
	if cmpFmt != "" {
		cmpFmts = []string{cmpFmt}
	}
	targets := make(map[string]bool, len(databases)*len(serFmts)*len(cmpFmts))
	for _, db := range databases {
		for _, sf := range serFmts {
			for _, cf := range cmpFmts {
				var exts []string
				for _, ext := range []string{db, sf, cf} {
					if ext != "" {
						exts = append(exts, ext)
					}
				}
				if len(exts) > 0 {
					targets[fixtureName+"."+strings.Join(exts, ".")] = true
				}
			}
		}
	}
	return targets
}

// parseName splits fixtureName into (name, serializationFormat,
// compressionFormat), porting loaddata.Command.parse_name.
func (s *loaddataState) parseName(fixtureName string) (name, serFmt, cmpFmt string, err error) {
	if fixtureName == readStdin {
		if s.cmd.format == "" {
			return "", "", "", manage.Errorf("--format must be specified when reading from stdin.")
		}
		return readStdin, s.cmd.format, "stdin", nil
	}

	parts := rsplitMax2(fixtureName, ".")
	if len(parts) > 1 && slices.Contains(loaddataCompressionFormats, parts[len(parts)-1]) {
		cmpFmt = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if slices.Contains(s.serializationFormats, last) {
			serFmt = last
			parts = parts[:len(parts)-1]
		} else {
			return "", "", "", manage.Errorf(
				"Problem installing fixture '%s': %s is not a known serialization format.",
				strings.Join(parts[:len(parts)-1], "."),
				last,
			)
		}
	}
	return strings.Join(parts, "."), serFmt, cmpFmt, nil
}

// rsplitMax2 splits str from the right on sep at most twice, matching Python's
// str.rsplit(sep, 2).
func rsplitMax2(str, sep string) []string {
	last := strings.LastIndex(str, sep)
	if last < 0 {
		return []string{str}
	}
	prev := strings.LastIndex(str[:last], sep)
	if prev < 0 {
		return []string{str[:last], str[last+len(sep):]}
	}
	return []string{str[:prev], str[prev+len(sep) : last], str[last+len(sep):]}
}

// humanizeFixtureDir formats dirname for loaddata messages.
func humanizeFixtureDir(dirname string) string {
	if dirname == "" {
		return "absolute path"
	}
	return "'" + dirname + "'"
}

// parseAppsAndModelLabels parses a slice of "app_label.ModelName" or
// "app_label" strings into sets of *orm.Model and app labels, porting
// django.core.management.utils.parse_apps_and_model_labels.
func parseAppsAndModelLabels(a *Application, labels []string) (map[*orm.Model]bool, map[string]bool, error) {
	models := make(map[*orm.Model]bool)
	appSet := make(map[string]bool)
	for _, label := range labels {
		if strings.Contains(label, ".") {
			appLabel, modelLabel, _ := strings.Cut(label, ".")
			if appLabel == "" || modelLabel == "" || strings.Contains(modelLabel, ".") {
				return nil, nil, manage.Errorf("Unknown model: %s", label)
			}
			if _, err := a.Apps.ByLabel(appLabel); err != nil {
				return nil, nil, manage.Errorf("Unknown model: %s", label)
			}
			model := findAppModel(a.Models, appLabel, modelLabel)
			if model == nil {
				return nil, nil, manage.Errorf("Unknown model: %s", label)
			}
			models[model] = true
		} else {
			cfg, err := a.Apps.ByLabel(label)
			if err != nil {
				return nil, nil, manage.Errorf("%w", err)
			}
			appSet[apps.Label(cfg)] = true
		}
	}
	return models, appSet, nil
}

// findAppModel looks up modelLabel in reg under appLabel, matching the model
// name case-insensitively as Django's AppConfig.get_model does.
func findAppModel(reg *orm.Registry, appLabel, modelLabel string) *orm.Model {
	if reg == nil {
		return nil
	}
	for _, m := range reg.AppModels(appLabel) {
		if strings.EqualFold(m.Name(), modelLabel) {
			return m
		}
	}
	return nil
}
