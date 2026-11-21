#!/usr/bin/env python3
"""Generate Django migration SQL goldens into fidelity/golden/migration_sql/{sqlite,postgres}.json."""

# Run from the repository root: python3 fidelity/oracle/migration_sql.py
# SQLite cases run against a fresh in-memory SQLite database per case so the
# goldens record both Django's sqlmigrate statements and the resulting schema
# (tables, columns, indexes, foreign keys) for semantic comparison of table
# rebuilds. PostgreSQL SQL is compiled offline through a minimal psycopg shim,
# so regenerating both goldens requires neither a live server nor extra packages.

from __future__ import annotations

import json
import sys
import types
import uuid
from datetime import date, datetime, time, timedelta
from decimal import Decimal
from pathlib import Path
from typing import Any

FIDELITY_DIR = Path(__file__).resolve().parent.parent
GOLDEN_DIR = FIDELITY_DIR / "golden" / "migration_sql"

# Target PostgreSQL version used for offline SQL compilation (PostgreSQL 16).
OFFLINE_PG_VERSION = 160000


def _pg_quote(val: Any, _conn: Any = None) -> str:
    """Quote a Python literal the way psycopg3 formats SQL parameters for PostgreSQL."""
    if val is None:
        return "NULL"
    if isinstance(val, bool):
        return "true" if val else "false"
    if isinstance(val, (int, float, Decimal)):
        return str(val)
    if isinstance(val, (bytes, memoryview)):
        return "'\\x" + bytes(val).hex() + "'::bytea"
    if isinstance(val, str):
        return "'" + val.replace("'", "''") + "'"
    if isinstance(val, datetime):
        return "'" + val.isoformat(sep=" ") + "'::timestamptz"
    if isinstance(val, date):
        return "'" + val.isoformat() + "'::date"
    if isinstance(val, time):
        return "'" + val.isoformat() + "'::time"
    if isinstance(val, timedelta):
        return f"'{val.total_seconds()} seconds'::interval"
    if isinstance(val, uuid.UUID):
        return f"'{val}'::uuid"
    return "'" + str(val).replace("'", "''") + "'"


def install_psycopg_shim_if_needed() -> None:
    """Stub psycopg so Django's PostgreSQL backend compiles DDL offline without a driver."""
    for mod in ("psycopg", "psycopg2"):
        try:
            __import__(mod)
            return
        except ImportError:
            pass

    class _Dummy:
        oid = 0

        def __init__(self, *args: Any, **kwargs: Any) -> None:
            pass

        def __getitem__(self, key: Any) -> _Dummy:
            return self

        def get_loader(self, *args: Any, **kwargs: Any) -> type[_Dummy]:
            return _Dummy

    class _IntWrapper(int):
        pass

    class _ClientCursor:
        def __init__(self, _conn: Any = None) -> None:
            pass

        def mogrify(self, query: str, params: Any = None) -> str:
            if not params:
                return query.replace("%%", "%")
            if isinstance(params, dict):
                return query % {k: _pg_quote(v) for k, v in params.items()}
            return query % tuple(_pg_quote(p) for p in params)

    class _AutoMod(types.ModuleType):
        def __getattr__(self, name: str) -> Any:
            cls = type(name, (_Dummy,), {})
            setattr(self, name, cls)
            return cls

    psycopg = _AutoMod("psycopg")
    psycopg.__version__ = "3.2.0"
    psycopg.ClientCursor = _ClientCursor
    psycopg.errors = _AutoMod("psycopg.errors")
    for err in (
        "Error",
        "DatabaseError",
        "DataError",
        "OperationalError",
        "IntegrityError",
        "InternalError",
        "ProgrammingError",
        "NotSupportedError",
        "InterfaceError",
    ):
        setattr(psycopg.errors, err, Exception)
        setattr(psycopg, err, Exception)
    psycopg.sql = _AutoMod("psycopg.sql")
    psycopg.sql.quote = _pg_quote
    psycopg.pq = _AutoMod("psycopg.pq")
    psycopg.pq.Format = types.SimpleNamespace(TEXT=0, BINARY=1)
    psycopg.adapt = _AutoMod("psycopg.adapt")
    psycopg.client_cursor = _AutoMod("psycopg.client_cursor")
    psycopg.adapters = _Dummy()
    psycopg.adapters.types = _Dummy()
    psycopg.postgres = _AutoMod("psycopg.postgres")
    psycopg.postgres.types = _Dummy()

    psycopg_types = _AutoMod("psycopg.types")
    psycopg_types.__path__ = []
    for sub in ("datetime", "json", "range", "string", "numeric", "net"):
        mod = _AutoMod(f"psycopg.types.{sub}")
        setattr(psycopg_types, sub, mod)
        sys.modules[f"psycopg.types.{sub}"] = mod
    for int_cls in ("Int2", "Int4", "Int8", "IntNumeric", "Oid"):
        setattr(psycopg_types.numeric, int_cls, _IntWrapper)

    for name, mod in (
        ("psycopg", psycopg),
        ("psycopg.pq", psycopg.pq),
        ("psycopg.adapt", psycopg.adapt),
        ("psycopg.client_cursor", psycopg.client_cursor),
        ("psycopg.postgres", psycopg.postgres),
        ("psycopg.types", psycopg_types),
    ):
        sys.modules[name] = mod


