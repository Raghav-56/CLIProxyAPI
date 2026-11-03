# Oh-My-CPA integration implementation plan

## Goal and status

Add persistent proxy diagnostics and, after verification, agent access from T3 Code while preserving the existing custom CLIProxyAPI account console and Windows Rainmeter widget. The user subsequently authorized implementation. The isolated local sidecar is installed; collection and tailnet exposure remain disabled after automatic approval review required specific authorization. `deploy/oh-my-cpa/VALIDATION.md` records actual deployment evidence and remaining gates; proposed steps below are not claims of completed acceptance.

Use the current host's native orchestration. Important planning and verification use CP `gpt-6.1-sol` with `reasoningEffort=high`; separately delegated implementation uses CP `gpt-6.1-terra`. The user clarified that exploration should use CP `gpt-6-luna`, which is available. Implementation was authorized after this plan's review; see `deploy/oh-my-cpa/VALIDATION.md` for execution evidence and remaining limits.

## Current context

- Oracler is Linux ARM64. Independent read-only host observations on 2026-11-03 establish that user `cliproxyapi.service` is active/running, PID 1814, zero restarts, started at 07:36:16 UTC. System-scope unit is absent. Repeat these observations before deployment; this is a timestamped baseline, not a current-health guarantee.
- Running proxy executable: `/home/ubuntu/.local/share/cliproxyapi/releases/custom-v8.0.51-6fab707b/cli-proxy-api`, SHA256 `b47745b0e151f028c2a23ecc9f7e7d9c20ee6c5062bd9c09e98b6467b4de0661`. Its running release predates the widget repository commits; installing the widget did not replace it.
- Active config: `/home/ubuntu/.local/share/cliproxyapi/config.yaml`, mode 0600, 1601 bytes, mtime_ns `1793700650987430025`, SHA256 `4293df5cdd6833fdcfd08272aaee1f9614bc59156066802ea7c142fdc25bb782`. The verifier observed the same fingerprint/PID/restarts twice without revealing config contents. Routing, credential counts, Home mode, usage-enabled state and other collectors remain unverified.
- Proxy listens on `127.0.0.1:8317`; private Tailscale port 8443 listeners exist. Port 18317 was free at review; port 8090 was occupied. Native loopback deployment fits the observed topology. Docker daemon access remains unverified and is unnecessary for the proposed native service.
- Widget source is committed as `00d41a0a`, atop `df0b6532`, per the authorized coordination update. MSI-Raghav has the compact Rainmeter widget and Ctrl+Alt+Q. Reported checks include 121 PowerShell checks, native Rainmeter Lua/layout checks, and hotkey/lifecycle checks. Screenshot verification remains unavailable.
- The widget owns live display, not history. It uses direct GETs for credentials, routing choice, recent requests, and optionally server information. It does not consume the queue, probe quotas, copy OAuth, or write routing.
- Recent requests are process-local, capped at 200 attempts. Quota freshness follows each credential's observation time and reset deadlines. Neither the widget nor standard usage records establish T3 thread attribution.
- T3 inference and its usage-hub limits connection are independent. Existing provider names, Codex homes, credential helpers, inference URL and hub setup stay in place.
- Observed T3 processes include `0.1.7-nightly.20261103.4106` and older `0.1.2-nightly.20261021.3499`. The local integration document inspected `0.1.7-nightly.20261102.4061`; verify behavior against the actual client rather than assuming the connector is identical or healthy because a process exists.

## Ownership and integration contract

