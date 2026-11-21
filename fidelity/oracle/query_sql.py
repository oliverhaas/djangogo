#!/usr/bin/env python3
"""Generate Django query SQL goldens into fidelity/golden/query_sql/{sqlite,postgres}.json."""

# Run from the repository root: python3 fidelity/oracle/query_sql.py
# SQLite cases also run on the seeded fixture, so the goldens hold the result rows.
# PostgreSQL SQL is compiled offline, so no server or Python driver is needed.

from __future__ import annotations

import json
import sys
import types
from datetime import UTC, date, datetime, time
from pathlib import Path
from typing import Any

FIDELITY_DIR = Path(__file__).resolve().parent.parent
GOLDEN_DIR = FIDELITY_DIR / "golden" / "query_sql"

# Target PostgreSQL version used for offline SQL compilation (PostgreSQL 16).
OFFLINE_PG_VERSION = 160000


def install_psycopg_shim_if_needed() -> None:
    """Stub psycopg so that Django's PostgreSQL backend imports without a driver."""
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

    class _AutoMod(types.ModuleType):
        def __getattr__(self, name: str) -> Any:
            cls = type(name, (_Dummy,), {})
            setattr(self, name, cls)
            return cls

    psycopg = _AutoMod("psycopg")
    psycopg.__version__ = "3.2.0"
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
    # Set pg_version directly so DatabaseFeatures never tries to query a server.
    connections["postgres"].pg_version = OFFLINE_PG_VERSION


def define_models() -> tuple[Any, Any, Any, Any]:
    """Define the canonical fid app models matching fidelity/query_sql_test.go."""
    from django.db import models

    class Country(models.Model):
        name = models.CharField(max_length=100)
        code = models.CharField(max_length=10)

        class Meta:
            app_label = "fid"

    class Publisher(models.Model):
        name = models.CharField(max_length=100)
        country = models.ForeignKey(Country, null=True, on_delete=models.SET_NULL)

        class Meta:
            app_label = "fid"

    class Author(models.Model):
        name = models.CharField(max_length=100)
        age = models.IntegerField()
        active = models.BooleanField()
        publisher = models.ForeignKey(Publisher, null=True, on_delete=models.SET_NULL)

        class Meta:
            app_label = "fid"

    class Book(models.Model):
        title = models.CharField(max_length=200)
        pages = models.IntegerField()
        published = models.BooleanField()
        created_at = models.DateTimeField()
        author = models.ForeignKey(Author, on_delete=models.CASCADE)
        editor = models.ForeignKey(
            Author,
            null=True,
            on_delete=models.SET_NULL,
            related_name="edited_books",
        )

        class Meta:
            app_label = "fid"

    return Country, Publisher, Author, Book


def setup_sqlite_fixture(
    Country: Any, Publisher: Any, Author: Any, Book: Any
) -> None:
    """Create schema and seed the canonical rows in SQLite."""
    from django.db import connections

    conn = connections["default"]
    with conn.schema_editor() as editor:
        editor.create_model(Country)
        editor.create_model(Publisher)
        editor.create_model(Author)
        editor.create_model(Book)

    c1 = Country.objects.create(id=1, name="United Kingdom", code="UK")
    c2 = Country.objects.create(id=2, name="United States", code="US")

    p1 = Publisher.objects.create(id=1, name="Analytic Press", country=c1)
    p2 = Publisher.objects.create(id=2, name="Compiler House", country=c2)
    p3 = Publisher.objects.create(id=3, name="Indie Press", country=None)

    a1 = Author.objects.create(id=1, name="Ada Lovelace", age=36, active=True, publisher=p1)
    a2 = Author.objects.create(id=2, name="Grace Hopper", age=85, active=True, publisher=p2)
    a3 = Author.objects.create(id=3, name="Alan Turing", age=41, active=False, publisher=p3)
    a4 = Author.objects.create(id=4, name="Barbara Liskov", age=84, active=True, publisher=None)

    Book.objects.create(
        id=1,
        title="Notes on the Engine",
        pages=120,
        published=True,
        created_at=datetime(2024, 1, 15, 10, 0, 0, tzinfo=UTC),
        author=a1,
        editor=a2,
    )
    Book.objects.create(
        id=2,
        title="Compilers 101",
        pages=340,
        published=True,
        created_at=datetime(2024, 3, 20, 14, 30, 0, tzinfo=UTC),
        author=a2,
        editor=None,
    )
    Book.objects.create(
        id=3,
        title="Computable Numbers",
        pages=210,
        published=False,
        created_at=datetime(2024, 6, 1, 8, 15, 0, tzinfo=UTC),
        author=a3,
        editor=a1,
    )
    Book.objects.create(
        id=4,
        title="Engine Design",
        pages=280,
        published=True,
        created_at=datetime(2024, 9, 10, 18, 45, 0, tzinfo=UTC),
        author=a1,
        editor=None,
    )
    Book.objects.create(
        id=5,
        title="Abstraction Patterns",
        pages=180,
        published=False,
        created_at=datetime(2024, 11, 5, 12, 0, 0, tzinfo=UTC),
        author=a4,
        editor=a2,
    )