class _OfflineCursor:
    """No-op cursor context manager for offline PostgreSQL schema SQL compilation."""

    connection = None

    def __enter__(self) -> _OfflineCursor:
        return self

    def __exit__(self, *args: Any) -> None:
        pass

    def execute(self, *args: Any, **kwargs: Any) -> None:
        pass

    def fetchone(self) -> None:
        return None

    def fetchall(self) -> list[Any]:
        return []


_PG_CONSTRAINTS: dict[str, dict[str, dict[str, Any]]] = {}


def configure_django() -> None:
    """Configure Django with SQLite (in-memory) and PostgreSQL (offline) connections."""
    install_psycopg_shim_if_needed()
    import django
    from django.conf import settings
    from django.db import connections

    settings.configure(
        DATABASES={
            "default": {"ENGINE": "django.db.backends.sqlite3", "NAME": ":memory:"},
            "postgres": {"ENGINE": "django.db.backends.postgresql", "NAME": "w_fid"},
        },
        DEFAULT_AUTO_FIELD="django.db.models.BigAutoField",
        INSTALLED_APPS=[],
        USE_TZ=True,
        TIME_ZONE="UTC",
    )
    django.setup()
    pg = connections["postgres"]
    pg.pg_version = OFFLINE_PG_VERSION
    pg.cursor = lambda: _OfflineCursor()
    pg.introspection.get_constraints = lambda _cursor, table: _PG_CONSTRAINTS.get(table, {})


