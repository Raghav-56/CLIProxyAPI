#!/usr/bin/env python3
"""Refuse startup if the approved analytics profile has been lost."""

import argparse
import contextlib
import json
import os
from pathlib import Path
import sqlite3

from bootstrap import POLICY


def check_policy(database, environment):
    for name in ("OMCPA_UPDATER_ENABLED", "OMCPA_UPDATE_CHECK_ENABLED",
                 "OMCPA_UPDATE_CHECK_ON_PAGE_LOAD"):
        if environment.get(name) != "false":
            raise ValueError("update actions/checks must remain disabled")
    if environment.get("OMCPA_USAGE_INGEST_MODE") not in ("off", "subscribe"):
        raise ValueError("ingest fallback is not permitted")
    if (environment.get("OMCPA_CPA_BASE_URL") != "http://127.0.0.1:8317" or
            environment.get("OMCPA_LISTEN_ADDR") != "127.0.0.1:18317"):
        raise ValueError("the approved loopback endpoints are required")
    with contextlib.closing(sqlite3.connect(database.resolve().as_uri() + "?mode=ro", uri=True)) as db:
        actual = {key: json.loads(value) for key, value in db.execute(
            "SELECT pref_key,pref_value FROM ui_preferences WHERE pref_key IN (?,?)", tuple(POLICY))}
        if json.dumps(actual, sort_keys=True) != json.dumps(POLICY, sort_keys=True):
            raise ValueError("required disabled preferences are missing or changed")
        for table in ("credential_warmup_policies", "credential_warmup_state", "credential_warmup_runs"):
            if db.execute("SELECT COUNT(*) FROM " + table).fetchone()[0]:
                raise ValueError("warmup state requires review")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", type=Path, required=True)
    args = parser.parse_args()
    try:
        check_policy(args.database, os.environ)
    except (OSError, ValueError, sqlite3.Error) as exc:
        print(json.dumps({"policy_check_failed": type(exc).__name__}))
        raise SystemExit(1)
    print('{"analytics_policy":"verified"}')


if __name__ == "__main__":
    main()
