package auth

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/orm"
)

// ModelPermission is one permission a model declares: the codename
// ("publish_post") and the readable name ("Can publish post").
type ModelPermission struct {
	Codename string
	Name     string
}

// PermissionsProvider is implemented by a model that declares permissions
// beyond the default ones (Django's Meta.permissions). SyncPermissions creates
// them.
type PermissionsProvider interface {
	Permissions() []ModelPermission
}

// DefaultPermissionsProvider is implemented by a model that changes the
// actions it gets default permissions for (Django's Meta.default_permissions).
// A model without it gets "add", "change", "delete" and "view".
type DefaultPermissionsProvider interface {
	DefaultPermissions() []string
}

// defaultActions is Django's default_permissions.
var defaultActions = []string{"add", "change", "delete", "view"}

// SyncPermissions creates the missing permissions of every registered model:
// "add", "change", "delete" and "view" (named "Can add post" and so on), plus
// the ones a model declares. It is the counterpart of the create_permissions
// handler Django runs after migrate, creates the content types it needs first,
// and is safe to run again. It returns the created permissions.
//
// SyncPermissions does nothing when the contenttypes app or the auth app is
// not installed.
func SyncPermissions(ctx context.Context, db *orm.DB) ([]Permission, error) {
	reg := db.Registry()
	if _, ok := reg.ModelOf(&contenttypes.ContentType{}); !ok {
		return nil, nil
	}
	if _, ok := reg.ModelOf(&Permission{}); !ok {
		return nil, nil
	}

	// Sync reads the table afresh, so a content type that was removed behind
	// the cache is created again before a permission refers to it.
	if _, err := contenttypes.Sync(ctx, db); err != nil {
		return nil, err
	}
	models := reg.Models()
	cts, err := contenttypes.GetForModels(ctx, db, models...)
	if err != nil {
		return nil, err
	}
	ctIDs := make([]int64, 0, len(models))
	for _, m := range models {
		ctIDs = append(ctIDs, cts[m].ID)
	}

	existing, err := orm.Query[Permission](db).Filter("content_type_id__in", ctIDs).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: load permissions: %w", err)
	}
	type permKey struct {
		contentType int64
		codename    string
	}
	have := make(map[permKey]bool, len(existing))
	for _, p := range existing {
		have[permKey{p.ContentTypeID, p.Codename}] = true
	}

	var created []Permission
	for _, m := range models {
		ct := cts[m]
		for _, mp := range modelPermissionsOf(m) {
			key := permKey{ct.ID, mp.Codename}
			if have[key] {
				continue
			}
			have[key] = true
			p := Permission{Name: mp.Name, ContentTypeID: ct.ID, Codename: mp.Codename}
			if err := orm.Query[Permission](db).Create(ctx, &p); err != nil {
				return created, fmt.Errorf("auth: create permission %s.%s: %w", ct.AppLabel, mp.Codename, err)
			}
			created = append(created, p)
		}
	}
	return created, nil
}

// modelPermissionsOf returns the default permissions of m followed by the
// ones it declares.
func modelPermissionsOf(m *orm.Model) []ModelPermission {
	instance := reflect.New(m.GoType()).Interface()
	actions := defaultActions
	if dp, ok := instance.(DefaultPermissionsProvider); ok {
		actions = dp.DefaultPermissions()
	}
	modelName := strings.ToLower(m.Name())
	verboseName := camelCaseToSpaces(m.Name())
	perms := make([]ModelPermission, 0, len(actions))
	for _, action := range actions {
		perms = append(perms, ModelPermission{
			Codename: action + "_" + modelName,
			Name:     "Can " + action + " " + verboseName,
		})
	}
	if pp, ok := instance.(PermissionsProvider); ok {
		perms = append(perms, pp.Permissions()...)
	}
	return perms
}

