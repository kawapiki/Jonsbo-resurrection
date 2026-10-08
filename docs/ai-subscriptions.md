# AI subscription displays

The compiled-in `ai-subscriptions` module connects ChatGPT/Codex and Claude
subscriptions through their official native clients. It shows quota windows,
reported or locally observed weekly tokens, session activity and current context
where supported. Subscription charges and API billing are separate: no API key
is required and the module does not estimate billed spend.

## Detect your clients and add a widget

1. Build with `scripts/build.ps1`, then start the tray app or `bin/jonsbo.exe serve`.
2. Open **Display Studio → Manage AI usage**. The usage page checks whether each
   native CLI is installed and signed in, and shows monitoring separately.
   Choose **Start monitoring** to reuse an existing subscription login. Newly
   detected accounts are not enabled automatically. Both providers require the
   official native executable on PATH; shell wrappers are unsupported.
   **Check again** detects a newly installed CLI or a changed native login.
3. If a subscription login is missing, sign in yourself through the native CLI:
   `codex login` or `claude auth login --claudeai`, then use **Check again**.
   API-key authentication does not count as a subscription login. The page
   reports unavailable usage and the required command; Jonsbo does not start a
   provider login or maintain a second provider account.
4. Open a screen or choose **Design without a display**. Use the provider's
   **Add widget** action, or add an **AI subscription** widget in the editor.
   Select compact/expanded detail and animation preference.
   Use **Save & apply** to assign it to a connected display.

The provider mark is unchanged; its surrounding connection indicator animates
when the account is connected and fresh. Activity has a separate indicator.
Browser reduced-motion settings pause editor previews; switch animation off in
the widget to pause it on a physical display.

The module also exposes `overview` and `sessions` (640×480), plus `openai` and
`claude` (640×180) views for direct API assignments. Normal screen rotation still
applies after rendering.

## Desktop and CLI feeds

Subscription quotas refresh while Jonsbo is running, even when your terminals
and chats are closed. OpenAI uses the managed native Codex account service.
Claude Code **2.1.293 or later** provides a headless usage-control request backed
by its native `/usage` implementation. Jonsbo launches a hidden, temporary
metadata reader at most once every three minutes; Claude handles its own OAuth
renewal. The reader sends no user/model message, disables hooks and other
customizations, and excludes transcript-based behavior summaries. Its
experimental structured protocol fails closed if unavailable or changed.

The widget retains account quota readings during an outage, marks them stale,
and shows their age. This is the age of Jonsbo's native-client observation;
Claude can itself answer from a cached snapshot and does not expose that
snapshot's fetch time in the structured reply. Fresh Claude account quota has a five-minute tolerance for
the three-minute polling interval. **Stop monitoring** removes cached account data
while keeping the native client signed in.
Account token history remains source-dependent: OpenAI can report daily buckets;
Claude's usage-control reply provides plan quotas, not an account token ledger.

Choose **Show activity setup** to obtain merge proposals for native hooks,
Claude status-line forwarding and OpenTelemetry HTTP/JSON logs. The packaged
`bin/jonsbo-ai-observer.exe` helper reads stdin and forwards only allowlisted
metadata to the local server. Replace `jonsbo-ai-observer` in proposals with the
absolute quoted executable path unless its folder is on PATH.

Merge hook arrays with existing hooks and approve native Codex hook trust.
Preserve any existing Claude status-line command by adding forwarding to its
wrapper; the proposal does not edit your settings. Preserve existing telemetry
exporters. Prompt, response and tool-content export should remain disabled.

For Claude Code with no existing status line or OTLP destination, the Windows
setup script installs both feeds and backs up `~/.claude/settings.json`:

```powershell
.\scripts\configure-claude-observer.ps1
```

Use `-WhatIf` to preview, or `-DataDirectory` for a custom server data directory.
Restart Claude Code after setup. Readings arrive after its next normal response;
the script does not send a model request. Existing unrelated status lines and
OTLP destinations are preserved and require a manual merge. Restoring the printed
backup removes this setup. If you move the release folder or change the server's
port or observer credential, run setup again.

The widget uses background native usage as subscription quota. Fresh status-line
limits are a fallback labelled **observed Claude Code**, and deduplicated telemetry
appears as **local tokens this week**. These local readings remain separate from
verified account totals. Status-line quota fields are documented for Pro/Max;
background native `/usage` also supports Team accounts when the service reports
limits. Missing limits show **Limits not reported**.
Tokens accumulate from requests observed after setup, not historical chats.

The managed OpenAI observer does not see chats owned by another Codex process;
install the local feeds in each client you want to observe. Claude Code Desktop
sessions can use Claude Code hooks; ordinary Claude desktop chats are distinct.

## Optional Chrome/Edge companion

1. Open `chrome://extensions` or `edge://extensions`, enable Developer mode and
   **Load unpacked** the `integrations/browser` folder. Note its 32-letter ID.
