#!/usr/bin/env python3
"""Regenerate every Django oracle golden suite in fidelity/golden/.

Runs each oracle generator in its own Python subprocess so Django settings
(which may only be configured once per process) stay isolated between suites:

  1. fidelity/oracle/render.py        -> fidelity/golden/*.txt
  2. fidelity/oracle/query_sql.py     -> fidelity/golden/query_sql/{sqlite,postgres}.json
  3. fidelity/oracle/migration_sql.py -> fidelity/golden/migration_sql/{sqlite,postgres}.json
  4. fidelity/oracle/mail.py          -> mail/testdata/headers.json

The mail goldens follow the email package of Python 3.14, which fixes
header folding bugs of earlier versions. mail.py needs no Django and runs
under the interpreter that `uv python find 3.14` or `python3.14` names.

Run from anywhere via:
    go generate ./fidelity/...
or from the repository root via:
    python3 fidelity/oracle/generate.py
"""

from __future__ import annotations

import shutil
import subprocess
import sys
from pathlib import Path

ORACLE_DIR = Path(__file__).resolve().parent
SCRIPTS = (
    "render.py",
    "query_sql.py",
    "migration_sql.py",
)


def python314() -> str:
    """Return the path of a Python 3.14 interpreter for mail.py."""
    if shutil.which("uv"):
        found = subprocess.run(["uv", "python", "find", "3.14"], capture_output=True, text=True, check=False)
        if found.returncode == 0 and found.stdout.strip():
            return found.stdout.strip()
    python = shutil.which("python3.14")
    if python is None:
        sys.exit("mail.py needs Python 3.14: install it with `uv python install 3.14`.")
    return python


def main() -> None:
    """Execute each oracle script in order and fail fast on any non-zero exit."""
    for name in SCRIPTS:
        script = ORACLE_DIR / name
        subprocess.run([sys.executable, str(script)], check=True)
    subprocess.run([python314(), "-I", str(ORACLE_DIR / "mail.py")], check=True)


if __name__ == "__main__":
    main()
