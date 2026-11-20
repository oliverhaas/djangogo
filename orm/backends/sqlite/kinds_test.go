package sqlite_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

func TestColumnType_AllNewKinds(t *testing.T) {
	d := sqlite.New()
	tests := []struct {
		name  string
		field *orm.Field
		want  string
	}{
		{"smallint", &orm.Field{Column: "n", Kind: orm.KindSmallInt}, "SMALLINT NOT NULL"},
		{"bigint", &orm.Field{Column: "n", Kind: orm.KindBigInt}, "BIGINT NOT NULL"},
		{"positive_smallint", &orm.Field{Column: "n", Kind: orm.KindPositiveSmallInt}, `SMALLINT UNSIGNED NOT NULL CHECK ("n" >= 0)`},
		{"positive_int", &orm.Field{Column: "n", Kind: orm.KindPositiveInt}, `INTEGER UNSIGNED NOT NULL CHECK ("n" >= 0)`},
		{"positive_bigint", &orm.Field{Column: "n", Kind: orm.KindPositiveBigInt}, `BIGINT UNSIGNED NOT NULL CHECK ("n" >= 0)`},
		{"positive_int_bare", &orm.Field{Kind: orm.KindPositiveInt, Null: true}, "INTEGER UNSIGNED"},
		{"float", &orm.Field{Column: "f", Kind: orm.KindFloat}, "REAL NOT NULL"},
		{"decimal", &orm.Field{Column: "d", Kind: orm.KindDecimal, MaxDigits: 10, DecimalPlaces: 2}, "DECIMAL NOT NULL"},
		{"date", &orm.Field{Column: "d", Kind: orm.KindDate}, "DATE NOT NULL"},
		{"time", &orm.Field{Column: "t", Kind: orm.KindTime}, "TIME NOT NULL"},
		{"duration", &orm.Field{Column: "d", Kind: orm.KindDuration}, "BIGINT NOT NULL"},
		{"uuid", &orm.Field{Column: "u", Kind: orm.KindUUID}, "CHAR(32) NOT NULL"},
		{"uuid_pk", &orm.Field{Column: "id", Kind: orm.KindUUID, PrimaryKey: true}, "CHAR(32) NOT NULL PRIMARY KEY"},
		{"json", &orm.Field{Column: "j", Kind: orm.KindJSON}, `TEXT NOT NULL CHECK (JSON_VALID("j") OR "j" IS NULL)`},
		{"json_null", &orm.Field{Column: "j", Kind: orm.KindJSON, Null: true}, `TEXT CHECK (JSON_VALID("j") OR "j" IS NULL)`},
		{"json_bare", &orm.Field{Kind: orm.KindJSON, Null: true}, "TEXT"},
		{"binary", &orm.Field{Column: "b", Kind: orm.KindBinary}, "BLOB NOT NULL"},
		{"slug", &orm.Field{Column: "s", Kind: orm.KindSlug, MaxLength: 50}, "VARCHAR(50) NOT NULL"},
		{"email", &orm.Field{Column: "e", Kind: orm.KindEmail, MaxLength: 254}, "VARCHAR(254) NOT NULL"},
		{"url", &orm.Field{Column: "u", Kind: orm.KindURL, MaxLength: 200}, "VARCHAR(200) NOT NULL"},
		{"ip", &orm.Field{Column: "ip", Kind: orm.KindGenericIPAddress, Null: true}, "CHAR(39)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.ColumnType(tc.field); got != tc.want {
				t.Errorf("ColumnType() = %q, want %q", got, tc.want)
			}
		})
	}
}