def derive_pg_constraints(conn: Any, state: Any) -> dict[str, dict[str, dict[str, Any]]]:
    """Derive PostgreSQL pg_constraint metadata from the pre-migration ProjectState.

    PostgreSQL's SchemaEditor queries introspection.get_constraints when dropping
    existing unique constraints, foreign keys, or unique_together sets. Deriving
    the exact constraint names PostgreSQL and Django create for the prior state
    keeps golden generation 100% offline.
    """
    apps = state.apps
    result: dict[str, dict[str, dict[str, Any]]] = {}
    with conn.schema_editor(collect_sql=True, atomic=False) as editor:
        for model in apps.get_models():
            table = model._meta.db_table
            table_cons: dict[str, dict[str, Any]] = {}
            for field in model._meta.local_fields:
                col = field.column
                if field.primary_key:
                    table_cons[f"{table}_pkey"] = {
                        "columns": [col],
                        "primary_key": True,
                        "unique": True,
                        "foreign_key": None,
                        "check": False,
                        "index": True,
                        "type": "idx",
                    }
                elif field.unique:
                    # Inline UNIQUE in CREATE TABLE gets PostgreSQL's default <table>_<col>_key name.
                    table_cons[f"{table}_{col}_key"] = {
                        "columns": [col],
                        "primary_key": False,
                        "unique": True,
                        "foreign_key": None,
                        "check": False,
                        "index": True,
                        "type": "idx",
                    }
                if field.db_check(conn):
                    # An inline CHECK in CREATE TABLE gets PostgreSQL's default <table>_<col>_check name.
                    table_cons[f"{table}_{col}_check"] = {
                        "columns": [col],
                        "primary_key": False,
                        "unique": False,
                        "foreign_key": None,
                        "check": True,
                        "index": False,
                        "type": None,
                    }
                if field.db_index and not field.unique and not field.primary_key:
                    idx_name = editor._create_index_name(table, [col])
                    table_cons[idx_name] = {
                        "columns": [col],
                        "primary_key": False,
                        "unique": False,
                        "foreign_key": None,
                        "check": False,
                        "index": True,
                        "type": "idx",
                    }
                if field.is_relation and getattr(field, "db_constraint", True):
                    target = field.target_field
                    target_table = target.model._meta.db_table
                    target_col = target.column
                    fk_name = editor._create_index_name(
                        table, [col], suffix=f"_fk_{target_table}_{target_col}"
                    )
                    table_cons[fk_name] = {
                        "columns": [col],
                        "primary_key": False,
                        "unique": False,
                        "foreign_key": (target_table, target_col),
                        "check": False,
                        "index": False,
                        "type": None,
                    }
            for fields_tuple in model._meta.unique_together:
                cols = [model._meta.get_field(f).column for f in fields_tuple]
                uniq_name = str(editor._unique_constraint_name(table, cols, quote=False))
                table_cons[uniq_name] = {
                    "columns": cols,
                    "primary_key": False,
                    "unique": True,
                    "foreign_key": None,
                    "check": False,
                    "index": True,
                    "type": "idx",
                }
            result[table] = table_cons
    return result


def snapshot_sqlite_schema(conn: Any) -> dict[str, Any]:
    """Capture SQLite tables, columns, indexes, and foreign keys via PRAGMAs."""
    tables: dict[str, Any] = {}
    with conn.cursor() as cur:
        cur.execute(
            "SELECT name FROM sqlite_master "
            "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' "
            "ORDER BY name"
        )
        table_names = [row[0] for row in cur.fetchall()]
        for table in table_names:
            cur.execute(f'PRAGMA table_info("{table}")')
            columns = [
                {
                    "name": row[1],
                    "type": row[2],
                    "notnull": bool(row[3]),
                    "pk": int(row[5]),
                }
                for row in cur.fetchall()
            ]
            cur.execute(f'PRAGMA index_list("{table}")')
            indexes = []
            for idx_row in cur.fetchall():
                idx_name = idx_row[1]
                unique = bool(idx_row[2])
                origin = idx_row[3]
                cur.execute(f'PRAGMA index_xinfo("{idx_name}")')
                idx_cols = []
                for col_row in cur.fetchall():
                    # col_row: (seqno, cid, name, desc, coll, key)
                    if col_row[5] == 1 and col_row[2] is not None:
                        col_entry = col_row[2]
                        if col_row[3] == 1:
                            col_entry = "-" + col_entry
                        idx_cols.append(col_entry)
                indexes.append(
                    {
                        "name": idx_name,
                        "unique": unique,
                        "origin": origin,
                        "columns": idx_cols,
                    }
                )
            indexes.sort(key=lambda item: (item["unique"], item["columns"], item["name"]))

            cur.execute(f'PRAGMA foreign_key_list("{table}")')
            fks = [
                {
                    "table": row[2],
                    "from": row[3],
                    "to": row[4],
                    "on_delete": row[6],
                }
                for row in cur.fetchall()
            ]
            fks.sort(key=lambda item: (item["from"], item["table"], item["to"]))

            tables[table] = {
                "columns": columns,
                "indexes": indexes,
                "foreign_keys": fks,
            }
    return {"tables": tables}


