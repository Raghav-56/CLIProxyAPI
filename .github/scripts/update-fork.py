"""Mirror official main and propose a stable release update to custom."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

UPSTREAM = "router-for-me/CLIProxyAPI"
FORK = "Raghav-56/CLIProxyAPI"


def run(*args, check=True):
    result = subprocess.run(args, text=True, encoding="utf-8", capture_output=True)
    if check and result.returncode:
        raise RuntimeError(f"{args[0]} {args[1]} failed: {result.stderr.strip()}")
    return result


def main():
    root = run("git", "rev-parse", "--show-toplevel").stdout.strip()
    os.chdir(root)
    actual = json.loads(run("gh", "repo", "view", "--json", "nameWithOwner").stdout)["nameWithOwner"]
    if actual != FORK:
        raise RuntimeError(f"Expected {FORK}, found {actual}")
    if run("git", "status", "--porcelain").stdout.strip():
        raise RuntimeError("Commit or stash local changes before preparing an update.")
    release = json.loads(run("gh", "api", f"repos/{UPSTREAM}/releases/latest").stdout)
    tag = release["tag_name"]
    if release["draft"] or release["prerelease"] or not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise RuntimeError("Expected a stable official release tag.")
    run("git", "fetch", "--quiet", "--no-tags", "origin",
        "refs/heads/main:refs/remotes/origin/main", "refs/heads/custom:refs/remotes/origin/custom")
    run("git", "fetch", "--quiet", "--no-tags", f"https://github.com/{UPSTREAM}.git",
        "refs/heads/main:refs/remotes/upstream/main", f"refs/tags/{tag}:refs/tags/{tag}")
    run("git", "merge-base", "--is-ancestor", "origin/main", "upstream/main")
    run("git", "push", "origin", "upstream/main:refs/heads/main")
    print("main now matches official upstream/main", flush=True)
    if run("git", "merge-base", "--is-ancestor", tag, "origin/custom", check=False).returncode == 0:
        print(f"custom already contains {tag}")
        return
    branch = f"updates/upstream-{tag}"
    existing = json.loads(run("gh", "pr", "list", "--repo", FORK, "--base", "custom",
                              "--head", branch, "--state", "all", "--json", "url,state").stdout)
    if existing:
        print(f"Existing update PR ({existing[0]['state']}): {existing[0]['url']}")
        return
    if run("git", "ls-remote", "--heads", "origin", branch).stdout.strip():
        raise RuntimeError(f"Remote branch {branch} already exists without a PR. Inspect it before retrying.")
    if run("git", "show-ref", "--verify", f"refs/heads/{branch}", check=False).returncode == 0:
        raise RuntimeError(f"Local branch {branch} already exists. Inspect it before retrying.")
    run("git", "switch", "--create", branch, "origin/custom")
    merge = run("git", "merge", "--no-ff", "--no-edit", tag, check=False)
    conflicts = []
    if merge.returncode:
        conflicts = run("git", "diff", "--name-only", "--diff-filter=U").stdout.splitlines()
        if not conflicts:
            raise RuntimeError(merge.stderr.strip() or merge.stdout.strip())
        run("git", "merge", "--abort")
        run("git", "switch", "--detach", tag)
        run("git", "branch", "--force", branch, tag)
        run("git", "switch", branch)
    run("git", "push", "--set-upstream", "origin", branch)
    body = (f"Update the custom proxy from official [{tag}]({release['html_url']}).\n\n"
            "Keep account selection, quota guidance, and request observability working. "
            "This PR does not deploy to oracler.\n\n")
    if conflicts:
        body += ("This draft has merge conflicts. Have an AI coding agent check out this branch, "
                 "merge `origin/custom`, resolve the conflicts, and follow `CUSTOM.md`.\n\n"
                 "Conflicting files:\n" + "".join(f"- `{name}`\n" for name in conflicts))
    else:
        body += "The release merged cleanly. The custom-ci workflow is dispatched on this branch. Check it before merging.\n"
    with tempfile.TemporaryDirectory() as temporary:
        body_path = Path(temporary) / "body.md"
        body_path.write_text(body, encoding="utf-8")
        args = ["gh", "pr", "create", "--repo", FORK, "--base", "custom", "--head", branch,
                "--title", f"chore(upstream): merge {tag} into custom", "--body-file", str(body_path)]
        if conflicts:
            args.append("--draft")
        url = run(*args).stdout.strip()
    print(url, flush=True)
    if not conflicts:
        run("gh", "workflow", "run", "custom-ci.yml", "--repo", FORK, "--ref", branch)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as output:
            output.write(f"main synced with upstream. Release update: {url}\n")


if __name__ == "__main__":
    main()