| Owner | Responsibility | Boundary |
| --- | --- | --- |
| Custom CLIProxyAPI | Inference, OAuth, routing, current quota observations | No binary replacement, route renaming, credential migration, or implicit configuration conversion |
| Existing account console | Manual selection and custom soonest-reset controls | Continue using existing v8 semantics and applied-state readback |
| Widget thread | `desktop/rainmeter/**`, `docs/rainmeter.md`, Windows installation | Exclusive ownership retained; coordinator and sidecar implementer must not edit, stage, commit, or restart these components |
| Oh-My-CPA sidecar | Durable request history and analytics | Own process, local database, secret storage, backups and release lifecycle; no competing pull collector |
| T3 Code | Coding and optional MCP diagnostics | Inference path does not depend on the sidecar; no claimed conversation attribution without explicit future correlation |
| Independent verifier | Compatibility evidence and acceptance checks | Read-only baseline now; fresh before/after verification at deployment, without changing production routing to demonstrate it |

Implementation files belong under `deploy/oh-my-cpa/`, outside the widget paths. The coordinator owns host deployment, bootstrap, startup guard, backups and final integration; the Terra worker supplied the service template and baseline verifier. No worker may revert another agent's changes. Commit only the current task's files.

## Deployment shape

Prefer one pinned native Linux ARM64 Oh-My-CPA release under a user service on Oracler. This matches the existing host-service topology and permits loopback HTTP and RESP access without adding Docker networking or enabling remote management. This is a proposed choice; verify the existing proxy listener and user service access before finalizing it.

Use a separate release directory, a stable executable symlink, a private environment file and a persistent data directory. Suggested locations are `/home/ubuntu/.local/share/oh-my-cpa/releases/`, `/home/ubuntu/.config/oh-my-cpa/service.env`, `/home/ubuntu/.local/share/oh-my-cpa/data/`, and `/home/ubuntu/.config/systemd/user/oh-my-cpa.service`. These host paths are outside the current writable workspace; use the host's normal permission process during implementation, never write around it. Do not mount, copy, or share the proxy's OAuth directory.

Proposed settings, to confirm against the pinned release:

```dotenv
OMCPA_CPA_BASE_URL=http://127.0.0.1:8317
OMCPA_LISTEN_ADDR=127.0.0.1:18317
OMCPA_USAGE_INGEST_MODE=off
OMCPA_USAGE_RETENTION_DAYS=90
OMCPA_USAGE_INBOX_RETENTION_DAYS=7
OMCPA_UPDATER_ENABLED=false
OMCPA_UPDATE_CHECK_ENABLED=false
OMCPA_UPDATE_CHECK_ON_PAGE_LOAD=false
TZ=Etc/UTC
```

These settings are for bootstrap, with CPA connectivity blocked or unconfigured until the stored-preference gates below pass. `off` disables ingestion, not all quota, warmup, pricing, FX or other network workers. Change ingestion to `subscribe` only after agreeing sole ownership of the residual usage queue and verifying Home/usage/collector state. In the reviewed implementation, `OMCPA_CPA_USAGE_ADDR` is parsed but RESP client addressing derives from `OMCPA_CPA_BASE_URL`; do not rely on the override to relay RESP through an HTTPS-only ingress.

The above values contain no secrets. Port 18317 must be checked again for availability. Set `OMCPA_DATA_DIR` explicitly. Inject the existing management key through protected storage and provision a separate `OMCPA_MASTER_KEY` through the host secret facility when needed. Never place secrets in this repository, process arguments, chat, generated reports or console URLs. Preserve the master key across upgrades and back it up separately from the database. Stored sensitive fields are encrypted; SQLite metadata as a whole is not encrypted. Protect files/directories with 0600/0700 permissions.

Do not install the optional Docker-socket updater. Do not create a second proxy or replace the custom one with an upstream image. If native deployment proves unsuitable, reconsider an isolated ARM64 container explicitly; confirm HTTP and RESP reachability separately and do not change proxy remote-management policy as an incidental workaround.

## Task 1: Complete the independent baseline

The independent read-only review's findings have been incorporated into this revision. It changed no files, config, credentials or services and performed no subscription, queue pop, quota refresh or inference probe. Its Windows evidence is explicitly owner-reported, not independent Windows execution. Source baseline is upstream main `63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d`; latest observed release is `v0.3.20`, source `d16465b34f6ebb38a4cb4b1fbdc144da25f37cd2`. Recheck and pin the actual ARM64 asset/checksum; do not treat a reviewed main commit as the downloaded release binary.

