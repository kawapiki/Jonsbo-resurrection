import test from 'node:test';
import assert from 'node:assert/strict';
import helpers from './app.js';
const {constrainLayer, adaptLayout, coverRect, orientLayout, metricValue, mediaPlan, readToken} = helpers;

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
