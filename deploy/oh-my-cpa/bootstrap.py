#!/usr/bin/env python3
"""Initialize a fresh pinned OMC database without reaching the live proxy."""

import argparse
import contextlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import threading
import time
import urllib.error
import urllib.request


POLICY = {
    "omc_quota_auto_refresh": {"interval_minutes": 0, "refresh_on_reset": False},
    "omc_plugin_source_enabled": False,
}


def seed_preferences(database):
    with contextlib.closing(sqlite3.connect(database.as_uri() + "?mode=rw", uri=True)) as db:
        with db:
            db.execute("BEGIN IMMEDIATE")
            if db.execute("SELECT COUNT(*) FROM credential_warmup_policies").fetchone()[0]:
                raise ValueError("existing warmup policies require separate review")
            for key, value in POLICY.items():
                db.execute(
                    "INSERT INTO ui_preferences(pref_key,pref_value,updated_at_ms) VALUES(?,?,?) "
                    "ON CONFLICT(pref_key) DO UPDATE SET pref_value=excluded.pref_value, "
                    "updated_at_ms=excluded.updated_at_ms",
                    (key, json.dumps(value, separators=(",", ":")), time.time_ns() // 1_000_000),
                )
            actual = {
                key: json.loads(value)
                for key, value in db.execute(
                    "SELECT pref_key,pref_value FROM ui_preferences "
                    "WHERE pref_key IN (?,?)", tuple(POLICY)
                )
            }
            if actual != POLICY:
                raise ValueError("stored bootstrap policy verification failed")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def bootstrap(binary, data_dir, env_file, port):
    binary = binary.resolve(strict=True)
    data_dir = data_dir.resolve()
    if not 1024 <= port <= 65535:
        raise ValueError("bootstrap port must be unprivileged")
    if data_dir.exists() and any(data_dir.iterdir()):
        raise ValueError("bootstrap requires a new empty data directory")
    if env_file.stat().st_mode & 0o077:
        raise ValueError("environment file must be private")
    settings = {}
    for line in env_file.read_text().splitlines():
        if line and not line.startswith("#"):
            key, separator, value = line.partition("=")
            if not separator or key.strip() != key or value.strip() != value:
                raise ValueError("environment file must use plain KEY=value lines")
            settings[key] = value
    if not settings.get("OMCPA_MASTER_KEY") or not settings.get("OMCPA_CPA_MANAGEMENT_KEY"):
        raise ValueError("environment file must provide both OMC keys")
    data_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(data_dir, 0o700)
    env = {key: value for key, value in os.environ.items() if not key.startswith("OMCPA_")}
    env.update(settings)
    env.update({
        "OMCPA_CPA_BASE_URL": "http://127.0.0.1:1",
        "OMCPA_CPA_USAGE_ADDR": "127.0.0.1:1",
        "OMCPA_DATA_DIR": str(data_dir),
        "OMCPA_LISTEN_ADDR": f"127.0.0.1:{port}",
        "OMCPA_BASE_PATH": "/",
        "OMCPA_USAGE_INGEST_MODE": "off",
        "OMCPA_UPDATER_ENABLED": "false",
        "OMCPA_UPDATE_CHECK_ENABLED": "false",
        "OMCPA_UPDATE_CHECK_ON_PAGE_LOAD": "false",
        "OMCPA_ENV_FILE": "/dev/null",
        "OMCPA_DEMO_MODE": "false",
    })
    database = data_dir / "oh-my-cpa.db"
    log_path = data_dir.parent / (data_dir.name + "-bootstrap.log")
    log_fd = os.open(log_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with os.fdopen(log_fd, "wb") as log:
        process = subprocess.Popen([str(binary)], cwd=data_dir, env=env, stdout=log, stderr=log,
                                   umask=0o077)
        try:
            deadline = time.monotonic() + 45
            while True:
                if process.poll() is not None:
                    raise RuntimeError("OMC bootstrap exited; inspect the private bootstrap log")
                try:
                    with opener.open(f"http://127.0.0.1:{port}/api/healthz") as response:
                        health = json.load(response)
                    if health.get("database_status") == "ok":
                        break
                except urllib.error.URLError:
                    pass
                if time.monotonic() >= deadline:
                    raise RuntimeError("OMC bootstrap did not become ready")
                threading.Event().wait(0.1)
        finally:
            process.terminate()
            try:
                process.wait(timeout=30)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise RuntimeError("bootstrap required forced termination; inspect before continuing")
    seed_preferences(database)
    os.chmod(database, 0o600)
    print(json.dumps({"database_initialized": True, "live_proxy_contacted": False,
                      "quota_auto_refresh": False, "plugin_source": False,
                      "warmup_policy_count": 0}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--data-dir", type=Path, required=True)
    parser.add_argument("--env-file", type=Path, required=True)
    parser.add_argument("--port", type=int, default=18318)
    args = parser.parse_args()
    try:
        bootstrap(args.binary, args.data_dir, args.env_file, args.port)
    except (OSError, ValueError, RuntimeError, sqlite3.Error) as exc:
        print(json.dumps({"bootstrap_failed": type(exc).__name__}))
        raise SystemExit(1)


if __name__ == "__main__":
    main()