def build_cases() -> list[dict[str, Any]]:
    """Return the canonical migration SQL differential cases matching migration_sql_cases_test.go."""
    from django.db import migrations, models

    tag_op = migrations.CreateModel(
        name="Tag",
        fields=[
            ("id", models.BigAutoField(primary_key=True, serialize=False)),
            ("name", models.CharField(max_length=100)),
            ("priority", models.SmallIntegerField()),
            ("active", models.BooleanField()),
            ("notes", models.CharField(max_length=200, null=True)),
            ("code", models.CharField(max_length=20, unique=True)),
        ],
    )

    all_kinds_op = migrations.CreateModel(
        name="AllKinds",
        fields=[
            ("id", models.BigAutoField(primary_key=True, serialize=False)),
            ("int_val", models.IntegerField()),
            ("small_int", models.SmallIntegerField()),
            ("big_int", models.BigIntegerField()),
            ("pos_small_int", models.PositiveSmallIntegerField()),
            ("pos_int", models.PositiveIntegerField()),
            ("pos_big_int", models.PositiveBigIntegerField()),
            ("float_val", models.FloatField()),
            ("decimal_val", models.DecimalField(max_digits=10, decimal_places=2)),
            ("char_val", models.CharField(max_length=100)),
            ("text_val", models.TextField()),
            ("slug_val", models.SlugField(max_length=50)),
            ("email_val", models.EmailField(max_length=254)),
            ("url_val", models.URLField(max_length=200)),
            ("ip_val", models.GenericIPAddressField()),
            ("bool_val", models.BooleanField()),
            ("datetime_val", models.DateTimeField()),
            ("date_val", models.DateField()),
            ("time_val", models.TimeField()),
            ("duration_val", models.DurationField()),
            ("uuid_val", models.UUIDField()),
            ("json_val", models.JSONField()),
            ("binary_val", models.BinaryField()),
        ],
    )

    author_op = migrations.CreateModel(
        name="Author",
        fields=[
            ("id", models.BigAutoField(primary_key=True, serialize=False)),
            ("name", models.CharField(max_length=100)),
            ("code", models.CharField(max_length=20, unique=True)),
        ],
    )

    article_op = migrations.CreateModel(
        name="Article",
        fields=[
            ("id", models.BigAutoField(primary_key=True, serialize=False)),
            ("title", models.CharField(max_length=100)),
            ("rank", models.SmallIntegerField()),
            ("summary", models.CharField(max_length=200, null=True)),
            ("author", models.ForeignKey(on_delete=models.deletion.CASCADE, to="migfid.author")),
        ],
    )

    article_meta_op = migrations.CreateModel(
        name="Article",
        fields=[
            ("id", models.BigAutoField(primary_key=True, serialize=False)),
            ("title", models.CharField(max_length=100)),
            ("created_at", models.DateTimeField()),
            ("author", models.ForeignKey(on_delete=models.deletion.CASCADE, to="migfid.author")),
            (
                "editor",
                models.ForeignKey(
                    null=True,
                    on_delete=models.deletion.SET_NULL,
                    related_name="edited_articles",
                    to="migfid.author",
                ),
            ),
        ],
        options={
            "indexes": [
                models.Index(
                    fields=["title", "-created_at"],
                    name="migfid_art_title_created_idx",
                )
            ],
            "unique_together": {("author", "title")},
        },
    )

    title_rank_index = models.Index(
        fields=["title", "-rank"],
        name="migfid_art_title_rank_idx",
    )

    return [
        # --- CreateModel / DeleteModel ---
        {
            "name": "create_model_basic",
            "setup": [],
            "target": [tag_op],
        },
        {
            "name": "create_model_all_kinds",
            "setup": [],
            "target": [all_kinds_op],
        },
        {
            "name": "create_model_fk_and_meta",
            "setup": [author_op],
            "target": [article_meta_op],
        },
        {
            "name": "delete_model",
            "setup": [author_op, article_op],
            "target": [migrations.DeleteModel(name="Article")],
        },
        # --- AddField ---
        {
            "name": "add_field_nullable",
            "setup": [author_op, article_op],
            "target": [
                migrations.AddField(
                    model_name="article",
                    name="subtitle",
                    field=models.CharField(max_length=120, null=True),
                )
            ],
        },
        {
            "name": "add_field_default_int",
            "setup": [author_op, article_op],
            "target": [
                migrations.AddField(
                    model_name="article",
                    name="rating",
                    field=models.IntegerField(default=5),
                    preserve_default=False,
                )
            ],
        },
        {
            "name": "add_field_default_char",
            "setup": [author_op, article_op],
            "target": [
                migrations.AddField(
                    model_name="article",
                    name="status",
                    field=models.CharField(default="draft", max_length=20),
                    preserve_default=False,
                )
            ],
        },
        {
            "name": "add_field_default_bool",
            "setup": [author_op, article_op],
            "target": [
                migrations.AddField(
                    model_name="article",
                    name="featured",
                    field=models.BooleanField(default=True),
                    preserve_default=False,
                )
            ],
        },
        {
            "name": "add_field_fk",
            "setup": [author_op, article_op],
            "target": [
                migrations.AddField(
                    model_name="article",
                    name="reviewer",
                    field=models.ForeignKey(
                        null=True,
                        on_delete=models.deletion.SET_NULL,
                        related_name="reviewed_articles",
                        to="migfid.author",
                    ),
                )
            ],
        },
        # --- AlterField ---
        {
            "name": "alter_field_varchar_len",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="title",
                    field=models.CharField(max_length=250),
                )
            ],
        },
        {
            "name": "alter_field_int_type",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.BigIntegerField(),
                )
            ],
        },
        {
            "name": "alter_field_char_to_text",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="title",
                    field=models.TextField(),
                )
            ],
        },
        {
            "name": "alter_field_int_to_decimal",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.DecimalField(max_digits=10, decimal_places=2),
                )
            ],
        },
        {
            "name": "alter_field_add_positive_check",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.PositiveSmallIntegerField(),
                )
            ],
        },
        {
            "name": "alter_field_drop_positive_check",
            "setup": [
                migrations.CreateModel(
                    name="Article",
                    fields=[
                        ("id", models.BigAutoField(primary_key=True, serialize=False)),
                        ("rank", models.PositiveSmallIntegerField()),
                    ],
                )
            ],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.SmallIntegerField(),
                )
            ],
        },
        {
            "name": "alter_field_null_to_not_null",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="summary",
                    field=models.CharField(default="none", max_length=200),
                    preserve_default=False,
                )
            ],
        },
        {
            "name": "alter_field_not_null_to_null",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.SmallIntegerField(null=True),
                )
            ],
        },
        {
            "name": "alter_field_add_unique",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="title",
                    field=models.CharField(max_length=100, unique=True),
                )
            ],
        },
        {
            "name": "alter_field_remove_unique",
            "setup": [author_op],
            "target": [
                migrations.AlterField(
                    model_name="author",
                    name="code",
                    field=models.CharField(max_length=20),
                )
            ],
        },
        {
            "name": "alter_field_add_index",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterField(
                    model_name="article",
                    name="rank",
                    field=models.SmallIntegerField(db_index=True),
                )
            ],
        },
        # --- RemoveField / RenameField ---
        {
            "name": "remove_field",
            "setup": [author_op, article_op],
            "target": [migrations.RemoveField(model_name="article", name="rank")],
        },
        {
            "name": "rename_field",
            "setup": [author_op, article_op],
            "target": [
                migrations.RenameField(
                    model_name="article",
                    old_name="title",
                    new_name="headline",
                )
            ],
        },
        # --- RenameModel / AlterModelTable ---
        {
            "name": "rename_model",
            "setup": [tag_op],
            "target": [migrations.RenameModel(old_name="Tag", new_name="Label")],
        },
        {
            "name": "rename_model_with_fk",
            "setup": [author_op, article_op],
            "target": [migrations.RenameModel(old_name="Article", new_name="Post")],
        },
        {
            "name": "alter_model_table",
            "setup": [tag_op],
            "target": [migrations.AlterModelTable(name="tag", table="custom_tags")],
        },
        # --- AddIndex / RemoveIndex / AlterUniqueTogether ---
        {
            "name": "add_index",
            "setup": [author_op, article_op],
            "target": [migrations.AddIndex(model_name="article", index=title_rank_index)],
        },
        {
            "name": "remove_index",
            "setup": [
                author_op,
                article_op,
                migrations.AddIndex(model_name="article", index=title_rank_index),
            ],
            "target": [
                migrations.RemoveIndex(
                    model_name="article",
                    name="migfid_art_title_rank_idx",
                )
            ],
        },
        {
            "name": "alter_unique_together_add",
            "setup": [author_op, article_op],
            "target": [
                migrations.AlterUniqueTogether(
                    name="article",
                    unique_together={("author", "title")},
                )
            ],
        },
        {
            "name": "alter_unique_together_remove",
            "setup": [
                author_op,
                article_op,
                migrations.AlterUniqueTogether(
                    name="article",
                    unique_together={("author", "title")},
                ),
            ],
            "target": [
                migrations.AlterUniqueTogether(
                    name="article",
                    unique_together=set(),
                )
            ],
        },
    ]


