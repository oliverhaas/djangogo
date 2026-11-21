// Package fidelity holds Djan-Go-Go's Django-as-oracle differential test
// harnesses. Each harness compares Djan-Go-Go's output against checked-in
// golden files produced by real Django (Python is a dev/CI dependency only;
// `go test ./fidelity/...` always runs from the checked-in goldens with no
// Python installed):
//
//   - Templates (render_test.go, cases.json, golden/*.txt, divergences.md):
//     renders canonical template+context cases with templates.Engine and
//     asserts byte-for-byte equality with Django DTL.
//   - ORM query SQL (query_sql_test.go, golden/query_sql/{sqlite,postgres}.json,
//     query_sql_divergences.md): executes canonical ORM queries on SQLite and
//     PostgreSQL, verifies semantic result equivalence, and compares
//     normalized SQL and bound parameters against Django's SQLCompiler.
//   - Migration SQL (migration_sql_test.go,
//     golden/migration_sql/{sqlite,postgres}.json,
//     migration_sql_divergences.md): compares migrations.Loader.CollectSQL
//     against Django's sqlmigrate on SQLite and PostgreSQL, verifies resulting
//     SQLite schema equivalence via PRAGMAs (especially for SQLite table
//     rebuilds), and executes every case against live PostgreSQL when
//     DJANGOGO_TEST_POSTGRES_DSN is set.
//
// # Regenerating all goldens
//
// With Python 3 and Django 6.0.3 installed, regenerate every golden file
// (templates, query SQL, and migration SQL) with one command:
//
//	go generate ./fidelity/...
//
// Or invoke the entry point directly from the repository root:
//
//	python3 fidelity/oracle/generate.py
//
// Neither command requires a live PostgreSQL server or a Python PostgreSQL
// driver: PostgreSQL SQL goldens compile offline through a minimal psycopg
// shim targeting PostgreSQL 16.
//
// # Adding a case
//
//   - Template case: append a JSON object (name, template, context, optional
//     templates) to fidelity/cases.json and run `go generate ./fidelity/...`.
//     If the template uses {% url %}, add the route to both _install_urlconf in
//     oracle/render.py and fidelityRouter in render_test.go.
//   - Query SQL case: add the Django queryset case to build_cases in
//     oracle/query_sql.py and the matching Go case to canonicalQueryCases in
//     query_sql_cases_test.go, then run `go generate ./fidelity/...`.
//   - Migration SQL case: add the Django operation(s) to build_cases in
//     oracle/migration_sql.py and the matching Go operation(s) to
//     canonicalMigrationCases in migration_sql_cases_test.go, then run
//     `go generate ./fidelity/...`.
//
// Every suite has a consistency test that fails if a Go case and its golden
// files fall out of sync.
//
// # Recording a divergence
//
// Never weaken assertions or silently drop a case when Djan-Go-Go legitimately
// differs from Django:
//
//  1. Add an entry with a concise reason to the suite's reviewed allowlist:
//     skippedCases (render_test.go), querySQLDivergences (query_sql_test.go),
//     or migrationSQLDivergences (migration_sql_test.go). Group similar cases
//     under one clear reason.
//  2. Document the Django vs Djan-Go-Go outputs side by side in the matching
//     markdown catalogue (divergences.md, query_sql_divergences.md, or
//     migration_sql_divergences.md).
//  3. Keep semantic checks passing: query SQL divergences still assert result
//     equivalence on the fixture dataset, and migration SQL divergences still
//     assert SQLite PRAGMA schema equivalence and live PostgreSQL execution.
//
// Query and migration allowlists are self-cleaning: if a future ORM or
// migrations change makes an allowlisted case match Django, the test fails
// until the stale allowlist entry is removed.
package fidelity

//go:generate python3 oracle/generate.py
