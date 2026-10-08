import test from 'node:test';
import assert from 'node:assert/strict';
import helpers from './app.js';
const {constrainLayer, adaptLayout, coverRect, orientLayout, metricValue, mediaPlan, readToken} = helpers;

test('widget connection status only asks for sign-in when the selected provider needs it',()=>{
 const options={connected:true};
 const active=helpers.aiProviderView({ready:true,login:'signed-in',monitoring:true},options);
 assert.equal(active.statusLabel,'Connected');assert.equal(active.statusTone,'success');assert.equal(active.signInLabel,null);
 const paused=helpers.aiProviderView({ready:true,login:'signed-in',monitoring:false},options);
 assert.equal(paused.statusLabel,'Monitoring paused');assert.equal(paused.signInLabel,null);
 const signedOut=helpers.aiProviderView({ready:true,login:'signed-out',monitoring:false},options);
 assert.equal(signedOut.statusLabel,'Sign in needed');assert.equal(signedOut.signInLabel,'Sign in');
 const expired=helpers.aiProviderView({ready:true,login:'signed-out',monitoring:true},options);
 assert.equal(expired.signInLabel,'Reconnect');
 for(const login of ['checking','error','unavailable'])assert.equal(helpers.aiProviderView({ready:true,login},options).signInLabel,null);
 const offline=helpers.aiProviderView({ready:true,login:'signed-out'},{connected:false});
 assert.equal(offline.statusLabel,'Service unavailable');assert.equal(offline.signInLabel,null);
});

test('native subscription sign-in is reused with monitoring on or paused',()=>{
 const paused=helpers.aiProviderView({ready:true,login:'signed-in',monitoring:false},{connected:true});
 assert.equal(paused.primaryAction,'monitor');assert.equal(paused.primaryLabel,'Start monitoring');assert.equal(paused.loginHelp,false);assert.equal(paused.monitoringLabel,'Off');
 const running=helpers.aiProviderView({ready:true,login:'signed-in',monitoring:true},{connected:true});
 assert.equal(running.primaryAction,'disconnect');assert.equal(running.primaryLabel,'Stop monitoring');assert.equal(running.loginHelp,false);assert.equal(running.monitoringLabel,'On');
 assert.equal(helpers.aiActionAllowed({ready:true,login:'signed-in',monitoring:false},'connect',{connected:true}),false);
});
test('only known signed-out or API-key native clients show manual subscription login guidance',()=>{
 for(const login of ['signed-out','api-key']){
  const view=helpers.aiProviderView({ready:true,login,monitoring:false},{connected:true});assert.equal(view.loginHelp,true);assert.equal(view.primaryAction,null);assert.equal(helpers.aiActionAllowed({ready:true,login},'connect',{connected:true}),false);
 }
 for(const login of ['checking','error','unavailable']){
  const view=helpers.aiProviderView({ready:true,login,monitoring:false},{connected:true});assert.equal(view.loginHelp,false);assert.equal(view.primaryAction,null);assert.notEqual(view.loginLabel,'Signed out');
 }
 const missing=helpers.aiProviderView({ready:false,login:'unavailable'},{connected:true});assert.equal(missing.install,true);assert.equal(missing.cliLabel,'Not installed');assert.equal(missing.loginHelp,false);
});
test('monitoring actions reject busy, offline and stale sign-in actions',()=>{
 const info={ready:true,login:'signed-in',monitoring:false};
 for(const options of [{connected:false},{connected:true,busy:true}])for(const action of ['check','monitor','connect','disconnect'])assert.equal(helpers.aiActionAllowed(info,action,options),false);
 assert.equal(helpers.aiActionAllowed(info,'monitor',{connected:true}),true);
 assert.equal(helpers.aiActionAllowed({ready:true,login:'error',monitoring:true},'disconnect',{connected:true}),true);
 assert.equal(helpers.aiActionAllowed({ready:true,login:'checking'},'connect',{connected:true}),false);
 assert.equal(helpers.aiActionAllowed({},'check',{connected:true}),true);
});
test('usage rows become unavailable when the CLI loses its subscription login',()=>{
 const options={connected:true};
 assert.equal(helpers.aiProviderView({ready:true,login:'signed-in',monitoring:true},options).usageAvailable,true);
 for(const login of ['signed-out','api-key','checking','error','unavailable'])assert.equal(helpers.aiProviderView({ready:true,login,monitoring:true},options).usageAvailable,false);
 assert.equal(helpers.aiProviderView({ready:true,login:'signed-in',monitoring:false},options).usageAvailable,false);
 assert.equal(helpers.aiProviderView({ready:true,login:'signed-in',monitoring:true},{connected:false}).usageAvailable,false);
});
test('provider-specific widget insertion preserves draft identity, content and preferences',()=>{
 const draft={id:'saved-fan',name:'Unsaved edits',width:640,height:180,rotation:270,background:{color:'#123456'},overlays:[{id:'text',type:'text',label:'Keep me'}]};
 const next=helpers.withAIWidget(draft,'claude','new-widget');
 assert.equal(next.id,draft.id);assert.equal(next.name,draft.name);assert.equal(next.rotation,270);assert.deepEqual(next.background,draft.background);assert.equal(next.overlays[0].label,'Keep me');assert.equal(draft.overlays.length,1);
 assert.equal(next.overlays[1].provider,'claude');assert.equal(next.overlays[1].h,164);assert.equal(next.overlays[1].animate,true);
 assert.throws(()=>helpers.withAIWidget({...draft,overlays:Array(32).fill({})},'openai','new-widget'),/32/);
});

