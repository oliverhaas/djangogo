package fidelity

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// canonicalQueryCases returns all Go query cases mirrored in query_sql.py.
func canonicalQueryCases() []queryCase {
	dtBook2 := utcTime(2024, 3, 20, 14, 30, 0)
	dtStart := utcTime(2024, 2, 1, 0, 0, 0)
	dtEnd := utcTime(2024, 10, 1, 0, 0, 0)

	books := func(db *orm.DB) *orm.QuerySet[book] { return orm.Query[book](db) }
	countries := func(db *orm.DB) *orm.QuerySet[country] { return orm.Query[country](db) }

	return []queryCase{
		// Filter with each lookup
		{
			name: "filter_exact_char",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title", "Compilers 101"), false)
			},
		},
		{
			name: "filter_exact_int",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages", 210), false)
			},
		},
		{
			name: "filter_exact_bool",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("published", true).OrderBy("id"), false)
			},
		},
		{
			name: "filter_exact_bool_false",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("published", false).OrderBy("id"), false)
			},
		},
		{
			name: "filter_exact_datetime",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("created_at", dtBook2), false)
			},
		},
		{
			name: "filter_exact_null",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("editor", nil).OrderBy("id"), false)
			},
		},
		{
			name: "filter_exact_fk_id",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("author", 1).OrderBy("id"), false)
			},
		},
		{
			name: "filter_iexact",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__iexact", "compilers 101"), false)
			},
		},
		{
			name: "filter_gt",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__gt", 200).OrderBy("id"), false)
			},
		},
		{
			name: "filter_gte",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__gte", 210).OrderBy("id"), false)
			},
		},
		{
			name: "filter_lt",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__lt", 200).OrderBy("id"), false)
			},
		},
		{
			name: "filter_lte",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__lte", 210).OrderBy("id"), false)
			},
		},
		{
			name: "filter_contains",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__contains", "Engine").OrderBy("id"), false)
			},
		},
		{
			name: "filter_icontains",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__icontains", "engine").OrderBy("id"), false)
			},
		},
		{
			name: "filter_startswith",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__startswith", "Comp").OrderBy("id"), false)
			},
		},
		{
			name: "filter_istartswith",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__istartswith", "comp").OrderBy("id"), false)
			},
		},
		{
			name: "filter_endswith",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__endswith", "101"), false)
			},
		},
		{
			name: "filter_iendswith",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__iendswith", "design"), false)
			},
		},
		{
			name: "filter_in",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__in", []int{120, 210, 340}).OrderBy("id"), false)
			},
		},
		{
			name: "filter_in_empty",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__in", []int{}), false)
			},
		},
		{
			name: "filter_range_int",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__range", []int{150, 280}).OrderBy("id"), false)
			},
		},
		{
			name: "filter_range_datetime",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("created_at__range", []time.Time{dtStart, dtEnd}).OrderBy("id"), false)
			},
		},
		{
			name: "filter_isnull_true",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("editor__isnull", true).OrderBy("id"), false)
			},
		},
		{
			name: "filter_isnull_false",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("editor__isnull", false).OrderBy("id"), false)
			},
		},
		{
			name: "filter_regex",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__regex", "^Comp").OrderBy("id"), false)
			},
		},
		{
			name: "filter_iregex",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("title__iregex", "^comp").OrderBy("id"), false)
			},
		},
		{
			name: "filter_f_expr",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("pages__gt", orm.F("author__age").Mul(4)).OrderBy("id"), false)
			},
		},
		// Exclude
		{
			name: "exclude_non_null",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Exclude("pages", 210).OrderBy("id"), false)
			},
		},
		{
			name: "exclude_nullable_fk",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Exclude("editor", 2).OrderBy("id"), false)
			},
		},
		{
			name: "exclude_multi",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Exclude("author", 1, "pages__gt", 200).OrderBy("id"), false)
			},
		},
		{
			name: "exclude_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Exclude("author__name", "Ada Lovelace").OrderBy("id"), false)
			},
		},
		// Q with OR / AND / NOT
		{
			name: "q_or",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter(orm.Q("pages__lt", 150).Or(orm.Q("pages__gt", 300))).OrderBy("id"), false)
			},
		},
		{
			name: "q_and_or",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				cond := orm.Q("pages__gte", 120).And(orm.Q("pages__gt", 300).Or(orm.Q("title", "Notes on the Engine")))
				return runBookIDs(t, db, books(db).Filter(cond).OrderBy("id"), false)
			},
		},
		{
			name: "q_not",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter(orm.Not(orm.Q("pages__gt", 200))).OrderBy("id"), false)
			},
		},
		{
			name: "q_or_nullable_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				cond := orm.Q("pages__gt", 300).Or(orm.Q("author__publisher__name", "Indie Press"))
				return runBookIDs(t, db, books(db).Filter(cond).OrderBy("id"), false)
			},
		},
		// Relation-spanning joins (INNER vs LEFT OUTER)
		{
			name: "join_inner_non_null_fk",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("author__name", "Ada Lovelace").OrderBy("id"), false)
			},
		},
		{
			name: "join_outer_nullable_fk_isnull",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("editor__name__isnull", true).OrderBy("id"), false)
			},
		},
		{
			name: "join_inner_demoted_nullable_fk",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("editor__name", "Grace Hopper").OrderBy("id"), false)
			},
		},
		{
			name: "join_chain_inner",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("author__publisher__country__code", "UK").OrderBy("id"), false)
			},
		},
		{
			name: "join_two_fks_same_table",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).Filter("author__name", "Ada Lovelace", "editor__name", "Grace Hopper").OrderBy("id"), false)
			},
		},
		// Ordering including through relations
		{
			name: "order_by_asc_desc",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("published", "-pages", "id"), false)
			},
		},
		{
			name: "order_by_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("author__name", "id"), false)
			},
		},
		{
			name: "order_by_nullable_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("editor__name", "id"), false)
			},
		},
		{
			name: "order_by_chain",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("author__publisher__name", "id"), false)
			},
		},
		{
			name: "order_by_random",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("?"), true)
			},
		},
		// Limit / Offset
		{
			name: "limit_only",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("id").Limit(2), false)
			},
		},
		{
			name: "offset_only",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("id").Offset(2), false)
			},
		},
		{
			name: "limit_and_offset",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runBookIDs(t, db, books(db).OrderBy("id").Limit(2).Offset(1), false)
			},
		},
		// Count
		{
			name: "count_all",
			run:  func(t *testing.T, db *orm.DB) queryCaseOutcome { return runCount(t, db, books(db).Count) },
		},
		{
			name: "count_filtered",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runCount(t, db, books(db).Filter("pages__gte", 200).Count)
			},
		},
		{
			name: "count_distinct",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runCount(t, db, books(db).Values("author").Distinct().Count)
			},
		},
		// Exists
		{
			name: "exists_all",
			run:  func(t *testing.T, db *orm.DB) queryCaseOutcome { return runExists(t, db, books(db)) },
		},
		{
			name: "exists_filtered",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runExists(t, db, books(db).Filter("pages__gt", 250))
			},
		},
		{
			name: "exists_none",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runExists(t, db, books(db).Filter("pages__gt", 9999))
			},
		},
		// Values / ValuesList
		{
			name: "values_fields",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValues(t, db, books(db).OrderBy("id").Values("id", "title", "author__name"))
			},
		},
		{
			name: "values_all_fields",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValues(t, db, countries(db).OrderBy("id").Values())
			},
		},
		{
			name: "values_list",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValuesList(t, db, books(db).OrderBy("id").ValuesList("title", "pages"))
			},
		},
		{
			name: "values_list_flat",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValuesListFlat(t, db, books(db).OrderBy("id").ValuesList("title"))
			},
		},
		// Distinct
		{
			name: "distinct_values",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValues(t, db, books(db).Values("published").Distinct().OrderBy("published"))
			},
		},
		{
			name: "distinct_values_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValues(t, db, books(db).Values("author__name").Distinct().OrderBy("author__name"))
			},
		},
		{
			name: "distinct_extra_order",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runValues(t, db, books(db).Values("author_id").Distinct().OrderBy("pages", "id"))
			},
		},
		// Aggregate and Annotate with GROUP BY
		{
			name: "aggregate_basic",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runAggregate(t, db, books(db), orm.Count("id"), orm.Sum("pages"), orm.Avg("pages"), orm.Min("pages"), orm.Max("pages"))
			},
		},
		{
			name: "aggregate_filtered",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runAggregate(t, db, books(db).Filter("pages__gte", 200), "total_pages", orm.Sum("pages"), "book_count", orm.Count("id"))
			},
		},
		{
			name: "aggregate_distinct",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runAggregate(t, db, books(db), "distinct_authors", orm.Count("author").Distinct())
			},
		},
		{
			name: "annotate_values_groupby",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				vs := books(db).Values("author__name").Annotate("book_count", orm.Count("id"), "total_pages", orm.Sum("pages")).OrderBy("author__name")
				return runValues(t, db, vs)
			},
		},
		{
			name: "annotate_having",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				vs := books(db).Values("author__name").Annotate("book_count", orm.Count("id")).Filter("book_count__gte", 2).OrderBy("author__name")
				return runValues(t, db, vs)
			},
		},
		{
			name: "annotate_expr",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				vs := books(db).Annotate("double_pages", orm.F("pages").Mul(2)).Values("title", "double_pages").OrderBy("id")
				return runValues(t, db, vs)
			},
		},
		// Update and Delete
		{
			name: "update_simple",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runUpdate(t, db, books(db).Filter("pages__lt", 200), "published", true)
			},
		},
		{
			name: "update_relation_expr",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runUpdate(t, db, books(db).Filter("author__name", "Ada Lovelace"), "pages", orm.F("pages").Add(10))
			},
		},
		{
			name: "delete_simple",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runDelete(t, db, books(db).Filter("pages__lt", 200))
			},
		},
		{
			name: "delete_relation",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runDelete(t, db, books(db).Filter("author__name", "Grace Hopper"))
			},
		},
		// select_related
		{
			name: "select_related_single",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runSelectRelated(t, db, books(db).SelectRelated("author").OrderBy("id"), func(rows []book) []map[string]any {
					out := make([]map[string]any, len(rows))
					for i, b := range rows {
						a, ok := b.Author.Object()
						if !ok || a == nil {
							t.Fatalf("book %d: author not loaded by SelectRelated", b.ID)
						}
						out[i] = map[string]any{"id": b.ID, "author": a.Name}
					}
					return out
				})
			},
		},
		{
			name: "select_related_nullable",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runSelectRelated(t, db, books(db).SelectRelated("editor").OrderBy("id"), func(rows []book) []map[string]any {
					out := make([]map[string]any, len(rows))
					for i, b := range rows {
						var ed any
						if e, ok := b.Editor.Object(); ok && e != nil {
							ed = e.Name
						}
						out[i] = map[string]any{"id": b.ID, "editor": ed}
					}
					return out
				})
			},
		},
		{
			name: "select_related_chain",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runSelectRelated(t, db, books(db).SelectRelated("author__publisher__country").OrderBy("id"), func(rows []book) []map[string]any {
					out := make([]map[string]any, len(rows))
					for i, b := range rows {
						a, ok := b.Author.Object()
						if !ok || a == nil {
							t.Fatalf("book %d: author not loaded", b.ID)
						}
						var pubName, countryCode any
						if p, ok := a.Publisher.Object(); ok && p != nil {
							pubName = p.Name
							if c, ok := p.Country.Object(); ok && c != nil {
								countryCode = c.Code
							}
						}
						out[i] = map[string]any{
							"id":        b.ID,
							"author":    a.Name,
							"publisher": pubName,
							"country":   countryCode,
						}
					}
					return out
				})
			},
		},
		{
			name: "select_related_multiple",
			run: func(t *testing.T, db *orm.DB) queryCaseOutcome {
				return runSelectRelated(t, db, books(db).SelectRelated("author", "editor").OrderBy("id"), func(rows []book) []map[string]any {
					out := make([]map[string]any, len(rows))
					for i, b := range rows {
						a, ok := b.Author.Object()
						if !ok || a == nil {
							t.Fatalf("book %d: author not loaded", b.ID)
						}
						var ed any
						if e, ok := b.Editor.Object(); ok && e != nil {
							ed = e.Name
						}
						out[i] = map[string]any{"id": b.ID, "author": a.Name, "editor": ed}
					}
					return out
				})
			},
		},
	}
}