class _CapturedSQL(Exception):
    """Control-flow exception carrying compiled (sql, params) out of execute_sql."""

    def __init__(self, sql_text: str, params: tuple[Any, ...]) -> None:
        self.sql = sql_text
        self.params = params


def capture_compiled_sql(fn: Any) -> tuple[str, list[Any], bool]:
    """Intercept SQLCompiler.execute_sql to capture compiled (sql, params) without DB I/O.

    Returns (sql, params, no_query). no_query is True when Django raises
    EmptyResultSet and skips querying the database altogether.
    """
    from django.core.exceptions import EmptyResultSet
    from django.db.models.sql.compiler import SQLCompiler

    orig_execute = SQLCompiler.execute_sql

    def spy(self: Any, *args: Any, **kwargs: Any) -> Any:
        sql_text, params = self.as_sql()
        raise _CapturedSQL(sql_text, params)

    SQLCompiler.execute_sql = spy
    try:
        fn()
    except _CapturedSQL as cap:
        return cap.sql, [encode_value(p) for p in cap.params], False
    except EmptyResultSet:
        return "", [], True
    finally:
        SQLCompiler.execute_sql = orig_execute
    return "", [], True


def compile_delete_sql(qs: Any, using: str) -> tuple[str, list[Any], bool]:
    """Compile Django's fast-delete SQL for a queryset on the given backend alias."""
    from django.core.exceptions import EmptyResultSet
    from django.db.models import sql

    dq = qs.query.chain(sql.DeleteQuery)
    try:
        sql_text, params = dq.get_compiler(using=using).as_sql()
        return sql_text, [encode_value(p) for p in params], False
    except EmptyResultSet:
        return "", [], True


