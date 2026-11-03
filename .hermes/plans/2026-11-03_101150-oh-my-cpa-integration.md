# Oh-My-CPA integration implementation plan

## Goal and status

Add persistent proxy diagnostics and, after verification, agent access from T3 Code while preserving the existing custom CLIProxyAPI account console and Windows Rainmeter widget. This document is a plan, not deployment authorization or evidence of an installed service. The user authorized cross-thread coordination and independent verification. No sidecar has been installed by this planning task.

Use the current host's native orchestration. Important planning and verification use CP `gpt-6.1-sol` with `reasoningEffort=high`; separately delegated implementation uses CP `gpt-6.1-terra`. The requested `gpt-6.1-luna` is not in the exposed provider catalog; do not silently substitute a different exploration model. No extra implementation worker is needed until this plan's baseline gates are satisfied.

## Current context

- Oracler is Linux ARM64. Native ARM64 releases and ARM64 containers are documented upstream. Docker CLI exists; daemon access and systemd runtime state are not verified. Sandbox access to the system bus was denied. User service files exist at `/home/ubuntu/.config/systemd/user/cliproxyapi.service` and `/home/ubuntu/.config/systemd/user/t3code.service`; their existence does not establish process health.
- Widget source is committed as `00d41a0a`, atop `df0b6532`, per the authorized coordination update. MSI-Raghav has the compact Rainmeter widget and Ctrl+Alt+Q. Reported checks include 121 PowerShell checks, native Rainmeter Lua/layout checks, and hotkey/lifecycle checks. Screenshot verification remains unavailable.
- The widget owns live display, not history. It uses direct GETs for credentials, routing choice, recent requests, and optionally server information. It does not consume the queue, probe quotas, copy OAuth, or write routing.
- Recent requests are process-local, capped at 200 attempts. Quota freshness follows each credential's observation time and reset deadlines. Neither the widget nor standard usage records establish T3 thread attribution.
- T3 inference and its usage-hub limits connection are independent. Existing provider names, Codex homes, credential helpers, inference URL and hub setup stay in place.

## Ownership and integration contract

| Owner | Responsibility | Boundary |
| --- | --- | --- |
| Custom CLIProxyAPI | Inference, OAuth, routing, current quota observations | No binary replacement, route renaming, credential migration, or implicit configuration conversion |
| Existing account console | Manual selection and custom soonest-reset controls | Continue using existing v8 semantics and applied-state readback |
| Widget thread | `desktop/rainmeter/**`, `docs/rainmeter.md`, Windows installation | Exclusive ownership retained; coordinator and sidecar implementer must not edit, stage, commit, or restart these components |
| Oh-My-CPA sidecar | Durable request history and analytics | Own process, local database, secret storage, backups and release lifecycle; no competing pull collector |
| T3 Code | Coding and optional MCP diagnostics | Inference path does not depend on the sidecar; no claimed conversation attribution without explicit future correlation |
| Independent verifier | Compatibility evidence and acceptance checks | Read-only baseline now; fresh before/after verification at deployment, without changing production routing to demonstrate it |

The coordinator owns only this plan during this task. Proposed implementation files belong under `deploy/oh-my-cpa/`, outside the widget paths. No worker may revert another agent's changes. Commit only the current task's files.

## Deployment shape

Prefer one pinned native Linux ARM64 Oh-My-CPA release under a user service on Oracler. This matches the existing host-service topology and permits loopback HTTP and RESP access without adding Docker networking or enabling remote management. This is a proposed choice; verify the existing proxy listener and user service access before finalizing it.

Use a separate release directory, a stable executable symlink, a private environment file and a persistent data directory. Suggested locations are `/home/ubuntu/.local/share/oh-my-cpa/releases/`, `/home/ubuntu/.config/oh-my-cpa/service.env`, `/home/ubuntu/.local/share/oh-my-cpa/data/`, and `/home/ubuntu/.config/systemd/user/oh-my-cpa.service`. These host paths are outside the current writable workspace; use the host's normal permission process during implementation, never write around it. Do not mount, copy, or share the proxy's OAuth directory.

Proposed settings, to confirm against the pinned release:

```dotenv
OMCPA_CPA_BASE_URL=http://127.0.0.1:8317
OMCPA_CPA_USAGE_ADDR=127.0.0.1:8317
OMCPA_LISTEN_ADDR=127.0.0.1:18317
OMCPA_USAGE_INGEST_MODE=subscribe
OMCPA_USAGE_RETENTION_DAYS=90
OMCPA_USAGE_INBOX_RETENTION_DAYS=7
OMCPA_UPDATER_ENABLED=false
TZ=Etc/UTC
```

