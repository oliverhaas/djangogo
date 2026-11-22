# Resume notes: feature completeness work (paused 2026-11-22)

Branch `wip/fixtures` holds these notes and the unfinished drafts. `main` and
`feat/m9-spec-completion` carry only finished, gated work. Go ignores this
`_wip` folder, so the drafts do not build until they move into `orm/`.

The goal is a feature-complete Go port of Django 6.0.3
(spec: `/home/ohaas/e1+/ideas/packages/go/djangogo.md`). The spec non-goals stay
documented limitations: CMS, MySQL and Oracle, GIS, full-text search, i18n and
l10n, async ORM, channels, caching backends.

## Done on main

1. go.mod.
2. Mail package (2895a21).
3. Password reset views (ade4763).
4. ModelChoiceField queries its options on each render (ca1c626).
5. (part) Foreign keys are `DEFERRABLE INITIALLY DEFERRED`, as in Django, so a
   transaction can insert rows that refer to each other in any order
   (`orm.DeferredClause`; dialects and `migrations/schema.go`). Commit errors
   are classified (`orm/transaction.go`).

## Next: item 5, fixtures

Item 5 adds serializers, dumpdata, loaddata and flush in four commits (5a to
5d); 5d is the XML format.

### 5a orm support

Drafts in this folder, written but not compiled or tested:

- `saveraw.go`: `SaveRaw(ctx, db, obj)` (Django's `save_base(raw=True)`) and
  `Raw(ctx)` for signal handlers.
- `modelqs.go`: `ModelQuerySet`, `QueryModel(db, m)`, `RelatedMany(db, owner, f)`
  for models known only at run time. TODO: drop the `m == nil` check in
  QueryModel (a nil model panics in chain methods anyway).
- `maintenance.go`: `Flush` (PG `TRUNCATE ... RESTART IDENTITY`; SQLite DELETE
  plus `sqlite_sequence` reset), `ResetSequences` (PG `setval`), and
  `CheckForeignKeys` (PG `SET CONSTRAINTS ALL IMMEDIATE` then `DEFERRED`;
  SQLite `PRAGMA foreign_key_check` with Django's message). Check that
  `scanOne` exists with that signature; otherwise use `QueryRowContext`.

Move them into `orm/`, write tests (Go-only ones say "No Django test exists"),
add CHANGELOG lines, gate, commit.

### 5b serializers package

- `internal/pyjson`: ordered `Object []Member{Key, Value}`; decoding with
  UseNumber that keeps key order and rejects trailing data; Python-compatible
  Marshal with Options{Indent (negative = none), ItemSep, KeySep, ASCII,
  Default}; FormatFloat with Python repr rules (exponent when exp < -4 or
  exp >= 16, ".0" on integral floats).
- `serializers`: Register, Unregister, Formats, PublicFormats, Get,
  SerializerDoesNotExist; Format interface with Serialize and Deserialize over
  `*Object{Model, PK, HasPK, Fields}`; Serialize, SerializeTo and Deserialize
  with options (Indent, Fields, NaturalForeignKeys, NaturalPrimaryKeys,
  Progress, IgnoreNonexistent, HandleForwardReferences); DeserializedObject
  with Save (SaveRaw + SetManyPKs), SaveDeferredFields and String;
  DeserializationError with Django's WithData message; NaturalKeyer,
  NaturalKeyGetter, NaturalKeyDependencies; SortDependencies; ProgressBar;
  json and jsonl formats. M2M values: CachedList first, else
  `RelatedMany(...).Only("pk")` in target ordering.
- Port tests: `tests/serializers` (tests.py, test_json.py, test_jsonl.py,
  test_natural.py, test_deserialization.py, parts of test_data.py).
- Django's tests use USE_TZ=False; djangogo writes UTC with "Z". Document it.

### 5c commands

dumpdata, loaddata and flush in the root package (pattern of
`clearsessions.go`); natural keys for ContentType, Permission, User and Group;
FixturesProvider app interface and a FixtureDirs setting; compressed fixtures
(read gz, zip, bz2; write gz); a djangotest fixtures helper. Port
`tests/fixtures/tests.py`.

## After item 5

6. More database functions.
7. Model inheritance, custom managers, custom user model, non-integer FK
   targets (char primary keys; several serializer tests skip on this).
8. File and image fields, storage, MEDIA, file form fields.
9. Contrib apps: humanize, sites, sitemaps, syndication, redirects, flatpages.
10. tz template tags and time-zone-aware date lookups.
11. Multiple databases.

After each item: gate, commit, push the branch, fast-forward main.

## How to work

- Port the matching Django test first (function-based, cited
  `// Ported from Django: tests/<file>.py::<Class>.<test>`), then implement.
- Read Django sources with `git show 6.0.3:<path>` in `/home/ohaas/e1+/django`.
- Tests on PostgreSQL use the container `djangogo-pg` on 127.0.0.1:55440:
  `DJANGOGO_TEST_POSTGRES_DSN='postgres://djangogo:djangogo@127.0.0.1:55440/w_main?sslmode=disable'`.
- Gate: gofmt, go vet, golangci-lint, then race tests on SQLite and PG.
