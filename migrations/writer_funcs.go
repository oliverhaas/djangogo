package migrations

import (
	"fmt"
	"go/token"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// The import paths of the packages every migration file can name.
var (
	migrationsImportPath = reflect.TypeFor[Migration]().PkgPath()
	ormImportPath        = reflect.TypeFor[orm.DB]().PkgPath()
)

// goFunc renders a RunGo function as a reference to a top-level function.
// Django writes a function as its module and name and refuses a lambda. A
// closure, a method value or an instantiated generic function has no name
// that a file can refer to, so it is refused too.
func (w *writer) goFunc(fn GoFunc) (string, error) {
	pc := reflect.ValueOf(fn).Pointer()
	f := runtime.FuncForPC(pc)
	if f == nil {
		return "", fmt.Errorf("%w: cannot serialize function: unknown function", ErrNotSerializable)
	}
	pkg, name, ok := splitFuncName(f.Name())
	if !ok {
		return "", fmt.Errorf("%w: cannot serialize function %s: only top-level functions can be written",
			ErrNotSerializable, f.Name())
	}
	if file, _ := f.FileLine(pc); isMigrationFile(file) {
		if w.porting == nil {
			w.porting = map[string]bool{}
		}
		w.porting[pkg+"/"+filepath.Base(file)] = true
	}
	switch pkg {
	case w.importPath:
		return name, nil
	case migrationsImportPath:
		return "migrations." + name, nil
	case ormImportPath:
		w.usesORM = true
		return "orm." + name, nil
	}
	if pkg == "main" || !token.IsExported(name) {
		return "", fmt.Errorf("%w: cannot serialize function %s: another package can only refer to an exported function",
			ErrNotSerializable, f.Name())
	}
	return w.importName(pkg) + "." + name, nil
}

// splitFuncName splits the runtime name of a function into its import path
// and its name. ok is false unless the name is a plain identifier, as it is
// for a top-level function.
func splitFuncName(full string) (pkg, name string, ok bool) {
	// The type arguments of a generic function can hold slashes and dots.
	if strings.Contains(full, "[") {
		return "", "", false
	}
	slash := strings.LastIndexByte(full, '/')
	dot := strings.IndexByte(full[slash+1:], '.')
	if dot < 0 {
		return "", "", false
	}
	dot += slash + 1
	// The runtime escapes the dots in the last element of the import path.
	pkg = strings.ReplaceAll(full[:dot], "%2e", ".")
	name = full[dot+1:]
	return pkg, name, token.IsIdentifier(name)
}

// isMigrationFile reports whether file is a migration file. Django flags a
// function from a module whose name starts with a digit, which migration
// names do.
func isMigrationFile(file string) bool {
	base := filepath.Base(file)
	return base != "" && base[0] >= '0' && base[0] <= '9'
}

// importName returns the name under which the file imports the package pkg.
// The name is explicit, because the last element of an import path need not
// be the package name.
func (w *writer) importName(pkg string) string {
	if name, ok := w.imports[pkg]; ok {
		return name
	}
	base := strings.Map(func(r rune) rune {
		if r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, path.Base(pkg))
	if base == "" || !token.IsIdentifier(base) || token.IsKeyword(base) {
		base = "pkg" + base
	}
	taken := map[string]bool{"migrations": true, "orm": true, "json": true, "time": true, "init": true}
	for _, used := range w.imports {
		taken[used] = true
	}
	name := base
	for i := 2; taken[name]; i++ {
		name = base + strconv.Itoa(i)
	}
	if w.imports == nil {
		w.imports = map[string]string{}
	}
	w.imports[pkg] = name
	return name
}
