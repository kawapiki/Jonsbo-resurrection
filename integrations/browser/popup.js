chrome.runtime.sendMessage({type:'jonsbo-bridge-status'},reply=>{
  const error=chrome.runtime.lastError;
  document.querySelector('#status').textContent=error||!reply?.connected?'Local bridge disconnected.':reply.accepted?`Local server receiving activity · ${reply.sessions} visible tabs`:'Bridge connected; local server unavailable or waiting for activity.';
});
