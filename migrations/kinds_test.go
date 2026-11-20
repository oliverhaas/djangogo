package migrations_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

func TestMigrationsCreateAndAddNewKindsWithDefaults(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		m1 := migrations.Migration{
			App: "kindsapp", Name: "0001_initial", Initial: true,
			Operations: []migrations.Operation{
				migrations.CreateModel{
					Name: "Item",
					Fields: []migrations.FieldState{
						{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
						{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 50},
					},
				},
			},
		}
		m2 := migrations.Migration{
			App: "kindsapp", Name: "0002_add_kinds",
			Dependencies: []migrations.Key{{App: "kindsapp", Name: "0001_initial"}},
			Operations: []migrations.Operation{
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Ratio", Column: "ratio", Kind: orm.KindFloat, Default: float64(1.25)},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{
						Name: "Price", Column: "price", Kind: orm.KindDecimal,
						MaxDigits: 8, DecimalPlaces: 2, Default: orm.MustDecimal("9.99"),
					},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Born", Column: "born", Kind: orm.KindDate, Default: orm.MustDate("2025-01-02")},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Alarm", Column: "alarm", Kind: orm.KindTime, Default: orm.MustTime("09:30:00")},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Elapsed", Column: "elapsed", Kind: orm.KindDuration, Default: 90 * time.Second},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{
						Name: "UID", Column: "uid", Kind: orm.KindUUID,
						Default: orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"),
					},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Payload", Column: "payload", Kind: orm.KindJSON, Default: json.RawMessage(`{"ok":true}`)},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Raw", Column: "raw", Kind: orm.KindBinary, Default: []byte{0xde, 0xad}},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Count", Column: "count", Kind: orm.KindPositiveInt, Default: int64(3)},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "Slug", Column: "slug", Kind: orm.KindSlug, MaxLength: 50, Blank: true, DBIndex: true},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "IP", Column: "ip", Kind: orm.KindGenericIPAddress, Null: true},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "StampedD", Column: "stamped_d", Kind: orm.KindDate, AutoNowAdd: true},
				},
				migrations.AddField{
					Model: "Item",
					Field: migrations.FieldState{Name: "StampedT", Column: "stamped_t", Kind: orm.KindTime, AutoNowAdd: true},
				},
			},
		}

		exec := newExecutor(t, db, []migrations.Migration{m1, m2})
		// Apply initial, insert a row so AddField must backfill defaults on both dialects, then apply m2.
		migrate(t, exec, k("kindsapp", "0001_initial"))
		_, err := db.Exec(ctx, `INSERT INTO "kindsapp_item" ("name") VALUES (?)`, "seed")
		djangotest.AssertEqual(t, err, nil)

		migrate(t, exec, k("kindsapp", "0002_add_kinds"))
	})
}

func TestRenderMigrationNewKindDefaultsAndFieldStateEqual(t *testing.T) {
	f1 := migrations.FieldState{Name: "Raw", Column: "raw", Kind: orm.KindBinary, Default: []byte{1, 2}}
	f2 := migrations.FieldState{Name: "Raw", Column: "raw", Kind: orm.KindBinary, Default: []byte{1, 2}}
	f3 := migrations.FieldState{Name: "Raw", Column: "raw", Kind: orm.KindBinary, Default: []byte{1, 3}}
	djangotest.AssertEqual(t, f1.Equal(f2), true)
	djangotest.AssertEqual(t, f1.Equal(f3), false)

	j1 := migrations.FieldState{Name: "Meta", Column: "meta", Kind: orm.KindJSON, Default: json.RawMessage(`{}`)}
	j2 := migrations.FieldState{Name: "Meta", Column: "meta", Kind: orm.KindJSON, Default: json.RawMessage(`{}`)}
	djangotest.AssertEqual(t, j1.Equal(j2), true)

	mig := migrations.Migration{
		App: "shop", Name: "0001_initial", Initial: true,
		Operations: []migrations.Operation{
			migrations.CreateModel{
				Name: "Product",
				Fields: []migrations.FieldState{
					{Name: "ID", Column: "id", Kind: orm.KindUUID, PrimaryKey: true, Default: orm.DefaultUUID4},
					{Name: "Price", Column: "price", Kind: orm.KindDecimal, MaxDigits: 10, DecimalPlaces: 2, Default: orm.MustDecimal("19.95")},
					{Name: "Ratio", Column: "ratio", Kind: orm.KindFloat, Default: float64(0.5)},
					{Name: "Born", Column: "born", Kind: orm.KindDate, Default: orm.MustDate("2025-02-03")},
					{Name: "Alarm", Column: "alarm", Kind: orm.KindTime, Default: orm.MustTime("10:15:00")},
					{Name: "TTL", Column: "ttl", Kind: orm.KindDuration, Default: 5 * time.Minute},
					{Name: "Meta", Column: "meta", Kind: orm.KindJSON, Default: json.RawMessage(`{}`)},
					{Name: "Raw", Column: "raw", Kind: orm.KindBinary, Default: []byte{0x08}},
				},
			},
		},
	}

	src, err := migrations.RenderMigration("shop", mig)
	djangotest.AssertEqual(t, err, nil)
	for _, want := range []string{
		`"encoding/json"`,
		`"time"`,
		`"github.com/oliverhaas/djangogo/orm"`,
		`orm.KindUUID`,
		`orm.DefaultUUID4`,
		`MaxDigits: 10`,
		`DecimalPlaces: 2`,
		`orm.MustDecimal("19.95")`,
		`float64(0.5)`,
		`orm.MustDate("2025-02-03")`,
		`orm.MustTime("10:15:00")`,
		`time.Duration(300000000000)`,
		`json.RawMessage("{}")`,
		`[]byte{0x8}`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("RenderMigration output missing %q:\n%s", want, src)
		}
	}
}