The above values contain no secrets. Port 18317 must be checked for availability. Set `OMCPA_DATA_DIR` explicitly. Inject the existing management key through protected storage and provision a separate `OMCPA_MASTER_KEY` through the host secret facility when needed. Never place secrets in this repository, process arguments, chat, generated reports or console URLs. Preserve the master key across upgrades and back it up separately from the database.

Do not install the optional Docker-socket updater. Do not create a second proxy or replace the custom one with an upstream image. If native deployment proves unsuitable, reconsider an isolated ARM64 container explicitly; confirm HTTP and RESP reachability separately and do not change proxy remote-management policy as an incidental workaround.

## Task 1: Complete the independent baseline

Read-only independent thread: `thread:mcp:d73c2596-4803-4870-83ad-bc7ccf5d1afa:ohmy-independent-compat-baseline-20261103:0`. Its findings are pending when this plan is first saved.

1. Record the custom commit, proxy executable identity, config fingerprint, credential inventory counts and routing choice/applied state without exposing content, keys or account identities. Config fingerprints detect writes; credential files can legitimately change through the proxy's own refresh lifecycle, so explain any differences rather than promising immutable OAuth files.
2. Verify the current inference provider and usage hub using existing non-mutating evidence. Do not print T3 settings or entire service environment files.
3. Confirm existing queue consumers and Home mode. Home mode can disable in-process RESP usage. A subscriber can change whether events enter the destructive pull queue; mixing subscribers with pull collectors is not safe coexistence.
4. Establish required usage-statistics configuration. If disabled, identify the smallest explicit change and its exact semantic scope before implementation. Do not replace the config with an upstream example.
5. Record the current widget status from the owning thread. Preserve its hashes, hotkey, MementoMori behavior and observation freshness rules.

Acceptance: exact pre-deployment observations and conflicts are documented. No queue pop, credential refresh, account selection, restart or inference request is used as a baseline probe.

## Task 2: Prepare an isolated sidecar deployment

Likely files to create during implementation:

- `deploy/oh-my-cpa/README.md`: pinned release/checksum, host layout, install/start/stop/update/rollback instructions and known limitations.
- `deploy/oh-my-cpa/oh-my-cpa.service`: native user-service template with explicit working directory, environment-file path and independent lifecycle.
- `deploy/oh-my-cpa/service.env.example`: non-secret settings only; no usable management or master key.
- `deploy/oh-my-cpa/verify.py`: a rerunnable, read-only baseline/after report that compares permitted fields and fingerprints without dumping secrets. Avoid adding a new helper if the independent verifier identifies a suitable existing one.

Verify the actual release asset, checksum and ARM64 support. Pin a version and executable hash; do not deploy `latest`. Verify default startup jobs and browser actions against this release, including account automation, scheduled quota probes and configuration writes. Disable optional management automation using verified settings; do not invent an environment variable that claims to make the service read-only.

Acceptance: the deployment adds only the sidecar and its owned files. The proxy service, configuration, API routes, account selection and T3 settings are unchanged, except for a separately specified usage-statistics setting if required.

## Task 3: Start collection and prove coexistence

1. Bind the sidecar to loopback and establish explicit authenticated RESP subscription. No fallback to `http_pull` or `resp_pull` during acceptance. The usage protocol is RESP, not necessarily reachable through an HTTP-only reverse proxy.
2. Validate health at the pinned release's documented `/api/healthz` endpoint and verify database state.
3. Observe ordinary user traffic. Match a new request's model/account identifiers, nullable tokens, timestamps and outcome between available proxy evidence and the sidecar. Do not compare total attempts to logical requests without checking retries and the decoder's record semantics.
4. Confirm the widget and console remain functional and sidecar ingestion does not change the selected account, quota freshness or recent-request feed.
5. Stop and restart only the sidecar. Previously recorded history must survive; inference and the widget must continue. Record any subscription disconnect gap honestly. Persistence after receipt does not imply exactly-once delivery or durable replay from the proxy stream.

Acceptance: durable records, unchanged custom controls, no competing queue consumer, no implicit credential/config writes, and a stop-sidecar-only rollback all work. Do not claim full lossless collection from a dashboard total.

## Task 4: Make history reachable from the existing workflow

First use an existing private access path or a dedicated SSH forward for the sidecar. Do not repoint the widget's configured proxy root to Oh-My-CPA. Adding private HTTPS ingress is a later implementation step subject to the host's existing deployment guidance; no public exposure or unrelated Caddy edits are part of this plan.