1. Record the custom commit, proxy executable identity, config fingerprint, credential inventory counts and routing choice/applied state without exposing content, keys or account identities. Config fingerprints detect writes; credential files can legitimately change through the proxy's own refresh lifecycle, so explain any differences rather than promising immutable OAuth files.
2. Verify the current inference provider and usage hub using existing non-mutating evidence. Do not print T3 settings or entire service environment files.
3. Confirm existing queue consumers and Home mode without subscribing as a probe. Home mode disables in-process RESP usage. A subscriber changes whether events enter the destructive pull queue; mixing subscribers with pull collectors is not safe coexistence. Oh-My-CPA's explicit subscription also runs periodic `LPOP` backfill, so it must own the residual queue too. If Home/usage/consumer state is unknown or another reader retains queue ownership, keep ingestion off.
4. Establish required usage-statistics configuration. If disabled, identify the smallest explicit change and its exact semantic scope before implementation. Do not replace the config with an upstream example.
5. Record the current widget status from the owning thread. Preserve its hashes, hotkey, MementoMori behavior and observation freshness rules.

Acceptance: exact pre-deployment observations and conflicts are documented. No queue pop, credential refresh, account selection, restart or inference request is used as a baseline probe.

## Task 2: Prepare an isolated sidecar deployment

Likely files to create during implementation:

- `deploy/oh-my-cpa/README.md`: pinned release/checksum, host layout, install/start/stop/update/rollback instructions and known limitations.
- `deploy/oh-my-cpa/oh-my-cpa.service`: native user-service template with explicit working directory, environment-file path and independent lifecycle.
- `deploy/oh-my-cpa/service.env.example`: non-secret settings only; no usable management or master key.
- `deploy/oh-my-cpa/verify.py`: a rerunnable, read-only baseline/after report that compares permitted fields and fingerprints without dumping secrets. Avoid adding a new helper if the independent verifier identifies a suitable existing one.

Verify the actual release asset, checksum and ARM64 support. Pin a version and executable hash; do not deploy `latest`. Before first live CPA reachability, bootstrap against a blocked/unconfigured CPA target and persist and read back these OMC-only preferences:

- `omc_quota_auto_refresh={"interval_minutes":0,"refresh_on_reset":false}`. Default interval is 360 minutes with reset refresh enabled; startup workers can discover credentials within the first minutes, probe upstream through CPA, and clear cooldowns. A successful manual, automatic or warmup quota read can change account eligibility. Ingest-off is not sufficient.
- `omc_plugin_source_enabled=false`. Plugin-store browsing is still excluded: its GET can reconcile CPA's plugin sources, and false can remove a previously managed source. Disabling the preference is not permission to browse the store.
- Verify all provider/credential warmup overrides are off, including restored database preferences. Warmup defaults alone do not prove effective policy.
- Keep updater and both update checks off. Inspect other startup jobs; normal pricing/FX workers can still make outbound requests. Do not claim a network-silent service.

No verified environment switch substitutes for those stored preferences. The implementer must prove the exact preference-persistence/readback procedure in the pinned release using an isolated database before exposing it to the live proxy. Do not invent a read-only setting or race the first worker tick.

Phase-one permitted operations exclude quota refresh, cooldown clearing/reset/redemption, credential renewal or verification, model probing, playground, built-in agent actions, configuration/routing/provider/credential writes and restores, plugin store and trusted plugin host. Ordinary GETs are not a sufficient allowlist: some initiate OAuth or upstream actions, reveal credentials, check external releases, or reconcile plugin sources. Any enforced boundary must name exact paths/operations; otherwise describe the service as admin-trusted with operational restrictions, not technically read-only.