// camelCaseToSpaces splits a type name at its capitals and lowercases it, as
// Django derives a verbose name: "ContentType" becomes "content type".
func camelCaseToSpaces(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			afterLower := unicode.IsLower(runes[i-1])
			beforeNonUpper := i+1 < len(runes) && !unicode.IsUpper(runes[i+1])
			if afterLower || beforeNonUpper {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimSpace(b.String())
}

// Label returns "<content type> | <name>", such as "auth | group | Can view
// group", Django's __str__. It labels the options of the permission fields of
// the forms and the admin (see orm.ContextLabeler). The content type reads as
// contenttypes.ContentType.String does, from the stored app label and model
// name.
func (p Permission) Label(ctx context.Context, db *orm.DB) (string, error) {
	ct, err := contenttypes.GetForID(ctx, db, p.ContentTypeID)
	if err != nil {
		return "", err
	}
	return ct.String() + " | " + p.Name, nil
}

// UserPermStr returns the permission in the form HasPerm takes,
// "<app_label>.<codename>" (Django's user_perm_str).
func (p Permission) UserPermStr(ctx context.Context, db *orm.DB) (string, error) {
	ct, err := contenttypes.GetForID(ctx, db, p.ContentTypeID)
	if err != nil {
		return "", err
	}
	return ct.AppLabel + "." + p.Codename, nil
}

// NaturalKeyContext returns the three-element slice [codename, appLabel, model]
// that identifies p independently of its primary key, loading its content type
// from db when it is not cached yet (Django's Permission.natural_key).
func (p Permission) NaturalKeyContext(ctx context.Context, db *orm.DB) ([]any, error) {
	ct, err := contenttypes.GetForID(ctx, db, p.ContentTypeID)
	if err != nil {
		return nil, err
	}
	return []any{p.Codename, ct.AppLabel, ct.Model}, nil
}

// NaturalKeyDependencies declares that Permission's natural key depends on
// contenttypes.contenttype (Django's natural_key.dependencies).
func (Permission) NaturalKeyDependencies() []string {
	return []string{"contenttypes.contenttype"}
}

// GetByNaturalKey looks up a permission by its (codename, app_label, model)
// natural key (Django's PermissionManager.get_by_natural_key).
func (*Permission) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 3 {
		return nil, orm.ErrDoesNotExist
	}
	codename, ok1 := key[0].(string)
	appLabel, ok2 := key[1].(string)
	model, ok3 := key[2].(string)
	if !ok1 || !ok2 || !ok3 {
		return nil, orm.ErrDoesNotExist
	}
	ct, err := contenttypes.GetByNaturalKey(ctx, db, appLabel, model)
	if err != nil {
		return nil, err
	}
	p, err := orm.Query[Permission](db).Get(ctx, "codename", codename, "content_type_id", ct.ID)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// permSource selects the permissions of a user by the way they were granted.
type permSource int

const (
	// fromUser selects the permissions granted to the user directly.
	fromUser permSource = iota
	// fromGroup selects the permissions granted through groups.
	fromGroup
)

// permCache holds the permission sets loaded for one User value.
type permCache struct {
	mu   sync.Mutex
	sets [2]map[string]struct{}
}

// permCacheInit guards the creation of a user's cache.
var permCacheInit sync.Mutex

// cache returns the permission cache of u, creating it on first use.
func (u *User) cache() *permCache {
	permCacheInit.Lock()
	defer permCacheInit.Unlock()
	if u.perms == nil {
		u.perms = &permCache{}
	}
	return u.perms
}

// modelPermissions returns the permissions of u from one source as a set of
// "<app_label>.<codename>" strings (Django's ModelBackend._get_permissions).
// An inactive or anonymous user has none and an active superuser has all. The
// set is cached on u and must not be changed by the caller.
func modelPermissions(ctx context.Context, db *orm.DB, u *User, from permSource) (map[string]struct{}, error) {
	if u == nil || !u.IsActive || u.IsAnonymous() {
		return nil, nil
	}
	c := u.cache()
	c.mu.Lock()
	defer c.mu.Unlock()
	if set := c.sets[from]; set != nil {
		return set, nil
	}

	var perms []Permission
	var err error
	switch {
	case u.IsSuperuser:
		perms, err = orm.Query[Permission](db).All(ctx)
	case from == fromUser:
		perms, err = userPermissionRows(ctx, db, u.ID)
	default:
		perms, err = groupPermissionRows(ctx, db, u.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load permissions: %w", err)
	}

	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		ct, err := contenttypes.GetForID(ctx, db, p.ContentTypeID)
		if errors.Is(err, orm.ErrDoesNotExist) {
			// The content type is gone, so the permission names no model.
			continue
		}
		if err != nil {
			return nil, err
		}
		set[ct.AppLabel+"."+p.Codename] = struct{}{}
	}
	c.sets[from] = set
	return set, nil
}

// userPermissionRows returns the permissions granted to a user directly.
func userPermissionRows(ctx context.Context, db *orm.DB, userID int64) ([]Permission, error) {
	return orm.Query[Permission](db).Filter("user__id", userID).All(ctx)
}

// groupPermissionRows returns the permissions granted to the groups of a
// user, Django's Permission.objects.filter(group__user=user).
func groupPermissionRows(ctx context.Context, db *orm.DB, userID int64) ([]Permission, error) {
	return orm.Query[Permission](db).Filter("group__user__id", userID).All(ctx)
}

// sortedPerms returns the members of set in sorted order. The result is never
// nil.
func sortedPerms(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for perm := range set {
		out = append(out, perm)
	}
	slices.Sort(out)
	return out
}

// permApp returns the app label of a permission string, the part before the
// first dot.
func permApp(perm string) string {
	app, _, _ := strings.Cut(perm, ".")
	return app
}

// collectPerms gathers the permissions every backend reports through get into
// one sorted list without duplicates.
func collectPerms(get func(Backend) ([]string, error)) ([]string, error) {
	set := make(map[string]struct{})
	for _, b := range backends() {
		perms, err := get(b)
		if err != nil {
			return nil, err
		}
		for _, perm := range perms {
			set[perm] = struct{}{}
		}
	}
	return sortedPerms(set), nil
}

// backendUserPerms returns the user permissions of a backend, or none when it
// has no notion of them.
func backendUserPerms(ctx context.Context, db *orm.DB, b Backend, u *User) ([]string, error) {
	if pb, ok := b.(UserPermissionsBackend); ok {
		return pb.GetUserPermissions(ctx, db, u)
	}
	return nil, nil
}

// backendGroupPerms returns the group permissions of a backend, or none when
// it has no notion of them.
func backendGroupPerms(ctx context.Context, db *orm.DB, b Backend, u *User) ([]string, error) {
	if pb, ok := b.(GroupPermissionsBackend); ok {
		return pb.GetGroupPermissions(ctx, db, u)
	}
	return nil, nil
}

// GetUserPermissions returns the permissions granted to u directly, from
// every backend, sorted, each as "<app_label>.<codename>" (Django's
// user.get_user_permissions).
func GetUserPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error) {
	return collectPerms(func(b Backend) ([]string, error) { return backendUserPerms(ctx, db, b, u) })
}

