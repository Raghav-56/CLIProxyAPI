# Choose a Codex account

Open `/accounts.html` on your CLIProxyAPI server and connect with its management key.
The console shows persistent Codex OAuth accounts, their plans, quota windows, and reset times.
Click **Use this account** to select the subscription used for new Codex OAuth requests.

The choice is global to this proxy. Each logical request keeps its choice through retries.
Other providers and Codex API-key credentials keep their existing routing behavior.
Account selection does not rewrite OAuth credentials or restart the server.

All accounts can remain enabled. The selected account is stored in the configuration:

```yaml
routing:
  codex-oauth-auth-id: "codex-account.json"
  session-affinity: true
```

Use the credential ID returned by `GET /v8/management/credentials`. Do not use an email address as the ID.
An empty `codex-oauth-auth-id` restores the existing routing strategy.
Manual selection is unavailable when CLIProxyAPIHome is enabled.

Select an account through the management API:

```http
PUT /v8/management/routing/codex-account
Authorization: Bearer <management-key>
Content-Type: application/json

{"auth_id":"codex-account.json"}
```

Read the saved and applied choice with `GET /v8/management/routing/codex-account`.
A disabled account must be enabled through the main management dashboard before selection.
A failed save or reload returns an error instead of claiming that the choice is active.
Configuration writes use the existing repository writer and are not crash-atomic.

Quota refresh works independently of selection and uses server-side token substitution.
The browser receives quota information, not OAuth tokens. Windows are labeled by their actual duration;
missing windows remain unknown. A Go account can report a 30-day window instead of the Plus plan's windows.
Open **Available models** on an account card to inspect its live model catalog.
Selecting an account does not change your coding tool's selected model; use a model available to that subscription.

The Plus recommendation uses quota snapshots retrieved within five minutes. An account must be enabled,
available, below its limits, and have positive 5-hour and weekly quota with known future reset times.
Among eligible Plus accounts, the console recommends the account whose weekly quota resets first.
Ties prefer more 5-hour quota, then more weekly quota. A reset or failed quota refresh invalidates the snapshot.
Click **Use recommended account** to apply the recommendation. Guidance never changes the saved selection automatically.

## Inspect actual routing

**Recent request attempts** shows the account, model, downstream and upstream transport, elapsed time,
time to first token, and input/output/cached-input tokens observed by the execution usage reporter.
Retries can produce multiple attempts. A successful attempt describes executor completion; it does not prove
that a disconnected client received the answer. Unknown token counts or first-token times remain unknown.

`GET /v8/management/observability/requests/recent` requires management authentication and returns up to 200
attempt summaries, newest first. History lives in the proxy process and resets on restart or host handoff.
It observes usage through an independent plugin subscription and does not consume the usage queue.
Only an allow-listed summary enters history. Prompts, response text, keys, OAuth tokens, token fingerprints,
and upstream error bodies are excluded. HTTP/SSE downstream transport is distinguished from WebSocket;
upstream transport is reported from the actual executor when known.

The management key stays in browser memory unless you choose to keep it for that tab.
Disconnect clears it. The standard `/management.html` interface remains available for full configuration.

## Connect another computer

Use an authenticated private connection, such as an SSH localhost forward or Tailscale Serve.
Keep the inference and management keys enabled. Direct remote management must follow the server's remote-management policy.
Before moving a proxy between hosts, stop the source and transfer its latest credentials and configuration.
Keep exactly one host refreshing a shared set of OAuth credentials.

The `config-export <config-path>` utility emits canonical v8 JSON for a stopped server's configuration.
Its output contains configured secrets. Redirect it into a protected staging file rather than displaying or committing it.
