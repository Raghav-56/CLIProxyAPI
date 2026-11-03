import contextlib
import importlib.util
import os
from pathlib import Path
import sqlite3
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("omc_backup", Path(__file__).with_name("backup.py"))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


class BackupTests(unittest.TestCase):
    def test_live_wal_snapshot_restores_committed_records_and_has_private_mode(self):
        with tempfile.TemporaryDirectory() as directory:
            database = Path(directory) / "live.db"
            source = sqlite3.connect(database)
            try:
                source.execute("PRAGMA journal_mode=WAL")
                source.execute("CREATE TABLE events(id INTEGER PRIMARY KEY, model TEXT)")
                source.execute("INSERT INTO events VALUES(7,'fixture-model')")
                source.commit()
                dest = backup.backup(database, Path(directory) / "backups")
                with contextlib.closing(sqlite3.connect(dest)) as restored:
                    self.assertEqual(restored.execute("SELECT * FROM events").fetchall(),
                                     [(7, "fixture-model")])
                self.assertEqual(os.stat(dest).st_mode & 0o777, 0o600)
            finally:
                source.close()

    def test_retention_preserves_unrelated_files(self):
        with tempfile.TemporaryDirectory() as directory:
            database = Path(directory) / "live.db"
            with contextlib.closing(sqlite3.connect(database)) as source, source:
                source.execute("CREATE TABLE events(id INTEGER)")
            target = Path(directory) / "backups"
            target.mkdir()
            unrelated = target / "keep.db"
            unrelated.write_text("preserve")
            for _ in range(9):
                backup.backup(database, target)
            self.assertEqual(len(list(target.glob("omc-*.db"))), 7)
            self.assertEqual(unrelated.read_text(), "preserve")


if __name__ == "__main__":
    unittest.main()
