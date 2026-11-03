import contextlib
import importlib.util
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("omc_bootstrap", Path(__file__).with_name("bootstrap.py"))
bootstrap = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bootstrap)


class BootstrapTests(unittest.TestCase):
    def make_database(self, directory):
        path = Path(directory) / "oh-my-cpa.db"
        with contextlib.closing(sqlite3.connect(path)) as db, db:
            db.execute("CREATE TABLE ui_preferences(pref_key TEXT PRIMARY KEY,pref_value TEXT,updated_at_ms INTEGER)")
            db.execute("CREATE TABLE credential_warmup_policies(document_json TEXT)")
            db.execute("INSERT INTO ui_preferences VALUES('display_theme','\"dark\"',1)")
        return path

    def test_seed_disables_automation_and_preserves_unrelated_preferences(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.make_database(directory)
            bootstrap.seed_preferences(path)
            bootstrap.seed_preferences(path)
            with contextlib.closing(sqlite3.connect(path)) as db:
                actual = {key: json.loads(value) for key, value in db.execute(
                    "SELECT pref_key,pref_value FROM ui_preferences")}
            self.assertEqual(actual, {
                "display_theme": "dark",
                "omc_quota_auto_refresh": {"interval_minutes": 0, "refresh_on_reset": False},
                "omc_plugin_source_enabled": False,
            })

    def test_existing_warmup_policy_refuses_all_preference_writes(self):
        with tempfile.TemporaryDirectory() as directory:
            path = self.make_database(directory)
            with contextlib.closing(sqlite3.connect(path)) as db, db:
                db.execute("INSERT INTO credential_warmup_policies VALUES('{}')")
            with self.assertRaises(ValueError):
                bootstrap.seed_preferences(path)
            with contextlib.closing(sqlite3.connect(path)) as db:
                self.assertEqual(db.execute("SELECT pref_key FROM ui_preferences").fetchall(),
                                 [("display_theme",)])

    def test_missing_database_is_not_created(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "missing.db"
            with self.assertRaises(sqlite3.OperationalError):
                bootstrap.seed_preferences(path)
            self.assertFalse(path.exists())

    def test_rebootstrap_refuses_an_existing_data_directory(self):
        with tempfile.TemporaryDirectory() as directory:
            data = Path(directory) / "data"
            data.mkdir()
            (data / "unrelated").write_text("preserve")
            with self.assertRaises(ValueError):
                bootstrap.bootstrap(Path(__file__), data, Path(directory) / "absent.env", 18318)
            self.assertEqual((data / "unrelated").read_text(), "preserve")


if __name__ == "__main__":
    unittest.main()
