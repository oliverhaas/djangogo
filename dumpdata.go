package djangogo

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// dumpdataCommand outputs the contents of the database as a fixture of the
// given format (Django's dumpdata).
type dumpdataCommand struct {
	app                   *Application
	format                string
	indent                optionalInt
	database              string
	exclude               stringList
	useNaturalForeignKeys bool
	useNaturalPrimaryKeys bool
	useBaseManager        bool
	primaryKeys           string
	output                string
	verbosity             verbosityValue
}

// Name returns "dumpdata".
func (*dumpdataCommand) Name() string { return "dumpdata" }

// Help returns Django's help text of the command.
func (*dumpdataCommand) Help() string {
	return "Output the contents of the database as a fixture of the given format " +
		"(using each model's default manager unless --all is specified)."
}

// Usage names the positional arguments.
func (*dumpdataCommand) Usage() string { return "[app_label[.ModelName] ...]" }

// Flags declares Django's options of dumpdata. It resets the values of an
// earlier run first.
func (c *dumpdataCommand) Flags(fs *flag.FlagSet) {
	*c = dumpdataCommand{
		app:       c.app,
		format:    "json",
		indent:    optionalInt{val: -1},
		database:  conf.DefaultDatabaseAlias,
		verbosity: 1,
	}
	fs.StringVar(&c.format, "format", "json", "Specifies the output serialization format for fixtures.")
	fs.Var(&c.indent, "indent", "Specifies the indent level to use when pretty-printing output.")
	fs.StringVar(&c.database, "database", conf.DefaultDatabaseAlias,
		"Nominates a specific database to dump fixtures from. Defaults to the \"default\" database.")
	const excludeHelp = "An app_label or app_label.ModelName to exclude (use multiple --exclude to exclude multiple apps/models)."
	fs.Var(&c.exclude, "exclude", excludeHelp)
	fs.Var(&c.exclude, "e", excludeHelp)
	fs.BoolVar(&c.useNaturalForeignKeys, "natural-foreign", false, "Use natural foreign keys if they are available.")
	fs.BoolVar(&c.useNaturalPrimaryKeys, "natural-primary", false, "Use natural primary keys if they are available.")
	const allHelp = "Use Django's base manager to dump all models stored in the database, " +
		"including those that would otherwise be filtered or modified by a custom manager."
	fs.BoolVar(&c.useBaseManager, "all", false, allHelp)
	fs.BoolVar(&c.useBaseManager, "a", false, allHelp)
	fs.StringVar(&c.primaryKeys, "pks", "",
		"Only dump objects with given primary keys. Accepts a comma-separated list of keys. "+
			"This option only works when you specify one model.")
	const outputHelp = "Specifies file to which the output is written."
	fs.StringVar(&c.output, "output", "", outputHelp)
	fs.StringVar(&c.output, "o", "", outputHelp)
	c.verbosity.declare(fs)
}

// Run queries the selected models and serializes their rows in primary-key
// order.
func (c *dumpdataCommand) Run(appLabels []string) error {
	a := c.app
	if err := checkDatabaseAliases(a.Settings, []string{c.database}); err != nil {
		return err
	}
	if a.DB == nil {
		return errNoDatabase
	}

	var primaryKeys []string
	if c.primaryKeys != "" {
		for _, pk := range strings.Split(c.primaryKeys, ",") {
			primaryKeys = append(primaryKeys, strings.TrimSpace(pk))
		}
	}

	excludedModels, excludedApps, err := parseAppsAndModelLabels(a, c.exclude)
	if err != nil {
		return err
	}

	models, err := c.collectModels(appLabels, primaryKeys, excludedModels, excludedApps)
	if err != nil {
		return err
	}

	if !slices.Contains(serializers.PublicFormats(), c.format) {
		return manage.Errorf("Unknown serialization format: %s", c.format)
	}

	ctx := context.Background()
	var progressOutput io.Writer
	objectCount := 0
	if c.output != "" && isTTY(a.out()) && c.verbosity > 0 {
		progressOutput = a.out()
		count, err := c.countObjects(ctx, models, primaryKeys)
		if err != nil {
			return manage.Errorf("Unable to serialize database: %s", err)
		}
		objectCount = count
	}

	objs, err := c.fetchObjects(ctx, models, primaryKeys)
	if err != nil {
		return manage.Errorf("Unable to serialize database: %s", err)
	}

	var opts []serializers.Option
	if c.indent.set {
		opts = append(opts, serializers.Indent(c.indent.val))
	}
	if c.useNaturalForeignKeys {
		opts = append(opts, serializers.NaturalForeignKeys())
	}
	if c.useNaturalPrimaryKeys {
		opts = append(opts, serializers.NaturalPrimaryKeys())
	}
	if progressOutput != nil {
		opts = append(opts, serializers.Progress(progressOutput, objectCount))
	}

	w, closeFn, err := c.openOutput()
	if err != nil {
		return manage.Errorf("Unable to serialize database: %s", err)
	}
	serErr := serializers.SerializeTo(ctx, a.DB, w, c.format, objs, opts...)
	closeErr := closeFn()
	if serErr != nil {
		return manage.Errorf("Unable to serialize database: %s", serErr)
	}
	if closeErr != nil {
		return manage.Errorf("Unable to serialize database: %s", closeErr)
	}
	return nil
}

