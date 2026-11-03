# T3 Code and the account console

T3 Code has separate connections for agent requests and subscription limits.
A Codex provider sends requests through the selected model provider. A CLIProxyAPI
usage hub supplies account quotas to **Usage → Limits**. Adding one does not configure
the other.

## Provider setup

This setup keeps direct access available while the proxy manages its account pool:

| T3 instance | State | Codex launch arguments | Purpose |
| --- | --- | --- | --- |
| Codex - Direct ChatGPT | Enabled | No provider override | Direct access through the local ChatGPT login |
| Codex - Proxy accounts | Enabled | `-c model_provider="cliproxy"` | Requests through CLIProxyAPI |
| Codex - HTTP diagnostic | Disabled | `-c model_provider="cliproxy_http"` | Explicit transport diagnostics |

The proxy provider uses a Responses API base URL, such as `http://127.0.0.1:8317/v1`.
Its credential helper supplies the inference key. The management key belongs in the usage-hub
connection. Provider settings belong to the execution environment. Identically named
providers on Windows and Linux are separate configurations.

Keep compatible providers on the same **CODEX_HOME path** to continue existing threads.
T3 supports a **Shadow home path** to isolate authentication while sharing sessions and
configuration. A separate CODEX_HOME prevents continuation from the other home.
Do not move homes or copy OAuth files just to change a Limits label.

## Limits from the proxy

Open **Settings → Providers → Usage providers → Add hub** on the environment that can reach
the proxy. Enter the hub root URL and management key. Use `http://127.0.0.1:8317` when T3 and
the proxy run on the same computer, or the proxy's private HTTPS root URL from another computer.
Do not append `/v1`, `/accounts.html`, or a management prefix. Check for an existing hub first.

The inspected T3 build, `0.1.7-nightly.20261102.4061`, appends
`/v0/management/auth-files` and `/v0/management/api-call`. This fork still registers those
deprecated routes. The console uses the maintained v8 routes. No proxy route changes are
required for the current connector. Future connector work belongs in T3 and should use
`/v8/management/credentials` and `/v8/management/requests/api-call`.

The hub reads enabled accounts and asks the proxy to fetch upstream quotas.
Its optional **Use reset** action redeems a credit and clears the corresponding cooldown.
Refreshing limits and redeeming a credit have different effects. A usage hub does not change
the proxy's selected account or automatic routing strategy.

## Interpret the Limits tab

T3's pooled percentage is an arithmetic average of accounts reporting that window.
It is not a count of available requests, tokens, or equivalent subscription capacity.
Monthly Go windows remain separate from Plus session and weekly windows.
Three session values of 98%, 98%, and 91% produce a rounded 96%. That arithmetic does not
establish that the segments represent three distinct accounts.

The inspected client deduplicates by normalized email and workspace identity, or a credential
fingerprint when email is absent. A workspace ID alone does not produce a deduplication key.
Entries without a key fall back to provider-instance or hub identity, so matching-looking
snapshots can remain separate when metadata is missing. Matching percentages and reset times
alone are insufficient proof of duplication.

T3's provider discovery calls Codex `account/read` and `account/rateLimits/read`, including
for this custom proxy setup. Changing the inference base URL does not make that native probe
report the account selected inside CLIProxyAPI. Providers sharing a Codex home can read limits
associated with the same local login. The hub supplies the proxy's account pool.
Current provider settings expose no switch to disable only this native usage probe.

In manual mode, a healthy pool can coexist with an exhausted selected account. Check the
selected account in [the account console](account-console.md). **Request activity** confirms
which account handled an attempt. Automatic routing can retain an existing session's account,
so a pooled quota does not guarantee a different account for the next turn.

## T3 improvements

The upstream review on 2026-11-03 found existing proposals for
[duplicate Codex identities](https://github.com/pingdotgg/t3code/pull/25784),
[matching reports without email](https://github.com/pingdotgg/t3code/pull/27555),
[hub-sourced limits](https://github.com/pingdotgg/t3code/pull/27901), and
[clearer pooled averages](https://github.com/pingdotgg/t3code/pull/26256).
Check these before starting another implementation of the same problem.

The most useful T3 change is an explicit limits source for each provider instance.
A proxy instance should use its hub instead of presenting the local login's quota as the
routed account's quota. Provider names and account-console CSS cannot correct that association.

Limits details should identify local-login probes, hub accounts, and unidentified sources.
Keep unidentified sources visible with a notice rather than silently merging them.
A workspace-only deduplication proposal needs fixtures for separate users in the same workspace.

For manual routing, show the selected account beside the pool. For automatic routing, show
the mode and link to the console. Only request observations can associate an attempt with
the account that handled it.

## Sources

- [T3 usage and limits](https://github.com/pingdotgg/t3code/blob/main/docs/user/usage.md)
- [T3 Codex homes and shadow homes](https://github.com/pingdotgg/t3code/blob/main/docs/user/providers-codex.md)
- [T3 Codex provider discovery](https://github.com/pingdotgg/t3code/blob/main/apps/server/src/provider/CodexProvider.ts)
- [T3 CLIProxyAPI connector](https://github.com/pingdotgg/t3code/blob/main/apps/server/src/usage/cliproxyApi.ts)
- [Custom-provider native usage-probe issue](https://github.com/pingdotgg/t3code/issues/11873)
- [OpenAI custom-provider authentication](https://developers.openai.com/codex/auth)

Implementation details were also checked against the installed server and client bundle
for the stated build. Upstream `main` can change independently of an installed release.