Acceptance: the deployment adds only the sidecar and its owned files. The proxy service, configuration, API routes, account selection and T3 settings are unchanged, except for a separately specified usage-statistics setting if required.

## Task 3: Start collection and prove coexistence

1. After bootstrap preferences and sole queue ownership are verified, bind the sidecar to loopback and establish explicit authenticated RESP subscription. No fallback to `http_pull` or `resp_pull`. Explicit `subscribe` still runs destructive LPOP backfill approximately every 30 seconds; stock OMC does not satisfy a strict no-drain requirement. Do not enable collection while that behavior conflicts with another owner. Raw RESP uses the admin key over TCP; keep it on loopback rather than sending it through an HTTP-only HTTPS endpoint.
2. Validate `/api/healthz`, authenticated `/api/v1/usage/ingest-status`, `/api/v1/usage/events`, and `/api/v1/preferences` against the pinned release. Require effective subscription mode, desired stored preferences, and absence of unexpected quota/cooldown/config/plugin-source mutations. Do not invoke manual ingest refresh, a second subscription test or a queue GET as health checks.
3. Observe ordinary user traffic. Match a new request's model/account identifiers, nullable tokens, timestamps and outcome between available proxy evidence and the sidecar. Do not compare total attempts to logical requests without checking retries and the decoder's record semantics.
4. Confirm the widget and console remain functional and sidecar ingestion does not change the selected account, quota freshness or recent-request feed. Observe existing passive/cached UI evidence: reloading the account console can trigger upstream quota refresh and is not a non-mutating baseline check.
5. Stop and restart only the sidecar. Previously recorded history must survive; inference and the widget must continue. Record any subscription disconnect gap honestly. Persistence after receipt does not imply exactly-once delivery or durable replay from the proxy stream.

Acceptance: durable records, unchanged custom controls, sole approved ownership of residual queue reads, no implicit credential/config/cooldown writes, and independent sidecar lifecycle all work. The proxy skips queue append while subscribers exist; subscriber buffers are bounded at 256 and overflow can disconnect a slow consumer. Without a subscriber, usage queue retention defaults to 60 seconds, capped at 3600; errors are push-only. Do not claim durable replay, exactly-once ingestion or lossless accounting from a dashboard total.

## Task 4: Make history reachable from the existing workflow

First use an existing private access path or a dedicated SSH forward for the sidecar. Do not repoint the widget's configured proxy root to Oh-My-CPA. Adding private HTTPS ingress is a later implementation step subject to the host's existing deployment guidance; no public exposure or unrelated Caddy edits are part of this plan.

Provide an Oh-My-CPA browser shortcut. Any widget link belongs to the widget owner and must use a separate validated analytics URL; it must not replace the `/accounts.html` action or contain an authentication token. No custom analytics rendering or new live API layer is necessary initially.

Acceptance: users can reach accounts, live widget and historical diagnostics independently. No field renaming or shared API change is required.

## Task 5: Gate T3 MCP integration separately

Oh-My-CPA exposes `/api/mcp`, but its management credential is administrator-equivalent. In pinned v0.3.20, the MCP caller is granted every MCP capability and the management-key identity has no per-capability scoping. High-risk writes require browser approval; low-risk writes, including quota refresh and refresh-policy changes, execute directly. No deployable scoped diagnostic credential was found in the reviewed stock code. Leave T3 MCP unattached for phase one. An HTTP-method filter cannot restrict MCP `tools/call`, writing GETs, or a raw admin credential's direct CPA access.

If a future pinned release provides enforceable scoping, configure only diagnostic tools using protected credential delivery on the actual execution environment. Otherwise retain browser analytics; broad management MCP would be a separate explicit product/permission choice. Do not silently claim a restricted connection or implement an unreviewed authorization proxy. Test denied operations against dummy targets and prove they never reach upstream and that the credential cannot bypass the boundary. Check environment-specific MCP support before changing T3/provider settings.