// appModelSelection tracks the models requested for one app in dumpdata.
type appModelSelection struct {
	appLabel  string
	allModels bool
	models    []*orm.Model
}

// collectModels resolves appLabels, excludedApps, and excludedModels into the
// ordered slice of models to dump.
func (c *dumpdataCommand) collectModels(
	appLabels []string,
	primaryKeys []string,
	excludedModels map[*orm.Model]bool,
	excludedApps map[string]bool,
) ([]*orm.Model, error) {
	a := c.app
	var order []string
	selections := make(map[string]*appModelSelection)

	if len(appLabels) == 0 {
		if len(primaryKeys) > 0 {
			return nil, manage.Errorf("You can only use --pks option with one model")
		}
		for _, cfg := range a.Apps.Configs() {
			if _, ok := cfg.(apps.ModelProvider); !ok {
				continue
			}
			label := apps.Label(cfg)
			if excludedApps[label] {
				continue
			}
			order = append(order, label)
			selections[label] = &appModelSelection{appLabel: label, allModels: true}
		}
	} else {
		if len(appLabels) > 1 && len(primaryKeys) > 0 {
			return nil, manage.Errorf("You can only use --pks option with one model")
		}
		for _, label := range appLabels {
			if strings.Count(label, ".") == 1 {
				appLabel, modelLabel, _ := strings.Cut(label, ".")
				cfg, err := a.Apps.ByLabel(appLabel)
				if err != nil {
					return nil, manage.Errorf("%w", err)
				}
				if _, ok := cfg.(apps.ModelProvider); !ok || excludedApps[appLabel] {
					continue
				}
				model := findAppModel(a.Models, appLabel, modelLabel)
				if model == nil {
					return nil, manage.Errorf("Unknown model: %s.%s", appLabel, modelLabel)
				}
				sel, exists := selections[appLabel]
				if !exists {
					sel = &appModelSelection{appLabel: appLabel}
					selections[appLabel] = sel
					order = append(order, appLabel)
				}
				if !sel.allModels && !slices.Contains(sel.models, model) {
					sel.models = append(sel.models, model)
				}
			} else {
				if len(primaryKeys) > 0 {
					return nil, manage.Errorf("You can only use --pks option with one model")
				}
				cfg, err := a.Apps.ByLabel(label)
				if err != nil {
					return nil, manage.Errorf("%w", err)
				}
				appLabel := apps.Label(cfg)
				if _, ok := cfg.(apps.ModelProvider); !ok || excludedApps[appLabel] {
					continue
				}
				sel, exists := selections[appLabel]
				if !exists {
					sel = &appModelSelection{appLabel: appLabel}
					selections[appLabel] = sel
					order = append(order, appLabel)
				}
				sel.allModels = true
				sel.models = nil
			}
		}
	}

	var models []*orm.Model
	for _, appLabel := range order {
		sel := selections[appLabel]
		if sel.allModels {
			models = append(models, a.Models.AppModels(appLabel)...)
		} else {
			models = append(models, sel.models...)
		}
	}

	if c.useNaturalForeignKeys {
		sorted, err := serializers.SortDependencies(a.Models, models, true)
		if err != nil {
			return nil, manage.Errorf("Unable to serialize database: %s", err)
		}
		models = sorted
	}

	kept := make([]*orm.Model, 0, len(models))
	for _, m := range models {
		if !excludedModels[m] {
			kept = append(kept, m)
		}
	}
	return kept, nil
}

