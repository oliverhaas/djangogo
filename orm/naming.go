package orm

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// maxNameLength returns the maximum identifier length for dialect d, matching
// Django's backend limits: 63 for PostgreSQL, 200 elsewhere.
func maxNameLength(d Dialect) int {
	if d.Name() == "postgres" {
		return 63
	}
	return 200
}

// namesDigest returns the first length hex digits of the MD5 of the
// concatenated args (Django's names_digest). MD5 is used solely to match
// Django's deterministic identifier shortening.
func namesDigest(length int, args ...string) string {
	h := md5.New()
	for _, a := range args {
		h.Write([]byte(a))
	}
	return hex.EncodeToString(h.Sum(nil))[:length]
}

// truncate cuts s to at most n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// CreateIndexName builds the name of an index or constraint from the table,
// columns, suffix, and maximum identifier length, matching Django's
// BaseDatabaseSchemaEditor._create_index_name.
func CreateIndexName(table string, columns []string, suffix string, maxLen int) string {
	args := make([]string, 0, len(columns)+1)
	args = append(args, table)
	args = append(args, columns...)
	hashPart := namesDigest(8, args...) + suffix
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

// IndexName returns the default 30-character name Django gives a model-level
// Index without an explicit name (Django's Index.set_name_with_model): up to 11
// characters of the table, up to 7 of the first column (without a leading "-"),
// a 6-character digest over the table, columns (with any leading "-"), and
// "idx", followed by "_idx".
func IndexName(table string, columns []string) string {
	first := ""
	if len(columns) > 0 {
		first = strings.TrimPrefix(columns[0], "-")
	}
	args := make([]string, 0, len(columns)+2)
	args = append(args, table)
	args = append(args, columns...)
	args = append(args, "idx")
	digest := namesDigest(6, args...)
	name := truncate(table, 11) + "_" + truncate(first, 7) + "_" + digest + "_idx"
	if name[0] == '_' || (name[0] >= '0' && name[0] <= '9') {
		name = "D" + name[1:]
	}
	return name
}
