/* The editor has no network dependencies. These pure helpers are also exercised
 * by Node's built-in test runner; the browser entry point is at the bottom. */
"use strict";
const clamp = (n, lo, hi) => Math.min(hi, Math.max(lo, Number.isFinite(Number(n)) ? Number(n) : lo));
const clone = value => JSON.parse(JSON.stringify(value));
function constrainLayer(layer, width, height) {
  layer.w = Math.round(clamp(layer.w, 1, width));
  layer.h = Math.round(clamp(layer.h, 1, height));
  layer.x = Math.round(clamp(layer.x, 0, width - layer.w));
  layer.y = Math.round(clamp(layer.y, 0, height - layer.h));
  return layer;
}
function adaptLayout(layout, height) {
  const next = clone(layout), scale = height / layout.height;
  next.height = height; next.width = 640;
  // A size change creates a separate layout: an existing binding still expects
  // its original format, and built-in default IDs have fixed dimensions.
  if (height !== layout.height && layout.id) next.id = "layout-" + (globalThis.crypto?.randomUUID?.() || Date.now().toString(36) + Math.random().toString(36).slice(2)).slice(0, 20);
  next.overlays = (next.overlays || []).map(layer => constrainLayer({...layer, y: Math.round(layer.y * scale), h: Math.max(1, Math.round(layer.h * scale)), font_size: Math.round(clamp(layer.font_size * Math.min(1, scale), 8, 96))}, 640, height));
  return next;
}
function coverRect(sourceWidth, sourceHeight, width, height) {
  const scale = Math.max(width / sourceWidth, height / sourceHeight);
  const w = sourceWidth * scale, h = sourceHeight * scale;
  return {x: (width - w) / 2, y: (height - h) / 2, w, h};
}
function orientLayout(layout, assignment) {
  const next = clone(layout);
  next.rotation = [0, 90, 180, 270].includes(assignment?.rotation) ? assignment.rotation : layout.height === 180 ? 90 : 0;
  return next;
}
function metricValue(metrics, id) {
  const metric = metrics.find(item => item.id === id);
  return metric && typeof metric.value === "number" && Number.isFinite(metric.value) ? metric.value : null;
}
function mediaPlan(width, height, duration) {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) throw new Error("Media has no readable dimensions.");
  const scale = Math.min(1, 640 / width, 480 / height);
  if (duration !== undefined && (!Number.isFinite(duration) || duration <= 0)) throw new Error("Video must have a finite, readable duration.");
  return {width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)), frames: duration === undefined ? 1 : Math.min(120, Math.max(1, Math.ceil(Math.min(60, duration) * 2))), fps: duration === undefined ? 1 : 2};
}
function readToken(location, history, storage) {
  const params = new URLSearchParams(location.hash.slice(1));
  const incoming = params.get("token");
  // Clear the fragment before any request or work with external resources.
  if (location.hash) history.replaceState(null, "", location.pathname + location.search);
  if (incoming) { try { storage.setItem("jonsbo-token", incoming); } catch (_) {} return incoming; }
  try { return storage.getItem("jonsbo-token") || ""; } catch (_) { return ""; }
}
async function saveThenApply(save, isCurrent, apply) {
  await save();
  if (!isCurrent()) throw new Error("The layout or selected display changed while saving. Review it and apply again.");
  await apply();
}
function layoutForDisplay(device, layouts, bindings, newID) {
  const assignment=device.assignment || bindings[device.serial];
  const bound=assignment?.module==='configurator' && layouts.find(l=>l.id===assignment.view);
  if(bound)return orientLayout(bound,assignment);
  const base=layouts.find(l=>l.id===(device.kind==='fan'?'default-fan':'default-pump'));
  if(!base)throw new Error('Default layout unavailable. Reconnect to the service.');
  return orientLayout({...clone(base),id:newID,name:(device.kind==='fan'?'Fan':'Pump')+' '+device.serial.slice(-4)+' custom'},assignment);
}
if (typeof module !== "undefined" && module.exports) module.exports = {clamp, constrainLayer, adaptLayout, coverRect, orientLayout, metricValue, mediaPlan, readToken, saveThenApply, layoutForDisplay};