// GetGroupPermissions returns the permissions u has through its groups, from
// every backend, sorted (Django's user.get_group_permissions).
func GetGroupPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error) {
	return collectPerms(func(b Backend) ([]string, error) { return backendGroupPerms(ctx, db, b, u) })
}

// GetAllPermissions returns the user and group permissions of u from every
// backend, sorted (Django's user.get_all_permissions). An inactive user has
// none and an active superuser has all.
func GetAllPermissions(ctx context.Context, db *orm.DB, u *User) ([]string, error) {
	return collectPerms(func(b Backend) ([]string, error) {
		user, err := backendUserPerms(ctx, db, b, u)
		if err != nil {
			return nil, err
		}
		group, err := backendGroupPerms(ctx, db, b, u)
		if err != nil {
			return nil, err
		}
		return append(user, group...), nil
	})
}

// HasPerm reports whether u holds the permission perm, given as
// "<app_label>.<codename>" (Django's user.has_perm). An active superuser holds
// every permission, an inactive user none; otherwise the backends are asked in
// order until one grants it.
func HasPerm(ctx context.Context, db *orm.DB, u *User, perm string) (bool, error) {
	if u == nil {
		return false, nil
	}
	if u.IsActive && u.IsSuperuser {
		return true, nil
	}
	for _, b := range backends() {
		ok, err := backendHasPerm(ctx, db, b, u, perm)
		if errors.Is(err, ErrPermissionDenied) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// backendHasPerm asks one backend for a permission. A backend that does not
// decide it itself holds the permission when it lists it.
func backendHasPerm(ctx context.Context, db *orm.DB, b Backend, u *User, perm string) (bool, error) {
	if pc, ok := b.(PermissionChecker); ok {
		return pc.HasPerm(ctx, db, u, perm)
	}
	user, err := backendUserPerms(ctx, db, b, u)
	if err != nil {
		return false, err
	}
	if slices.Contains(user, perm) {
		return true, nil
	}
	group, err := backendGroupPerms(ctx, db, b, u)
	if err != nil {
		return false, err
	}
	return slices.Contains(group, perm), nil
}

// HasPerms reports whether u holds every one of perms (Django's
// user.has_perms).
func HasPerms(ctx context.Context, db *orm.DB, u *User, perms ...string) (bool, error) {
	for _, perm := range perms {
		ok, err := HasPerm(ctx, db, u, perm)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// HasModulePerms reports whether u holds any permission of the app labeled
// appLabel (Django's user.has_module_perms). An active superuser does.
func HasModulePerms(ctx context.Context, db *orm.DB, u *User, appLabel string) (bool, error) {
	if u == nil {
		return false, nil
	}
	if u.IsActive && u.IsSuperuser {
		return true, nil
	}
	for _, b := range backends() {
		mc, ok := b.(ModulePermissionChecker)
		if !ok {
			continue
		}
		has, err := mc.HasModulePerms(ctx, db, u, appLabel)
		if errors.Is(err, ErrPermissionDenied) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}
