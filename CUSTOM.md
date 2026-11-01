# Maintain the custom proxy

The `custom` branch is maintained for my personal use. It regularly incorporates upstream updates from the history mirrored in `main`, using the stable release update process below, while preserving the features I need.
Keep personal features and customizations on `custom` only. Keep `main` as an unchanged mirror of the official upstream branch.

| Branch | Contents |
| --- | --- |
| `main` | Exact official `router-for-me/CLIProxyAPI` main history |
| `custom` | Account console, manual Codex OAuth routing, quota guidance, request observability, and fork maintenance |
| `updates/upstream-vX.Y.Z` | A proposed official release update |

Use `custom` for development and builds. Keep custom commits out of `main`.
Merge upstream update PRs with a merge commit. Squashing or rebasing loses the upstream ancestry used by later updates.
The fork disables squash and rebase merging to preserve that history.
The repository default branch is `custom` so its maintenance workflows can run.
Upstream-only PR guards and release jobs are restricted to the official repository.
This fork uses `custom-ci` instead, so an upstream translator change does not block a release update.

## Prepare an official update

The `update-fork` workflow runs weekly on Monday at 03:17 UTC, or manually from GitHub Actions.
It fast-forwards `main` to official upstream main and prepares the latest stable release update branch against `custom`.
It does not force-push main, merge a PR, switch accounts, or deploy a server.
Repeated runs reuse the existing release PR or update branch. A closed PR is left closed until you choose what to do.

For a local run, start in a clean checkout with Git, Python 3, and authenticated GitHub CLI:

```sh
git switch custom
git pull --ff-only origin custom
python .github/scripts/update-fork.py
```

A clean merge dispatches `custom-ci` on its branch. A conflict prepares a branch from the official release.
The Actions run summary links to a comparison where you or an AI agent can open a PR.
The scheduled workflow does not create or approve PRs and does not require a repository permission change.
A local run uses your existing GitHub CLI login to open a PR for a new update branch.
Resolve conflicts by merging `origin/custom` into the update branch. Never select an entire side without examining the behavior.
The workflow uses GitHub's repository token. It needs no OpenAI key or personal access token.
AI conflict resolution runs when you ask an agent to work on the update PR. It is not an unattended AI service.

## Ask an AI agent to resolve an update

Give the agent this instruction with the update PR URL:

> Update this custom CLIProxyAPI fork using the release PR. Read AGENTS.md, CUSTOM.md, and docs/account-console.md. Check out the PR branch, merge origin/custom if needed, and resolve conflicts by preserving the behavior of both upstream and the custom features. Review upstream routing and config changes even if Git reports no conflicts. Run the checks below, review the diff for secrets, and update the PR with the results. Do not deploy or change live account selection.

## Verify an update

```sh
node --test internal/api/assets/accounts.test.cjs
go test ./...
go build -o cli-proxy-api ./cmd/server
go build -o config-export ./cmd/config-export
```

The routing tests must preserve selection of one persistent Codex OAuth account, request-level consistency through retries,
exclusion of other accounts, hot-reload verification, and rejection of manual routing when Home is enabled.
The console tests cover quota windows, model visibility, recommendations, and removal of credentials from displayed summaries.

Before deployment, exercise account selection and a real response on a private staging proxy.
Record the selected account before a staging check and restore it afterward.
Keep server deployment separate from source updates. Retain the previous binary for rollback.

## Record a deployment

Tag the exact tested custom commit as `custom-vX.Y.Z.N`. Build with the commit and version in binary metadata.
Record the tag, full commit, binary SHA-256, upstream release, target host, and deployment time in a deployment record.
An official version tag alone does not identify custom changes.

The existing oracler installation uses the local release name `account-console-v4`, based on official v8.0.27.
Its old binary reports a dirty tree and cannot prove an exact custom source commit. Preserve it as a rollback artifact.
Do not relabel it as a reproducible clean build.

Live `config.yaml`, OAuth files, client keys, and management keys stay outside Git.
Commit sanitized templates and scripts only. The installed Windows and oracler client configuration is separate from this source repository.

## Initial verification note

On Windows, `TestEnsureClientsWaitsForPreviousTargetClose` fails on both the preserved custom source and untouched official v8.0.27.
The custom Go packages and both binaries pass their checks. Full-suite success is required before deploying an updated build.
