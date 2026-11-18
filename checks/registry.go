package checks

import (
	"slices"
	"sync"
)

// The tags of Django's built-in checks (django.core.checks.Tags). A check may
// carry any other tag as well.
const (
	TagAdmin         = "admin"
	TagAsyncSupport  = "async_support"
	TagCaches        = "caches"
	TagCommands      = "commands"
	TagCompatibility = "compatibility"
	TagDatabase      = "database"
	TagFiles         = "files"
	TagModels        = "models"
	TagSecurity      = "security"
	TagSignals       = "signals"
	TagSites         = "sites"
	TagStaticfiles   = "staticfiles"
	TagTemplates     = "templates"
	TagTranslation   = "translation"
	TagURLs          = "urls"
)

// TagSettings is the tag of the checks that validate conf.Settings. Django has
// no such tag: it validates most settings where it reads them.
const TagSettings = "settings"

// Registry holds check functions with their tags
// (django.core.checks.registry.CheckRegistry). The zero value is an empty
// registry. A Registry is safe for concurrent use.
type Registry struct {
	mu     sync.RWMutex
	checks []Check
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a check that carries the given tags (Django's
// register(check, *tags)). It panics when fn is nil.
func (r *Registry) Register(fn Func, tags ...string) {
	r.Add(Check{Run: fn, Tags: tags})
}

// RegisterDeploy adds a deployment check: one that runs only when
// RunOptions.Deploy is set (Django's register(check, *tags, deploy=True)). It
// panics when fn is nil.
func (r *Registry) RegisterDeploy(fn Func, tags ...string) {
	r.Add(Check{Run: fn, Tags: tags, Deploy: true})
}

// Add adds prepared checks, such as the ones an app returns from its Checks
// method. It panics when a check has no function: a registration that cannot
// run is a programming error, and failing here names the place that made it.
func (r *Registry) Add(checks ...Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range checks {
		if c.Run == nil {
			panic("checks: a check without a function was registered")
		}
		// Copy the tags so that a caller that reuses its slice cannot retag
		// the registered check.
		c.Tags = slices.Clone(c.Tags)
		r.checks = append(r.checks, c)
	}
}

// Clone returns an independent copy of the registry: a check added to one is
// not in the other. A project uses a clone of Default to add checks of its own
// to the built-in ones.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &Registry{checks: slices.Clone(r.checks)}
}

// RunOptions selects the checks of a run.
type RunOptions struct {
	// Tags limits the run to the checks that carry at least one of the tags
	// (check --tag). Empty selects every check.
	Tags []string
	// Deploy includes the deployment checks (check --deploy).
	Deploy bool
}

// Run runs the selected checks and returns their messages in the order the
// checks were registered (Django's run_checks). Every check receives ctx; a
// nil ctx stands for an empty Context.
//
// A check tagged TagDatabase does more than read the configuration, so it runs
// only when ctx names Databases or when opts.Tags selects it.
func (r *Registry) Run(ctx *Context, opts RunOptions) []Message {
	if ctx == nil {
		ctx = &Context{}
	}
	var messages []Message
	for _, c := range r.selected(opts.Deploy) {
		if len(opts.Tags) > 0 {
			if !sharesTag(c.Tags, opts.Tags) {
				continue
			}
		} else if len(ctx.Databases) == 0 && slices.Contains(c.Tags, TagDatabase) {
			continue
		}
		messages = append(messages, c.Run(ctx)...)
	}
	return messages
}

// TagExists reports whether a check of the run carries tag: any check when
// deploy is set, else any check that is not a deployment check (Django's
// tag_exists).
func (r *Registry) TagExists(tag string, deploy bool) bool {
	for _, c := range r.selected(deploy) {
		if slices.Contains(c.Tags, tag) {
			return true
		}
	}
	return false
}

// TagsAvailable returns the tags of the checks of a run, sorted and each one
// once (Django's tags_available). The tags of deployment checks are included
// only when deploy is set.
func (r *Registry) TagsAvailable(deploy bool) []string {
	var tags []string
	for _, c := range r.selected(deploy) {
		tags = append(tags, c.Tags...)
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

// selected returns the checks of a run in registration order: all of them when
// deploy is set, else the ones that are not deployment checks. The result is a
// copy, so the checks run without the lock held and may use the registry.
func (r *Registry) selected(deploy bool) []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Check, 0, len(r.checks))
	for _, c := range r.checks {
		if deploy || !c.Deploy {
			out = append(out, c)
		}
	}
	return out
}

// sharesTag reports whether have and want share a tag.
func sharesTag(have, want []string) bool {
	for _, tag := range have {
		if slices.Contains(want, tag) {
			return true
		}
	}
	return false
}

// Default is the registry of the process. It holds the built-in checks; other
// packages add theirs with Register and RegisterDeploy in an init function.
var Default = NewRegistry()

// Register adds a check that carries the given tags to Default.
func Register(fn Func, tags ...string) {
	Default.Register(fn, tags...)
}

// RegisterDeploy adds a deployment check that carries the given tags to
// Default.
func RegisterDeploy(fn Func, tags ...string) {
	Default.RegisterDeploy(fn, tags...)
}