def encode_value(value: Any) -> Any:
    """Encode a Python value into JSON-compatible canonical form."""
    if isinstance(value, bool):
        return value
    if isinstance(value, int):
        return int(value)
    if isinstance(value, float):
        return float(value)
    if isinstance(value, datetime):
        dt = value.astimezone(UTC) if value.tzinfo else value.replace(tzinfo=UTC)
        layout = "%Y-%m-%dT%H:%M:%S.%fZ" if dt.microsecond else "%Y-%m-%dT%H:%M:%SZ"
        return {"__datetime__": dt.strftime(layout)}
    if isinstance(value, (date, time)):
        return value.isoformat()
    if isinstance(value, dict):
        return {k: encode_value(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [encode_value(v) for v in value]
    return value


def build_cases(Country: Any, Publisher: Any, Author: Any, Book: Any) -> list[dict[str, Any]]:
    """Return the canonical table of query SQL differential cases."""
    from django.db.models import Avg, Count, F, Max, Min, Q, Sum

    dt_book2 = datetime(2024, 3, 20, 14, 30, 0, tzinfo=UTC)
    dt_start = datetime(2024, 2, 1, 0, 0, 0, tzinfo=UTC)
    dt_end = datetime(2024, 10, 1, 0, 0, 0, tzinfo=UTC)

    def books(using: str) -> Any:
        return Book.objects.using(using)

    def countries(using: str) -> Any:
        return Country.objects.using(using)

    return [
        # --- Filter with each lookup ---
        {
            "name": "filter_exact_char",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title="Compilers 101"),
        },
        {
            "name": "filter_exact_int",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages=210),
        },
        {
            "name": "filter_exact_bool",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(published=True).order_by("id"),
        },
        {
            "name": "filter_exact_bool_false",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(published=False).order_by("id"),
        },
        {
            "name": "filter_exact_datetime",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at=dt_book2),
        },
        {
            "name": "filter_exact_null",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(editor=None).order_by("id"),
        },
        {
            "name": "filter_exact_fk_id",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(author=1).order_by("id"),
        },
        {
            "name": "filter_iexact",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__iexact="compilers 101"),
        },
        {
            "name": "filter_gt",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__gt=200).order_by("id"),
        },
        {
            "name": "filter_gte",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__gte=210).order_by("id"),
        },
        {
            "name": "filter_lt",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__lt=200).order_by("id"),
        },
        {
            "name": "filter_lte",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__lte=210).order_by("id"),
        },
        {
            "name": "filter_contains",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__contains="Engine").order_by("id"),
        },
        {
            "name": "filter_icontains",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__icontains="engine").order_by("id"),
        },
        {
            "name": "filter_startswith",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__startswith="Comp").order_by("id"),
        },
        {
            "name": "filter_istartswith",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__istartswith="comp").order_by("id"),
        },
        {
            "name": "filter_endswith",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__endswith="101"),
        },
        {
            "name": "filter_iendswith",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__iendswith="design"),
        },
        {
            "name": "filter_in",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__in=[120, 210, 340]).order_by("id"),
        },
        {
            "name": "filter_in_empty",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__in=[]),
        },
        {
            "name": "filter_range_int",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__range=[150, 280]).order_by("id"),
        },
        {
            "name": "filter_range_datetime",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__range=[dt_start, dt_end]).order_by("id"),
        },
        # --- Date and time transforms ---
        {
            "name": "filter_transform_year",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__year=2024).order_by("id"),
        },
        {
            "name": "filter_transform_month_gte",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__month__gte=6).order_by("id"),
        },
        {
            "name": "filter_transform_date",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__date=date(2024, 3, 20)),
        },
        {
            "name": "filter_transform_week_day",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__week_day=3).order_by("id"),
        },
        {
            "name": "filter_transform_hour_lt",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__hour__lt=12).order_by("id"),
        },
        {
            "name": "filter_transform_time",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(created_at__time=time(14, 30)),
        },
        {
            "name": "exclude_transform_quarter",
            "kind": "model_ids",
            "build": lambda u: books(u).exclude(created_at__quarter=2).order_by("id"),
        },
        {
            "name": "order_by_transform",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("-created_at__hour", "id"),
        },
        {
            "name": "order_by_transforms_same_column",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by(
                "created_at__year", "-created_at__month", "id"
            ),
        },
        {
            "name": "values_transform_groupby",
            "kind": "values",
            "build": lambda u: books(u)
            .values("created_at__quarter")
            .annotate(n=Count("id"))
            .order_by("created_at__quarter"),
        },
        {
            "name": "values_transform_date",
            "kind": "values",
            "build": lambda u: books(u).values("id", "created_at__date").order_by("id"),
        },
        {
            "name": "filter_isnull_true",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(editor__isnull=True).order_by("id"),
        },
        {
            "name": "filter_isnull_false",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(editor__isnull=False).order_by("id"),
        },
        {
            "name": "filter_regex",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__regex="^Comp").order_by("id"),
        },
        {
            "name": "filter_iregex",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(title__iregex="^comp").order_by("id"),
        },
        {
            "name": "filter_f_expr",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(pages__gt=F("author__age") * 4).order_by("id"),
        },
        # --- Exclude ---
        {
            "name": "exclude_non_null",
            "kind": "model_ids",
            "build": lambda u: books(u).exclude(pages=210).order_by("id"),
        },
        {
            "name": "exclude_nullable_fk",
            "kind": "model_ids",
            "build": lambda u: books(u).exclude(editor=2).order_by("id"),
        },
        {
            "name": "exclude_multi",
            "kind": "model_ids",
            "build": lambda u: books(u).exclude(author=1, pages__gt=200).order_by("id"),
        },
        {
            "name": "exclude_relation",
            "kind": "model_ids",
            "build": lambda u: books(u).exclude(author__name="Ada Lovelace").order_by("id"),
        },
        # --- Q with OR / AND / NOT ---
        {
            "name": "q_or",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(Q(pages__lt=150) | Q(pages__gt=300)).order_by("id"),
        },
        {
            "name": "q_and_or",
            "kind": "model_ids",
            "build": lambda u: books(u)
            .filter(Q(pages__gte=120) & (Q(pages__gt=300) | Q(title="Notes on the Engine")))
            .order_by("id"),
        },
        {
            "name": "q_not",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(~Q(pages__gt=200)).order_by("id"),
        },
        {
            "name": "q_or_nullable_relation",
            "kind": "model_ids",
            "build": lambda u: books(u)
            .filter(Q(pages__gt=300) | Q(author__publisher__name="Indie Press"))
            .order_by("id"),
        },
        # --- Relation-spanning joins (INNER vs LEFT OUTER) ---
        {
            "name": "join_inner_non_null_fk",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(author__name="Ada Lovelace").order_by("id"),
        },
        {
            "name": "join_outer_nullable_fk_isnull",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(editor__name__isnull=True).order_by("id"),
        },
        {
            "name": "join_inner_demoted_nullable_fk",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(editor__name="Grace Hopper").order_by("id"),
        },
        {
            "name": "join_chain_inner",
            "kind": "model_ids",
            "build": lambda u: books(u).filter(author__publisher__country__code="UK").order_by("id"),
        },
        {
            "name": "join_two_fks_same_table",
            "kind": "model_ids",
            "build": lambda u: books(u)
            .filter(author__name="Ada Lovelace", editor__name="Grace Hopper")
            .order_by("id"),
        },
        # --- Ordering including through relations ---
        {
            "name": "order_by_asc_desc",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("published", "-pages", "id"),
        },
        {
            "name": "order_by_relation",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("author__name", "id"),
        },
        {
            "name": "order_by_nullable_relation",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("editor__name", "id"),
        },
        {
            "name": "order_by_chain",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("author__publisher__name", "id"),
        },
        {
            "name": "order_by_random",
            "kind": "model_ids_unordered",
            "build": lambda u: books(u).order_by("?"),
        },
        # --- Limit / Offset ---
        {
            "name": "limit_only",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("id")[:2],
        },
        {
            "name": "offset_only",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("id")[2:],
        },
        {
            "name": "limit_and_offset",
            "kind": "model_ids",
            "build": lambda u: books(u).order_by("id")[1:3],
        },
        # --- Count ---
        {
            "name": "count_all",
            "kind": "count",
            "build": lambda u: books(u),
        },
        {
            "name": "count_filtered",
            "kind": "count",
            "build": lambda u: books(u).filter(pages__gte=200),
        },
        {
            "name": "count_distinct",
            "kind": "count",
            "build": lambda u: books(u).values("author").distinct(),
        },
        # --- Exists ---
        {
            "name": "exists_all",
            "kind": "exists",
            "build": lambda u: books(u),
        },
        {
            "name": "exists_filtered",
            "kind": "exists",
            "build": lambda u: books(u).filter(pages__gt=250),
        },
        {
            "name": "exists_none",
            "kind": "exists",
            "build": lambda u: books(u).filter(pages__gt=9999),
        },
        # --- Values / ValuesList ---
        {
            "name": "values_fields",
            "kind": "values",
            "build": lambda u: books(u).order_by("id").values("id", "title", "author__name"),
        },
        {
            "name": "values_all_fields",
            "kind": "values",
            "build": lambda u: countries(u).order_by("id").values(),
        },
        {
            "name": "values_list",
            "kind": "values_list",
            "build": lambda u: books(u).order_by("id").values_list("title", "pages"),
        },
        {
            "name": "values_list_flat",
            "kind": "values_list_flat",
            "build": lambda u: books(u).order_by("id").values_list("title", flat=True),
        },
        # --- Distinct ---
        {
            "name": "distinct_values",
            "kind": "values",
            "build": lambda u: books(u).values("published").distinct().order_by("published"),
        },
        {
            "name": "distinct_values_relation",
            "kind": "values",
            "build": lambda u: books(u).values("author__name").distinct().order_by("author__name"),
        },
        {
            "name": "distinct_extra_order",
            "kind": "values",
            "build": lambda u: books(u).values("author_id").distinct().order_by("pages", "id"),
        },
        # --- Aggregate and Annotate with GROUP BY ---
        {
            "name": "aggregate_basic",
            "kind": "aggregate",
            "build": lambda u: books(u),
            "agg": lambda: (
                (Count("id"), Sum("pages"), Avg("pages"), Min("pages"), Max("pages")),
                {},
            ),
        },
        {
            "name": "aggregate_filtered",
            "kind": "aggregate",
            "build": lambda u: books(u).filter(pages__gte=200),
            "agg": lambda: ((), {"total_pages": Sum("pages"), "book_count": Count("id")}),
        },
        {
            "name": "aggregate_distinct",
            "kind": "aggregate",
            "build": lambda u: books(u),
            "agg": lambda: ((), {"distinct_authors": Count("author", distinct=True)}),
        },
        {
            "name": "annotate_values_groupby",
            "kind": "values",
            "build": lambda u: books(u)
            .values("author__name")
            .annotate(book_count=Count("id"), total_pages=Sum("pages"))
            .order_by("author__name"),
        },
        {
            "name": "annotate_having",
            "kind": "values",
            "build": lambda u: books(u)
            .values("author__name")
            .annotate(book_count=Count("id"))
            .filter(book_count__gte=2)
            .order_by("author__name"),
        },
        {
            "name": "annotate_expr",
            "kind": "values",
            "build": lambda u: books(u)
            .annotate(double_pages=F("pages") * 2)
            .values("title", "double_pages")
            .order_by("id"),
        },
        # --- Update and Delete ---
        {
            "name": "update_simple",
            "kind": "update",
            "build": lambda u: books(u).filter(pages__lt=200),
            "update": lambda: {"published": True},
        },
        {
            "name": "update_relation_expr",
            "kind": "update",
            "build": lambda u: books(u).filter(author__name="Ada Lovelace"),
            "update": lambda: {"pages": F("pages") + 10},
        },
        {
            "name": "delete_simple",
            "kind": "delete",
            "build": lambda u: books(u).filter(pages__lt=200),
        },
        {
            "name": "delete_relation",
            "kind": "delete",
            "build": lambda u: books(u).filter(author__name="Grace Hopper"),
        },
        # --- select_related ---
        {
            "name": "select_related_single",
            "kind": "select_related",
            "build": lambda u: books(u).select_related("author").order_by("id"),
            "extract": lambda rows: [
                {"id": b.id, "author": b.author.name} for b in rows
            ],
        },
        {
            "name": "select_related_nullable",
            "kind": "select_related",
            "build": lambda u: books(u).select_related("editor").order_by("id"),
            "extract": lambda rows: [
                {"id": b.id, "editor": b.editor.name if b.editor else None} for b in rows
            ],
        },
        {
            "name": "select_related_chain",
            "kind": "select_related",
            "build": lambda u: books(u)
            .select_related("author__publisher__country")
            .order_by("id"),
            "extract": lambda rows: [
                {
                    "id": b.id,
                    "author": b.author.name,
                    "publisher": b.author.publisher.name if b.author.publisher else None,
                    "country": (
                        b.author.publisher.country.code
                        if b.author.publisher and b.author.publisher.country
                        else None
                    ),
                }
                for b in rows
            ],
        },
        {
            "name": "select_related_multiple",
            "kind": "select_related",
            "build": lambda u: books(u).select_related("author", "editor").order_by("id"),
            "extract": lambda rows: [
                {
                    "id": b.id,
                    "author": b.author.name,
                    "editor": b.editor.name if b.editor else None,
                }
                for b in rows
            ],
        },
    ]


