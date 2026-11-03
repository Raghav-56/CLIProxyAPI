import contextlib
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

import check_policy


class PolicyTests(unittest.TestCase):
    def test_startup_guard_rejects_unsafe_policy_without_changing_database(self):
        with tempfile.TemporaryDirectory() as directory:
            database = Path(directory) / "omc.db"
            env = {"OMCPA_CPA_BASE_URL": "http://127.0.0.1:8317",
                   "OMCPA_LISTEN_ADDR": "127.0.0.1:18317",
                   "OMCPA_USAGE_INGEST_MODE": "subscribe",
                   "OMCPA_UPDATER_ENABLED": "false", "OMCPA_UPDATE_CHECK_ENABLED": "false",
                   "OMCPA_UPDATE_CHECK_ON_PAGE_LOAD": "false"}
            with contextlib.closing(sqlite3.connect(database)) as db, db:
                db.execute("CREATE TABLE ui_preferences(pref_key TEXT PRIMARY KEY,pref_value TEXT)")
                for table in ("credential_warmup_policies", "credential_warmup_state", "credential_warmup_runs"):
                    db.execute("CREATE TABLE " + table + "(document TEXT)")
                for key, value in check_policy.POLICY.items():
                    db.execute("INSERT INTO ui_preferences VALUES(?,?)", (key, json.dumps(value)))
            check_policy.check_policy(database, env)
            for value in ("0", "null", '"false"'):
                with contextlib.closing(sqlite3.connect(database)) as db, db:
                    db.execute("UPDATE ui_preferences SET pref_value=? WHERE pref_key='omc_plugin_source_enabled'",
                               (value,))
                with self.assertRaises(ValueError):
                    check_policy.check_policy(database, env)
            with contextlib.closing(sqlite3.connect(database)) as db, db:
                db.execute("UPDATE ui_preferences SET pref_value='false' WHERE pref_key='omc_plugin_source_enabled'")
            for patch in ({"OMCPA_USAGE_INGEST_MODE": "auto"}, {"OMCPA_UPDATER_ENABLED": "true"}):
                with self.assertRaises(ValueError):
                    check_policy.check_policy(database, env | patch)
            with contextlib.closing(sqlite3.connect(database)) as db, db:
                db.execute("DELETE FROM ui_preferences WHERE pref_key='omc_plugin_source_enabled'")
            before = database.read_bytes()
            with self.assertRaises(ValueError):
                check_policy.check_policy(database, env)
            self.assertEqual(database.read_bytes(), before)

    def test_missing_database_is_not_created(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "missing.db"
            env = {"OMCPA_CPA_BASE_URL": "http://127.0.0.1:8317",
                   "OMCPA_LISTEN_ADDR": "127.0.0.1:18317", "OMCPA_USAGE_INGEST_MODE": "off",
                   "OMCPA_UPDATER_ENABLED": "false", "OMCPA_UPDATE_CHECK_ENABLED": "false",
                   "OMCPA_UPDATE_CHECK_ON_PAGE_LOAD": "false"}
            with self.assertRaises(sqlite3.OperationalError):
                check_policy.check_policy(path, env)
            self.assertFalse(path.exists())


if __name__ == "__main__":
    unittest.main()
