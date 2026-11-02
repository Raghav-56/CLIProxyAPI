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

test('removing an account confirms, preserves failures, and deletes only the chosen credential', async () => {
  const vm = require('node:vm');
  const fs = require('node:fs');
  class Element {
    constructor() { this.children = []; this.dataset = {}; this.listeners = {}; this.style = {}; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute() {}
    addEventListener(event, handler) { this.listeners[event] = handler; }
    querySelector() { return new Element(); }
    click() { if (!this.disabled) return this.listeners.click?.(); }
  }
  const elements = new Map();
  const get = id => {
    if (!elements.has(id)) elements.set(id, new Element());
    return elements.get(id);
  };
  let files = [
    {id:'selected',name:'selected.json',auth_index:'1',provider:'codex',account_type:'oauth',email:'selected@example.test'},
    {id:'other',name:'other+account.json',auth_index:'2',provider:'codex',account_type:'oauth',email:'other@example.test'},
  ];
  let confirmed = false;
  let failure = false;
  const deleted = [];
  const context = {
    document: {getElementById:get,createElement:() => new Element(),body:{dataset:{host:'Test proxy'}}},
    window: {confirm:() => confirmed},
    sessionStorage: {getItem:() => 'test-key',setItem() {},removeItem() {}},
    setInterval() {},
    fetch: async (path, options) => {
      if (options.method === 'DELETE') {
        deleted.push(path);
        if (failure) return {status:500,ok:false,json:async () => ({error:'disk unavailable'})};
        files = files.filter(file => file.id !== 'other');
      }
      const data = path === '/v8/management/credentials' ? {files} :
        path.endsWith('/routing/codex-account') ? {mode:'manual',auth_id:'selected'} :
        path.endsWith('/server/info') ? {hostname:'Test proxy'} :
        path.endsWith('/observability/requests/recent') ? {requests:[]} : {};
      return {status:200,ok:true,json:async () => data};
    },
  };
  vm.runInNewContext(fs.readFileSync(require.resolve('./accounts.js'), 'utf8'), context);
  const settle = () => new Promise(resolve => setImmediate(resolve));
  const card = id => get('account-grid').children.find(item => item.dataset.accountId === id);
  const remove = id => card(id).children[1].children.find(item => item.className === 'remove-account');
  await settle();
  assert.equal(remove('selected').disabled, true);
  await remove('selected').click();
  assert.equal(deleted.length, 0);
  await remove('other').click();
  assert.equal(deleted.length, 0);
  assert.ok(card('other'));
  confirmed = true;
  failure = true;
  await remove('other').click();
  assert.ok(card('other'));
  assert.match(get('page-error').textContent, /disk unavailable/);
  failure = false;
  await remove('other').click();
  assert.equal(card('other'), undefined);
  assert.equal(get('account-count').textContent, '1');
  assert.match(get('notice').textContent, /other@example.test was removed/);
  assert.deepEqual(deleted, [
    '/v8/management/credentials?name=other%2Baccount.json',
    '/v8/management/credentials?name=other%2Baccount.json',
  ]);
});

test('routing controls switch automatic and manual modes, verify application, and respect Home', async () => {
  const vm = require('node:vm');
  const fs = require('node:fs');
  class Element {
    constructor() { this.children = []; this.dataset = {}; this.listeners = {}; this.style = {}; this.attributes = {}; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute(name, value) { this.attributes[name] = value; }
    addEventListener(event, handler) { this.listeners[event] = handler; }
    querySelector() { return new Element(); }
    click() { if (!this.disabled) return this.listeners.click?.(); }
  }
  const elements = new Map();
  const get = id => {
    if (!elements.has(id)) elements.set(id, new Element());
    return elements.get(id);
  };
  const files = ['first', 'second'].map((id, index) => ({
    id, name:id + '.json', auth_index:String(index), provider:'codex', account_type:'oauth', email:id + '@example.test',
  }));
  const routing = {mode:'manual', auth_id:'first', strategy:'', applied:true, home_enabled:false};
  const puts = [];
  let failSave = false;
  let apply = true;
  const context = {
    document:{getElementById:get, createElement:() => new Element(), body:{dataset:{host:'Test proxy'}}},
    window:{confirm:() => false},
    sessionStorage:{getItem:() => 'test-key', setItem() {}, removeItem() {}},
    setInterval() {},
    fetch:async (path, options) => {
      if (path.endsWith('/routing/codex-account') && options.method === 'PUT') {
        const choice = JSON.parse(options.body);
        puts.push(choice);
        if (failSave) return {status:500, ok:false, json:async () => ({error:'disk unavailable'})};
        routing.auth_id = choice.auth_id;
        routing.mode = choice.auth_id ? 'manual' : 'automatic';
        if ('strategy' in choice) routing.strategy = choice.strategy;
        routing.applied = apply;
      }
      const data = path.endsWith('/credentials') ? {files} :
        path.endsWith('/routing/codex-account') ? {...routing} :
        path.endsWith('/server/info') ? {hostname:'Test proxy'} :
        path.endsWith('/observability/requests/recent') ? {requests:[]} :
        path.endsWith('/requests/api-call') ? {status_code:200, body:'{"rate_limit":{}}'} : {};
      return {status:200, ok:true, json:async () => data};
    },
  };
  vm.runInNewContext(fs.readFileSync(require.resolve('./accounts.js'), 'utf8'), context);
  await new Promise(resolve => setImmediate(resolve));
  const select = id => get('account-grid').children.find(card => card.dataset.accountId === id)
    .children[1].children.find(button => button.className.startsWith('select-account '));

  assert.equal(get('routing-mode').textContent, 'Manual · one account');
  await get('use-soonest-reset').click();
  assert.deepEqual(puts[0], {auth_id:null, strategy:'soonest-reset'});
  assert.equal(get('routing-mode').textContent, 'Automatic · soonest reset');
  assert.equal(get('use-soonest-reset').attributes['aria-pressed'], 'true');
  assert.match(get('notice').textContent, /Soonest-reset routing is active/);
  assert.match(get('routing-detail').textContent, /page closed/);

  await select('second').click();
  assert.deepEqual(puts[1], {auth_id:'second'});
  assert.equal(routing.strategy, 'soonest-reset');
  assert.equal(get('routing-mode').textContent, 'Manual · one account');
  assert.equal(get('selected-account').textContent, 'second@example.test');
  assert.equal(get('use-soonest-reset').disabled, false);

  await get('use-standard-routing').click();
  assert.deepEqual(puts[2], {auth_id:null, strategy:''});
  assert.equal(get('routing-mode').textContent, 'Automatic · standard routing');
  assert.equal(get('use-standard-routing').disabled, true);
  failSave = true;
  await get('use-soonest-reset').click();
  assert.equal(get('routing-mode').textContent, 'Automatic · standard routing');
  assert.equal(get('notice').hidden, true);
  assert.match(get('page-error').textContent, /disk unavailable/);

  failSave = false;
  apply = false;
  await get('use-soonest-reset').click();
  assert.equal(get('routing-mode').textContent, 'Automatic · soonest reset · pending');
  assert.equal(get('notice').hidden, true);
  assert.match(get('page-error').textContent, /has not reached the running proxy/);
  assert.match(get('routing-detail').textContent, /has not reached the running proxy/);

  Object.assign(routing, {mode:'manual', auth_id:'first', applied:true, home_enabled:true});
  await get('refresh-accounts').click();
  assert.equal(get('use-soonest-reset').disabled, true);
  assert.equal(get('use-standard-routing').disabled, true);
  assert.equal(get('use-recommended').disabled, true);
  assert.equal(select('second').disabled, true);
  assert.match(get('routing-detail').textContent, /managed by Home/);
  await get('use-soonest-reset').click();
  await get('use-standard-routing').click();
  await select('second').click();
  assert.equal(puts.length, 5);
});