function startStudio() {
  const $ = id => document.getElementById(id);
  const tokenStorage = (() => { try { return window.sessionStorage; } catch (_) { return {getItem:()=>"",setItem:()=>{},removeItem:()=>{}}; } })();
  let token = readToken(location, history, tokenStorage);
  const state = {layouts: [], themes: [], themeCache: {}, themeHeight: 0, themeRequestHeight: 0, assets: [], metrics: [], history: [], devices: [], bindings: {}, draft: null, selected: "", serial: "", height: 480, dirty: false, revision: 0, connected: false, initialized: false, pollTimer: null, polling: false, previewing: false, importing: null, background: null, backgroundID: "", backgroundPending: "", renderURL: "", epoch: 0};
  const canvas = $("canvas"), ctx = canvas.getContext("2d");
  const well=document.querySelector('.canvas-well'), frame=document.querySelector('.canvas-frame');
  function fitCanvas(){frame.style.width=Math.max(40,Math.min(656,well.clientWidth-32,(well.clientHeight-32)*canvas.width/canvas.height))+'px';}
  new ResizeObserver(fitCanvas).observe(well);
  const id = prefix => prefix + "-" + (globalThis.crypto?.randomUUID?.() || Date.now().toString(36) + Math.random().toString(36).slice(2)).slice(0, 20);
  const currentAssignment = () => state.devices.find(device => device.serial === state.serial)?.assignment || state.bindings[state.serial];
  const blank = height => orientLayout({id:id("layout"), name:height === 180 ? "My fan display" : "My pump display", width:640, height, background:{color:"#121a26", asset_id:"", opacity:1}, overlays:[], rotation:0}, currentAssignment());
  const selected = () => state.draft?.overlays.find(layer => layer.id === state.selected);
  let applying = false;
  let page='home', deviceSignature='', previewBusy=false;
  const deviceImages=new Map();
  function displayName(device){return (device.kind==='fan'?'Fan':'Pump')+' · '+device.serial.slice(-6);}
  function showPage(next){page=next;document.body.dataset.page=next;requestAnimationFrame(fitCanvas);updateActions();}
  $('back-displays').addEventListener('click',()=>{showPage('home');renderDevices();refreshDisplayPreviews();});
  $('resume-design').addEventListener('click',()=>{showPage('editor');renderDevices();});
  $('design-offline').addEventListener('click',()=>{if(!state.serial&&state.dirty){showPage('editor');return;}if(!allowReplace())return;state.serial='';setDraft(blank(480));showPage('editor');renderDevices();});
  function showLibrary(name) {
    document.querySelectorAll('[data-library]').forEach(button=>button.setAttribute('aria-pressed',String(button.dataset.library===name)));
    document.querySelectorAll('[data-library-panel]').forEach(panel=>panel.hidden=panel.dataset.libraryPanel!==name);
  }
  document.querySelectorAll('[data-library]').forEach(button=>button.addEventListener('click',()=>showLibrary(button.dataset.library)));
  document.querySelectorAll('[data-widget]').forEach(button=>button.addEventListener('click',()=>{$('add-type').value=button.dataset.widget;$('add-layer').click();}));
  $('browse-widgets').addEventListener('click',()=>showLibrary('widgets'));
  $('duplicate-layout').addEventListener('click',()=>{const copy=clone(state.draft);copy.id=id('layout');copy.name+=' copy';setDraft(copy,true);status('Independent copy created. Your original layout is unchanged.');});
  const status = (message, error = false) => { $("status").textContent = message; $("status").classList.toggle("error", error); };
  function connected(yes) {
    state.connected = yes; $("connection").textContent = yes ? "Local service connected" : token ? "Local service disconnected" : "Connection needed";
    $("connection-dot").className = "dot " + (yes ? "live" : "error"); updateActions();
  }
  async function request(path, options = {}) {
    const controller = new AbortController(), external = options.signal;
    const abort = () => controller.abort();
    if (external?.aborted) controller.abort(); else external?.addEventListener("abort", abort, {once:true});
    const timer = setTimeout(abort, options.timeout || 20000);
    try {
      const headers = {...options.headers, Authorization:"Bearer " + token};
      if (options.body !== undefined) headers["Content-Type"] = "application/json";
      const response = await fetch(path, {...options, headers, signal:controller.signal, cache:"no-store", credentials:"omit", referrerPolicy:"no-referrer"});
      if (!response.ok) {
        if (response.status === 401) { connected(false); $("auth").hidden = false; throw new Error("Token not accepted. Open from the tray or enter the current API token."); }
        const message = (await response.text()).slice(0, 300);
        throw new Error(message.trim() || "Request failed (" + response.status + ").");
      }
      if (options.blob) return await response.blob();
      return response.status === 204 ? null : await response.json();
    } catch (error) {
      if (error.name === "AbortError") throw new Error(external?.aborted ? "Import cancelled." : "The local service timed out. Try again.");
      throw error;
    } finally { clearTimeout(timer); external?.removeEventListener("abort", abort); }
  }
  function updateActions() {
    const device = state.devices.find(item => item.serial === state.serial);
    $("apply-button").disabled = applying || !state.connected || !device || !state.draft;
    $("apply-button").textContent = applying ? "Applying…" : "Save & apply";
    $("apply-button").title = !device ? "Select a connected display first" : "Save and send this layout to " + device.serial;
    $("save-button").disabled = page!=='editor' || applying || !state.connected || !state.draft;
    $("preview-button").disabled = !state.connected || !state.draft || state.previewing;
    $("draft-state").textContent = state.dirty ? "Unsaved changes" : state.layouts.some(item => item.id === state.draft?.id) ? "Saved" : "New layout";
    $('resume-design').hidden=!state.dirty;
  }
  function changed() { state.dirty = true; state.revision++; $("render-state").textContent = "Draft changed"; updateActions(); draw(); }
  function allowReplace() { return !state.dirty || window.confirm("Discard the unsaved changes in this layout?"); }
  function setDraft(layout, dirty = false) {
    document.body.dataset.format=layout.height===180?'fan':'pump';
    state.draft = clone(layout); state.height = layout.height; state.selected = ""; state.dirty = dirty; state.revision++;
    canvas.width = layout.width; canvas.height = layout.height;
    requestAnimationFrame(fitCanvas);
    $("layout-name").value = layout.name; $("rotation").value = String(layout.rotation || 0);
    $("background-color").value = layout.background.color; $("background-opacity").value = layout.background.opacity;
    $("canvas-size").textContent = layout.width + " × " + layout.height + " px";
    document.querySelectorAll("[data-height]").forEach(button=>button.classList.toggle("active", Number(button.dataset.height)===state.height));
    $("saved-layout").value = state.layouts.some(item => item.id === layout.id) ? layout.id : "";
    $("render-state").textContent = "Not rendered"; $("render-image").hidden = true;
    renderAssets(); renderLayers(); renderProperties(); updateActions(); loadBackground(); draw(); refreshThemes();
  }
  function element(tag, className, text) { const node=document.createElement(tag); if(className)node.className=className; if(text!==undefined)node.textContent=text; return node; }
  function renderDevices() {
    const list = $("devices"); list.replaceChildren(); $("device-count").textContent = state.devices.length;
    $("device-note").textContent = state.serial ? "Editing " + (state.devices.find(d=>d.serial===state.serial)?.kind || "display") + " · " + state.serial : "Preview workspace. Choose a connected display above to send your design.";
    const selectedDevice=state.devices.find(d=>d.serial===state.serial);
    $('selected-display').textContent=selectedDevice?displayName(selectedDevice)+' — '+selectedDevice.serial:'Offline layout preview';
    $('overview-count').textContent=state.connected?state.devices.length+' detected · '+state.devices.filter(d=>d.status==='live').length+' live':'Display service disconnected';
    $('overview-empty').hidden=state.connected&&state.devices.length>0;
    $('overview-empty').textContent=!state.connected?'Connect to the local display service to detect screens and read hardware values.':'No displays are exposed by this service. Start the Windows app with display monitoring enabled, then reconnect. Offline layout design is available below.';
    const signature=JSON.stringify(state.devices.map(d=>[d.serial,d.kind,d.assignment,d.status,d.error,state.connected]));
    if(signature!==deviceSignature){
      deviceSignature=signature;
      const cards=$('display-cards');cards.replaceChildren();
      const sorted=[...state.devices].sort((a,b)=>(a.kind==='pump'?-1:1)-(b.kind==='pump'?-1:1)||a.serial.localeCompare(b.serial));
      for(const device of sorted){
        const card=element('button','display-card');card.disabled=!state.connected;
        card.setAttribute('aria-label','Edit '+displayName(device));
        const screen=element('div','display-card-screen '+device.kind),img=element('img');img.alt='Current layout on '+displayName(device);img.dataset.displaySerial=device.serial;
        if(deviceImages.has(device.serial))img.src=deviceImages.get(device.serial);else img.hidden=true;
        screen.append(img,element('span','preview-label','Current display output'));
        card.append(screen,element('strong','',displayName(device)),element('span','card-serial',device.serial),element('span','card-status',state.connected?(device.error || device.status || 'Detected'):'Service disconnected'),element('span','card-assignment','Showing '+(device.assignment?.module || 'unassigned')+' / '+(device.assignment?.view || 'none')),element('span','card-edit','Edit display'));
        card.addEventListener('click',()=>selectDevice(device));cards.append(card);
      }
      for(const [serial,url] of deviceImages){if(!state.devices.some(d=>d.serial===serial)){URL.revokeObjectURL(url);deviceImages.delete(serial);}}
    }
    for (const device of state.devices) {
      const button=element("button", "device" + (state.serial===device.serial?" active":""));
      const icon=element("span", "screen-icon " + (device.kind==="fan"?"fan":"pump"));
      const info=element("span"); info.append(element("strong", "", displayName(device)),element("small", "", device.serial),element("small", "", device.status || "Connected")); button.append(icon, info);
      button.setAttribute('aria-pressed',String(state.serial===device.serial));
      button.addEventListener("click",()=>selectDevice(device)); list.append(button);
    }
    updateActions();
  }
  function selectDevice(device) {
    if(state.serial===device.serial&&state.draft){showPage('editor');renderDevices();return;}
    if(!allowReplace())return;
    const layout=layoutForDisplay(device,state.layouts,state.bindings,id('layout'));
    state.serial = device.serial;
    setDraft(layout);showPage('editor');
    renderDevices(); renderThemes(); status(device.assignment?.module==='configurator'?'Editing the saved layout for '+displayName(device)+'.':'Currently showing the '+(device.assignment?.module || 'default')+' dashboard. Customize this draft, then Save & apply to replace it on '+displayName(device)+'.');
  }
  async function refreshDisplayPreviews(){
    if(previewBusy||page!=='home'||!state.connected)return;previewBusy=true;const epoch=state.epoch;
    try{for(const device of state.devices){
      if(page!=='home'||epoch!==state.epoch)break;
      const a=device.assignment;if(!a?.module||!a?.view)continue;
      try{const blob=await request('/v1/modules/'+encodeURIComponent(a.module)+'/views/'+encodeURIComponent(a.view)+'.png',{blob:true});
        if(epoch!==state.epoch||!state.connected)break;
        const img=[...document.querySelectorAll('[data-display-serial]')].find(el=>el.dataset.displaySerial===device.serial);
        if(img){const url=URL.createObjectURL(blob),old=deviceImages.get(device.serial);deviceImages.set(device.serial,url);img.src=url;img.hidden=false;if(old)URL.revokeObjectURL(old);}
      }catch(_){/* Keep device status visible even if its current renderer is unavailable. */}
    }}finally{previewBusy=false;}
  }
  function renderSaved() {
    const select=$("saved-layout"); select.replaceChildren(new Option("Choose a layout…", ""));
    state.layouts.forEach(layout=>select.add(new Option(layout.name + " · " + layout.width + " × " + layout.height, layout.id)));
    select.value=state.draft?.id || "";
  }
  async function refreshThemes() {
    const height=state.height;
    if(state.themeCache[height]){state.themes=state.themeCache[height];state.themeHeight=height;renderThemes();return;}
    state.themes=[];state.themeHeight=0;renderThemes();
    if(!state.connected||state.themeRequestHeight===height)return;
    state.themeRequestHeight=height;
    try{const data=await request("/v1/configurator?width=640&height="+height);state.themeCache[height]=data.themes||[];if(state.height===height){state.themes=state.themeCache[height];state.themeHeight=height;renderThemes();}}
    catch(error){status("Could not load themes for this format: "+error.message,true);}
    finally{if(state.themeRequestHeight===height)state.themeRequestHeight=0;}
  }
  function renderThemes() {
    const list=$("themes"); list.replaceChildren();
    for (const theme of state.themes) {
      const button=element("button","theme"), thumbnail=element("canvas"); thumbnail.width=640; thumbnail.height=state.height; thumbnail.setAttribute("aria-hidden","true");
      const layout=adaptLayout(theme.layout,state.height); paint(thumbnail.getContext("2d"), layout, false);
      button.append(thumbnail,element("strong","",theme.name),element("small","",theme.description));
      button.addEventListener("click",()=>{if(!allowReplace())return;layout.id=id("layout");layout.name=theme.name+" custom";setDraft(orientLayout(layout,{rotation:state.draft.rotation}),true);status("Theme loaded. Select any layer to change its reading, position or style.");}); list.append(button);
    }
  }
  function renderAssets() {
    const select=$("background-asset"); select.replaceChildren(new Option("No media", ""));
    state.assets.forEach(asset=>select.add(new Option(asset.name + (asset.frame_count>1?" · video":""),asset.id)));
    select.value=state.draft?.background.asset_id || "";
  }
  async function loadBackground() {
    const assetID=state.draft?.background.asset_id || "";
    if (assetID===state.backgroundID || assetID===state.backgroundPending) return;
    state.background=null; state.backgroundID=""; state.backgroundPending=assetID;
    if (!assetID) { draw(); return; }
    let url="";
    try {
      const blob=await request("/v1/configurator/assets/"+encodeURIComponent(assetID)+"/preview.png",{blob:true});
      url=URL.createObjectURL(blob);const img=new Image();img.src=url;await img.decode();
      if(state.draft.background.asset_id===assetID){state.background=img;state.backgroundID=assetID;draw();}
    } catch(error) {if(state.draft.background.asset_id===assetID)status("Background preview: "+error.message,true);}
    finally {if(url)URL.revokeObjectURL(url);if(state.backgroundPending===assetID)state.backgroundPending="";}
  }
  function renderMetrics() {
    const list=$("metrics"); list.replaceChildren();
    for (const metric of state.metrics) {
      const item=element("div","metric"), value=metricValue(state.metrics,metric.id);
      item.append(element("small","",metric.label),element("strong","",value===null?"—":Number(value.toFixed(1)).toString()),element("span","",metric.unit));list.append(item);
    }
    $("reading-time").textContent=state.metrics.some(item=>metricValue(state.metrics,item.id)!==null)?"Updated "+new Date().toLocaleTimeString():"Sensors unavailable";
    $('overview-metrics').replaceChildren(...[...list.children].map(node=>node.cloneNode(true)));
    $('overview-reading-time').textContent=$('reading-time').textContent;
    const select=$("prop-metric"), previous=select.value; select.replaceChildren(); state.metrics.forEach(metric=>select.add(new Option(metric.label+" ("+metric.unit+")",metric.id))); select.value=selected()?.metric || previous;
  }
  function renderLayers() {
    const list=$("layers");list.replaceChildren();$("layer-count").textContent=state.draft.overlays.length+" layers";
    for (const layer of [...state.draft.overlays].reverse()) {
      const button=element("button","layer"+(state.selected===layer.id?" active":"")),swatch=element("span","swatch");swatch.style.backgroundColor=layer.color;
      button.append(swatch,element("span","",layer.label||layer.type));button.setAttribute("aria-pressed",String(state.selected===layer.id));
      button.addEventListener("click",()=>selectLayer(layer.id));list.append(button);
    }
    if(!state.draft.overlays.length)list.append(element("p","hint","Add a reading, text or chart. Drag it anywhere on the display."));
    $("add-layer").disabled=state.draft.overlays.length>=32;
    document.querySelectorAll('[data-widget]').forEach(button=>button.disabled=state.draft.overlays.length>=32);
  }
  function selectLayer(layerID) {state.selected=layerID;renderLayers();renderProperties();draw();}
  const propNames=["label","metric","x","y","w","h","font_size","color","min","max","opacity"];
  function renderProperties() {
    const layer=selected();$("properties").hidden=!layer;$("layer-tools").hidden=!layer;
    $('inspector-empty').hidden=!!layer;
    $("selection-note").textContent=layer?"Drag to move. Bottom-right handle to resize. Arrow keys to nudge.":"Select a layer to move or resize it.";
    if(!layer)return;
    $('inspector-title').textContent=({metric:'Live value',line:'History graph',pie:'Usage ring',bar:'Usage bar',text:'Text'})[layer.type] || 'Widget settings';
    $('range-fields').hidden=!['line','pie','bar'].includes(layer.type);
    for(const key of propNames)$("prop-"+key).value=layer[key] ?? "";
    $("metric-field").hidden=layer.type==="text";
    $("prop-x").max=state.draft.width-layer.w;$("prop-y").max=state.draft.height-layer.h;$("prop-w").max=state.draft.width;$("prop-h").max=state.draft.height;
    const index=state.draft.overlays.indexOf(layer);$("layer-down").disabled=index===0;$("layer-up").disabled=index===state.draft.overlays.length-1;
  }
  function paint(context, layout, interactive) {
    context.save();context.globalAlpha=1;context.fillStyle=layout.background.color;context.fillRect(0,0,layout.width,layout.height);
    if(interactive && state.background && layout.background.asset_id===state.backgroundID) {const rect=coverRect(state.background.naturalWidth,state.background.naturalHeight,layout.width,layout.height);context.globalAlpha=layout.background.opacity;context.drawImage(state.background,rect.x,rect.y,rect.w,rect.h);context.globalAlpha=1;}
    const metrics=state.metrics;
    for(const layer of layout.overlays) {
      context.save();context.beginPath();context.rect(layer.x,layer.y,layer.w,layer.h);context.clip();context.globalAlpha=layer.opacity;
      context.fillStyle=layer.color;context.strokeStyle=layer.color;context.lineWidth=2;context.textBaseline="top";
      const font=layer.font_size || 20, metric=metrics.find(item=>item.id===layer.metric), value=metricValue(metrics,layer.metric);
      context.font=font+"px Segoe UI, sans-serif";
      if(layer.type==="text") context.fillText(layer.label,layer.x,layer.y,layer.w);
      else {
        const labelSize=Math.max(8,Math.min(16,font*.5));context.font=labelSize+"px Segoe UI, sans-serif";
        context.fillText(layer.label || metric?.label || layer.metric,layer.x,layer.y,layer.w);
        const top=layer.y+labelSize+6, available=Math.max(1,layer.h-labelSize-6), ratio=value===null?null:clamp((value-layer.min)/(layer.max-layer.min||1),0,1);
        const valueText=value===null?"—":Number(value.toFixed(1))+" "+(metric?.unit || "");
        if(layer.type==="metric") {context.font=font+"px Segoe UI, sans-serif";context.fillText(valueText,layer.x,top,layer.w);}
        if(layer.type==="bar") {context.globalAlpha=layer.opacity*.15;context.fillRect(layer.x,top,layer.w,Math.max(3,available*.4));context.globalAlpha=layer.opacity;if(ratio!==null)context.fillRect(layer.x,top,layer.w*ratio,Math.max(3,available*.4));context.font=Math.min(font,available*.45)+"px Segoe UI, sans-serif";context.fillText(valueText,layer.x,top+available*.5,layer.w);}
        if(layer.type==="pie") {const radius=Math.max(2,Math.min(layer.w,available)/2-5),cx=layer.x+layer.w/2,cy=top+available/2;context.lineWidth=Math.max(3,radius*.13);context.globalAlpha=layer.opacity*.16;context.beginPath();context.arc(cx,cy,radius,0,Math.PI*2);context.stroke();context.globalAlpha=layer.opacity;if(ratio!==null&&ratio>0){context.beginPath();context.arc(cx,cy,radius,-Math.PI/2,Math.PI*2*ratio-Math.PI/2);context.stroke();}context.font=Math.min(font,radius*.48)+"px Segoe UI, sans-serif";context.textAlign="center";context.textBaseline="middle";context.fillText(valueText,cx,cy,radius*1.8);}
        if(layer.type==="line") {
          const points=state.history.slice(-120);let pen=false;context.beginPath();
          points.forEach((point,index)=>{const v=point.values?.[layer.metric];if(typeof v!=="number"||!Number.isFinite(v)){pen=false;return;}const x=layer.x+index/Math.max(1,points.length-1)*layer.w,y=top+available*(1-clamp((v-layer.min)/(layer.max-layer.min||1),0,1));if(pen)context.lineTo(x,y);else context.moveTo(x,y);pen=true;});context.stroke();
          if(points.length<2){context.font=Math.min(14,font)+"px Segoe UI, sans-serif";context.fillText("Waiting for history",layer.x,top,layer.w);}
        }
      }
      context.restore();
      if(interactive && layer.id===state.selected) {context.save();context.strokeStyle="#a9c9ff";context.lineWidth=1.5;context.setLineDash([5,3]);context.strokeRect(layer.x+.5,layer.y+.5,layer.w-1,layer.h-1);context.setLineDash([]);context.fillStyle="#ef93c8";context.fillRect(layer.x+layer.w-10,layer.y+layer.h-10,10,10);context.restore();}
    }
    context.restore();
  }
  function draw(){if(state.draft)paint(ctx,state.draft,true);}
  async function poll() {
    clearTimeout(state.pollTimer);if(state.polling||!token)return;state.polling=true;const epoch=state.epoch,requestedHeight=state.height;
    try {
      const results=await Promise.all([request("/v1/configurator?width=640&height="+requestedHeight),request("/v1/displays")]);
      if(epoch!==state.epoch)return;
      const [data,devices]=results;state.layouts=data.layouts||[];state.themeCache[requestedHeight]=data.themes||[];
      if(state.height===requestedHeight){state.themes=state.themeCache[requestedHeight];if(state.themeHeight!==requestedHeight){state.themeHeight=requestedHeight;renderThemes();}}
      state.assets=data.assets||[];state.metrics=data.metrics||[];state.history=data.history||[];state.bindings=data.bindings||{};state.devices=devices||[];
      connected(true);$("auth").hidden=true;
      if(!state.initialized) {
        state.initialized=true;renderSaved();showPage('home');
        status(state.devices.length?'Choose a display to edit its layout.':'No displays detected by this service. Check that display monitoring is running.');
      }
      if(state.serial&&!state.devices.some(device=>device.serial===state.serial)){state.serial="";status("Display disconnected. Your layout is still here; reconnect the display to apply it.",true);}
      renderDevices();renderSaved();renderAssets();renderMetrics();draw();
      if(page==='home')await refreshDisplayPreviews();
      if($("render-panel").open&&!state.dirty)await renderPreview(true);
    } catch(error) {if(epoch===state.epoch){connected(false);state.metrics=state.metrics.map(m=>({...m,value:null}));renderMetrics();renderDevices();status(error.message || "Could not reach the local service. Keep it running and try again.",true);}}
    finally {state.polling=false;if(token)state.pollTimer=setTimeout(poll,state.connected?2000:5000);}
  }
  async function renderPreview(quiet=false) {
    if(state.previewing||!state.draft||!state.connected)return;
    state.previewing=true;updateActions();const revision=state.revision;
    try {
      const blob=await request("/v1/configurator/preview.png",{method:"POST",body:JSON.stringify(state.draft),blob:true});
      if(revision!==state.revision)return;
      if(state.renderURL)URL.revokeObjectURL(state.renderURL);state.renderURL=URL.createObjectURL(blob);$("render-image").src=state.renderURL;$("render-image").hidden=false;$("render-state").textContent="Updated "+new Date().toLocaleTimeString();
      if(!quiet){$("render-panel").open=true;status("Device preview rendered. This uses the same renderer as your display.");}
    } catch(error){$("render-state").textContent="Render failed";status(error.message,true);}
    finally{state.previewing=false;updateActions();}
  }
  async function saveLayout(propagate=false) {
    const revision=state.revision, layout=clone(state.draft);$("save-button").disabled=true;
    try {
      const saved=await request("/v1/configurator/layouts/"+encodeURIComponent(layout.id),{method:"PUT",body:JSON.stringify(layout)});
      const index=state.layouts.findIndex(item=>item.id===saved.id);if(index<0)state.layouts.push(saved);else state.layouts[index]=saved;
      if(revision===state.revision)state.dirty=false;renderSaved();updateActions();status(revision===state.revision?"Layout saved. Displays already using this layout update automatically; use Apply to assign it to another display.":"Saved the previous revision. Your newest edits still need saving.");
    }catch(error){status(error.message,true);if(propagate===true)throw error;}finally{updateActions();}
  }
  $("auth-toggle").addEventListener("click",()=>{$("auth").hidden=!$("auth").hidden;if(!$("auth").hidden)$("token").focus();});
  $("auth-form").addEventListener("submit",event=>{event.preventDefault();token=$("token").value.trim();if(!token)return;state.epoch++;try{tokenStorage.setItem("jonsbo-token",token);}catch(_){}$("token").value="";state.initialized=false;poll();});
  $("forget-token").addEventListener("click",()=>{token="";state.epoch++;try{tokenStorage.removeItem("jonsbo-token");}catch(_){}clearTimeout(state.pollTimer);connected(false);state.metrics=state.metrics.map(m=>({...m,value:null}));renderMetrics();renderDevices();status("Token forgotten in this tab. Paste a token to reconnect.");});
  document.querySelectorAll("[data-height]").forEach(button=>button.addEventListener("click",()=>{const height=Number(button.dataset.height);state.serial="";if(height!==state.height||state.draft.rotation!==(height===180?90:0))setDraft(orientLayout(adaptLayout(state.draft,height)),true);renderDevices();renderThemes();status("Virtual "+(height===180?"fan":"pump")+" preview. Select a connected display to apply.");}));
  $("saved-layout").addEventListener("change",event=>{const chosen=state.layouts.find(layout=>layout.id===event.target.value);if(!chosen)return;if(!allowReplace()){event.target.value=state.draft.id;return;}const device=state.devices.find(item=>item.serial===state.serial);if(device&&(device.kind==="fan"?180:480)!==chosen.height)state.serial="";setDraft(state.serial?orientLayout(chosen,currentAssignment()):chosen);renderDevices();renderThemes();});
  $("new-layout").addEventListener("click",()=>{if(allowReplace())setDraft(blank(state.height),true);});
  $("layout-name").addEventListener("input",event=>{state.draft.name=event.target.value;changed();});
  $("rotation").addEventListener("change",event=>{state.draft.rotation=Number(event.target.value);changed();});
  $("background-color").addEventListener("input",event=>{state.draft.background.color=event.target.value;changed();});
  $("background-opacity").addEventListener("input",event=>{state.draft.background.opacity=Number(event.target.value);changed();});
  $("background-asset").addEventListener("change",event=>{state.draft.background.asset_id=event.target.value;changed();loadBackground();});
  $("properties").addEventListener("submit",event=>event.preventDefault());
  for(const key of propNames)$("prop-"+key).addEventListener("input",event=>{
    const layer=selected();if(!layer)return;
    if(key==='metric'){
      const previous=state.metrics.find(item=>item.id===layer.metric), next=state.metrics.find(item=>item.id===event.target.value);
      if(next&&(!layer.label||layer.label===previous?.label)){layer.label=next.label;$('prop-label').value=next.label;renderLayers();}
    }
    if(["label","metric","color"].includes(key))layer[key]=event.target.value;
    else{if(event.target.value===""||!Number.isFinite(Number(event.target.value)))return;layer[key]=Number(event.target.value);}
    if(key==="font_size")layer.font_size=Math.round(clamp(layer.font_size,8,96));
    constrainLayer(layer,state.draft.width,state.draft.height);changed();if(key==="label"||key==="color")renderLayers();
  });
  for(const key of ["x","y","w","h","font_size"])$("prop-"+key).addEventListener("change",renderProperties);
  $("add-layer").addEventListener("click",()=>{
    if(state.draft.overlays.length>=32)return;const type=$("add-type").value,index=state.draft.overlays.length;
    const layer={id:id("layer"),type,metric:"cpu.usage",label:type==="text"?"Your text":"CPU usage",x:20+(index%5)*16,y:20+(index%4)*16,w:type==="line"?280:190,h:type==="pie"?160:type==="text"?48:90,color:"#a8c9ff",font_size:28,min:0,max:100,opacity:1};
    constrainLayer(layer,640,state.height);state.draft.overlays.push(layer);state.selected=layer.id;changed();renderLayers();renderProperties();
  });
  $("delete-layer").addEventListener("click",()=>{state.draft.overlays=state.draft.overlays.filter(layer=>layer.id!==state.selected);state.selected="";changed();renderLayers();renderProperties();});
  function reorder(direction){const i=state.draft.overlays.findIndex(layer=>layer.id===state.selected),j=i+direction;if(i<0||j<0||j>=state.draft.overlays.length)return;[state.draft.overlays[i],state.draft.overlays[j]]=[state.draft.overlays[j],state.draft.overlays[i]];changed();renderLayers();renderProperties();}
  $("layer-down").addEventListener("click",()=>reorder(-1));$("layer-up").addEventListener("click",()=>reorder(1));
  let drag=null;
  function canvasPoint(event){const rect=canvas.getBoundingClientRect();return{x:(event.clientX-rect.left)*canvas.width/rect.width,y:(event.clientY-rect.top)*canvas.height/rect.height};}
  canvas.addEventListener("pointerdown",event=>{if(event.button!==0)return;const point=canvasPoint(event),layer=[...state.draft.overlays].reverse().find(item=>point.x>=item.x&&point.y>=item.y&&point.x<=item.x+item.w&&point.y<=item.y+item.h);selectLayer(layer?.id||"");canvas.focus();if(!layer)return;drag={point,layer:clone(layer),resize:point.x>=layer.x+layer.w-14&&point.y>=layer.y+layer.h-14};canvas.setPointerCapture(event.pointerId);});
  canvas.addEventListener("pointermove",event=>{if(!drag)return;const layer=selected();if(!layer)return;const point=canvasPoint(event),dx=point.x-drag.point.x,dy=point.y-drag.point.y;if(drag.resize){layer.w=Math.round(clamp(drag.layer.w+dx,1,640-layer.x));layer.h=Math.round(clamp(drag.layer.h+dy,1,state.height-layer.y));}else{layer.x=drag.layer.x+dx;layer.y=drag.layer.y+dy;}constrainLayer(layer,640,state.height);changed();renderProperties();});
  for(const name of ["pointerup","pointercancel","lostpointercapture"])canvas.addEventListener(name,()=>{drag=null;});
  canvas.addEventListener("keydown",event=>{const layer=selected();if(!layer)return;const step=event.shiftKey?10:1,delta={ArrowLeft:[-step,0],ArrowRight:[step,0],ArrowUp:[0,-step],ArrowDown:[0,step]}[event.key];if(!delta)return;event.preventDefault();layer.x+=delta[0];layer.y+=delta[1];constrainLayer(layer,640,state.height);changed();renderProperties();});
  $("preview-button").addEventListener("click",()=>renderPreview());$("save-button").addEventListener("click",saveLayout);
  $("apply-button").addEventListener("click",async()=>{
    if($("apply-button").disabled)return;
    const revision=state.revision,serial=state.serial,layout=clone(state.draft);
    applying=true;updateActions();
    try{await saveThenApply(()=>saveLayout(true),()=>revision===state.revision&&serial===state.serial,()=>request("/v1/configurator/apply",{method:"POST",body:JSON.stringify({serial,layout_id:layout.id,rotation:layout.rotation})}));status("Saved and applied to "+serial+". You can close the editor.");}
    catch(error){status(error.message,true);}finally{applying=false;updateActions();}
  });
  function waitMedia(target,event,signal,action,timeout=12000){return new Promise((resolve,reject)=>{let timer;const cleanup=()=>{clearTimeout(timer);target.removeEventListener(event,done);target.removeEventListener("error",failed);signal.removeEventListener("abort",aborted);};const done=()=>{cleanup();resolve();},failed=()=>{cleanup();reject(new Error("This media could not be decoded. Try a PNG/JPEG image or a browser-supported MP4/WebM video."));},aborted=()=>{cleanup();reject(new Error("Import cancelled."));};target.addEventListener(event,done,{once:true});target.addEventListener("error",failed,{once:true});signal.addEventListener("abort",aborted,{once:true});timer=setTimeout(()=>{cleanup();reject(new Error("Media decoding timed out. Try a shorter or smaller file."));},timeout);if(signal.aborted){aborted();return;}try{action();}catch(error){cleanup();reject(error);}});}
  async function importMedia(file){
    if(!file||state.importing)return;
    if(!state.connected){status("Connect to the local service before importing media.",true);return;}
    if(file.size>64*1024*1024){status("Choose a file smaller than 64 MB.",true);return;}
    const videoFile=/^video\/(mp4|webm)$/.test(file.type),imageFile=/^image\/(png|jpeg|webp)$/.test(file.type);
    if(!videoFile&&!imageFile){status("Choose a PNG, JPEG, WebP, MP4 or WebM file.",true);return;}
    const controller=new AbortController();state.importing=controller;$("import-progress").hidden=false;$("media-file").disabled=true;$("import-bar").value=0;
    const overall=setTimeout(()=>controller.abort(),180000),sourceURL=URL.createObjectURL(file),draftID=state.draft.id;
    let media;
    try {
      let plan;
      if(videoFile){media=document.createElement("video");media.muted=true;media.preload="auto";media.playsInline=true;await waitMedia(media,"loadeddata",controller.signal,()=>{media.src=sourceURL;media.load();});plan=mediaPlan(media.videoWidth,media.videoHeight,media.duration);}
      else{media=new Image();await waitMedia(media,"load",controller.signal,()=>{media.src=sourceURL;});plan=mediaPlan(media.naturalWidth,media.naturalHeight);}
      const buffer=document.createElement("canvas");buffer.width=plan.width;buffer.height=plan.height;const context=buffer.getContext("2d");const frames=[];let bytes=0;
      for(let frame=0;frame<plan.frames;frame++){
        if(controller.signal.aborted)throw new Error("Import cancelled.");
        if(videoFile){const time=Math.min(frame/plan.fps,Math.max(0,media.duration-.001));if(Math.abs(media.currentTime-time)>.001)await waitMedia(media,"seeked",controller.signal,()=>{media.currentTime=time;});}
        context.clearRect(0,0,buffer.width,buffer.height);context.drawImage(media,0,0,buffer.width,buffer.height);
        const data=buffer.toDataURL(imageFile&&file.type==="image/png"?"image/png":"image/jpeg",.78).split(",")[1];bytes+=data.length;
        if(bytes>31*1024*1024)throw new Error("Imported frames exceed 32 MB. Use a shorter video or a simpler background.");frames.push(data);
        $("import-bar").value=(frame+1)/plan.frames*.9;$("import-status").textContent="Preparing frame "+(frame+1)+" of "+plan.frames;
        await new Promise(resolve=>setTimeout(resolve,0));
      }
      $("import-status").textContent="Saving media to this computer…";
      const body=JSON.stringify({name:file.name.slice(0,100),fps:plan.fps,frames});
      if(new Blob([body]).size>32*1024*1024)throw new Error("Imported media exceeds the 32 MB request limit.");
      const asset=await request("/v1/configurator/assets",{method:"POST",body,signal:controller.signal,timeout:60000});state.assets.push(asset);
      if(state.draft.id===draftID){state.draft.background.asset_id=asset.id;changed();}renderAssets();loadBackground();$("import-bar").value=1;
      status("Imported "+asset.name+". "+(asset.frame_count>1?asset.frame_count+" frames stored for playback without the browser. ":"")+"Save the layout to keep this background.");
    }catch(error){status(error.message,true);}
    finally{clearTimeout(overall);if(media instanceof HTMLVideoElement){media.pause();media.removeAttribute("src");media.load();}else if(media)media.src="";URL.revokeObjectURL(sourceURL);state.importing=null;$("import-progress").hidden=true;$("media-file").disabled=false;$("media-file").value="";}
  }
  $("media-file").addEventListener("change",event=>importMedia(event.target.files[0]));$("cancel-import").addEventListener("click",()=>state.importing?.abort());
  window.addEventListener("beforeunload",event=>{if(state.dirty||state.importing){event.preventDefault();event.returnValue="";}});
  window.addEventListener("pagehide",()=>{state.importing?.abort();if(state.renderURL)URL.revokeObjectURL(state.renderURL);});
  setDraft(blank(480));showPage('home');renderDevices();if(token)poll();else{$("auth").hidden=false;connected(false);status("Open Display studio from the tray, or enter your local API token to connect.");}
}
if(typeof document!=="undefined")startStudio();
