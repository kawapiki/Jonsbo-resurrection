# Display configurator implementation

User authorized an editable connected-display configurator, image/video backgrounds, at least five preset themes, history/pie/usage charts, and free-space overlays. Use Go-hosted static local web UI, no cloud service. All code goes through codex/display-configurator PR; owner alone merges, no release tag or master push.

## Contract shared by frontend/backend

- GET /v1/configurator -> {layouts:[],themes:[],assets:[],metrics:[],history:[],limits:{...}}. State includes no secrets. Existing /v1/displays lists physical screens; API-only mode supports virtual preview canvas.
- Layout JSON: {id,name,width,height,background:{color,asset_id,opacity},overlays:[{id,type,metric,label,x,y,w,h,color,font_size,min,max,opacity}],rotation}. IDs lower-case alnum/dashes starting letter. width,height only640x480 or640x180. types metric,line,pie,bar,text; metrics cpu.usage,cpu.temp,gpu.usage,gpu.temp,gpu.vram,ram.usage,ram.free,disk.used,disk.free. Positions/pixel sizes bounded tocanvas; color #RRGGBB; opacity0..1; metric min/max range, textuseslabel.
- Theme JSON: {id,name,description,layout} five genuinely different preset palettes/compositions. Themes logical640x480; frontend adapts y/h tofanheight (orbackendthemesforsizes endpointquerywidth/height).
- GET /v1/configurator?width=640&height=180 returns themes matching requestedsize.
- PUT /v1/configurator/layouts/{id} strictLayout -> savedLayout; DELETE same ->204. GET /v1/configurator/layouts/{id}/preview.png savedrender; POST /v1/configurator/preview.png bodyLayout ->PNG without persistence.
- POST /v1/configurator/assets {name,fps,frames:[base64 encoded PNG/JPEG]} -> asset {id,name,kind,width,height,frame_count,fps}. fps1..2, <=120frames, <=32MiB HTTPbody, <=24MiBdecoded compressed aggregate, <=640x480image dimensions. Staticpicture browserresizesfirst. Video browserdecoder imports<=60seconds at2fps, muted loop, progress/cancel; storesframes server-side forheadlessplayback. Noffmpeg/networkdecoderdependency. GET /v1/configurator/assets/{id}/preview.png firstframe. DELETE asset rejects referencedassets.
- POST /v1/configurator/apply {serial,layout_id,rotation} persistsbinding and calls genericoutput assignment Module=configurator View=<layout_id>. Savedlayoutsloadbeforeengine descriptor registration; runtime descriptor mustsupportnewlayoutids via predeclared perdisplay views OR source routing facade ValidateView/Frame configuredlayouts. Rootwill implement sourcefacade (notdynamicengine) invoking configurator directly forviewIDs. Configurator implements Module (descriptorviews defaultpump/fan only), Run collects sharedhardware snapshots and bounded10minhistory, Renderer directRender(id). Root facadehandlesdynamiclayout routing.
- Module Backend Go contract: New(dir string, source interface{Snapshot()[]module.State}) (*Module,error); Handler(displays interface{Displays()[]module.Display;Assign(string,module.Assignment)error}) http.Handler (receives already-authenticated prefixrequests); Frame(ctx,id) (image.Image,error); ValidateView(id) error; Bindings()map[string]module.Assignment. DefaultLayout presets available nohardwarefabrication. Module.Render callsFrame. Root registersmodule withruntime separatelyafternewRuntime; nosecondhardwarecollector.
- Metrics response objects {id,label,unit,value,max}; history []{time,values:map[string]*float64}. Missing/stalesensors null,gapsinlines. gpu uses primarylargestVRAM GPU. disk defaultsystemvolume (collectfixedvolumes, fieldvolume optional future).
- Persist layouts, assetmetadata and bindings under dirname(tokenfile)/configurator using boundedfiles, atomicreplace; referencenamesneverfilesystempaths. Diskquota<=128MiB totalmedia and max32assets/32layouts,32overlays/layout. No appuserdata overwritten byrebuild. Clear errors validation/read/write limits.

## Authentication/UI

Serve embedded /, /app.js,/style.css,/signature.png publicly afterloopbackHost/Originchecks; no secret/serverstate inHTML. Existing Bearer API retained. Trayopens http://127.0.0.1:8787/#token=<urlencodedtoken>; JS consumes hash into sessionStorage then history.replaceState clearsaddress (noquerytokens/referrer). CSP self/blob only withnoexternal scripts/images/frames; authenticatedfetchforPNG convertsblobURL. Manualtokenpastefallback. No unauthenticatedmutations.

## Tasks

- [x] Backend configurator,persistence,renderer,presets,assets/history API tests.
- [x] Embedded web editor,devicepicker,themes,drag/resizeoverlaypropertypanel,image/videoimport,preview/apply,empty/error/accessibility states.
- [x] Fixed-volume storage telemetry and null/error/unit tests.
- [x] API/static/auth glue, runtime/displayfacade,savedbindingrestore,trayOpenconfigurator,docs.
- [x] Go tests/vet/build, browser flows, exact render review, independent code review and feature PR.
- [ ] Required CI on final revision and physical USB application check. Elevation was canceled; the installed v0.3.0 controller remains active.

Ruling: use a localwebsite forlayoutcompositionandnativebrowsermediadecode. Hardwareupdates remain500msminimum; videoisexplicitlyresampled2fpsmax60sec, notfull-motionUSB. Browsermaycloseafterimport. Userexplicitlyauthorizesimplementation, so no repeateddesignapprovalgate. Subagent-driven-development skill directsboundedparallel implementation. Existinginstalled0.3.0 staysavailable untiltestedlocalpreviewreplacement.