def collect_case(conn: Any, case: dict[str, Any]) -> dict[str, Any]:
    """Collect Django sqlmigrate statements (and SQLite resulting schema) for one case."""
    from django.db import migrations
    from django.db.migrations.state import ProjectState

    global _PG_CONSTRAINTS

    setup_ops = case["setup"]
    target_ops = case["target"]
    state = ProjectState()

    if conn.vendor == "sqlite":
        conn.close()
        conn.connect()
        if setup_ops:
            m_setup = migrations.Migration("0001_setup", "migfid")
            m_setup.operations = setup_ops
            with conn.schema_editor(collect_sql=False, atomic=True) as editor:
                state = m_setup.apply(state, editor, collect_sql=False)
    else:
        for op in setup_ops:
            op.state_forwards("migfid", state)
        _PG_CONSTRAINTS = derive_pg_constraints(conn, state)

    m_target = migrations.Migration("0002_target", "migfid")
    m_target.operations = target_ops

    with conn.schema_editor(collect_sql=True, atomic=False) as editor:
        m_target.apply(state.clone(), editor, collect_sql=True)
    # Read collected_sql after __exit__ so deferred statements (FKs, indexes,
    # unique_together constraints) are included just as sqlmigrate prints them.
    statements = [s for s in editor.collected_sql if not s.startswith("--")]

    entry: dict[str, Any] = {
        "name": case["name"],
        "statements": statements,
    }

    if conn.vendor == "sqlite":
        entry["sqlite_rebuild"] = any(
            'CREATE TABLE "new__' in stmt for stmt in statements
        )
        with conn.schema_editor(collect_sql=False, atomic=True) as editor:
            m_target.apply(state, editor, collect_sql=False)
        entry["schema"] = snapshot_sqlite_schema(conn)

    return entry


