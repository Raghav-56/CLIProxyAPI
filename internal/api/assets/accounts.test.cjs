const test = require('node:test');
const assert = require('node:assert/strict');
const {parseQuota, codexAccounts, accountModels, recommendPlusAccount, recentRequests} = require('./accounts.js');

test('quota preserves a weekly primary window and missing secondary', () => {
  assert.deepEqual(parseQuota({plan_type:'plus', rate_limit:{primary_window:{used_percent:37,limit_window_seconds:604800,reset_at:2000000000},secondary_window:null}}, 1000), {
    windows:[{key:'primary_window',seconds:604800,label:'Weekly quota',remaining:63,resetsAt:2000000000000}],
    plan:'plus',fetchedAt:1000,limitReached:false,
  });
});

test('missing percentages are unknown rather than zero consumption', () => {
  assert.deepEqual(parseQuota({rate_limit:{primary_window:{used_percent:null,limit_window_seconds:18000}}},1000).windows, []);
  assert.throws(() => parseQuota({error:'provider failed'},1000), /unrecognized/);
});

test('relative reset is based on retrieval and depleted quota remains zero', () => {
  assert.deepEqual(parseQuota({rate_limit:{primary_window:{used_percent:110,limit_window_seconds:18000,reset_after_seconds:20},limit_reached:true}}, 1000), {
    windows:[{key:'primary_window',seconds:18000,label:'5-hour quota',remaining:0,resetsAt:21000}],
    plan:'',fetchedAt:1000,limitReached:true,
  });
});

test('account cards exclude API keys and other providers and never return credentials', () => {
  const files = [
    {id:'a',name:'a.json',auth_index:'index',provider:'codex',account_type:'oauth',email:'a@example.test',access_token:'secret',id_token:{chatgpt_account_id:'account',plan_type:'plus'}},
    {id:'b',name:'b.json',auth_index:'b',provider:'codex',account_type:'apikey'},
    {id:'c',name:'c.json',auth_index:'c',provider:'claude',account_type:'oauth'},
    {id:'d',name:'d.json',auth_index:'d',provider:'codex',account_type:'oauth',runtime_only:true},
  ];
  assert.deepEqual(codexAccounts(files), [{id:'a',name:'a.json',authIndex:'index',email:'a@example.test',label:'',disabled:false,unavailable:false,accountId:'account',plan:'plus'}]);
});

test('model catalog shows only visible models supported by this account', () => {
  assert.deepEqual(accountModels({models:[
    {slug:'gpt-6-luna',visibility:'list',supported_in_api:true},
    {slug:'gpt-reserve',visibility:'hide',supported_in_api:true},
    {slug:'gpt-6.1-sol',visibility:'list',supported_in_api:false},
  ]}), ['gpt-6-luna']);
  assert.throws(() => accountModels({models:[]}), /No visible/);
});

const recommendationTime = 2000000000000;
const recommendationAccounts = [
  {id:'persona',plan:'plus',disabled:false,unavailable:false},
  {id:'second',plan:'plus',disabled:false,unavailable:false},
  {id:'go',plan:'go',disabled:false,unavailable:false},
];
function quotaFor(fiveHourLeft, weeklyLeft, weeklyResetAfter, plan = 'plus', fetchedAt = recommendationTime) {
  return parseQuota({plan_type:plan, rate_limit:{
    primary_window:{used_percent:100-fiveHourLeft,limit_window_seconds:18000,reset_after_seconds:3600},
    secondary_window:{used_percent:100-weeklyLeft,limit_window_seconds:604800,reset_after_seconds:weeklyResetAfter},
  }}, fetchedAt);
}

test('recommendation uses usable Plus quota with earliest weekly reset without mutating selection', () => {
  const quota = new Map([
    ['persona',quotaFor(85,95,172800)],
    ['second',quotaFor(40,30,3600)],
    ['go',quotaFor(100,100,60,'go')],
  ]);
  const before = JSON.stringify(recommendationAccounts);
  const suggested = recommendPlusAccount(recommendationAccounts, quota, recommendationTime);
  assert.equal(suggested.account.id, 'second');
  assert.equal(suggested.fiveHour.remaining, 40);
  assert.equal(JSON.stringify(recommendationAccounts), before);
});

test('depleted, unavailable, disabled, stale and reset-crossed accounts are excluded', () => {
  for (const badQuota of [quotaFor(0,40,3600), quotaFor(40,0,3600),
    quotaFor(40,40,3600,'plus',recommendationTime-300001),
    quotaFor(40,40,0)]) {
    const quota = new Map([['persona',badQuota]]);
    assert.equal(recommendPlusAccount(recommendationAccounts,quota,recommendationTime), null);
  }
  const quota = new Map([['persona',quotaFor(40,40,3600)]]);
  assert.equal(recommendPlusAccount([{...recommendationAccounts[0],disabled:true}],quota,recommendationTime), null);
  assert.equal(recommendPlusAccount([{...recommendationAccounts[0],unavailable:true}],quota,recommendationTime), null);
});

test('unknown windows or reset times do not become a recommendation', () => {
  const quota = new Map([['persona',parseQuota({plan_type:'plus',rate_limit:{primary_window:{used_percent:10,limit_window_seconds:604800}}},recommendationTime)]]);
  assert.equal(recommendPlusAccount(recommendationAccounts,quota,recommendationTime), null);
});

test('request history is bounded and malformed rows are ignored', () => {
  const requests = Array.from({length:250},(_,id) => ({id:String(id),model:'gpt-6.1-sol'}));
  assert.equal(recentRequests({requests}).length, 200);
  assert.deepEqual(recentRequests({requests:[null,{id:'bad'},{id:'ok',model:'gpt-6.1-sol'}]}), [{id:'ok',model:'gpt-6.1-sol'}]);
  assert.throws(() => recentRequests({}), /invalid request/);
});
