package templates_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// kindTemplateItem exercises every new ORM kind inside a template context.
type kindTemplateItem struct {
	ID       int64
	Price    orm.Decimal `orm:"max_digits=8;decimal_places=2"`
	DueDate  orm.Date
	Alarm    orm.Time
	Token    orm.UUID
	Elapsed  time.Duration
	IP       netip.Addr
	NullDec  *orm.Decimal   `orm:"max_digits=8;decimal_places=2;null"`
	NullDate *orm.Date      `orm:"null"`
	NullTime *orm.Time      `orm:"null"`
	NullDur  *time.Duration `orm:"null"`
	NullUUID *orm.UUID      `orm:"null"`
	Priority int
}

// Meta declares typed choices on Priority to verify get_priority_display.
func (kindTemplateItem) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"Priority": {
				orm.ChoiceOf(1, "Low"),
				orm.ChoiceOf(2, "High"),
			},
		},
	}
}

func TestRenderNewKindValuesBare(t *testing.T) {
	runRenderCases(t, []renderCase{
		{
			name: "decimal_bare",
			src:  "{{ dec }}",
			ctx:  map[string]any{"dec": orm.MustDecimal("12.50")},
			want: "12.50",
		},
		{
			name: "decimal_floatformat",
			src:  "{{ dec|floatformat:1 }}|{{ dec|floatformat:3 }}",
			ctx:  map[string]any{"dec": orm.MustDecimal("12.50")},
			want: "12.5|12.500",
		},
		{
			name: "date_bare_default_format",
			src:  "{{ d }}",
			ctx:  map[string]any{"d": orm.NewDate(2024, 12, 25)},
			want: "Dec. 25, 2024",
		},
		{
			name: "date_zero_prints_empty",
			src:  "[{{ d }}]",
			ctx:  map[string]any{"d": orm.Date{}},
			want: "[]",
		},
		{
			name: "date_iso_c_filter",
			src:  `{{ d|date:"c" }}`,
			ctx:  map[string]any{"d": orm.NewDate(2024, 12, 25)},
			want: "2024-12-25",
		},
		{
			name: "date_timesince",
			src:  `{{ d|timesince:d2 }}`,
			ctx:  map[string]any{"d": orm.NewDate(2024, 12, 25), "d2": orm.NewDate(2024, 12, 30)},
			want: "5\u00a0days",
		},
		{
			name: "time_bare_default_format",
			src:  "{{ t }}|{{ t0 }}|{{ t12 }}",
			ctx: map[string]any{
				"t":   orm.NewTime(15, 30, 0, 0),
				"t0":  orm.NewTime(0, 0, 0, 0),
				"t12": orm.NewTime(12, 0, 0, 0),
			},
			want: "3:30 p.m.|midnight|noon",
		},
		{
			name: "time_with_date_default_format_is_empty",
			src:  "[{{ t|date }}]",
			ctx:  map[string]any{"t": orm.NewTime(4, 0, 0, 0)},
			want: "[]",
		},
		{
			name: "uuid_bare",
			src:  "{{ u }}",
			ctx:  map[string]any{"u": orm.MustUUID("550e8400-e29b-41d4-a716-446655440000")},
			want: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "duration_bare",
			src:  "{{ zero }}|{{ hm }}|{{ neg }}|{{ days }}|{{ one_day }}|{{ us }}",
			ctx: map[string]any{
				"zero":    time.Duration(0),
				"hm":      time.Hour + 30*time.Minute,
				"neg":     -90 * time.Second,
				"days":    2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
				"one_day": 24*time.Hour + time.Minute,
				"us":      5*time.Second + 123456*time.Microsecond,
			},
			want: "0:00:00|1:30:00|-1 day, 23:58:30|2 days, 3:04:05|1 day, 0:01:00|0:00:05.123456",
		},
		{
			name: "json_script_new_kinds",
			src:  `{{ d|json_script:"d" }}{{ t|json_script:"t" }}{{ dec|json_script:"dec" }}{{ u|json_script:"u" }}`,
			ctx: map[string]any{
				"d":   orm.NewDate(2024, 12, 25),
				"t":   orm.NewTime(4, 0, 0, 0),
				"dec": orm.MustDecimal("12.50"),
				"u":   orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"),
			},
			want: `<script id="d" type="application/json">"2024-12-25"</script>` +
				`<script id="t" type="application/json">"04:00:00"</script>` +
				`<script id="dec" type="application/json">"12.50"</script>` +
				`<script id="u" type="application/json">"550e8400-e29b-41d4-a716-446655440000"</script>`,
		},
	})
}

func TestRenderModelWithNewKinds(t *testing.T) {
	reg := orm.NewRegistry()
	if _, err := reg.Register(&kindTemplateItem{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	eng := templates.New()
	eng.SetModels(reg)

	item := kindTemplateItem{
		ID:       1,
		Price:    orm.MustDecimal("49.90"),
		DueDate:  orm.NewDate(2025, 7, 4),
		Alarm:    orm.NewTime(9, 15, 0, 0),
		Token:    orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"),
		Elapsed:  90 * time.Minute,
		IP:       netip.MustParseAddr("192.0.2.1"),
		Priority: 2,
	}

	got, err := eng.RenderString(
		`{{ obj.price }}|{{ obj.due_date }}|{{ obj.due_date|date:"Y-m-d" }}|`+
			`{{ obj.alarm }}|{{ obj.alarm|time:"H:i" }}|{{ obj.token }}|`+
			`{{ obj.elapsed }}|{{ obj.ip }}|{{ obj.get_priority_display }}|`+
			`[{{ obj.null_dec }}{{ obj.null_date }}{{ obj.null_time }}{{ obj.null_dur }}{{ obj.null_uuid }}]`,
		map[string]any{"obj": item},
	)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	want := "49.90|July 4, 2025|2025-07-04|9:15 a.m.|09:15|550e8400-e29b-41d4-a716-446655440000|1:30:00|192.0.2.1|High|[]"
	djangotest.AssertEqual(t, got, want)
}
