import {test} from 'node:test';
import assert from 'node:assert/strict';
import {visibleEvent} from './metadata.js';
test('projects visible metadata without account, content or usage claims', () => {
  const e=visibleEvent({title:'hello\u0000',activity:'running',account_key:'forged',tokens:123,prompt:'private'},5,'https://chatgpt.com/c/abc?private=1');
  assert.deepEqual(e,{url:'https://chatgpt.com/c/abc',provider:'openai',session_id:'browser:5:abc',title:'hello',model:'',activity:'running'});
});
test('accepts Claude conversations, rejects unrelated and deceptive hosts',()=>{
  assert.equal(visibleEvent({},2,'https://claude.ai/chat/xyz').provider,'claude');
  for(const u of ['https://chatgpt.com.evil.test/c/a','http://chatgpt.com/c/a','https://claude.ai/settings','https://chatgpt.com:123/c/a']) assert.equal(visibleEvent({},2,u),null);
});
test('tab IDs separate visible sessions and completed survives close',()=>{
  assert.notEqual(visibleEvent({},1,'https://claude.ai/chat/a').session_id,visibleEvent({},2,'https://claude.ai/chat/a').session_id);
  assert.equal(visibleEvent({activity:'completed'},1,'https://claude.ai/chat/a').activity,'completed');
});
