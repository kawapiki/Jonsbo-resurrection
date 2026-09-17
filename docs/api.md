# Local HTTP API

The application exposes a versioned API on an explicit loopback IP, such as `127.0.0.1:8787` or `[::1]:8787`. Wildcard addresses, remote addresses, and DNS names are rejected as listen addresses. The command owns the listener and token file; `internal/api.NewHandler` provides the handler for embedders.

Every request requires `Authorization: Bearer TOKEN`, including unknown routes. Use the generated token file from your `serve` command. Keep it private. Tokens must contain at least 32 characters. Tokens in query parameters do not authenticate requests. Responses disable caching, and the API does not enable CORS. The Host must be localhost or a literal loopback address; if an Origin header is present, it must equal `http://` plus the request Host exactly.

Start the server with `.\bin\jonsbo.exe serve --listen 127.0.0.1:8787 --token-file bin/api-token --example`. The default token path is `bin/api-token`. Without `--all`, the API and previews run without controlling USB displays. Add `--all` only when this process should own the displays. `--config configs/example.json` loads explicit configuration. The example module must be enabled with `--example` or configuration for the example event and preview below.

PowerShell examples (set the path to the token file used by your running server):

```powershell
$tokenFile = '.\bin\api-token'
$apiToken = (Get-Content -Raw -LiteralPath $tokenFile).Trim()
$headers = @{ Authorization = "Bearer $apiToken" }
$base = 'http://127.0.0.1:8787'
Invoke-RestMethod "$base/v1/modules" -Headers $headers
Invoke-RestMethod "$base/v1/state" -Headers $headers
Invoke-RestMethod "$base/v1/modules/example/events" -Method Post -Headers $headers -ContentType 'application/json' -Body '{"type":"message","payload":{"text":"Hello from an adapter"}}'
Invoke-WebRequest "$base/v1/modules/example/views/demo.png" -Headers $headers -OutFile '.\preview.png'
curl.exe -N -H "Authorization: Bearer $apiToken" "$base/v1/events"
```

Use module/view IDs and event payloads documented by the installed module. Discovery returns descriptors and their supported views. The examples above assume a module named `example` with a `demo` view; replace them with discovery results when using another module.

Equivalent curl example in a POSIX shell:

```sh
TOKEN_FILE=./bin/api-token
API_TOKEN=$(cat "$TOKEN_FILE")
curl -H "Authorization: Bearer $API_TOKEN" http://127.0.0.1:8787/v1/state
curl -N -H "Authorization: Bearer $API_TOKEN" http://127.0.0.1:8787/v1/events
```

| Method and route | Result or body |
| --- | --- |
| `GET /v1/modules` | Array of module descriptors: ID, name, version, description, views. |
| `GET /v1/state` | Array of states, each containing module descriptor, status, updated_at, data, and optional error. |
| `GET /v1/modules/{id}` | One module state; unknown module is 404. |
| `GET /v1/events` | SSE stream of `event: state` events; each JSON data value is a complete state array. Initial snapshot is sent immediately. |
| `GET /v1/modules/{id}/views/{view}.png` | PNG preview of the logical view, before physical display scaling/rotation. |
| `POST /v1/modules/{id}/events` | `{"type":"message","payload":{"text":"Hello"}}`; event types and payload semantics depend on the module. |
| `GET /v1/displays` | Array of display status objects including serial, kind, assignment, status, last_success, and optional error. Empty array when no display controller is attached. |
| `PUT /v1/displays/{serial}/assignment` | `{"module":"example","view":"demo","rotation":90}`. Rotation is 0, 90, 180, or 270 degrees clockwise. |

Successful event submissions and assignments return 204 without a body. Example assignment:

```powershell
$displaySerial = 'YOUR_DISPLAY_SERIAL'
Invoke-RestMethod "$base/v1/displays/$displaySerial/assignment" -Method Put -Headers $headers -ContentType 'application/json' -Body '{"module":"example","view":"demo","rotation":90}'
```

JSON request bodies reject unknown fields and extra JSON values. A request body is limited to 64 KiB plus a 1 KiB envelope; the event payload itself is limited to 64 KiB. Module/view IDs start with a lowercase letter and contain up to 64 lowercase letters, digits, underscores, or hyphens. Event types additionally permit dots. Display serials accept up to 256 ASCII letters, digits, underscores, or hyphens.

| Status | Meaning |
| --- | --- |
| 400 | Invalid JSON, ID, event, or assignment. |
| 401 | Missing or incorrect bearer token. |
| 403 | Disallowed Host or Origin. |
| 404 | Unknown route, module, view, or display. |
| 405 | Wrong HTTP method (`Allow` identifies the route's method), or unsupported module/display operation. |
| 413 | Request body exceeds the size limit. |
| 500 | Internal failure or invalid renderer output; details are not returned. |
| 503 | Module unavailable, cooperative request timeout, or request/render/SSE concurrency exhausted. |

Non-streaming requests receive a five-second context deadline and read/write deadlines. Modules are trusted native Go code compiled into the application: cancellation is cooperative, and a stalled native call cannot be forcibly stopped by the handler. The server command also configures HTTP connection/header limits. At most 32 non-streaming requests run concurrently, including event handlers; a slot remains occupied until the call returns even if a native module ignores cancellation. PNG output is restricted to two million pixels and two concurrent rendering requests. At most 32 SSE streams are allowed. Each SSE write has a five-second deadline, and idle streams receive comment heartbeats every ten seconds. Disconnects cancel the subscription. Consumers should replace their current state with each full snapshot; identical successive snapshots are valid, and intermediate updates may be coalesced. Reconnect to obtain a fresh snapshot; no event replay history is retained.

Posting an event from another process provides an adapter entry point. It does not automatically implement Claude or Codex integration, obtain account usage, run an agent, or scrape a session. An external adapter must independently obtain authorized data and translate it to events understood by a module. The API accepts no arbitrary shell commands, file paths, or remote URLs as built-in operations.
