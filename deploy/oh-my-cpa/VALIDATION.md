# Oh-My-CPA installation validation

Recorded on 2026-11-03. Scope is the isolated native sidecar on Oracler. Existing Rainmeter source and installation remain owned by the widget work; no widget file, T3 setting, provider home, inference URL or OAuth file was changed by this implementation.

## Installed state

- Official native Linux ARM64 release `v0.3.20`, pinned source `d16465b34f6ebb38a4cb4b1fbdc144da25f37cd2`.
- Archive SHA256 `305c013f65a9e002c343bcfddbf9f170a249857ba17fd4012d8e346c02256dd0` matches release metadata and published `checksums.txt`.
- Executable SHA256 `b7251c817e8f140af9a661ec6871b7fa6ccaad44ac34a618c01ea93dde3cac71`.
- App directory `/home/ubuntu/.local/share/oh-my-cpa`, private settings `/home/ubuntu/.config/oh-my-cpa/service.env`, single user service `oh-my-cpa.service`.
- Running on `http://127.0.0.1:18317/`; no new tailnet listener or public exposure. Main service has not been enabled for boot. It has `Restart=no`.
- Ingestion remains `off`. No usage subscription or queue pop was performed. History collection, event matching and ingest-mode restart persistence are not accepted yet.
- Quota auto-refresh is disabled with interval 0 and reset refresh false; plugin-source management preference is false; all three warmup policy/state/run tables are empty. Updater and both update checks are off. MCP is not attached to T3.
- Startup guard is installed and passed during a real sidecar restart. It checks strict JSON types, persisted preferences, absence of warmup state and the approved environment profile. It refuses to create or repair a missing database. This is not a runtime read-only authorization boundary for an administrator using OMC.

## Live proxy preservation

The only authorized proxy configuration change was enabling `observability.usage.usage-statistics-enabled` through its v8 leaf endpoint with an `If-Match` guard. A protected exact pre-change YAML backup is stored outside this repository. Comparing parsed YAML proved that every other value remained unchanged.

- Before config SHA256 `4293df5cdd6833fdcfd08272aaee1f9614bc59156066802ea7c142fdc25bb782`.
- After metrics leaf SHA256 `103aa85a27d42ba111d9042658d95fcb3a8c6b5017ad256d7d94cc49ae54917b`.
- After sidecar start and guarded restart, the new config fingerprint/stat, service identity/start time/restarts, executable hash, routing selection/mode/strategy/applied/Home state and credential count matched the post-metrics baseline.
- Proxy PID 1814, zero restarts, executable SHA256 `b47745b0e151f028c2a23ecc9f7e7d9c20ee6c5062bd9c09e98b6467b4de0661`.
- Manual selection remained applied; Home remained disabled; credential count remained three. Account IDs and credentials were not written to reports.
- Service/container inventory found no configured usage collector. This is deployment evidence, not a claim that arbitrary transient clients cannot exist. Residual queue ownership must be explicitly approved before enabling stock OMC subscription/backfill.

## Actual binary and backup checks

1. Executed first-install bootstrap against an unreachable CPA at `127.0.0.1:1` in a fixture directory, then for the private production database. Both initialized successfully, stopped the process before seeding, and verified the two disabled preferences with zero warmup policies.
2. Started the production sidecar with ingestion off. `/api/healthz` reported version `v0.3.20`, database `ok`, CPA connected and management API `v8`.
3. Authenticated locally using the existing management key without displaying it. Preferences readback matched the seeded values; ingest-status explicitly reported disabled. No quota/model/inference/playground/plugin-store action was invoked.
4. Created consistent SQLite backups and checked integrity. Daily backup timer is active, scheduled for 03:15 UTC, retaining seven snapshots. A separate private on-host recovery settings copy preserves the master key. No off-host backup was configured.
5. Restored a backup to an isolated temporary directory and started the actual pinned binary using the matching recovery key, ingestion off and CPA forced to unreachable port 1. Database health, authentication, persisted disabled preferences and SQLite integrity passed. The temporary process was stopped and its directory removed. This validates the pre-collection database; no real usage-event restoration is claimed.
6. Restarted only the production sidecar with its startup guard installed. It remained active/running; the proxy baseline matched. This is a sidecar lifecycle check with ingestion off, not evidence of subscription replay or lossless collection.

## Source and automated verification

- CP GPT-6.0 Luna inspected the pinned local source for bootstrap and preference contracts. Parent source review corrected an overbroad claim about dashboard reconciliation: plugin-store/config paths reconcile sources; ordinary management overview reads are not themselves that reconciliation path. Root base path is explicitly configured as `/`.
- CP GPT-6.1 Terra produced service/environment templates and the non-mutating verifier.
- An independent CP GPT-6.1 Sol high review found the verifier accepted unhealthy/unapplied baselines. Parent fixed that issue and added negative tests. Its suggested persisted-policy startup check was also implemented and tested.
- 13 Python tests passed with ResourceWarning treated as an error. Tests include strict false-versus-zero policy checks, no repair of missing databases, rejected unsafe restores, account routing consistency, redaction and failed-request handling, committed live-WAL records, private backup permissions and retention boundaries.
- Focused existing Go tests passed in `internal/redisqueue` and `internal/api`: subscriber broadcast/queue behavior, recent-request independent observation, recent-route authentication, and RESP subscription support-refresh.
- User-service/timer unit validation passed with `systemd-analyze --user verify` outside the restricted sandbox. The initial sandbox-only attempt could not use the required user lookup sockets.
- No Go source changes. The required server compile passed before committing the deployment files. Final read-only health was `ok`/`v8`, ingestion was still off, the usage-event count was zero, and the only management audit action was the single local login. UI screenshot/layout verification is outside this task; existing widget checks remain owner-reported.

## Remaining approval gates

Automatic approval review rejected two commands before execution:

1. Adding private Tailscale HTTPS port 8444 for this dashboard. The stated reason was that it exposes the administrative dashboard/history to the tailnet without specific destination approval. Existing Serve listeners remain unchanged; a protected pre-change Serve snapshot exists.
2. Enabling explicit subscription. The stated reason was that it consumes the live stream and periodically destructively LPOPs residual queue records, with possible loss/interference, and required specific collection authorization.

No alternative collector, indirect queue reader, port forward, or other exposure was used to bypass either rejection. Approval must identify both the collection behavior and the intended private dashboard destination. Until then the useful installed artifact is a healthy local service with tested bootstrap/guard/backups, and collection stays off. Do not label this deployment's history collection complete.