2. While the server is running, locate `ai-subscriptions/bridge.json` beside the
   API token directory. CLI default: `bin/ai-subscriptions/bridge.json`; tray
   default: `%LOCALAPPDATA%/JonsboResurrection/ai-subscriptions/bridge.json`.
   A configured `storage_dir` overrides this location.
3. Run the local registration script, substituting your ID and configuration:

```powershell
.\scripts\register-ai-browser-host.ps1 -ExtensionId YOUR_EXTENSION_ID -ConfigPath .\bin\ai-subscriptions\bridge.json -Browser Chrome
```

Use `-WhatIf` to preview the changes. The script writes the host configuration
and manifest beside `bin/jonsbo-ai-bridge.exe` and registers only that extension
under the current user's Chrome/Edge native-messaging key. Reload the extension,
then open a ChatGPT `/c/…` or Claude `/chat/…` conversation. Its popup reports the
local bridge status. If you move the release folder, change the server port or
load the extension with a different ID, register again.

The extension reads the visible title, selected model label when available and
generation/idle controls. It sends a 15-second heartbeat through the native
host. Closing a tab completes its observation; suspended tabs expire after 45
seconds. New chats are observed after they acquire a conversation URL. The host
reads a browser-only local credential: the extension receives no API master
token, provider OAuth token, cookies, private application state or chat bodies.
Visible titles may contain personal information; they are kept locally.

To remove pairing, disable/remove the extension and delete its
`com.jonsbo.subscription_display` key under
`HKCU\Software\Google\Chrome\NativeMessagingHosts` and/or
`HKCU\Software\Microsoft\Edge\NativeMessagingHosts`. Remove the adjacent host
manifest/configuration if no longer needed.

## What each number means

| Field | Coverage |
| --- | --- |
| Quota | Provider percentage and reset time for the reported window, commonly 5 hours/7 days. Missing windows stay `--`. |
| OpenAI reported tokens | Optional account daily buckets from a supporting app-server; not a sum of local observations. Provider date boundaries are identified separately from the local week. |
| Observed tokens | Deduplicated request counters from configured local feeds, labelled partial. Unverified session accounts remain local and are excluded from linked-account totals. |
| Current context | Native current-window data where available; separate from cumulative tokens and cleared when absent/compacted. |
| Browser activity | Visible generation/idle state. Browser token counts and exact context are unavailable. |
| Claude weekly account tokens | No supported account history feed is implemented. Native request counters can show local consumption. |

Claude status-line quota fields do not establish the originating account's
identity. They are therefore not attached to a linked account just because a
global native login exists. Fresh local quotas and unattributed Claude telemetry
can still appear with an explicit local/partial label. Hooks and telemetry remain
unattributed unless the feed supplies its own verified account identity and
subscription mode. Explicit API-mode observations never join subscription or
local widget totals; telemetry without an authentication mode is only a local
observation, not proof of subscription consumption.

Account identity and OpenAI usage polls run approximately every 60 seconds with
error backoff; Claude quota is fetched at most every three minutes. Account
identity data is stale after 180 seconds; Claude account quota after five minutes.
Widgets publish at one second. Local request
weeks start Monday in Europe/Budapest, including daylight-saving changes.
Retention is bounded to 35 days, 128 sessions and 200,000 deduplication keys;
capacity loss marks the data partial. Request IDs survive restarts to prevent
replay from inflating counts. Unknown data is `null`, never a fabricated zero.

## Storage, monitoring and API

The module is enabled by default. It detects native sign-in status without
starting login, a model turn, or quota collection for providers whose monitoring
is off. Disable it with `"ai-subscriptions":{"enabled":false}` in `modules`.
An optional `options.storage_dir` selects its local data directory.
Codex uses the existing native `CODEX_HOME`, or `~/.codex` when unset. Claude uses
the user's native login. Each native client owns credential storage and renewal.
Legacy isolated `codex-home` directories are left untouched; Jonsbo does not copy
their credentials. Keep the module data directory private; Windows inherits its
parent directory's ACL.

Stop monitoring detaches this observer and drops the linked account's cached data.
It does not sign out the shared native client or revoke provider consent.
Provider-side logout/revocation remains in the native client/account controls.

Master-authenticated routes: `GET /v1/ai/status`, `GET /v1/ai/setup`, and
`POST /v1/ai/providers/{openai|claude}/{check|monitor|connect|refresh|disconnect}`.
`check` detects installation and authentication without enabling monitoring;
`monitor` reuses an existing subscription login without starting OAuth.
The legacy `connect` action aliases `monitor` and never starts sign-in;
`disconnect` stops monitoring.
Metadata-only credentials authorize exact POST routes for browser events,
native hooks/status-line and OTLP logs. They cannot manage accounts or displays.
All routes retain loopback Host/Origin checks; native ingress rejects browser
Origin headers. Jonsbo does not copy Claude OAuth credentials, call private
subscription HTTP endpoints, or parse transcripts. Native clients retain their
own authentication and service requests.

See [official documentation research](ai-subscription-research.md) for source
links and capability/version limits. Live OAuth and your own native/browser
feeds must be verified after setup; synthetic tests do not establish provider
availability or quota access for your plan.
