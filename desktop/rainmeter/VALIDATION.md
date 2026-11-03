# MSI-Raghav validation

Executed 2026-11-03 using Windows PowerShell `5.1.26100.9818`. All authored project changes are inside `desktop/rainmeter/`. No commit or push was made. No proxy binary, OAuth files, configuration, credentials, routing, or service was changed on Oracler. The only remote shell operation read the existing authorized management-key file over SSH into memory for DPAPI storage.

## Commands and results

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File desktop/rainmeter/Test.ps1
# 101 checks passed on PowerShell 5.1.26100.9818

powershell.exe -NoProfile -ExecutionPolicy Bypass -File desktop/rainmeter/Install.ps1 -BaseUrl https://oracler.tail0f2859.ts.net:8443 -ImportExistingOraclerKey
# Installed helper: C:\Users\ragha\AppData\Local\CLIProxyAccounts
# Installed skin: C:\Users\ragha\AppData\Local\Programs\MementoMori\Skins\CLIProxyAccounts

powershell.exe -NoProfile -ExecutionPolicy Bypass -File desktop/rainmeter/TestLifecycle.ps1
# PASS: hidden helper and singleton
# PASS: installed helper stopped
# PASS: hidden helper restarted and connected
# PASS: no default startup mutation

powershell.exe -NoProfile -ExecutionPolicy Bypass -File desktop/rainmeter/LiveCheck.ps1
# All four authorized endpoints returned 200. Shapes recorded below.

