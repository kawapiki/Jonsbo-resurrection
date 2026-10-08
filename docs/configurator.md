# Display configurator

The Go app serves an embedded local web editor. In tray mode choose **Open configurator**; it opens the local page with a credential in the URL fragment, which the page immediately removes. API calls remain authenticated, and no media or telemetry is sent to a cloud service. If you open the page manually, use your local API token to connect.

For source builds run `bin/jonsbo.exe serve --all`. Open `http://127.0.0.1:8787/`; CLI credentials default to `bin/api-token`. Tray credentials and saved compositions live beneath `%LOCALAPPDATA%/JonsboResurrection`. The editor is included from v0.4.0; AI widgets and the redesigned workspace are included in v0.5.0.

## Compose a screen

1. Start on **Your displays**: each detected screen has a card with its current layout, serial number and output status. Live hardware readings appear below. Choose a card to open that display in the editor. **Back to displays** returns to the overview; unsaved work can be resumed. **Design without a display** is an explicit offline option.
2. Open **Themes** to choose one of five presets, or expand **Saved layouts**. Pump canvases are 640×480; horizontal fan canvases are 640×180. Rotation handles physical mounting separately.
3. Open **Widgets** to add a live value, history graph, usage ring, usage bar, text, **ChatGPT / Codex** or **Claude**. Move and resize overlays on the canvas or enter precise coordinates in their properties. Edit labels, colors, font size, opacity and chart range; reorder or remove layers. AI connection controls appear inside the selected widget's settings.
4. Open **Background** to choose a color or import a picture/video. Preview renders the current draft without applying it to a physical display.
5. Choose **Save & apply** to save and assign the layout in one action. **Save layout** only saves it. Applied bindings and media survive application restarts. Editing and saving a layout already bound to screens updates those screens; choose **Duplicate** first if you want a separate design. Precise position and chart scale are available in expandable inspector sections.

![Display workspace showing separate pump, CPU, GPU and memory outputs](screenshots/display-overview.png)

## See the selected screen

**On display** shows the selected serial's current assigned renderer. **Edit layout**
opens its saved composition, or an editable starting layout matching its CPU,
GPU, memory or AI assignment. Selecting the three fans no longer opens the same
first fan design each time. Delayed image responses from previous assignments
are discarded. Physical displays hide the offline preview-format controls.

![Fan editor with CPU content and the selected temperature widget settings](screenshots/fan-editor.png)

## Add AI activity

Add the provider from **Widgets**, then select its layer. The **Subscription** row
detects the existing Codex or Claude Code sign-in. **Sign in / Reconnect** appears
only when needed; **Start monitoring** reuses a detected login. Monitoring and
optional session setup are under **Connection & activity options**. See the
[AI guide](ai-subscriptions.md) for native-client requirements and data coverage.

The animated blossom has white strokes on the dark screen, without a background
box. The Claude crab responds to working/waiting/idle activity. Both are drawn
from code. Full-width fan cards have larger quota, token and supporting text;
expanded session activity fits pump displays.

![Two AI widgets on a pump layout and the selected provider's connection settings](screenshots/ai-widgets.png)

*These screenshots use a temporary four-display demo with sample sensor readings,
subscription quotas and activity. They do not establish provider availability
or represent a user's account usage.*

## Real metrics and history

Metrics include CPU/GPU usage and temperature, dedicated GPU memory used, RAM usage/free space, and system-drive storage used/free space. Storage readings cover fixed local drives and are refreshed every ten seconds; the editor's default storage metric uses the system drive (the first collected fixed drive when the system drive is unavailable). Capacity respects the Windows user's disk quota.

Charts retain at most ten minutes of in-memory history while the server is running. Restarting clears history, not saved layouts. Missing or stale values show as unavailable and leave gaps; they are never replaced with made-up usage numbers. GPU metrics select the adapter with the most dedicated VRAM.

## Images and video

Images are resized in the browser before upload. The browser imports MP4/WebM files it can decode into a bounded sequence of JPEG frames, at up to **2 frames per second and 60 seconds per clip**. Unsupported codecs report an import error. Import has progress and cancellation. The saved frames loop in the Go renderer, so the browser can close after import completes.

These are dashboard-rate video backgrounds, not full-motion playback. Physical refresh remains configurable at 500 ms or slower; the default is one second. There is no audio playback and no external media decoder/FFmpeg installation. Rendering selects the frame for the current playback time instead of queuing stale frames.

Limits: 32 layouts, 32 overlays per layout, 32 media assets, 120 frames per asset, 24 MiB compressed data per asset, 128 MiB total media, and a 32 MiB upload request. Referenced assets cannot be deleted until layouts stop using them.

## API and storage

Authenticated editor routes start with `/v1/configurator`:

- `GET /v1/configurator?width=640&height=180` lists saved layouts, size-matched themes, assets, current metrics, bounded history and limits.
- `PUT /v1/configurator/layouts/{id}` saves a layout; `DELETE` removes an unused layout.
- `POST /v1/configurator/preview.png` renders a draft layout; `GET /v1/configurator/layouts/{id}/preview.png` renders a saved layout.
- `POST /v1/configurator/assets` imports `{name,fps,frames:[base64EncodedImage,...]}`; asset preview/delete routes use its generated ID.
- `POST /v1/configurator/apply` accepts `{serial,layout_id,rotation}` and persists the display binding.

Saved data is in `configurator/` beside the API token file. Media filenames are generated locally; uploaded names are labels, never filesystem paths. Back up this folder to preserve customizations. Treat compiled modules as trusted code; the configurator does not execute uploaded scripts or fetch remote background URLs.
