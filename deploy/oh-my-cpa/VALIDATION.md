# Oh-My-CPA installation validation

Recorded on 2026-11-03. Scope is the isolated native sidecar on Oracler. Existing Rainmeter source and installation remain owned by the widget work; no widget file, T3 setting, provider home, inference URL or OAuth file was changed by this implementation.

## Installed state

- Official native Linux ARM64 release `v0.3.20`, pinned source `d16465b34f6ebb38a4cb4b1fbdc144da25f37cd2`.
- Archive SHA256 `305c013f65a9e002c343bcfddbf9f170a249857ba17fd4012d8e346c02256dd0` matches release metadata and published `checksums.txt`.
- Executable SHA256 `b7251c817e8f140af9a661ec6871b7fa6ccaad44ac34a618c01ea93dde3cac71`.
- App directory `/home/ubuntu/.local/share/oh-my-cpa`, private settings `/home/ubuntu/.config/oh-my-cpa/service.env`, single user service `oh-my-cpa.service`.
- Running on `http://127.0.0.1:18317/`, privately accessible at `https://oracler.tail0f2859.ts.net:8444/` after explicit user approval. Existing Serve listeners are unchanged and public Funnel is not enabled for this port. Main service is enabled for boot, with `Restart=no`.
- Explicit `subscribe` ingestion is active and healthy. The user approved sole collection, including stock periodic destructive residual-queue backfill. No extra subscription or direct queue probe was used for verification. Ordinary request history, event matching and restart persistence have passed the checks below.
- Quota auto-refresh is disabled with interval 0 and reset refresh false; plugin-source management preference is false; all three warmup policy/state/run tables are empty. Updater and both update checks are off. MCP is not attached to T3.
- Startup guard is installed and passed during a real sidecar restart. It checks strict JSON types, persisted preferences, absence of warmup state and the approved environment profile. It refuses to create or repair a missing database. This is not a runtime read-only authorization boundary for an administrator using OMC.

## Live proxy preservation

The only authorized proxy configuration change was enabling `observability.usage.usage-statistics-enabled` through its v8 leaf endpoint with an `If-Match` guard. A protected exact pre-change YAML backup is stored outside this repository. Comparing parsed YAML proved that every other value remained unchanged.

- Before config SHA256 `4293df5cdd6833fdcfd08272aaee1f9614bc59156066802ea7c142fdc25bb782`.
- After metrics leaf SHA256 `103aa85a27d42ba111d9042658d95fcb3a8c6b5017ad256d7d94cc49ae54917b`.
- After sidecar start and guarded restart, the new config fingerprint/stat, service identity/start time/restarts, executable hash, routing selection/mode/strategy/applied/Home state and credential count matched the post-metrics baseline.
- Proxy PID 1814, zero restarts, executable SHA256 `b47745b0e151f028c2a23ecc9f7e7d9c20ee6c5062bd9c09e98b6467b4de0661`.
- Manual selection remained applied; Home remained disabled; credential count remained three. Account IDs and credentials were not written to reports.
- Service/container inventory found no configured usage collector. This is deployment evidence, not a claim that arbitrary transient clients cannot exist. The user explicitly approved sole ownership of residual queue reads before activation.

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

## Approved activation and final checks

Automatic approval review initially rejected two commands before execution:

1. Adding private Tailscale HTTPS port 8444 for this dashboard. The stated reason was that it exposes the administrative dashboard/history to the tailnet without specific destination approval. Existing Serve listeners remain unchanged; a protected pre-change Serve snapshot exists.
2. Enabling explicit subscription. The stated reason was that it consumes the live stream and periodically destructively LPOPs residual queue records, with possible loss/interference, and required specific collection authorization.

The user then explicitly approved both stated final steps with "do". The same intended actions were retried with that authorization, without an alternative collector or indirect exposure. On 2026-11-03:

1. Captured a fresh pre-activation baseline. The account/config had changed between the earlier installation and activation: the active config hash was now `17c33009013230b96620e54be7ba0a59c029e6411038ee706cb51a59f85fdd74`. The first comparison correctly reported those existing differences; activation did not restore the old selection. All subsequent comparisons used the fresh baseline and matched config/stat, routing, applied/Home state, inventory, process identity and binary. Proxy PID remained 1814 with zero restarts.
2. Changed only OMC's private environment to explicit subscription and public origin `https://oracler.tail0f2859.ts.net:8444`, preserving disabled automation/updaters. Restarted and enabled only the sidecar, updating its private recovery settings copy.
3. Added separate Tailscale HTTPS port 8444 forwarding to loopback 18317. Compared every prior Serve entry against the new state; existing entries remained identical, and no public Funnel entry was enabled. HTTPS returned the actual page with status 200. Login succeeded and issued a Secure session cookie.
4. Authenticated status reported healthy collector mode `subscribe`, enabled/running decoder, zero discarded records and zero reported coverage gaps at the observed checkpoints. Do not infer future lossless operation or guaranteed replay from those counters.
5. Matched 22 ordinary executor attempts against the proxy recent feed using request/trace identity plus requested-at timestamps. Models, outcomes, input/output/cache token counts and latency/TTFT matched. Recent-feed timing uses fractional milliseconds while the usage wire serializes integer milliseconds; comparisons use that actual precision. Request IDs alone can group several attempts, so timestamp matching is required. No synthetic inference or quota request was sent to generate evidence.
6. Restarted only the sidecar with subscription configured. All ten recorded IDs captured before the restart survived; the collector resumed healthy subscription mode. The proxy baseline matched. This verifies persistence after receipt, not lossless capture during a stop.
7. Ran the consistent backup service after collection. The resulting snapshot passed SQLite integrity checking and contained 25 real usage events plus the preserved disabled preferences. Earlier real-binary recovery-key verification remains applicable; this later check additionally verifies recorded-event backup contents.
8. Confirmed both the main user service and daily backup timer are enabled. The only observed OMC management audit action was local `auth.login`; no quota, cooldown, configuration or plugin-source mutation was observed. T3 MCP remains unattached, and widget files/routes are unchanged.

A fresh CP Sol-high post-activation review delegate was blocked before execution by automatic approval review because its prompt contained operational context/local paths for the model connector. No review payload was delivered and no replacement export was attempted. Final live checks were performed locally; they are not an independent post-activation review. The earlier independent source review remains documented above.