& "$env:LOCALAPPDATA\Programs\MementoMori\Rainmeter.exe" '!CommandMeasure' 'ReadSnapshot' 'SelfTest()' 'CLIProxyAccounts'
# Actual Rainmeter.log:
# CLIProxyAccounts Lua SelfTest PASS: sanitization, stopped helper, freshness and reset boundaries
```

The 101 checks cover parsing every PowerShell entrypoint, URL rejection, actual 5-hour and 30-day labels, observation-relative resets, stale/future/exhausted/reset-crossed/unknown data, exact five-minute boundary, malformed flags and numeric data, schema rejection, manual versus automatic display, unknown TTFT, transport allow-listing, identity masking, Rainmeter expansion character rejection, 50 complete atomic replacements, absence of temporary write leftovers, DPAPI round-trip, plaintext absence, protected directory and explicit key-file DACLs, real HTTP 401/403/302 rejection, and cancellation of an established stalled HTTP connection using the stop file.

The hidden-start integration check caught and fixed a real Windows lifecycle issue: a child process inherited another PowerShell edition's module path. Core now explicitly imports this runtime's built-in Microsoft.PowerShell.Security manifest. Atomic File.Replace uses PowerShell's NullString sentinel to pass a real null backup path. DACL updates use DirectoryInfo/FileInfo SetAccessControl, which avoid requesting SACL privileges. DPAPI text loading trims the file's trailing newline.

Actual uninstall was run and checked before reinstalling. It removed the marked skin, retained the exact key-file SHA-256 and MementoMori Meter.ini SHA-256, and left other skins active. The widget was reinstalled using its retained key and left running. Repeated installs and repeated launches were exercised. No Startup shortcut exists. The key file has two access rules, for the current user and SYSTEM, with inheritance disabled. The containing directory also has inheritance disabled.

## Redacted live response shapes

- `credentials`: `{files, observed_at}`, three credentials. Passive quota is `{observed_at, signals}`. Canonical signal names include `X-Codex-Allowed`, `X-Codex-Limit-Reached`, `X-Codex-Plan-Type` and both `X-Codex-Primary/Secondary-{Used-Percent,Window-Minutes,Reset-At,Reset-After-Seconds}`.
- `routing/codex-account`: `{applied,auth_id,home_enabled,mode,strategy}`. Manual mode, selected ID present, applied true.
- `observability/requests/recent`: `{capacity,host,requests,retention}`. Request fields include IDs, requested time, provider/account/model metadata, actual downstream/upstream transports, latency, nullable TTFT/status/token counts, success, and usage-known state. No field values or identities were retained in this report.
- `server/info`: `{architecture,hostname,os}`. Host values were not printed.

The installed helper produced fresh connected snapshots with the applied manual route, masked selection, duration-labelled passive windows, pool known/blocked/unknown counts and list, and a recent observed attempt with transport/timing. Connection and quota observation ages were separate. No sample fixture was installed as live data.

## Preservation and limitations

The original MementoMori layout remains active with `WindowX=1408`, `WindowY=144`, `AnchorX=100%`, `AnchorY=100%`, and `AlwaysOnTop=2`. Its Meter.ini SHA-256 remained `5981778C6375E57F1F08A223512C45C393AC7369C91790D41CC268A34710CA9D` through uninstall/reinstall. Rainmeter's normal configuration discovery refreshes active skins; it does not replace their files or layout.

Rainmeter's log confirms the skin loaded and its real Lua engine ran the checks without errors. The host's supported computer-use inventory lists ordinary app windows but does not expose the Rainmeter desktop skin window, so no screenshot or visual clipping assessment was possible. The console button was inspected in source and targets a fixed script with URL revalidation; it was not clicked to open a browser. Custom private DNS outside Tailscale's machine.tailnet.ts.net pattern is intentionally unsupported.

Raw HTTP content is processed transiently in memory and not saved. PowerShell immutable strings cannot provide guaranteed memory erasure. The helper remains running after the skin closes until explicitly stopped. No separate Windows login autostart entry is enabled by default. Loading the skin starts the helper, including when Rainmeter restores that active skin.

## Final parent review and verification

The parent retrieved and checked the original 14-file SHA-256 manifest, independently reran the 101-check suite on MSI-Raghav, and obtained a separate source review. Review found that the pool filter could include Codex API-key accounts. The final filter requires persistent OAuth credentials with valid ID, name, and auth index. A mixed-credential regression now excludes API keys, runtime credentials, missing account types, and malformed identities.

The selected line now includes the subscription plan when recognized. The attempt section includes input, output, and cached-input token counts. Tests distinguish observed zero tokens, missing individual counts, and entirely unknown usage.

The revised suite passed **105 checks** on Windows PowerShell `5.1.26100.9818`. The parent then reinstalled the reviewed revision using the retained DPAPI key and independently verified:

- Hidden helper launch, singleton behavior, stop, restart, and reconnection.
- No separate Startup-folder entry.
- HTTP 200 and the expected redacted response shapes for all four authorized GETs.
- Exact installed/source hashes for the six helper scripts, Accounts.ini, and Snapshot.lua.
- Live snapshot fields for the masked selected account, subscription plan, and tokens.
- A passing SelfTest entry from the actual Rainmeter Lua engine.
- The original MementoMori Meter.ini hash unchanged.

On Oracler, the existing server build passed with Go 1.26.10. Existing API, management, and routing tests passed:

```sh
go build -o /tmp/rainmeter-server-build-check ./cmd/server
go test ./internal/api ./internal/api/handlers/management ./sdk/cliproxy/auth
```

The toolchain was invoked from its installed module-cache path with a temporary writable build cache. Tests required loopback socket access outside the restricted sandbox. No Go source changed. Source review and secret-pattern checks passed. The final manifest describes the reviewed source, including these corrections. Screenshot and console-button browser verification remain unavailable as described above.

## Compact view and visibility shortcut

The widget now defaults to 304 by 246 logical pixels, with two quota bars,
reported window labels, reset countdowns, and pool status. Details expands to
420 pixels and reveals masked account identity and request diagnostics. Native
Ctrl+Alt+Q hides or restores the same Rainmeter window, preserving its view.
MementoMori retains Ctrl+Alt+M independently.

The Windows suite passed 121 checks after adding structured snapshot fields.
The compact Lua view tests cover percentages, 5-hour/weekly/30-day labels,
human-readable reset countdowns, stale/reset boundaries, automatic mode,
unavailable accounts, and expiration of availability in Details. Independent
review found the last two display-state issues; both were corrected with tests.

The Windows hotkey handler was compiled and installed using built-in .NET tools.
Native registration, singleton behavior, cancellation, key release, and restart
passed. Injected Ctrl+Alt+Q chords in compact and expanded views hid and restored
the same HWND while preserving the foreground window. Eight repeated keydowns
in one atomic input batch produced one toggle. Earlier separately injected
held-key tests were inconsistent, so the verified no-repeat result is limited
to the atomic input test.

The installed Rainmeter engine passed SelfTest and LayoutTest in compact,
expanded, and restored compact modes. The original MementoMori hotkey process
remained running. The final state is compact, with one helper and one quota
hotkey handler in the interactive session. No new Startup entry was added.
Full uninstall of the newly added hotkey was not exercised; its cancellation
and release paths were tested. Screenshot verification remains unavailable.

The parent's independent SSH run passed compilation and mock dispatch. Its
`-Live` stage correctly refused execution outside Rainmeter's interactive
session. The actual registration, input, and foreground results above came
from the Windows-side agent in the interactive session, not that SSH run.
The server build passed again after the redesign; no Go source changed.
