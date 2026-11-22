package djangogo

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/orm"
)

// removeStaleContentTypesCommand deletes the content types whose model no
// longer exists, with the rows that depend on them (Django's
// remove_stale_contenttypes). It is registered when the contenttypes app is
// installed.
type removeStaleContentTypesCommand struct {
	app              *Application
	noInput          bool
	database         string
	includeStaleApps bool
	verbosity        verbosityValue
	// answers reads the answers of a run; one reader serves every question.
	answers *bufio.Reader
}

// Name returns "remove_stale_contenttypes".
func (*removeStaleContentTypesCommand) Name() string { return "remove_stale_contenttypes" }

// Help returns Django's help text of the command.
func (*removeStaleContentTypesCommand) Help() string {
	return "Deletes stale content types in the database."
}

// Flags declares Django's options of remove_stale_contenttypes. It resets the
// values of an earlier run first.
func (c *removeStaleContentTypesCommand) Flags(fs *flag.FlagSet) {
	c.verbosity = 1
	const noInputHelp = "Tells Django to NOT prompt the user for input of any kind."
	fs.BoolVar(&c.noInput, "noinput", false, noInputHelp)
	fs.BoolVar(&c.noInput, "no-input", false, noInputHelp)
	fs.StringVar(&c.database, "database", conf.DefaultDatabaseAlias,
		"Nominates the database to use. Defaults to the \"default\" database.")
	fs.BoolVar(&c.includeStaleApps, "include-stale-apps", false,
		"Deletes stale content types including ones from previously installed apps that have been removed from INSTALLED_APPS.")
	c.verbosity.declare(fs)
}

// Run deletes the stale content types of each installed app, and of the
// apps that are no longer installed with --include-stale-apps. Without
// --noinput it lists them with the rows that depend on them and asks first.
func (c *removeStaleContentTypesCommand) Run(_ []string) error {
	a := c.app
	if err := checkDatabaseAliases(a.Settings, []string{c.database}); err != nil {
		return err
	}
	db := a.DB
	if db == nil {
		return errNoDatabase
	}
	ctx := context.Background()
	c.answers = bufio.NewReader(a.in())
	contenttypes.ClearCache(db)
	all, err := orm.Query[contenttypes.ContentType](db).OrderBy("app_label", "model").All(ctx)
	if err != nil {
		return err
	}
	for len(all) > 0 {
		label := all[0].AppLabel
		n := 1
		for n < len(all) && all[n].AppLabel == label {
			n++
		}
		group := all[:n]
		all = all[n:]
		if _, err := a.Apps.ByLabel(label); err != nil && !c.includeStaleApps {
			continue
		}
		var stale []contenttypes.ContentType
		for _, ct := range group {
			if _, ok := contenttypes.ModelClass(db, ct); !ok {
				stale = append(stale, ct)
			}
		}
		if len(stale) == 0 {
			continue
		}
		ok := true
		if !c.noInput {
			if ok, err = c.confirm(ctx, db, stale); err != nil {
				return err
			}
		}
		if !ok {
			if c.verbosity >= 2 {
				_, _ = fmt.Fprintln(a.out(), "Stale content types remain.")
			}
			continue
		}
		for _, ct := range stale {
			if c.verbosity >= 2 {
				_, _ = fmt.Fprintf(a.out(), "Deleting stale content type '%s | %s'\n", ct.AppLabel, ct.Model)
			}
			if _, err := orm.Query[contenttypes.ContentType](db).Filter("pk", ct.ID).Delete(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// confirm lists the stale content types with the rows that deleting them
// deletes, asks Django's question and reports whether the answer is "yes".
func (c *removeStaleContentTypesCommand) confirm(ctx context.Context, db *orm.DB, stale []contenttypes.ContentType) (bool, error) {
	ctModel, ok := db.Registry().ModelOf(contenttypes.ContentType{})
	if !ok {
		return false, errors.New("djangogo: the ContentType model is not registered")
	}
	var info []string
	for _, ct := range stale {
		info = append(info, fmt.Sprintf("    - Content type for %s.%s", ct.AppLabel, ct.Model))
		plan, err := orm.CollectDelete(ctx, db, ctModel, []any{ct.ID})
		if err != nil {
			return false, err
		}
		for _, g := range plan.Groups {
			if g.Model == ctModel {
				continue
			}
			info = append(info, fmt.Sprintf("    - %d %s object(s)", len(g.Objects), g.Model.Label()))
		}
	}
	_, _ = io.WriteString(c.app.out(), "Some content types in your database are stale and can be deleted.\n"+
		"Any objects that depend on these content types will also be deleted.\n"+
		"The content types and dependent objects that would be deleted are:\n\n"+
		strings.Join(info, "\n")+"\n\n"+
		"This list doesn't include any cascade deletions to data outside of Django\n"+
		"models (uncommon).\n\n"+
		"Are you sure you want to delete these content types?\n"+
		"If you're unsure, answer 'no'.\n"+
		"Type 'yes' to continue, or 'no' to cancel: ")
	answer, err := c.answers.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("djangogo: read answer: %w", err)
	}
	return strings.TrimRight(answer, "\r\n") == "yes", nil
}
