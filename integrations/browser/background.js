import {visibleEvent} from './metadata.js';
let port;
let accepted=false;
const sessions=new Map();
function send(event) {
  try {
    if (!port) {
      port=chrome.runtime.connectNative('com.jonsbo.subscription_display');
      port.onMessage.addListener(reply=>{accepted=reply?.accepted===true;});
      port.onDisconnect.addListener(()=>{void chrome.runtime.lastError;port=null;accepted=false;});
    }
    port.postMessage(event);
  } catch { port=null; }
}
chrome.runtime.onMessage.addListener((message,sender,respond)=>{
  if(message?.type==='jonsbo-bridge-status'&&!sender.tab){respond({connected:!!port,accepted,sessions:sessions.size});return;}
  if(message?.type!=='jonsbo-visible-session'||!Number.isInteger(sender.tab?.id)) return;
  const e=visibleEvent(message,sender.tab.id,sender.url);
  if(!e) {
    const old=sessions.get(sender.tab.id);
    if(old){send({...old,activity:'completed'});sessions.delete(sender.tab.id);}
    return;
  }
  const old=sessions.get(sender.tab.id);
  if(old && old.session_id!==e.session_id) send({...old,activity:'completed'});
  sessions.set(sender.tab.id,e);
  send(e);
});
chrome.tabs.onRemoved.addListener(tabID=>{
  const e=sessions.get(tabID);if(e){send({...e,activity:'completed'});sessions.delete(tabID);}
});