test('AI widget defaults, explicit animation off and reduced motion survive editing',()=>{
 assert.deepEqual(helpers.aiPreferences({}),{provider:'openai',detail:'compact',animate:true});
 assert.deepEqual(helpers.aiPreferences({provider:'claude',detail:'compact',animate:false}),{provider:'claude',detail:'compact',animate:false});
 assert.equal(helpers.aiPreferences({animate:true},true).animate,false);
 const layer={type:'ai-provider',provider:'claude',detail:'compact',animate:false,x:10,y:10,w:300,h:150,font_size:14};
 const result=adaptLayout({width:640,height:480,overlays:[layer]},180).overlays[0];
 assert.equal(result.provider,'claude');assert.equal(result.animate,false);assert.equal(result.detail,'compact');
 assert.equal(metricValue([{id:'ai.openai.tokens',value:null}],'ai.openai.tokens'),null);
});
test('changing a pump AI widget to fan preserves readable compact quota and token rows',()=>{
 const layer={id:'ai',type:'ai-provider',provider:'claude',detail:'expanded',animate:false,x:200,y:160,w:400,h:300,font_size:14};
 const pump={id:'saved-pump',width:640,height:480,overlays:[layer]};
 const fan=adaptLayout(pump,180), widget=fan.overlays[0];
 assert.equal(widget.detail,'compact');assert.equal(widget.h,164);assert.equal(widget.y,16);
 assert.equal(widget.provider,'claude');assert.equal(widget.animate,false);
 assert.equal(layer.detail,'expanded');assert.equal(layer.h,300);
 assert.equal(adaptLayout(fan,180).overlays[0].h,164);
});

