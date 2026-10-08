export function visibleEvent(input, tabID, senderURL) {
  let url;
  try { url = new URL(senderURL); } catch { return null; }
  const parts = url.pathname.split('/').filter(Boolean);
  let provider;
  if (url.protocol !== 'https:' || url.port || parts.length < 2) return null;
  if (url.hostname === 'chatgpt.com' && parts[0] === 'c') provider = 'openai';
  else if (url.hostname === 'claude.ai' && parts[0] === 'chat') provider = 'claude';
  else return null;
  const clean = (s, max) => typeof s === 'string' ? [...s.replace(/[\u0000-\u001f\u007f]/g, '')].slice(0, max).join('') : '';
  const activity = ['running','idle','waiting','completed','unknown'].includes(input?.activity) ? input.activity : 'unknown';
  return {url: url.origin + url.pathname, provider, session_id: `browser:${tabID}:${clean(parts[1],128)}`, title: clean(input?.title,128), model: clean(input?.model,80), activity};
}
