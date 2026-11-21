#!/usr/bin/env python3
"""Render fidelity/cases.json with Django's DTL into fidelity/golden/<name>.txt."""

# Run from the repository root: python3 fidelity/oracle/render.py
# The case format is described in fidelity/divergences.md ("Case format").

from __future__ import annotations

import json
import sys
import types
from datetime import UTC, datetime
from pathlib import Path

import django
from django.conf import settings
from django.template import Context, Engine
from django.utils.safestring import mark_safe

# Resolve paths relative to this script so it runs from any working directory.
FIDELITY_DIR = Path(__file__).resolve().parent.parent
CASES_PATH = FIDELITY_DIR / "cases.json"
GOLDEN_DIR = FIDELITY_DIR / "golden"

_URLCONF_MODULE = "_fidelity_oracle_urls"


def _install_urlconf() -> None:
    """Register the shared URLconf module matching fidelityRouter() in render_test.go."""
    from django.urls import include, path, re_path

    def _nop(*_args: object, **_kwargs: object) -> None:
        return None

    blog_patterns = (
        [path("<int:pk>/", _nop, name="detail")],
        "blog",
    )

    mod = types.ModuleType(_URLCONF_MODULE)
    mod.urlpatterns = [
        path("", _nop, name="index"),
        path("articles/<int:pk>/", _nop, name="article_detail"),
        path("articles/<int:year>/<slug:slug>/", _nop, name="article_archive"),
        re_path(r"^search/(?P<q>[^/]+)/$", _nop, name="search"),
        path("blog/", include(blog_patterns, namespace="blog")),
    ]
    sys.modules[_URLCONF_MODULE] = mod


def configure_django() -> None:
    """Configure and initialize Django for English rendering in UTC."""
    settings.configure(
        USE_TZ=True,
        TIME_ZONE="UTC",
        USE_I18N=False,
        STATIC_URL="/static/",
        ROOT_URLCONF=_URLCONF_MODULE,
        INSTALLED_APPS=["django.contrib.humanize"],
    )
    django.setup()
    _install_urlconf()


def decode(value: object) -> object:
    """Turn the JSON conventions of a context value into Python objects."""
    if isinstance(value, dict):
        if set(value) == {"__datetime__"}:
            # A Go time.Time always has a zone, so the counterpart is an aware datetime.
            return datetime.fromisoformat(value["__datetime__"]).replace(tzinfo=UTC)
        if set(value) == {"__safe__"}:
            return mark_safe(value["__safe__"])
        return {key: decode(item) for key, item in value.items()}
    if isinstance(value, list):
        return [decode(item) for item in value]
    return value


def render_cases() -> int:
    """Render every case to ``golden/<name>.txt``; return the number written."""
    cases = json.loads(CASES_PATH.read_text(encoding="utf-8"))
    GOLDEN_DIR.mkdir(exist_ok=True)
    wanted = {f"{case['name']}.txt" for case in cases}
    for stale in GOLDEN_DIR.glob("*.txt"):
        if stale.name not in wanted:
            stale.unlink()

    written = 0
    for case in cases:
        name = case["name"]
        engine = Engine(
            autoescape=True,
            loaders=[("django.template.loaders.locmem.Loader", case.get("templates", {}))],
            libraries={
                "static": "django.templatetags.static",
                "humanize": "django.contrib.humanize.templatetags.humanize",
            },
        )
        template = engine.from_string(case["template"])
        output = template.render(Context(decode(case.get("context", {}))))
        (GOLDEN_DIR / f"{name}.txt").write_text(output, encoding="utf-8")
        written += 1
    return written


def main() -> None:
    """Configure Django, render all cases, and report the version used."""
    configure_django()
    count = render_cases()
    print(f"Django {django.get_version()}: wrote {count} golden file(s) to {GOLDEN_DIR}")


if __name__ == "__main__":
    main()