test('dragging and resizing never leave the device canvas', () => {
  assert.deepEqual(constrainLayer({x:-10,y:900,w:999,h:20},640,180),{x:0,y:160,w:640,h:20});
  assert.deepEqual(constrainLayer({x:639,y:179,w:0,h:-1},640,180),{x:639,y:179,w:1,h:1});
});
test('pump themes adapt to fan canvas without mutating the original', () => {
  const source={width:640,height:480,overlays:[{x:500,y:400,w:140,h:80,font_size:32}]};
  const fan=adaptLayout(source,180);
  assert.equal(fan.height,180);assert.equal(fan.overlays[0].y,150);assert.equal(fan.overlays[0].h,30);
  assert.equal(fan.overlays[0].font_size,12);assert.equal(source.height,480);assert.equal(source.overlays[0].y,400);
});
test('changing display format creates a fresh ID while same format retains identity',()=>{
  const layout={id:'default-pump',width:640,height:480,overlays:[]};
  const fan=adaptLayout(layout,180);
  assert.notEqual(fan.id,layout.id);assert.match(fan.id,/^[a-z][a-z0-9-]+$/);
  assert.equal(adaptLayout(layout,480).id,layout.id);
  assert.notEqual(adaptLayout(fan,480).id,fan.id);
});
test('backgrounds cover and center-crop like the physical display renderer',()=>{
  assert.deepEqual(coverRect(640,480,640,180),{x:0,y:-150,w:640,h:480});
  assert.deepEqual(coverRect(320,180,640,480),{x:-106.66666666666663,y:0,w:853.3333333333333,h:480});
});
test('virtual fan layouts default to physical 90-degree rotation, pump defaults to zero',()=>{
  assert.equal(orientLayout({height:180,rotation:0}).rotation,90);
  assert.equal(orientLayout({height:480,rotation:90}).rotation,0);
});
test('device or binding rotation takes precedence over saved layout and theme rotation',()=>{
  const preset={height:180,rotation:90,overlays:[]};
  for(const rotation of [0,90,180,270])assert.equal(orientLayout(preset,{rotation}).rotation,rotation);
  const adjusted=orientLayout(preset,{rotation:270});
  assert.equal(preset.rotation,90);assert.notEqual(adjusted,preset);
  assert.equal(orientLayout(preset,{rotation:45}).rotation,90);
});
test('adapting to a device retains its assignment orientation and creates a separate format',()=>{
  const original={id:'saved-pump',width:640,height:480,rotation:0,overlays:[]};
  const fan=orientLayout(adaptLayout(original,180),{rotation:270});
  assert.equal(fan.height,180);assert.equal(fan.rotation,270);assert.notEqual(fan.id,original.id);
  assert.equal(original.rotation,0);assert.equal(original.height,480);
});
test('missing, nonfinite and null hardware readings remain unavailable, zero remains valid', () => {
  const metrics=[{id:'zero',value:0},{id:'null',value:null},{id:'bad',value:NaN},{id:'infinite',value:Infinity}];
  assert.equal(metricValue(metrics,'zero'),0);
  for(const id of ['null','bad','infinite','absent'])assert.equal(metricValue(metrics,id),null);
});
test('media sampling is bounded to device size, 60 seconds and 120 frames', () => {
  assert.deepEqual(mediaPlan(3840,2160,7200),{width:640,height:360,frames:120,fps:2});
  assert.deepEqual(mediaPlan(100,100),{width:100,height:100,frames:1,fps:1});
  assert.deepEqual(mediaPlan(1000,4000,.1),{width:120,height:480,frames:1,fps:2});
  for(const args of [[0,100],[100,0],[Infinity,100],[100,100,Infinity],[100,100,0]])assert.throws(()=>mediaPlan(...args));
});
test('fragment token is cleared before storage access and preserves literal token characters', () => {
  const calls=[];const storage={setItem:(key,value)=>calls.push(['store',key,value]),getItem:()=>{throw Error('unexpected');}};
  const token=readToken({hash:'#token=abc%2Bdef%3D',pathname:'/',search:''},{replaceState:(_state,_title,url)=>calls.push(['clear',url])},storage);
  assert.equal(token,'abc+def=');assert.deepEqual(calls,[['clear','/'],['store','jonsbo-token','abc+def=']]);
});
test('blocked browser storage still permits a tray token, absent token is empty',()=>{
  const blocked={getItem:()=>{throw Error('blocked');},setItem:()=>{throw Error('blocked');}};
  const history={replaceState:()=>{}};
  assert.equal(readToken({hash:'#token=local',pathname:'/',search:''},history,blocked),'local');
  assert.equal(readToken({hash:'',pathname:'/',search:''},history,blocked),'');
});

test('save and apply never applies a failed or superseded save', async()=>{
  let applied=0;
  await assert.rejects(()=>helpers.saveThenApply(async()=>{throw new Error('disk full');},()=>true,async()=>{applied++;}),/disk full/);
  await assert.rejects(()=>helpers.saveThenApply(async()=>{},()=>false,async()=>{applied++;}),/changed/);
  assert.equal(applied,0);
  const order=[];
  await helpers.saveThenApply(async()=>{order.push('saved');},()=>true,async()=>{order.push('applied');});
  assert.deepEqual(order,['saved','applied']);
});