Provide an Oh-My-CPA browser shortcut. Any widget link belongs to the widget owner and must use a separate validated analytics URL; it must not replace the `/accounts.html` action or contain an authentication token. No custom analytics rendering or new live API layer is necessary initially.

Acceptance: users can reach accounts, live widget and historical diagnostics independently. No field renaming or shared API change is required.

## Task 5: Gate T3 MCP integration separately

Oh-My-CPA exposes `/api/mcp`, but its management credential is administrator-equivalent. Human approval of high-risk operations is not a read-only credential. Verify whether the pinned release can enforce read-only capabilities server-side; inspect actual tool discovery and authorization, not only prompts or hidden UI buttons.

If enforceable scoping exists, configure only the diagnostic tools using protected credential delivery on the execution environment actually running the agent. Otherwise retain browser analytics and document the broad capability before choosing a full management connection. Do not silently claim a restricted connection or implement an unreviewed authorization proxy to bypass this decision. Check whether the selected T3/provider instance supports environment-specific MCP configuration before changing settings.

Acceptance: an agent can query recent failures and latency without receiving OAuth tokens or raw secret/config output; an attempted routing/config write through the restricted connection is rejected in a disposable test, never by changing production. If no restricted mode is available, mark that capability gate unresolved rather than claiming success.

## Task 6: Backups, updates and independent after-check

Use consistent SQLite backup, such as documented `VACUUM INTO`, or stop only Oh-My-CPA and back up its entire data directory. Copying a live `.db` alone can omit WAL transactions. Verify backup integrity and a restore in an isolated data directory with a matching master key; keep a pre-upgrade snapshot because forward migrations cannot be undone just by replacing the executable.

Initially retain request rows for 90 days and raw inbox messages for 7 days. Measure actual growth before tuning. Define a backup schedule and retention explicitly during implementation; upstream migration backups are not a replacement for a tested routine backup. Update the sidecar independently from the custom proxy, testing the pinned v8/subscription contract after upgrades.

Have the independent Sol-high thread verify the final artifacts and before/after evidence at deployment. If a new implementation diff needs a fresh review, create a fresh delegated review task with the artifact and objections; do not reuse a completed delegate review by sending a message. The independent top-level thread can retain continuity for its own ongoing baseline and post-deployment responsibility.

Stop only Oh-My-CPA if it writes unrequested proxy settings, triggers unexpected account automation, disrupts another collector, or cannot demonstrate compatible records. Restore only its owned data/version as appropriate; do not overwrite proxy credentials or rewind custom widget commits.

## Checks for implementation

Existing source targets:

```bash
go test ./internal/redisqueue ./internal/api ./internal/api/handlers/management ./sdk/cliproxy/auth
go build -o /tmp/cliproxy-ohmy-check ./cmd/server
```

The following named tests define useful invariants: `TestEnqueueBroadcastsToUsageSubscribersAndSkipsQueue`, `TestRecentRequestsObservesUsageWithoutConsumingOtherSinks`, `TestRecentRequestsV8RequiresManagementAuthentication`, and `TestRedisProtocol_SUBSCRIBE_UsageSendsSupportRefresh`. Use offline fixtures for any new config-preservation or malformed-event decoder test. Test routing modes in fixtures; never flip the live global selection merely to satisfy an acceptance checklist.

Run the build required by repository guidance for committed implementation changes. Run Windows checks only through the widget owner when those files change. Before committing, review the diff and stage only `deploy/oh-my-cpa/` and task documentation; never stage unrelated widget changes.

## Evidence and open gates

Primary sources inspected on 2026-11-03:

- https://github.com/WizisCool/oh-my-cpa/blob/main/docs/install.md
- https://github.com/WizisCool/oh-my-cpa/blob/main/docs/operations.md
- https://github.com/WizisCool/oh-my-cpa/blob/main/docs/cpa-v8-compat.md
- https://github.com/WizisCool/oh-my-cpa/blob/main/docs/ops/sqlite-operations.md
- https://github.com/WizisCool/oh-my-cpa/blob/main/docs/agent-capabilities.md
- Local `docs/account-console.md`, `docs/t3-code-integration.md`, `desktop/rainmeter/README.md`, `internal/api/redis_queue_protocol.go`, and `internal/redisqueue/queue.go`.

Unresolved before implementation: independent verifier findings, actual proxy/service topology and listeners, usage-statistics state, other collectors/Home mode, pinned release and checksum, default startup automation, sidecar port availability, supported MCP scoping and client attachment, and routine backup destination/schedule. These are explicit discovery gates, not reasons to alter the existing deployment speculatively.