def compile_case(case: dict[str, Any], using: str) -> tuple[str, list[Any], bool]:
    """Compile the case's Django SQL and bound parameters for the given DB alias."""
    kind = case["kind"]
    qs = case["build"](using)
    if kind in ("model_ids", "model_ids_unordered", "values", "values_list", "values_list_flat", "select_related"):
        return capture_compiled_sql(lambda: list(qs))
    if kind == "count":
        return capture_compiled_sql(lambda: qs.count())
    if kind == "exists":
        return capture_compiled_sql(lambda: qs.exists())
    if kind == "aggregate":
        args, kwargs = case["agg"]()
        return capture_compiled_sql(lambda: qs.aggregate(*args, **kwargs))
    if kind == "update":
        kwargs = case["update"]()
        return capture_compiled_sql(lambda: qs.update(**kwargs))
    if kind == "delete":
        return compile_delete_sql(qs, using)
    raise ValueError(f"unknown case kind: {kind}")


def execute_case_sqlite(case: dict[str, Any], Book: Any) -> Any:
    """Execute the case on the seeded SQLite DB and return its canonical semantic result.

    Mutating cases (update, delete) run inside a rolled-back atomic block so
    every case sees the exact same initial fixture state.
    """
    from django.db import transaction

    kind = case["kind"]
    qs = case["build"]("default")
    if kind == "model_ids":
        return [obj.id for obj in qs]
    if kind == "model_ids_unordered":
        return sorted(obj.id for obj in qs)
    if kind == "count":
        return qs.count()
    if kind == "exists":
        return qs.exists()
    if kind == "values":
        return [encode_value(row) for row in qs]
    if kind == "values_list":
        return [encode_value(list(row)) for row in qs]
    if kind == "values_list_flat":
        return [encode_value(val) for val in qs]
    if kind == "aggregate":
        args, kwargs = case["agg"]()
        return encode_value(qs.aggregate(*args, **kwargs))
    if kind == "select_related":
        return encode_value(case["extract"](list(qs)))
    if kind == "update":
        kwargs = case["update"]()
        with transaction.atomic(using="default"):
            affected = qs.update(**kwargs)
            snapshot = list(
                Book.objects.using("default")
                .order_by("id")
                .values("id", "pages", "published")
            )
            transaction.set_rollback(True, using="default")
        return {"affected": affected, "books": encode_value(snapshot)}
    if kind == "delete":
        with transaction.atomic(using="default"):
            affected, _ = qs.delete()
            remaining = list(
                Book.objects.using("default")
                .order_by("id")
                .values_list("id", flat=True)
            )
            transaction.set_rollback(True, using="default")
        return {"affected": affected, "remaining_ids": remaining}
    raise ValueError(f"unknown case kind: {kind}")