test('selecting a display loads only its bound layout, not another display draft',()=>{
  const defaults=[{id:'default-fan',height:180,overlays:[],rotation:90},{id:'default-pump',height:480,overlays:[],rotation:0}];
  const bound={id:'saved-fan',height:180,overlays:[],rotation:0};
  const a={serial:'fan-a',kind:'fan',assignment:{module:'configurator',view:'saved-fan',rotation:270}};
  const result=helpers.layoutForDisplay(a,[...defaults,bound],{},'new-a');
  assert.equal(result.id,'saved-fan');assert.equal(result.rotation,270);assert.equal(bound.rotation,0);
  const b={serial:'fan-b',kind:'fan',assignment:{module:'hardware',view:'saved-fan',rotation:90}};
  const fresh=helpers.layoutForDisplay(b,[...defaults,bound],{},'new-b');
  assert.equal(fresh.id,'new-b');assert.equal(fresh.rotation,90);assert.equal(fresh.height,180);
  assert.equal(helpers.layoutForDisplay({...a,assignment:{...a.assignment,view:'missing'}},defaults,{},'new-c').id,'new-c');
});

test('CPU, GPU and memory displays each open their own assigned hardware content',()=>{
  const defaults=[{id:'default-fan',height:180,background:{color:'#000000',opacity:1},overlays:[]}];
  const readings={cpu:'cpu.usage',gpu:'gpu.usage',memory:'ram.usage'};
  for(const [view,metric] of Object.entries(readings)){
    const device={serial:'fan-'+view,kind:'fan',assignment:{module:'hardware',view,rotation:270}};
    const layout=helpers.layoutForDisplay(device,defaults,{},'layout-'+view);
    assert.ok(layout.overlays.some(layer=>layer.metric===metric));
    assert.equal(layout.rotation,270);assert.equal(layout.height,180);
    assert.equal(layout.id,'layout-'+view);
  }
  assert.equal(defaults[0].overlays.length,0);
});

test('AI assigned screens load editable provider content and fan widgets use readable dimensions',()=>{
  const defaults=[{id:'default-fan',width:640,height:180,background:{color:'#000000',opacity:1},overlays:[]}];
  const device={serial:'fan-ai',kind:'fan',assignment:{module:'ai-subscriptions',view:'claude',rotation:90}};
  const layout=helpers.layoutForDisplay(device,defaults,{},'layout-ai');
  assert.equal(layout.overlays[0].provider,'claude');
  const added=helpers.withAIWidget(defaults[0],'openai','ai');
  assert.ok(added.overlays[0].w>=560);assert.ok(added.overlays[0].h>=160);
  assert.equal(added.overlays[0].font_size,28);
});

test('current display preview uses the selected serial assignment, including binding fallback',()=>{
  const a={serial:'a',assignment:{module:'hardware',view:'cpu'}};
  const b={serial:'b',assignment:{module:'hardware',view:'gpu'}};
  assert.equal(helpers.displayFramePath(a,{}),'/v1/modules/hardware/views/cpu.png');
  assert.equal(helpers.displayFramePath(b,{}),'/v1/modules/hardware/views/gpu.png');
  assert.equal(helpers.displayFramePath({serial:'c'},{c:{module:'configurator',view:'custom-c'}}),'/v1/modules/configurator/views/custom-c.png');
  assert.equal(helpers.displayFramePath({serial:'d'},{}),null);
});

test('delayed overview frames are discarded after assignment, connection or display changes',async()=>{
 for(const change of ['assignment','binding','disconnect','removed','epoch']){
  const device={serial:'fan',assignment:{module:'hardware',view:'cpu'}};
  const state={devices:[device],bindings:{},connected:true,epoch:1};
  if(change==='binding'){delete device.assignment;state.bindings.fan={module:'hardware',view:'cpu'};}
  let finish;
  const pending=helpers.loadDisplayPreview(device,state.bindings,state.epoch,()=>new Promise(resolve=>{finish=resolve;}),()=>state);
  if(change==='assignment')state.devices=[{serial:'fan',assignment:{module:'hardware',view:'gpu'}}];
  if(change==='binding')state.bindings={fan:{module:'hardware',view:'gpu'}};
  if(change==='disconnect')state.connected=false;
  if(change==='removed')state.devices=[];
  if(change==='epoch')state.epoch++;
  finish('old CPU frame');
  assert.equal(await pending,null,change+' allowed a stale frame');
 }
 const device={serial:'fan',assignment:{module:'hardware',view:'memory'}};
 const frame=await helpers.loadDisplayPreview(device,{},1,async()=> 'memory frame',()=>({devices:[device],bindings:{},connected:true,epoch:1}));
 assert.equal(frame.blob,'memory frame');assert.equal(frame.path,'/v1/modules/hardware/views/memory.png');
});
