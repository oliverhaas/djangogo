package postgres_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
)

func TestColumnType_AllNewKinds(t *testing.T) {
	d := postgres.New()
	tests := []struct {
		name  string
		field *orm.Field
		want  string
	}{
		{"smallint", &orm.Field{Column: "n", Kind: orm.KindSmallInt}, "SMALLINT NOT NULL"},
		{"bigint", &orm.Field{Column: "n", Kind: orm.KindBigInt}, "BIGINT NOT NULL"},
		{"positive_smallint", &orm.Field{Column: "n", Kind: orm.KindPositiveSmallInt}, `SMALLINT NOT NULL CHECK ("n" >= 0)`},
		{"positive_int", &orm.Field{Column: "n", Kind: orm.KindPositiveInt}, `INTEGER NOT NULL CHECK ("n" >= 0)`},
		{"positive_bigint", &orm.Field{Column: "n", Kind: orm.KindPositiveBigInt}, `BIGINT NOT NULL CHECK ("n" >= 0)`},
		{"positive_int_bare", &orm.Field{Kind: orm.KindPositiveInt, Null: true}, "INTEGER"},
		{"float", &orm.Field{Column: "f", Kind: orm.KindFloat}, "DOUBLE PRECISION NOT NULL"},
		{"decimal_prec", &orm.Field{Column: "d", Kind: orm.KindDecimal, MaxDigits: 10, DecimalPlaces: 2}, "NUMERIC(10, 2) NOT NULL"},
		{"decimal_noprec", &orm.Field{Column: "d", Kind: orm.KindDecimal}, "NUMERIC NOT NULL"},
		{"date", &orm.Field{Column: "d", Kind: orm.KindDate}, "DATE NOT NULL"},
		{"time", &orm.Field{Column: "t", Kind: orm.KindTime}, "TIME NOT NULL"},
		{"duration", &orm.Field{Column: "d", Kind: orm.KindDuration}, "INTERVAL NOT NULL"},
		{"uuid", &orm.Field{Column: "u", Kind: orm.KindUUID}, "UUID NOT NULL"},
		{"uuid_pk", &orm.Field{Column: "id", Kind: orm.KindUUID, PrimaryKey: true}, "UUID NOT NULL PRIMARY KEY"},
		{"json", &orm.Field{Column: "j", Kind: orm.KindJSON}, "JSONB NOT NULL"},
		{"binary", &orm.Field{Column: "b", Kind: orm.KindBinary}, "BYTEA NOT NULL"},
		{"slug", &orm.Field{Column: "s", Kind: orm.KindSlug, MaxLength: 50}, "VARCHAR(50) NOT NULL"},
		{"email", &orm.Field{Column: "e", Kind: orm.KindEmail, MaxLength: 254}, "VARCHAR(254) NOT NULL"},
		{"url", &orm.Field{Column: "u", Kind: orm.KindURL, MaxLength: 200}, "VARCHAR(200) NOT NULL"},
		{"ip", &orm.Field{Column: "ip", Kind: orm.KindGenericIPAddress, Null: true}, "INET"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.ColumnType(tc.field); got != tc.want {
				t.Errorf("ColumnType() = %q, want %q", got, tc.want)
			}
		})
	}
}
