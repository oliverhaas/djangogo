#!/usr/bin/env python3
"""Django-as-oracle renderer for the template fidelity harness.

Configures a minimal Django template engine, reads the canonical cases from
``fidelity/cases.json``, renders each template+context with Django's DTL, and
writes the exact output to ``fidelity/golden/<name>.txt``.

Run from the repository root:

    python3 fidelity/oracle/render.py

The Go differential test (``fidelity/render_test.go``) compares the project's
pongo2 engine against these committed golden files; it never invokes Python.
"""

from __future__ import annotations

import json
from pathlib import Path

import django
from django.conf import settings
from django.template import Context, Template

# Resolve paths relative to this script so it runs from any working directory.
FIDELITY_DIR = Path(__file__).resolve().parent.parent
CASES_PATH = FIDELITY_DIR / "cases.json"
GOLDEN_DIR = FIDELITY_DIR / "golden"


def configure_django() -> None:
    """Configure and initialize Django with a minimal autoescaping DTL engine."""
    settings.configure(
        TEMPLATES=[
            {
                "BACKEND": "django.template.backends.django.DjangoTemplates",
                "OPTIONS": {"autoescape": True},
            }
        ]
    )
    django.setup()


def render_cases() -> int:
    """Render every case to ``golden/<name>.txt``; return the number written."""
    cases = json.loads(CASES_PATH.read_text(encoding="utf-8"))
    GOLDEN_DIR.mkdir(exist_ok=True)

    written = 0
    for case in cases:
        name = case["name"]
        template = Template(case["template"])
        output = template.render(Context(case.get("context", {})))
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
