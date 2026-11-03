# Show proxy accounts on a Windows desktop

The Rainmeter companion shows this proxy's Codex accounts on Windows while the
proxy runs on another computer. It complements `/accounts.html` and does not
change account selection, routing, credentials, or the proxy installation.

The companion source is in [desktop/rainmeter](../desktop/rainmeter/).
Use its installation and test instructions on the Windows computer running
Rainmeter. Keep the helper and its protected settings on that computer.

## Read the widget

Press **Ctrl+Alt+Q** to hide or restore the widget. MementoMori keeps its separate
**Ctrl+Alt+M** shortcut. **Hide** does the same without unloading the skin.
**Details** expands account identities and request diagnostics. Hiding preserves
the current compact or expanded view.

The compact view shows two quota windows side by side, remaining percentages,
bars, reset countdowns, and pool status. Window labels come from their actual
durations. Unknown or stale values show dashes rather than empty quota.

The saved account selection and the last observed execution describe different
things. Manual selection identifies the configured account. In automatic mode,
session affinity can retain an account for an existing conversation. The latest
attempt identifies the account that actually handled that attempt, but does not
identify the current T3 thread.

Pool counts describe the listed Codex OAuth accounts. They do not establish
capacity for a particular model or request. Disabled accounts, unavailable
accounts, exhausted quota, and unknown quota need separate treatment.

Quota windows use their reported duration. A primary window is not necessarily
five hours, and a secondary window is not necessarily a week. Missing values
remain unknown. A successful HTTP poll does not refresh the timestamp of a quota
observation. Observations older than five minutes, future observations, and
windows whose reset has passed must not appear current.

The server refreshes quotas in the background while automatic soonest-reset
routing is active. In manual mode, passive observations may become stale. Use
**Refresh quotas** in the account console when fresh upstream quota is needed.
The desktop helper does not perform upstream probes or refresh OAuth credentials.

Request activity is a process-local history of at most 200 execution attempts.
It resets when the proxy restarts. Unknown time to first token and token counts
remain unknown. This widget is not a persistent usage ledger.

## Keep authentication outside the skin

Use the existing private HTTPS endpoint or an SSH localhost forward. Keep both
management and inference authentication enabled. Direct remote management still
follows the server's remote-access policy.

The management key authorizes management operations broadly. The helper protects
its saved key with Windows DPAPI for the current user and restricts access to its
settings directory. The Rainmeter skin reads only a local display snapshot. That
snapshot must contain no management key, inference key, OAuth token, raw
credential object, or upstream response body.

The console link contains no key. Opening it may require connecting in the
browser. Do not put the management key in a Rainmeter variable, URL, shortcut
argument, or shell history. DPAPI protects the saved secret from other users;
it does not protect it from another process running as the same Windows user.

## API contract

The helper uses only these read operations under `/v8/management`:

| Path | Fields needed by the companion |
| --- | --- |
| `/credentials` | `files`, credential identity and availability, `quota.observed_at`, `quota.signals` |
| `/routing/codex-account` | `mode`, `auth_id`, `strategy`, `applied`, `home_enabled` |
| `/observability/requests/recent` | `requests`, `capacity`, `retention` |
| `/server/info` | Optional server identity check |

Join routing to credentials by `id`, not email or list position. Filter the account
pool to persistent Codex OAuth accounts. Read window durations from
`X-Codex-Primary-Window-Minutes` and `X-Codex-Secondary-Window-Minutes`, percentages
from `Used-Percent`, and resets from `Reset-At` or observation-relative
`Reset-After-Seconds`. These names are keys within `quota.signals`, not headers
on the management response. Treat malformed values as unknown.

The recent feed supplies `auth_id`, account, model, downstream and upstream
transport, latency, nullable TTFT, and nullable token counts. Render only a small
allow-list of these fields. Sanitize all displayed API strings before writing a
file that Rainmeter can interpret.

Never poll `/observability/usage/queue` or `/v0/management/usage-queue` from this
helper. Both consume the same destructive queue. The recent-attempt feed uses an
independent subscription and can coexist with an analytics collector.

An authentication rejection stops automatic requests until the connection is
corrected and restarted. Do not repeatedly retry an invalid management key.

## Verified server behavior

A read-only probe on Oracler on 2026-11-03 returned HTTP 200 for all four paths
above. The credential response contained `files` and `observed_at`, with three
accounts. Per-account quota observations contained the primary and secondary
signal names above, including absolute and relative resets. The routing response
reported manual mode, a selected ID, `applied: true`, and `home_enabled: false`.
The recent feed reported `retention: "process"` and nullable timing/token fields.
The server identified itself as Linux ARM64.

The implementation contract agrees with the live response. The important
distinction is that the account-list response timestamp and each quota
observation timestamp are separate. No production routing writes, credential
changes, or deliberate authentication failures were used for this probe.

## Relation to Quota Tray

[CLIProxy Quota Tray](https://github.com/ZYHUO/CLIProxy-Quota-Tray/tree/f153fc54ab32aa8e5e59c8332d9576b06e0da6cb)
informed the exploration. Its Electron collector is not a dependency of this
companion. That revision consumes the HTTP usage queue, uses v0 management paths,
and caches quotas for 20 minutes. The Rainmeter companion uses this fork's v8
read contract and the server's existing observations instead.