Acceptance: an agent can query recent failures and latency without receiving OAuth tokens or raw secret/config output; an attempted routing/config write through the restricted connection is rejected in a disposable test, never by changing production. If no restricted mode is available, mark that capability gate unresolved rather than claiming success.

## Task 6: Backups, updates and independent after-check

Use consistent SQLite backup, such as documented `VACUUM INTO`, or stop only Oh-My-CPA and back up its entire data directory. Copying a live `.db` alone can omit WAL transactions. Verify backup integrity and a restore in an isolated data directory with a matching master key; keep a pre-upgrade snapshot because forward migrations cannot be undone just by replacing the executable.

Initially retain request rows for 90 days and raw inbox messages for 7 days. Measure actual growth before tuning. Define a backup schedule and retention explicitly during implementation; upstream migration backups are not a replacement for a tested routine backup. Update the sidecar independently from the custom proxy, testing the pinned v8/subscription contract after upgrades.

Have the independent Sol-high thread verify the final artifacts and before/after evidence at deployment. If a new implementation diff needs a fresh review, create a fresh delegated review task with the artifact and objections; do not reuse a completed delegate review by sending a message. The independent top-level thread can retain continuity for its own ongoing baseline and post-deployment responsibility.

Stop only Oh-My-CPA and prevent automatic restart if it writes unrequested proxy settings, triggers quota/api-call/cooldown/warmup/renewal actions, changes plugin sources, interferes with another collector, silently falls back, misdecodes events, attempts a proxy update or leaks secrets. Also stop on the first unexpected authentication rejection; CPA can ban a client IP after repeated failures. Do not test bad keys against production. Preserve evidence and report the specific failure. Stopping the sidecar cannot undo an already applied mutation, lost event or secret exposure; do not promise that it does. Restore only OMC-owned data/version as appropriate; do not overwrite proxy credentials, restart CPA/T3/widget, or rewind widget commits.

Before allowing any future CPA configuration writes, fixture tests must preserve `routing.codex-oauth-auth-id`, `routing.codex-oauth-strategy`, session affinity, payload rules, unknown nested siblings, arrays, secrets and comments; cover legacy-layout conversion, stale ETags and partially applied multi-call writes. OMC uses sparse changes in some paths but also mapping replacement and whole-YAML PUT, so neither universal preservation nor universal erasure is established. Its generic routing projection omits the fork's two selection fields and must not be used to report the custom selected account. Exact pre-write backups intentionally use legacy raw/config reads; v8 support does not mean zero v0 calls. No deprecated route changes are needed for this integration.

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
- Pinned subscription/backfill implementation: https://github.com/WizisCool/oh-my-cpa/blob/63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d/internal/usage/ingest/runner.go#L499
- Pinned quota automation: https://github.com/WizisCool/oh-my-cpa/blob/63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d/internal/api/quota_auto_refresh.go#L82
- Pinned plugin-source reconciliation: https://github.com/WizisCool/oh-my-cpa/blob/63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d/internal/api/management_plugin_source.go#L40
- Pinned capability authorization: https://github.com/WizisCool/oh-my-cpa/blob/63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d/internal/capability/executor.go#L62
- Pinned config client: https://github.com/WizisCool/oh-my-cpa/blob/63a799b325ff220b2a9c54ec8a0d29e53a5c9f4d/internal/cpa/management/client_config.go#L168

Unresolved before implementation: current routing and redacted credential inventory; usage-statistics state, Home mode and other collectors; sole residual-queue ownership; actual ARM64 release artifact/checksum; isolated bootstrap/preference-persistence procedure and effective warmup policy; repeated port check; version-specific T3 connector evidence; and routine backup destination/schedule. Stock MCP remains deferred because no scoped credential was found. Existing host metadata and independent source findings resolve the earlier topology and startup-behavior uncertainties but are not deployment acceptance. This review made no installation or live configuration changes.
