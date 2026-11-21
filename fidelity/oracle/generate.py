#!/usr/bin/env python3
"""Regenerate every Django oracle golden suite in fidelity/golden/.

Runs each oracle generator in its own Python subprocess so Django settings
(which may only be configured once per process) stay isolated between suites:

  1. fidelity/oracle/render.py        -> fidelity/golden/*.txt
  2. fidelity/oracle/query_sql.py     -> fidelity/golden/query_sql/{sqlite,postgres}.json
  3. fidelity/oracle/migration_sql.py -> fidelity/golden/migration_sql/{sqlite,postgres}.json

Run from anywhere via:
    go generate ./fidelity/...
or from the repository root via:
    python3 fidelity/oracle/generate.py
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

ORACLE_DIR = Path(__file__).resolve().parent
SCRIPTS = (
    "render.py",
    "query_sql.py",
    "migration_sql.py",
)


def main() -> None:
    """Execute each oracle script in order and fail fast on any non-zero exit."""
    for name in SCRIPTS:
        script = ORACLE_DIR / name
        subprocess.run([sys.executable, str(script)], check=True)


if __name__ == "__main__":
    main()
