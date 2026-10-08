# Security

The local API binds to a literal loopback IP address and requires a bearer token.
It is intended for applications running on the same machine. Do not expose it
through a public listener, reverse proxy or tunnel. Requests have bounded bodies,
timeouts and concurrency limits; there is no permissive CORS policy.

The API accepts defined module events and display assignments. It does not accept
arbitrary executable code, shell commands, file paths or remote URLs. Text events
are data. Future event handlers must keep that boundary and validate their own
payload schema, sizes and allowed values.

Compiled-in modules are trusted code, not a sandbox. They run with the controller's
permissions and can access the host. Review their source and dependencies before
building or enabling them. The runtime requests cancellation but cannot forcibly
stop a blocked Go module. Go shared-object loading is not supported.

## Credentials and elevation

The controller creates its API token in `bin/api-token` by default. Keep that file
in a directory accessible only to the intended user. On Windows the parent
directory's inherited ACL controls access; Unix-style file modes do not establish
a Windows ACL. Anyone who obtains the token can use the local API while it is valid.
If a token is exposed, stop the API server, remove the compromised token file and
restart to generate a replacement. Update authorized clients afterwards.

Do not include tokens, account credentials or private event contents in logs,
screenshots, issues, patches or release archives. AI adapters must use documented
APIs or user-supplied exports; do not scrape account sessions. Keep upstream AI
credentials in the external adapter's protected configuration, never in module
events or display payloads.

Run the application without elevation unless an explicitly approved hardware
operation requires it. PawnIO CPU-temperature access requires approved driver
setup and elevation. Installing drivers or running an elevated controller expands
the impact of every compiled-in module; keep these steps opt-in and outside CI.
Do not bypass Windows security controls to make a sensor available. Missing
readings should remain unavailable.

## Reporting a concern

Do not put working tokens, personal device identifiers or exploitable private
details in a public issue. If the repository offers private vulnerability
reporting, use it. Otherwise ask the maintainer to establish a private reporting
channel before sharing sensitive details. No dedicated security contact or
response-time guarantee has been established yet.

Provide a minimal reproduction, affected version or commit, impact and a redacted
description of the environment. Do not test against another person's machine or
connected hardware without permission.

## Optional AI subscription monitoring

Subscription authentication is performed by native Codex/Claude Code. The
application does not read shared OAuth credential files or send model turns.
The native clients retain their own credential storage and renewal. CLI detection
reads only native authentication status; starting monitoring reuses a subscription
login. Jonsbo does not initiate provider sign-in; users sign in through their CLI.
Scoped metadata credentials, local session titles and bounded usage history should be kept in a
private user directory. Stopping monitoring detaches this observer; provider
logout/revocation is separate.

The optional Chromium companion reports visible interface metadata through a
native host paired to an exact extension ID. It has no cookies permission and
does not observe request bodies or chat text. The native host authorizes only
metadata ingress with a browser-only token, disables inherited proxies and
redirects, and enforces bounded message framing and a numeric loopback endpoint.
Native hooks/status-line/telemetry use a different scoped credential; controller
and display actions still require the master bearer token. Native ingress
rejects Origin headers, and all routes retain the existing Host checks.
