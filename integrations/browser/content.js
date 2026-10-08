// Read only visible interface metadata. No message text, credentials or requests.
(() => {
  let last=0;
  const visible=el=>!!el && el.getClientRects().length>0 && getComputedStyle(el).visibility!=='hidden';
  function report(force=false){
    if(!force && Date.now()-last<1500)return;
    last=Date.now();
    const stop=[...document.querySelectorAll('button[data-testid="stop-button"],button[aria-label*="Stop"],button[aria-label*="stop"]')].some(visible);
    // Titles and the selected model label are UI metadata, not conversation bodies.
    const modelButton=document.querySelector('[data-testid="model-switcher-dropdown-button"]');
    const model=visible(modelButton)?modelButton.textContent.trim().slice(0,80):'';
    try{chrome.runtime.sendMessage({type:'jonsbo-visible-session',activity:stop?'running':'idle',title:document.title.slice(0,128),model});}catch{}
  }
  new MutationObserver(()=>report()).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['aria-label','data-testid']});
  setInterval(()=>report(true),15000);
  document.addEventListener('visibilitychange',()=>report(true));
  window.addEventListener('popstate',()=>report(true));
  report(true);
})();
