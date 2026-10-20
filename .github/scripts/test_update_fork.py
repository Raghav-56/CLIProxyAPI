import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("update_fork", Path(__file__).with_name("update-fork.py"))
updater = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updater)


class UpdateForkTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.original_cwd = Path.cwd()
        self.addCleanup(os.chdir, self.original_cwd)
        self.root = Path(self.directory.name)
        self.upstream = self.root / "upstream.git"
        self.origin = self.root / "origin.git"
        self.checkout = self.root / "checkout"
        for path in (self.upstream, self.origin):
            self.git("init", "--bare", str(path), cwd=self.root)
        self.git("init", "--initial-branch=main", str(self.checkout), cwd=self.root)
        os.chdir(self.checkout)
        self.git("config", "user.name", "Maintenance test")
        self.git("config", "user.email", "maintenance@example.test")
        self.git("config", "core.autocrlf", "false")
        self.git("config", "commit.gpgsign", "false")
        self.write("routing.txt", "official base\n")
        self.git("add", ".")
        self.git("commit", "-m", "official base")
        self.base = self.git("rev-parse", "HEAD").stdout.strip()
        self.git("remote", "add", "origin", str(self.origin))
        self.git("push", "origin", "main")
        self.git("push", str(self.upstream), "main")
        self.git("switch", "-c", "custom")
        self.write("console.txt", "custom console\n")
        self.git("add", ".")
        self.git("commit", "-m", "custom console")
        self.git("push", "origin", "custom")
        self.release = self.base
        self.tag = "v8.0.28"
        self.prs = []
        self.dispatches = []
        self.real_run = updater.run

    def git(self, *args, cwd=None):
        result = subprocess.run(["git", *args], cwd=cwd, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def write(self, name, text):
        (self.checkout / name).write_text(text, encoding="utf-8")

    def upstream_release(self, conflict=False):
        self.git("switch", "main")
        self.write("routing.txt" if conflict else "official.txt", "new official behavior\n")
        self.git("add", ".")
        self.git("commit", "-m", "official release")
        self.git("tag", self.tag)
        self.release = self.git("rev-parse", "HEAD").stdout.strip()
        self.git("push", str(self.upstream), "main", self.tag)
        self.git("switch", "custom")
        if conflict:
            self.write("routing.txt", "custom routing behavior\n")
            self.git("add", ".")
            self.git("commit", "-m", "custom routing")
            self.git("push", "origin", "custom")

    def fake_run(self, *args, check=True):
        if args[0] == "gh":
            if args[1:3] == ("repo", "view"):
                data = {"nameWithOwner": updater.FORK}
            elif args[1] == "api":
                data = {"tag_name": self.tag, "draft": False, "prerelease": False,
                        "html_url": f"https://example.test/releases/{self.tag}"}
            elif args[1:3] == ("pr", "list"):
                data = self.prs
            elif args[1:3] == ("pr", "create"):
                self.created_args = args
                self.created_body = Path(args[args.index("--body-file") + 1]).read_text(encoding="utf-8")
                self.prs = [{"state": "OPEN", "url": "https://example.test/pull/1"}]
                return subprocess.CompletedProcess(args, 0, self.prs[0]["url"], "")
            elif args[1:3] == ("workflow", "run"):
                self.dispatches.append(args)
                data = {}
            else:
                raise AssertionError(args)
            return subprocess.CompletedProcess(args, 0, json.dumps(data), "")
        args = tuple(str(self.upstream) if item == f"https://github.com/{updater.UPSTREAM}.git" else item for item in args)
        return self.real_run(*args, check=check)

    def update(self):
        with patch.object(updater, "run", side_effect=self.fake_run), contextlib.redirect_stdout(io.StringIO()):
            updater.main()

    def test_clean_update_preserves_custom_and_is_repeatable(self):
        self.upstream_release()
        self.update()
        self.assertEqual(self.git("--git-dir", str(self.origin), "rev-parse", "main").stdout.strip(), self.release)
        self.assertEqual((self.checkout / "console.txt").read_text(), "custom console\n")
        self.assertEqual((self.checkout / "official.txt").read_text(), "new official behavior\n")
        self.assertNotIn("--draft", self.created_args)
        self.assertEqual(len(self.dispatches), 1)
        self.assertNotEqual(self.git("--git-dir", str(self.origin), "rev-parse", "custom").stdout.strip(), self.release)
        self.update()
        self.assertEqual(len(self.dispatches), 1)
        self.assertEqual(len(self.prs), 1)

    def test_conflict_creates_draft_without_claiming_validation(self):
        self.upstream_release(conflict=True)
        self.update()
        self.assertIn("--draft", self.created_args)
        self.assertIn("routing.txt", self.created_body)
        self.assertEqual(self.dispatches, [])
        self.assertFalse((self.checkout / ".git/MERGE_HEAD").exists())
        self.assertEqual(self.git("--git-dir", str(self.origin), "rev-parse", "updates/upstream-v8.0.28").stdout.strip(), self.release)

    def test_divergent_main_is_not_overwritten(self):
        self.upstream_release()
        self.git("switch", "main")
        self.git("reset", "--hard", self.base)
        self.write("fork-only.txt", "must not disappear\n")
        self.git("add", ".")
        self.git("commit", "-m", "divergent main")
        divergent = self.git("rev-parse", "HEAD").stdout.strip()
        self.git("push", "origin", "main")
        self.git("switch", "custom")
        with self.assertRaisesRegex(RuntimeError, "merge-base failed"):
            self.update()
        self.assertEqual(self.git("--git-dir", str(self.origin), "rev-parse", "main").stdout.strip(), divergent)
        self.assertEqual(self.prs, [])

    def test_dirty_checkout_is_rejected(self):
        self.write("console.txt", "uncommitted changes\n")
        with self.assertRaisesRegex(RuntimeError, "Commit or stash"):
            self.update()
        self.assertEqual((self.checkout / "console.txt").read_text(), "uncommitted changes\n")


if __name__ == "__main__":
    unittest.main()