// buildModelQuerySet returns the primary-key-ordered ModelQuerySet for m,
// filtered by primaryKeys when non-empty.
func (c *dumpdataCommand) buildModelQuerySet(m *orm.Model, primaryKeys []string) *orm.ModelQuerySet {
	qs := orm.QueryModel(c.app.DB, m).OrderBy("pk")
	if len(primaryKeys) > 0 {
		qs = qs.Filter("pk__in", convertPKFilterValues(m.PrimaryKey(), primaryKeys))
	}
	return qs
}

// convertPKFilterValues converts string primary-key arguments into typed values
// suitable for an "in" lookup on pkField.
func convertPKFilterValues(pkField *orm.Field, primaryKeys []string) []any {
	out := make([]any, len(primaryKeys))
	isInt := false
	if pkField != nil {
		switch pkField.Kind {
		case orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
			orm.KindPositiveSmallInt, orm.KindPositiveInt, orm.KindPositiveBigInt:
			isInt = true
		}
	}
	for i, s := range primaryKeys {
		if isInt {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				out[i] = n
				continue
			}
		}
		out[i] = s
	}
	return out
}

// countObjects returns the total number of objects to be serialized across
// models.
func (c *dumpdataCommand) countObjects(ctx context.Context, models []*orm.Model, primaryKeys []string) (int, error) {
	total := 0
	for _, m := range models {
		n, err := c.buildModelQuerySet(m, primaryKeys).OrderBy().Count(ctx)
		if err != nil {
			return 0, err
		}
		total += int(n)
	}
	return total, nil
}

// fetchObjects loads all rows to be serialized across models in primary-key
// order.
func (c *dumpdataCommand) fetchObjects(ctx context.Context, models []*orm.Model, primaryKeys []string) ([]any, error) {
	var all []any
	for _, m := range models {
		rows, err := c.buildModelQuerySet(m, primaryKeys).All(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

// openOutput opens the destination writer for dumpdata (stdout, a plain file,
// or a .gz file), stripping unsupported compression extensions (.bz2, .lzma,
// .xz, .zip) and writing a warning to the application's error stream as
// Django's dumpdata does.
func (c *dumpdataCommand) openOutput() (io.Writer, func() error, error) {
	if c.output == "" {
		return c.app.out(), func() error { return nil }, nil
	}
	fileRoot, fileExt := splitExt(c.output)
	filePath := c.output
	useGzip := false
	switch fileExt {
	case ".gz":
		useGzip = true
	case ".bz2", ".lzma", ".xz", ".zip":
		filePath = fileRoot
	}
	if filePath != c.output {
		fileName := filepath.Base(filePath)
		_, _ = fmt.Fprintf(
			c.app.errOut(),
			"Unsupported file extension (%s). Fixtures saved in '%s'.\n",
			fileExt,
			fileName,
		)
	}
	f, err := os.Create(filePath)
	if err != nil {
		return nil, nil, err
	}
	if useGzip {
		gz := gzip.NewWriter(f)
		return gz, func() error {
			err1 := gz.Close()
			err2 := f.Close()
			if err1 != nil {
				return err1
			}
			return err2
		}, nil
	}
	return f, f.Close, nil
}

// splitExt splits path into (root, ext) matching Python's os.path.splitext.
func splitExt(p string) (root, ext string) {
	base := filepath.Base(p)
	trimmed := strings.TrimLeft(base, ".")
	if trimmed == "" || !strings.Contains(trimmed, ".") {
		return p, ""
	}
	ext = filepath.Ext(p)
	return strings.TrimSuffix(p, ext), ext
}

// optionalInt is an integer flag that tracks whether it was explicitly set.
type optionalInt struct {
	val int
	set bool
}

// String returns the integer value when set, or the empty string otherwise.
func (o *optionalInt) String() string {
	if o == nil || !o.set {
		return ""
	}
	return strconv.Itoa(o.val)
}

// Set parses an integer flag value.
func (o *optionalInt) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid int value: '%s'", s)
	}
	o.val = n
	o.set = true
	return nil
}

// isTTY reports whether w is a terminal character device or implements
// IsTTY() bool returning true.
func isTTY(w io.Writer) bool {
	if t, ok := w.(interface{ IsTTY() bool }); ok {
		return t.IsTTY()
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
