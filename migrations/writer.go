package migrations

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// RenderMigration renders an editable Go source file (in package pkgName) that
// reconstructs m and registers it in init() via migrations.Register.
// The output is passed through go/format before being returned.
func RenderMigration(pkgName string, m Migration) (string, error) {
	var buf bytes.Buffer

	needsORM := migrationUsesFields(m)

	// Package declaration.
	fmt.Fprintf(&buf, "package %s\n\n", pkgName)

	// Imports.
	buf.WriteString("import (\n")
	buf.WriteString("\t\"github.com/oliverhaas/djangogo/migrations\"\n")
	if needsORM {
		buf.WriteString("\t\"github.com/oliverhaas/djangogo/orm\"\n")
	}
	buf.WriteString(")\n\n")

	// Package-level var holding the Migration value.
	buf.WriteString("var migration = migrations.Migration{\n")
	fmt.Fprintf(&buf, "\tApp:  %q,\n", m.App)
	fmt.Fprintf(&buf, "\tName: %q,\n", m.Name)

	// Dependencies.
	if len(m.Dependencies) > 0 {
		buf.WriteString("\tDependencies: []string{\n")
		for _, d := range m.Dependencies {
			fmt.Fprintf(&buf, "\t\t%q,\n", d)
		}
		buf.WriteString("\t},\n")
	}

	// Operations.
	if len(m.Operations) > 0 {
		buf.WriteString("\tOperations: []migrations.Operation{\n")
		for _, op := range m.Operations {
			renderOperation(&buf, op)
		}
		buf.WriteString("\t},\n")
	}

	buf.WriteString("}\n\n")

	// init function.
	buf.WriteString("func init() { migrations.Register(migration) }\n")

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("migrations: RenderMigration format error: %w\n\nsource:\n%s", err, buf.String())
	}
	return string(src), nil
}

// migrationUsesFields reports whether any operation in m references a FieldState,
// which requires the orm import for Kind constants.
func migrationUsesFields(m Migration) bool {
	for _, op := range m.Operations {
		switch op.(type) {
		case CreateModel, AddField, AlterField:
			return true
		}
	}
	return false
}

// renderOperation writes the Go literal for a single Operation into buf.
func renderOperation(buf *bytes.Buffer, op Operation) {
	switch o := op.(type) {
	case CreateModel:
		buf.WriteString("\t\tmigrations.CreateModel{\n")
		fmt.Fprintf(buf, "\t\t\tName:  %q,\n", o.Name)
		fmt.Fprintf(buf, "\t\t\tTable: %q,\n", o.Table)
		if len(o.Fields) > 0 {
			buf.WriteString("\t\t\tFields: []migrations.FieldState{\n")
			for _, f := range o.Fields {
				renderFieldStateElem(buf, f)
			}
			buf.WriteString("\t\t\t},\n")
		}
		buf.WriteString("\t\t},\n")

	case DeleteModel:
		buf.WriteString("\t\tmigrations.DeleteModel{\n")
		fmt.Fprintf(buf, "\t\t\tName: %q,\n", o.Name)
		buf.WriteString("\t\t},\n")

	case AddField:
		buf.WriteString("\t\tmigrations.AddField{\n")
		fmt.Fprintf(buf, "\t\t\tModel: %q,\n", o.Model)
		buf.WriteString("\t\t\tField: ")
		renderFieldStateInline(buf, o.Field)
		buf.WriteString("\t\t},\n")

	case RemoveField:
		buf.WriteString("\t\tmigrations.RemoveField{\n")
		fmt.Fprintf(buf, "\t\t\tModel: %q,\n", o.Model)
		fmt.Fprintf(buf, "\t\t\tField: %q,\n", o.Field)
		buf.WriteString("\t\t},\n")

	case AlterField:
		buf.WriteString("\t\tmigrations.AlterField{\n")
		fmt.Fprintf(buf, "\t\t\tModel: %q,\n", o.Model)
		buf.WriteString("\t\t\tField: ")
		renderFieldStateInline(buf, o.Field)
		buf.WriteString("\t\t},\n")
	}
}

// renderFieldStateInline writes a migrations.FieldState{...} literal for
// non-slice contexts (AddField, AlterField) where the type name is required.
func renderFieldStateInline(buf *bytes.Buffer, f FieldState) {
	buf.WriteString("migrations.FieldState{")
	buf.WriteString(fieldStateParts(f))
	buf.WriteString("},\n")
}

// renderFieldStateElem writes a FieldState element inside a []migrations.FieldState
// slice literal. The leading type name is omitted so that gofmt -s is satisfied.
func renderFieldStateElem(buf *bytes.Buffer, f FieldState) {
	buf.WriteString("\t\t\t\t{")
	buf.WriteString(fieldStateParts(f))
	buf.WriteString("},\n")
}

// fieldStateParts returns the comma-separated key:value fields for a FieldState literal.
func fieldStateParts(f FieldState) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Name: %q", f.Name))
	parts = append(parts, fmt.Sprintf("Column: %q", f.Column))
	parts = append(parts, fmt.Sprintf("Kind: orm.Kind%s", f.Kind.String()))
	if f.PrimaryKey {
		parts = append(parts, "PrimaryKey: true")
	}
	if f.Null {
		parts = append(parts, "Null: true")
	}
	if f.Unique {
		parts = append(parts, "Unique: true")
	}
	if f.MaxLength != 0 {
		parts = append(parts, fmt.Sprintf("MaxLength: %d", f.MaxLength))
	}
	if f.RelKind != orm.RelNone {
		parts = append(parts, "RelKind: "+relKindSource(f.RelKind))
	}
	if f.RelTargetTable != "" {
		parts = append(parts, fmt.Sprintf("RelTargetTable: %q", f.RelTargetTable))
	}
	if f.RelTargetColumn != "" {
		parts = append(parts, fmt.Sprintf("RelTargetColumn: %q", f.RelTargetColumn))
	}
	if f.RelOnDelete != orm.OnDeleteDoNothing {
		parts = append(parts, "RelOnDelete: "+onDeleteSource(f.RelOnDelete))
	}
	return strings.Join(parts, ", ")
}

// onDeleteSource maps an orm.OnDelete to its Go source constant name (e.g.
// "orm.OnDeleteCascade") so generated migrations reference the constant rather
// than a raw integer.
func onDeleteSource(od orm.OnDelete) string {
	switch od {
	case orm.OnDeleteCascade:
		return "orm.OnDeleteCascade"
	case orm.OnDeleteSetNull:
		return "orm.OnDeleteSetNull"
	case orm.OnDeleteRestrict:
		return "orm.OnDeleteRestrict"
	default:
		return "orm.OnDeleteDoNothing"
	}
}

// relKindSource maps an orm.RelKind to its Go source constant name (e.g.
// "orm.RelFK"), so generated migrations reference the constant rather than a
// raw integer.
func relKindSource(k orm.RelKind) string {
	switch k {
	case orm.RelFK:
		return "orm.RelFK"
	default:
		return "orm.RelNone"
	}
}

// WriteMigration renders m and writes it to <dir>/<m.Name>.go, creating dir if needed.
// It returns the file path.
func WriteMigration(dir, pkgName string, m Migration) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("migrations: WriteMigration mkdir %q: %w", dir, err)
	}
	src, err := RenderMigration(pkgName, m)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, m.Name+".go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("migrations: WriteMigration write %q: %w", path, err)
	}
	return path, nil
}
