# Subscription display integration research

Researched: 2026-10-07. Scope: personal ChatGPT/Codex and Claude subscriptions, including local coding sessions and browser conversations. The initial sections are feasibility research; follow-ups record implementation checks. Linked primary documentation was opened; search snippets were not treated as authoritative.

Implementation check, 2026-10-08: `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
also blocks the native usage request. Its structured handler can still return a
successful response with empty limits, or a cached snapshot, which is insufficient
evidence of a fresh account read. Removing that broad flag while keeping the
individual telemetry/error/updater disables produced a native HTTP 200 and a
fresh quota snapshot, with behavior summaries excluded. Jonsbo uses these
individual controls and sends no model prompt.
The structured reply omits the native cache's provenance and fetch timestamp;
the collector records observation time, rather than claiming server-fetch time.

Cold-start check, 2026-10-08: native Codex authentication can take longer than
one second after the collector starts. Quota and daily usage observations must
be timestamped after their native replies arrive. Timestamping them before
authentication can put a successful quota read behind the newer account
observation, causing the intake's ordering guard to retain an older cached quota.
A regression exercises a delayed native reply with the real aggregation module.

Implementation follow-up, 2026-10-08: the installed Claude Code 2.1.293 supports a
native headless `get_usage` control request, with `skip_behaviors:true`. A bounded
probe returned Team five-hour and seven-day quota without any user/model message,
initialization request, or transcript behavior scan. Jonsbo uses this native
surface in the background, leaving OAuth renewal with Claude Code. The structured
control interface is experimental; its schema and minimum compatible version are
checked. [Claude's command reference](https://code.claude.com/docs/en/commands)
documents `/usage` for session cost, plan limits, and activity stats, including
Pro, Max, Team, and Enterprise. A newly created metadata reader has no historical
session-token consumption; its empty session counters must not become an account
token total. OpenAI's native `account/usage/read` was also verified returning real
daily token buckets while no interactive Codex terminal was open.

## Findings

| Requested feature | OpenAI / Codex | Claude |
| --- | --- | --- |
| Subscription sign-in | Codex app-server provides a managed ChatGPT browser/device OAuth flow | Native Claude Code provides subscription OAuth; `claude auth login --claudeai` is available locally |
| Short and weekly quota | Account rate-limit windows, when returned | Native headless usage windows; local status-line fallback |
| Tokens consumed | Account daily buckets when available; observed session telemetry | Observed Claude Code telemetry; no verified personal account-wide token-history interface |
| Running coding sessions | Lifecycle hooks; app-server events for sessions in that server | Lifecycle hooks, including local Claude Code Desktop sessions |
| Coding-session context | App-server token/context updates; standalone telemetry coverage requires validation | Status-line context fields |
| Existing browser chats via OAuth | OAuth permission does not grant existing ChatGPT conversation access | No documented personal subscription OAuth interface for live browser-chat/context discovery was established |
| Money actually charged | No billing ledger established for this module | Session cost is an estimate and does not represent subscription billing |

### OpenAI account connection

[Codex app-server documentation](https://learn.chatgpt.com/docs/app-server) documents managed account login, account reads, quota windows, account token activity, and thread notifications. Daily usage buckets can be absent. A server's loaded-thread list concerns that server's memory; a new helper is not a live observer of every existing desktop/CLI process. Account reporting must retain the source's coverage rather than claim all consumer ChatGPT activity.

[Subscription authentication](https://learn.chatgpt.com/docs/auth) distinguishes ChatGPT subscription access from API-key billing.

[Sign in with ChatGPT](https://learn.chatgpt.com/docs/sign-in-with-chatgpt) explicitly separates identity from permission to use a plan and states that plan permission does not expose existing ChatGPT conversations or memories. Thus OAuth alone cannot fulfill browser-chat monitoring.

[Open-source registration](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) documents a direct OAuth registration flow with PKCE, state, nonce, a stable host identifier, and an issued client ID. This differs from the [commercial client-ID request](https://developers.openai.com/siwc/request-client-id), which currently has partner restrictions. Direct sign-in is a possible future integration, but access to a monitoring-only personal usage feed through that flow has not been established. Prefer the documented Codex-managed account surface initially.

### OpenAI local activity

[Advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced) describes opt-in OpenTelemetry, conversation metadata, completed-response token counts, and per-turn token metrics. [Configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference) documents HTTP JSON export and exporter headers. These are local telemetry exports, not API billing requests.

[Codex hooks](https://learn.chatgpt.com/docs/hooks) provide session/turn lifecycle events and identifiers. Hook definitions require native review/trust. The transcript format is explicitly unstable; hooks should forward selected metadata without reading transcripts. Hooks alone do not document a context-token gauge.

### Claude connection and local activity

[Claude Code authentication](https://code.claude.com/docs/en/authentication) documents subscription OAuth separately from Console/provider credentials. The installed CLI exposes subscription login and JSON authentication-status commands. Login should be handled by native Claude Code; the display module does not need its OAuth tokens.

[Status-line documentation](https://code.claude.com/docs/en/statusline) exposes session identifiers, models, context usage/size, and optional five-hour/seven-day quota percentages/reset times. Subscription windows are documented for Pro/Max and appear after a response; availability differs by account/version. Context fields can be null around startup or compaction. These context totals describe the current window and must not become cumulative weekly consumption.

[Monitoring documentation](https://code.claude.com/docs/en/monitoring-usage) exposes request token categories, session identity, request IDs, and opt-in telemetry. Input/cache categories must be normalized without double counting. Request IDs support deduplication; event sequence alone is unsafe across resumed processes. Prompt/response content export can stay disabled.

[Claude hooks](https://code.claude.com/docs/en/hooks) provide lifecycle and permission events. [Claude Code Desktop](https://code.claude.com/docs/en/desktop) documents shared local hooks/settings with the CLI. Ordinary Claude desktop chat is a separate surface; do not assume Code hooks cover it.

[Claude cost guidance](https://code.claude.com/docs/en/costs) distinguishes plan usage from locally estimated API-equivalent session costs. Actual extra-usage charges and subscription invoices require their own authoritative source.

### Claude third-party OAuth scope

[Claude account sign-in guidance](https://support.claude.com/en/articles/13189465-log-in-to-your-claude-account) describes subscription credentials for native applications and restrictions on third-party use. It does not supply an unrestricted personal usage-monitor OAuth API. Passive native telemetry avoids impersonating Claude Code or replaying its credentials against private endpoints.

The live [Agent SDK subscription article](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan) was updated on 2026-10-07: it now points to monthly API credits claimed into Console and used with API keys. Earlier search snippets described a paused June change. Those snippets are stale. API credits are outside this subscription-monitor scope.

## Local compatibility evidence

Read-only command checks found Codex `0.162.0-alpha.2` and Claude Code `2.1.289`. Codex's installed schema was generated into ignored `.cache/ai-subscription-research/codex-schema/`; no account requests, login, model inference, or chat-content reads were performed.

The generated stable schema includes account token activity and rate limits, thread status updates, and token notifications containing `last`, `total`, and nullable `modelContextWindow`. Installed fields do not establish actual service availability or global session visibility. Native Claude help confirms the `--claudeai` login selector and JSON status output.

## Recommendation

Use native OAuth/account connections for subscription identity, native hooks/telemetry for observed coding activity, and an optional companion for visible browser metadata. Keep exact quota, observed consumption, current context, and estimated costs as separate quantities. Label unsupported or stale fields explicitly. A detailed proposed module contract is in [the design draft](superpowers/specs/2026-10-07-ai-subscriptions-design.md).
