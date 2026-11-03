#!/usr/bin/env python3
"""Create a consistent OMC SQLite snapshot; retain seven snapshots."""

import argparse
import contextlib
import datetime
import json
import os
from pathlib import Path
import re
import sqlite3


def backup(database, directory):
    database = database.resolve(strict=True)
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(directory, 0o700)
    name = datetime.datetime.now(datetime.timezone.utc).strftime("omc-%Y%m%dT%H%M%S.%fZ.db")
    destination = directory / name
    fd = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    try:
        with contextlib.closing(sqlite3.connect(database.as_uri() + "?mode=ro", uri=True)) as source:
            with contextlib.closing(sqlite3.connect(destination)) as target:
                source.backup(target)
                if target.execute("PRAGMA integrity_check").fetchone() != ("ok",):
                    raise ValueError("backup integrity check failed")
        owned = sorted(p for p in directory.iterdir() if re.fullmatch(
            r"omc-\d{8}T\d{6}\.\d{6}Z\.db", p.name) and p.is_file())
        for old in owned[:-7]:
            old.unlink()
        return destination
    except Exception:
        destination.unlink(missing_ok=True)
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--backup-dir", type=Path, required=True)
    args = parser.parse_args()
    try:
        result = backup(args.database, args.backup_dir)
    except (OSError, ValueError, sqlite3.Error) as exc:
        print(json.dumps({"backup_failed": type(exc).__name__}))
        raise SystemExit(1)
    print(json.dumps({"backup": result.name, "integrity": "ok"}))


if __name__ == "__main__":
    main()