def generate() -> tuple[int, str]:
    """Generate sqlite.json and postgres.json goldens; return (count, django_version)."""
    import django

    configure_django()
    Country, Publisher, Author, Book = define_models()
    setup_sqlite_fixture(Country, Publisher, Author, Book)
    cases = build_cases(Country, Publisher, Author, Book)

    sqlite_entries: list[dict[str, Any]] = []
    postgres_entries: list[dict[str, Any]] = []

    for case in cases:
        sq_sql, sq_params, sq_no_query = compile_case(case, "default")
        pg_sql, pg_params, pg_no_query = compile_case(case, "postgres")
        result = execute_case_sqlite(case, Book)

        sqlite_entry: dict[str, Any] = {
            "name": case["name"],
            "kind": case["kind"],
            "sql": sq_sql,
            "params": sq_params,
            "result": result,
        }
        if sq_no_query:
            sqlite_entry["no_query"] = True
        sqlite_entries.append(sqlite_entry)

        pg_entry: dict[str, Any] = {
            "name": case["name"],
            "kind": case["kind"],
            "sql": pg_sql,
            "params": pg_params,
        }
        if pg_no_query:
            pg_entry["no_query"] = True
        postgres_entries.append(pg_entry)

    GOLDEN_DIR.mkdir(parents=True, exist_ok=True)
    version = django.get_version()
    for filename, dialect, entries in (
        ("sqlite.json", "sqlite", sqlite_entries),
        ("postgres.json", "postgres", postgres_entries),
    ):
        payload = {
            "django_version": version,
            "dialect": dialect,
            "cases": entries,
        }
        out_path = GOLDEN_DIR / filename
        out_path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")

    return len(cases), version


def main() -> None:
    """Run the oracle and print a summary."""
    count, version = generate()
    print(f"Django {version}: wrote {count} query SQL cases to {GOLDEN_DIR}")


if __name__ == "__main__":
    main()
