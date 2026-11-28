package migrations

import (
	"crypto/md5"
	"encoding/hex"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// maxNameLength is the longest identifier the dialect accepts. The values are
// the ones Django uses: 63 for PostgreSQL, and 200 as its fallback elsewhere.
func maxNameLength(d orm.Dialect) int {
	if d.Name() == "postgres" {
		return 63
	}
	return 200
}

// namesDigest returns the first length hex digits of the MD5 of the
// concatenated args (Django's names_digest), so names match Django's. MD5
// only shortens names here; nothing relies on it for security.
func namesDigest(length int, args ...string) string {
	h := md5.New()
	for _, a := range args {
		h.Write([]byte(a))
	}
	return hex.EncodeToString(h.Sum(nil))[:length]
}

// createIndexName builds the name of an index or unique constraint from the
// table, the columns and a digest, shortened to maxLen as Django's
// _create_index_name does.
func createIndexName(table string, columns []string, suffix string, maxLen int) string {
	hashPart := namesDigest(8, append([]string{table}, columns...)...) + suffix
	joined := strings.Join(columns, "_")
	name := table + "_" + joined + "_" + hashPart
	if len(name) <= maxLen {
		return name
	}
	if len(hashPart) > maxLen/3 {
		hashPart = hashPart[:maxLen/3]
	}
	other := (maxLen-len(hashPart))/2 - 1
	name = truncate(table, other) + "_" + truncate(joined, other) + "_" + hashPart
	if name[0] == '_' || (name[0] >= '0' && name[0] <= '9') {
		name = "D" + name[:len(name)-1]
	}
	return name
}

// truncate cuts s to at most n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// IndexName returns the name Django gives an index without an explicit name:
// 11 characters of the table, 7 of the first column, a digest and "idx". It
// fits the 30 character limit all databases accept.
func IndexName(table string, columns []string) string {
	first := ""
	if len(columns) > 0 {
		first = columns[0]
	}
	digest := namesDigest(6, append(append([]string{table}, columns...), "idx")...)
	name := truncate(table, 11) + "_" + truncate(first, 7) + "_" + digest + "_idx"
	if name[0] == '_' || (name[0] >= '0' && name[0] <= '9') {
		name = "D" + name[1:]
	}
	return name
}

// defaultTable returns the table name the ORM derives for a model without a
// Meta.Table override: "<app>_<model>" in lowercase.
func defaultTable(app, name string) string {
	if app == "" {
		return strings.ToLower(name)
	}
	return app + "_" + strings.ToLower(name)
}

// fieldClassName returns the Django field class a field state corresponds to.
// The rename question shows it, as Django does.
func fieldClassName(f FieldState) string {
	if f.IsRelation() {
		return "ForeignKey"
	}
	return f.Kind.FieldClassName()
}

// isValidGoIdent reports whether s can be used as a Go identifier.
func isValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
