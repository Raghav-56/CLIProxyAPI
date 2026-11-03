# Oh-My-CPA sidecar

These files support a native Linux ARM64 user service independent of the proxy service. The templates do not install themselves. Pin the v0.3.20 ARM64 archive with SHA256 `305c013f65a9e002c343bcfddbf9f170a249857ba17fd4012d8e346c02256dd0`; its source revision is `d16465b34f6ebb38a4cb4b1fbdc144da25f37cd2`. Verify the downloaded archive before extracting it. The verified executable SHA256 is `b7251c817e8f140af9a661ec6871b7fa6ccaad44ac34a618c01ea93dde3cac71`. See [VALIDATION.md](VALIDATION.md) for the actual installed state and remaining approval gates.

Install the executable under `~/.local/share/oh-my-cpa/releases/<version>/` and point `~/.local/share/oh-my-cpa/current` to that release. Create the app and data directories with mode 0700. Copy the service to `~/.config/systemd/user/` and the environment example to `~/.config/oh-my-cpa/service.env` with mode 0600. Replace `REPLACE_USER` with the actual absolute home path: systemd environment files do not expand `%h` or shell variables. Privately provision the existing CPA management key and a separate persistent `OMCPA_MASTER_KEY` using the release's supported configuration; never put secrets in the repository or command arguments. Preserve and back up the master key separately from the database. The unit uses no Docker, socket updater, CPA dependency, or automatic restart.

Before first live CPA reachability, use the verified binary to initialize a new empty data directory against an unreachable loopback CPA target, then stop it and seed disabled preferences. `bootstrap.py` performs this sequence using the private environment file, forces CPA to `127.0.0.1:1`, and refuses an existing data directory. It persists and reads back `omc_quota_auto_refresh={"interval_minutes":0,"refresh_on_reset":false}` and `omc_plugin_source_enabled=false`, and rejects existing warmup policies. Keep updater and both update checks false. Ingestion off alone does not prevent quota or warmup actions. Disabling the plugin preference does not authorize browsing its store. Other pricing and FX workers can still contact the network.

```sh
python3 -B deploy/oh-my-cpa/bootstrap.py --binary /home/ubuntu/.local/share/oh-my-cpa/current/oh-my-cpa --data-dir /home/ubuntu/.local/share/oh-my-cpa/data --env-file /home/ubuntu/.config/oh-my-cpa/service.env
```

The command is for first installation only. It creates a private bootstrap log alongside the new data directory. `check_policy.py` and `bootstrap.py` must also be installed in the app directory: the service's read-only `ExecStartPre` guard imports the shared policy and refuses missing/changed preferences, warmup state, update checks, ingest fallback, or endpoints outside the approved loopback profile. This guard checks startup, not settings changed while the service is running. The console retains administrative capabilities.

Only after those gates, port availability, usage/Home state, and collector ownership are verified, use `systemctl --user daemon-reload` and start `oh-my-cpa.service` manually. Stop it on the first unexpected authentication rejection; it will not restart automatically. Stock `subscribe` also performs destructive LPOP backfill, so enable it only when the sidecar owns the residual queue and coexistence has been established. MCP is deferred: the stock management credential is administrator-equivalent. The sidecar is admin-trusted and operational restrictions are not a technical read-only boundary.

For upgrades, stop only the sidecar and back up its whole data directory, including SQLite WAL files; test a restore with the matching master key in an isolated directory. Pin and verify the new release, retain the old release and pre-upgrade data snapshot, and update the `current` symlink. Rollback requires restoring the matching pre-upgrade data as well as the prior executable; migrations may be irreversible. Do not rerun first-install bootstrap on restored data. This service does not provide lossless replay or exactly-once delivery.

`backup.py` uses SQLite's online backup API, verifies integrity, and retains seven snapshots with its own timestamped filename pattern. It does not delete unrelated files or copy keys. Install it into the app directory and the two `oh-my-cpa-backup` units into the user unit directory. The timer runs daily at 03:15 UTC and catches up after downtime. Keep a separate private recovery copy of the service environment, including the master key. The current backups and recovery settings are on-host; no off-host copy has been configured.

```sh
python3 -B /home/ubuntu/.local/share/oh-my-cpa/backup.py --database /home/ubuntu/.local/share/oh-my-cpa/data/oh-my-cpa.db --backup-dir /home/ubuntu/.local/share/oh-my-cpa/backups/database
systemctl --user start oh-my-cpa-backup.service
systemctl --user status oh-my-cpa-backup.timer
systemctl --user stop oh-my-cpa.service
```

Stop only the sidecar on an unexpected authentication rejection, proxy mutation, quota/cooldown action, or collector interference. Stop does not undo an applied change or recover lost events. Do not restore old proxy credentials/configuration or restart CPA/T3/Rainmeter as an incidental recovery action.

## Read-only baseline

Run only when explicitly authorized, against the canonical loopback proxy root and a private file containing the management key:

```sh
python3 deploy/oh-my-cpa/verify.py --proxy-root http://127.0.0.1:8317 --key-file /private/key --config-file /path/to/config.yaml --output /private/before.json
python3 deploy/oh-my-cpa/verify.py --proxy-root http://127.0.0.1:8317 --key-file /private/key --config-file /path/to/config.yaml --compare /private/before.json --output /private/after.json
```

Output files are created exclusively with mode 0600. The verifier makes only five named GETs; it refuses redirects and stops on authentication rejection. It emits hashes, selected routing and service health fields, and counts, never raw response bodies or account IDs. It requires active/running service state, consistent account selection, applied routing and Home disabled for this deployment. It compares service identity and health, executable and config fingerprints, config version, routing and credential count; recent activity is informational. Missing process fingerprints prevent a valid baseline. A matching count does not prove credential files are unchanged, and an ordinary proxy refresh may legitimately change them. This does not establish queue ownership, effective sidecar preferences, or collection correctness.

Run offline checks with `python3 -B -W error::ResourceWarning -m unittest discover -s deploy/oh-my-cpa -p 'test_*.py' -v`. Unit fixtures cover redaction, policy fail-closed behavior, bootstrap refusal and consistent live-WAL backups. The actual binary bootstrap and restore checks are separately recorded in validation.