def generate() -> tuple[int, int]:
    """Generate both SQLite and PostgreSQL migration SQL golden JSON files."""
    import django
    from django.db import connections

    cases = build_cases()
    GOLDEN_DIR.mkdir(parents=True, exist_ok=True)
    version = django.get_version()

    sqlite_entries = [collect_case(connections["default"], c) for c in cases]
    postgres_entries = [collect_case(connections["postgres"], c) for c in cases]

    sqlite_doc = {
        "django_version": version,
        "dialect": "sqlite",
        "cases": sqlite_entries,
    }
    postgres_doc = {
        "django_version": version,
        "dialect": "postgres",
        "cases": postgres_entries,
    }

    (GOLDEN_DIR / "sqlite.json").write_text(
        json.dumps(sqlite_doc, indent=2) + "\n", encoding="utf-8"
    )
    (GOLDEN_DIR / "postgres.json").write_text(
        json.dumps(postgres_doc, indent=2) + "\n", encoding="utf-8"
    )
    return len(sqlite_entries), len(postgres_entries)


def main() -> None:
    """Configure Django, compile all migration SQL cases, and write both goldens."""
    configure_django()
    import django

    n_sqlite, n_pg = generate()
    print(
        f"Django {django.get_version()}: wrote {n_sqlite} sqlite and "
        f"{n_pg} postgres migration SQL cases to {GOLDEN_DIR}"
    )


if __name__ == "__main__":
    main()
