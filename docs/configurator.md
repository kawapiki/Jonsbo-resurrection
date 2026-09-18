# Display configurator

The Go app serves an embedded local web editor. In tray mode choose **Open configurator**; it opens the local page with a credential in the URL fragment, which the page immediately removes. API calls remain authenticated, and no media or telemetry is sent to a cloud service. If you open the page manually, use your local API token to connect.

For source builds run `bin/jonsbo.exe serve --all`. Open `http://127.0.0.1:8787/`; CLI credentials default to `bin/api-token`. Tray credentials and saved compositions live beneath `%LOCALAPPDATA%/JonsboResurrection`. The installed public v0.3.0 release predates this editor; use a build containing the configurator feature.

## Compose a screen

1. Select a connected pump or fan. Without connected hardware, use a virtual pump/fan canvas for previews.
2. Choose one of five presets or start editing a saved layout. Pump canvases are 640×480; horizontal fan canvases are 640×180. Rotation handles physical mounting separately.
3. Add metric values, history lines, usage pie charts, bars, or text. Move and resize overlays on the canvas or enter precise coordinates in their properties. Edit labels, colors, font size, opacity and chart range; reorder or remove layers.
4. Choose a background color or import a picture/video. Preview renders the current draft without applying it to a physical display.
5. Save the layout and apply it to the selected display. Applied bindings and media survive application restarts. Editing and saving a layout already bound to screens updates those screens; duplicate it first if you want a separate design.

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
