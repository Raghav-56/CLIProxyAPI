# CLIProxyAccounts for Rainmeter

Passive 340-pixel widget for the custom CLIProxyAPI v8 account console. Requires Windows PowerShell 5.1+ and an existing Rainmeter installation. No additional dependencies, proxy process, OAuth copies, quota probes, queue consumption, routing changes, or scheduled tasks.

Run from Windows PowerShell:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\desktop\rainmeter\Test.ps1

# Existing authorized Oracler SSH configuration and management-key file:
.\desktop\rainmeter\Install.ps1 -BaseUrl https://oracler.tail0f2859.ts.net:8443 -ImportExistingOraclerKey

# Or supply a key without putting it in process arguments or shell history:
$key = Read-Host 'Management key' -AsSecureString
.\desktop\rainmeter\Install.ps1 -BaseUrl http://127.0.0.1:8317 -ManagementKey $key
$key = $null
```

The loopback URL requires your existing SSH localhost forward. The helper does not create tunnels. URL validation accepts only canonical `127.0.0.1`, `[::1]`, or HTTPS names matching `machine.tailnet.ts.net`. Other private HTTPS DNS names are deliberately unsupported. TLS certificate validation remains enabled. All redirects are refused, including same-origin redirects. A 401 or 403 stops polling until an explicit restart. Polls are serial with a 15-second pause after completion. Network operations have no timeout; the stop file cancels an in-flight HTTP task. No raw exception details are logged.

`-RainmeterDirectory` selects a portable Rainmeter installation with a `Rainmeter.ini` containing `SkinPath`. Default is `%LOCALAPPDATA%\Programs\MementoMori`. Installation adds only `CLIProxyAccounts` and preserves the MementoMori skin and unrelated layout settings. An existing skin without our ownership marker is never overwritten. Reinstall stops the previous helper and reuses its protected key. Add `-StartWithWindows` only if you explicitly want a Startup-folder shortcut. No autostart change occurs by default.

The helper and its private `settings.json`, `key.dpapi`, and atomic `snapshot.ini` live in `%LOCALAPPDATA%\CLIProxyAccounts`. The directory DACL grants only the current Windows user and SYSTEM access. DPAPI uses CurrentUser. No key is stored in Rainmeter, a shortcut, the snapshot, arguments, or logs. The installer can transiently capture the existing management key through SSH stdout in memory and immediately DPAPI-protect it. PowerShell/.NET cannot guarantee erasure of immutable strings from process memory. Do not share process dumps.

The skin's refresh action starts the hidden helper. A user-session mutex allows only one instance. Closing the skin leaves the helper running; stop it explicitly when desired. Restart with the installed shortcut or refresh the skin:

```powershell
& "$env:LOCALAPPDATA\CLIProxyAccounts\Stop.ps1"
& "$env:LOCALAPPDATA\CLIProxyAccounts\Start.ps1"
.\desktop\rainmeter\LiveCheck.ps1  # Redacted response shapes only
.\desktop\rainmeter\TestLifecycle.ps1  # Opt-in installed helper stop/restart check
& "$env:LOCALAPPDATA\CLIProxyAccounts\Uninstall.ps1"
# Also remove the protected key:
& "$env:LOCALAPPDATA\CLIProxyAccounts\Uninstall.ps1" -RemoveKey
```

Use `-RemoveKey` on the first uninstall invocation. The protected directory and `Core.ps1`/`Uninstall.ps1` remain for inspection; only owned files and the marked skin are removed. The Rainmeter layout may retain its inactive CLIProxyAccounts entry. No unrelated skin is deactivated. Startup removal checks the shortcut's target arguments before removing it.

Quota values use each credential's passive `quota.observed_at` and `signals`. The response's top-level `observed_at` and successful connection time do not renew quota freshness. Primary and secondary labels use actual minutes, so Go 30-day windows stay separate from 5-hour and weekly windows. Missing, future, malformed, older-than-five-minute, and reset-crossed observations do not count as available. `Reset-After-Seconds` is relative to the observation. Reset-at takes precedence. The Lua reader expires quota displays at the earliest known reset or freshness boundary between polls, and blanks snapshot data if the helper has not written for 35 seconds.

The pool includes only persistent Codex OAuth accounts with valid credential IDs, names, and auth indexes. API-key accounts, runtime credentials, and malformed identities are excluded. The manual selection stays separate from pool counts and a masked account list. Counts describe observed account states, never pooled subscription capacity or the next account. Automatic routing does not select a displayed account. Hash labels are deterministic pseudonyms of credential IDs, not emails. The list shows up to six entries; the console contains the full pool. Latest attempt is the newest observed executor attempt across providers, with allow-listed model text, fixed transports, elapsed time, nullable TTFT, and input/output/cached-input token counts. Missing token counts remain unknown, including when usage was not reported. The selected line also identifies the subscription plan when reported. Executor completion does not prove client delivery. History resets with the server process.

The snapshot uses fixed field names and a strict display alphabet that excludes bangs, brackets, hashes, percent signs, quotes, backslashes, and controls. Lua validates every field again before passing text as a separate Rainmeter API argument. API text never enters an action, variable, include, or executable Lua file. The console action runs a fixed local script which validates the configured root again before opening `/accounts.html`. There are no account-switch actions.

Source contracts: `docs/account-console.md`, `internal/api/handlers/management/auth_files.go`, `sdk/cliproxy/auth/codex_quota_selector.go`, `sdk/cliproxy/auth/quota_signals.go`, and `internal/api/recent_requests.go`. Rainmeter Lua API was checked against `/rainmeter/rainmeter-docs` through Context7. Canonical `X-Codex-*` signals are verified by source and live responses. The upstream quota headers use the same canonical names. Unverified aliases are ignored.

`Test.ps1` uses deterministic times for freshness/reset tests, real atomic replacements, CurrentUser DPAPI and DACL verification, and local TCP fixtures for HTTP 401/403/302. Fixtures are temporary and never installed as live data. `LiveCheck.ps1` only performs the four authorized GETs and prints field names and non-identifying counts.

Run Lua checks in the installed Rainmeter engine:

```powershell
& "$env:LOCALAPPDATA\Programs\MementoMori\Rainmeter.exe" '!CommandMeasure' 'ReadSnapshot' 'SelfTest()' 'CLIProxyAccounts'
# Rainmeter.log records: CLIProxyAccounts Lua SelfTest PASS
```

See `VALIDATION.md` for the MSI-Raghav execution record and remaining visual limitation. `SHA256SUMS.txt` contains source hashes, excluding itself.
