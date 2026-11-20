# Query SQL differential harness and divergences

`query_sql_test.go` executes the canonical ORM query cases on both SQLite and
PostgreSQL, captures each SQL statement and its bound parameters via
`djangotest.CaptureQueries` (backed by `orm.DB.AddQueryHook`), and compares
them against checked-in Django 6.0.3 goldens in `golden/query_sql/sqlite.json`
and `golden/query_sql/postgres.json`. Every case also verifies semantic
equivalence against the fixture result executed by Django.

## Regenerating goldens

Run from the repository root with Python 3 and Django installed:

```
python3 fidelity/oracle/query_sql.py
```

SQLite cases are compiled and executed against an in-memory SQLite database
seeded with the canonical fixture. PostgreSQL SQL is compiled offline through
Django's real PostgreSQL backend (`pg_version = 160000`) using a minimal
in-process `psycopg` module shim when no PostgreSQL Python driver is installed,
so regenerating both goldens requires neither a live server nor extra Python
packages.

## Documented SQL normalization

Before comparing SQL text, `normalizeSQL` applies four transformations outside
single-quoted SQL string literals:

1. **Placeholder style**: `%s` (Django), `?` (SQLite), and `$1`/`$2`/...
   (PostgreSQL) normalize to `?`.
2. **Identifier quoting**: simple double-quoted identifiers (`"fid_book"`,
   `"T3"`, `"subquery"`) are unquoted so quoted and unquoted aliases compare
   equal.
3. **Subquery table aliases**: Django's generated `U0`, `U1`, ... aliases
   inside `IN (SELECT ...)` subqueries are expanded back to their table names
   when each table appears once in the subquery.
4. **Whitespace**: runs of whitespace collapse to a single space.

`normalizeParams` normalizes integer widths to `int64` and UTC timestamps
(including Django SQLite's `"YYYY-MM-DD HH:MM:SS"` strings) to a single UTC
RFC3339 representation.

## Allowlisted SQL divergences

Every case below passes semantic equivalence on both SQLite and PostgreSQL. The
test fails if an allowlisted case begins matching Django so stale entries are
caught and removed.

### Shared (SQLite and PostgreSQL)

| Cases | Django 6.0.3 | Djan-Go-Go |
| --- | --- | --- |
| `filter_exact_bool`, `filter_exact_bool_false` | Emits bare `WHERE fid_book.published` / `WHERE NOT fid_book.published` with no bound parameter | Emits parameterized `WHERE fid_book.published = ?` with `true` / `false` |
| `order_by_random` | Emits `ORDER BY RAND() ASC` (SQLite) / `ORDER BY RANDOM() ASC` (PostgreSQL) | Emits `ORDER BY RANDOM()` without `ASC` on both dialects |
| `count_distinct` | Emits `SELECT COUNT(*) FROM (SELECT DISTINCT fid_book.author_id AS author FROM fid_book) subquery` | Emits `SELECT COUNT(*) AS __count FROM (SELECT DISTINCT fid_book.author_id FROM fid_book) subquery` |
| `exists_all`, `exists_filtered`, `exists_none` | Emits `SELECT %s AS a FROM ... LIMIT 1` with bound param `1` | Emits `SELECT EXISTS(SELECT 1 FROM ...)` |
| `values_list`, `values_list_flat`, `distinct_extra_order`, `annotate_expr` | Emits `AS <key>` aliases on every selected `values()` / `values_list()` column | Omits redundant aliases on plain columns (keeps aliases on annotations); on PostgreSQL `annotate_expr` also casts `$1::bigint` |
| `values_fields`, `distinct_values`, `distinct_values_relation` | Aliases selected columns and emits positional `ORDER BY 1 ASC` | Writes explicit column names in `ORDER BY` without redundant column aliases |
| `annotate_values_groupby`, `annotate_having` | Aliases selected columns and emits positional `GROUP BY 1` and `ORDER BY 1 ASC` | Writes explicit column names in `GROUP BY` and `ORDER BY` without redundant column aliases |

### PostgreSQL only

| Cases | Django 6.0.3 | Djan-Go-Go |
| --- | --- | --- |
| `filter_iexact` | `UPPER(fid_book.title::text) = UPPER(%s)` | `UPPER(fid_book.title::text) LIKE UPPER($1) ESCAPE '\'` (`orm/backends/postgres/query.go:40-41`) |
| `filter_contains`, `filter_icontains`, `filter_startswith`, `filter_istartswith`, `filter_endswith`, `filter_iendswith` | Omits `ESCAPE '\'`, relying on PostgreSQL's default backslash escape | Emits explicit `ESCAPE '\'` (`orm/backends/postgres/query.go:38-41`) |
| `filter_f_expr`, `update_relation_expr` | Binds expression literals as `%s` without a cast | Appends `::bigint` via `LiteralSQL` (`orm/backends/postgres/query.go:55-70`) so untyped parameter contexts resolve |

## Dialect-native semantic note

For `order_by_nullable_relation` (`ORDER BY fid_author.name ASC, fid_book.id ASC`)
and `order_by_chain` (`ORDER BY fid_publisher.name ASC, fid_book.id ASC`), both
Django and Djan-Go-Go emit identical SQL with no `NULLS FIRST`/`NULLS LAST`
clause. SQLite sorts NULLs first while PostgreSQL sorts NULLs last; the test
asserts exact SQL equality on both dialects and verifies each dialect's native
NULL sort order.
