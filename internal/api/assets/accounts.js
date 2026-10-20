(function () {
  'use strict';

  function numeric(value) {
    return typeof value === 'number' && Number.isFinite(value) ? value : null;
  }

  function windowLabel(seconds) {
    if (seconds === 18000) return '5-hour quota';
    if (seconds === 604800) return 'Weekly quota';
    if (seconds && seconds % 86400 === 0) return `${seconds / 86400}-day quota`;
    if (seconds && seconds % 3600 === 0) return `${seconds / 3600}-hour quota`;
    return seconds ? `${Math.round(seconds / 60)}-minute quota` : 'Quota window';
  }

  function parseQuota(payload, fetchedAt) {
    if (!payload || typeof payload !== 'object' || !payload.rate_limit || typeof payload.rate_limit !== 'object') {
      throw new Error('The provider returned an unrecognized quota response.');
    }
    const windows = [];
    for (const key of ['primary_window', 'secondary_window']) {
      const source = payload.rate_limit[key];
      if (!source || typeof source !== 'object') continue;
      const used = numeric(source.used_percent);
      if (used === null || used < 0) continue;
      const seconds = numeric(source.limit_window_seconds);
      const resetAt = numeric(source.reset_at);
      const resetAfter = numeric(source.reset_after_seconds);
      const resetsAt = resetAt !== null && resetAt > 0 ? resetAt * 1000 :
        resetAfter !== null && resetAfter >= 0 ? fetchedAt + resetAfter * 1000 : null;
      windows.push({
        key,
        seconds: seconds !== null && seconds > 0 ? seconds : null,
        label: windowLabel(seconds !== null && seconds > 0 ? seconds : null),
        remaining: Math.max(0, 100 - used),
        resetsAt,
      });
    }
    return {
      windows,
      plan: typeof payload.plan_type === 'string' ? payload.plan_type : '',
      fetchedAt,
      limitReached: payload.rate_limit.limit_reached === true,
    };
  }

  function codexAccounts(files) {
    if (!Array.isArray(files)) throw new Error('The proxy returned an invalid account list.');
    return files.filter(file => file && String(file.provider || file.type).toLowerCase() === 'codex' &&
      file.account_type === 'oauth' && !file.runtime_only && typeof file.id === 'string' &&
      typeof file.name === 'string' && typeof file.auth_index === 'string').map(file => ({
      id: file.id,
      name: file.name,
      authIndex: file.auth_index,
      email: typeof file.email === 'string' && file.email ? file.email : file.name,
      label: typeof file.label === 'string' ? file.label : '',
      disabled: file.disabled === true,
      unavailable: file.unavailable === true,
      accountId: typeof file.id_token?.chatgpt_account_id === 'string' ? file.id_token.chatgpt_account_id : '',
      plan: typeof file.id_token?.plan_type === 'string' ? file.id_token.plan_type : '',
    }));
  }

  function accountModels(payload) {
    if (!Array.isArray(payload?.models)) throw new Error('The provider returned an invalid model catalog.');
    const slugs = payload.models.filter(model => model && model.supported_in_api === true && model.visibility === 'list' && typeof model.slug === 'string').map(model => model.slug);
    if (!slugs.length) throw new Error('No visible API models were reported for this account.');
    return [...new Set(slugs)];
  }

  function recommendPlusAccount(accounts, quotas, now) {
    const candidates = [];
    for (const account of accounts) {
      const quota = quotas.get(account.id);
      if (account.disabled || account.unavailable || !quota || quota.limitReached ||
          (quota.plan || account.plan).toLowerCase() !== 'plus' ||
          quota.fetchedAt > now || now - quota.fetchedAt > 300000) continue;
      const fiveHour = quota.windows.find(window => window.seconds === 18000);
      const weekly = quota.windows.find(window => window.seconds === 604800);
      if (!fiveHour || !weekly || fiveHour.remaining <= 0 || weekly.remaining <= 0 ||
          !fiveHour.resetsAt || !weekly.resetsAt || fiveHour.resetsAt <= now || weekly.resetsAt <= now) continue;
      candidates.push({account, fiveHour, weekly});
    }
    candidates.sort((a, b) => a.weekly.resetsAt - b.weekly.resetsAt ||
      b.fiveHour.remaining - a.fiveHour.remaining || b.weekly.remaining - a.weekly.remaining ||
      a.account.id.localeCompare(b.account.id));
    return candidates[0] || null;
  }

  function recentRequests(payload) {
    if (!Array.isArray(payload?.requests)) throw new Error('The proxy returned invalid request history.');
    return payload.requests.slice(0, 200).filter(row => row && typeof row.id === 'string' && typeof row.model === 'string');
  }

  if (typeof module !== 'undefined' && module.exports) module.exports = { parseQuota, windowLabel, codexAccounts, accountModels, recommendPlusAccount, recentRequests };
  if (typeof document === 'undefined') return;

  const $ = id => document.getElementById(id);
  const storageKey = 'cliproxy-account-console-key';
  let managementKey = '';
  let accounts = [];
  let policy = null;
  let busy = false;
  const quotas = new Map();
  const loadingQuota = new Set();
  const quotaErrors = new Map();
  const models = new Map();
  const loadingModels = new Set();
  const modelErrors = new Map();
  const expandedModels = new Set();
  let diagnosticsLoading = false;
  let diagnosticsRows = [];
  let visibleRequests = 20;
  $('hostname').textContent = document.body.dataset.host || location.hostname;

  function node(tag, className, text) {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text !== undefined) element.textContent = text;
    return element;
  }

  function showError(message) {
    $('page-error').textContent = message;
    $('page-error').hidden = !message;
  }

  function notify(message) {
    $('notice').textContent = message;
    $('notice').hidden = !message;
  }

  function disconnect() {
    managementKey = '';
    accounts = [];
    policy = null;
    quotas.clear();
    quotaErrors.clear();
    models.clear();
    modelErrors.clear();
    expandedModels.clear();
    diagnosticsRows = [];
    visibleRequests = 20;
    try { sessionStorage.removeItem(storageKey); } catch (_) { /* Storage may be unavailable. */ }
    $('management-key').value = '';
    $('workspace').hidden = true;
    $('login-panel').hidden = false;
    $('logout').hidden = true;
    $('account-grid').replaceChildren();
    $('request-list').replaceChildren();
  }

  async function api(path, options) {
    const response = await fetch('/v8/management' + path, {
      ...options,
      headers: { Authorization: 'Bearer ' + managementKey, 'Content-Type': 'application/json' },
      cache: 'no-store',
    });
    const data = await response.json().catch(() => null);
    if (response.status === 401) {
      disconnect();
      throw new Error('The management key was rejected. Connect again with the correct key.');
    }
    if (!response.ok) {
      throw new Error(typeof data?.error === 'string' ? data.error : `The proxy returned HTTP ${response.status}.`);
    }
    return data;
  }

  function dateLabel(timestamp) {
    if (!timestamp || !Number.isFinite(new Date(timestamp).getTime())) return 'Reset time unknown';
    const date = new Date(timestamp);
    const absolute = new Intl.DateTimeFormat(undefined, { weekday: 'short', hour: '2-digit', minute: '2-digit' }).format(date);
    const minutes = Math.max(0, Math.ceil((timestamp - Date.now()) / 60000));
    const relative = minutes < 1 ? 'now' : minutes < 60 ? `in ${minutes}m` :
      minutes < 1440 ? `in ${Math.ceil(minutes / 60)}h` : `in ${Math.ceil(minutes / 1440)}d`;
    return `Resets ${absolute} · ${relative}`;
  }

  function quotaWindow(window) {
    const section = node('div', 'quota-window' + (window.remaining <= 15 ? ' low' : ''));
    const heading = node('div', 'quota-heading');
    heading.append(node('span', '', window.label), node('span', 'quota-value', `${Math.round(window.remaining)}% left`));
    const track = node('div', 'quota-track');
    track.setAttribute('role', 'progressbar');
    track.setAttribute('aria-label', `${window.label} remaining`);
    track.setAttribute('aria-valuemin', '0');
    track.setAttribute('aria-valuemax', '100');
    track.setAttribute('aria-valuenow', String(Math.min(100, window.remaining)));
    const fill = node('div', 'quota-fill');
    fill.style.width = Math.min(100, window.remaining) + '%';
    track.append(fill);
    section.append(heading, track, node('div', 'quota-reset', dateLabel(window.resetsAt)));
    return section;
  }

  function renderRecommendation(updateCards = true) {
    const selectedId = policy?.mode === 'manual' ? policy.auth_id : null;
    const recommendation = recommendPlusAccount(accounts, quotas, Date.now());
    $('recommendation-title').textContent = recommendation ?
      (recommendation.account.id === selectedId ? 'Your selected Plus account is recommended' : 'A Plus account is ready for your next task') : 'Check Plus quotas for a recommendation';
    $('recommendation-detail').textContent = recommendation ?
      `${recommendation.account.email} · ${Math.round(recommendation.fiveHour.remaining)}% of 5-hour quota left · ${Math.round(recommendation.weekly.remaining)}% weekly left. ${dateLabel(recommendation.weekly.resetsAt)}.` :
      'Recommendations need usable 5-hour and weekly quota checked within five minutes. Refresh after a quota reset.';
    $('use-recommended').hidden = !recommendation || recommendation.account.id === selectedId;
    $('use-recommended').disabled = busy;
    $('use-recommended').onclick = recommendation ? () => {
      const current = recommendPlusAccount(accounts, quotas, Date.now());
      if (current?.account.id !== recommendation.account.id) {
        renderRecommendation();
        notify('The recommendation changed. Refresh quotas before choosing an account.');
        return;
      }
      selectAccount(current.account);
    } : null;
    if (updateCards) {
      for (const card of $('account-grid').children) {
        let badge = card.querySelector('.recommendation-badge');
        if (recommendation?.account.id !== card.dataset.accountId) {
          badge?.remove();
        } else if (!badge) {
          badge = node('p', 'recommendation-badge', 'Recommended · weekly quota resets first');
          card.querySelector('.account-top').after(badge);
        }
      }
    }
    return recommendation;
  }

  function render() {
    if (!managementKey || !policy) return;
    const selected = accounts.find(account => policy.mode === 'manual' && account.id === policy.auth_id);
    $('selected-account').textContent = selected?.email || (policy.mode === 'manual' ? 'Selected account is missing' : 'No manual selection');
    $('routing-mode').textContent = policy.mode === 'manual' ? 'Manual · one account' : 'Existing routing strategy';
    $('account-count').textContent = String(accounts.length);
    const checked = Array.from(quotas.values()).map(quota => quota.fetchedAt);
    $('checked-at').textContent = checked.length ? new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(Math.max(...checked)) : 'Not yet';
    $('refresh-accounts').disabled = busy;
    $('refresh-quotas').disabled = busy || loadingQuota.size > 0 || accounts.length === 0;
    $('empty').hidden = accounts.length !== 0;
    const recommendation = renderRecommendation(false);
    const cards = [];
    for (const account of accounts) {
      const isSelected = selected?.id === account.id;
      const card = node('article', 'account-card' + (isSelected ? ' selected' : ''));
      card.dataset.accountId = account.id;
      const body = node('div', 'card-body');
      const top = node('div', 'account-top');
      const avatarText = account.email.split('@')[0].split(/[._-]/).map(word => word.charAt(0)).slice(0, 2).join('').toUpperCase();
      const badgeText = isSelected ? 'Selected' : account.disabled ? 'Disabled' : account.unavailable ? 'Unavailable' : 'Ready';
      top.append(node('div', 'avatar', avatarText || 'CX'), node('span', 'badge' + (isSelected ? ' selected' : ''), badgeText));
      const quota = quotas.get(account.id);
      if (recommendation?.account.id === account.id) body.append(node('p', 'recommendation-badge', 'Recommended · weekly quota resets first'));
      body.append(top, node('h3', '', account.email), node('p', 'plan', `Codex · ${quota?.plan || account.plan || 'Plan unknown'}`));
      if (quota?.windows.length) {
        for (const window of quota.windows) body.append(quotaWindow(window));
      } else {
        body.append(node('div', 'quota-unknown', loadingQuota.has(account.id) ? 'Checking quota…' :
          quota ? 'The provider did not report a quota window.' : 'Quota has not been checked yet.'));
      }
      if (quotaErrors.has(account.id)) body.append(node('p', 'quota-error', quotaErrors.get(account.id)));
      if (quota) body.append(node('p', 'quota-time', `Checked ${new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(quota.fetchedAt)}`));
      const modelDetails = node('details', 'model-details');
      modelDetails.open = expandedModels.has(account.id);
      modelDetails.append(node('summary', '', 'Available models'));
      if (models.has(account.id)) {
        const list = node('ul', 'model-list');
        for (const model of models.get(account.id)) list.append(node('li', '', model));
        modelDetails.append(list);
      } else {
        modelDetails.append(node('p', 'muted small', loadingModels.has(account.id) ? 'Checking model access…' : modelErrors.get(account.id) || 'Open this section to check model access.'));
        if (modelErrors.has(account.id)) {
          const retry = node('button', 'quiet', 'Retry model check');
          retry.addEventListener('click', () => refreshModels(account));
          modelDetails.append(retry);
        }
      }
      modelDetails.addEventListener('toggle', () => {
        if (!modelDetails.isConnected) return;
        if (modelDetails.open) {
          expandedModels.add(account.id);
          if (!models.has(account.id) && !loadingModels.has(account.id) && !modelErrors.has(account.id)) refreshModels(account);
        } else expandedModels.delete(account.id);
      });
      body.append(modelDetails);
      if ((quota?.plan || account.plan) === 'go') body.append(node('p', 'status-line', 'Go has a different model catalog. Choose a listed model in your coding tool.'));
      if (account.disabled) body.append(node('p', 'status-line', 'Enable this credential in Advanced settings before selecting it.'));
      else if (account.unavailable) body.append(node('p', 'status-line error', 'The proxy reports this account as unavailable.'));
      const footer = node('div', 'card-footer');
      const select = node('button', 'select-account ' + (isSelected ? 'selected-button' : 'primary'), isSelected ? 'Currently selected' : 'Use this account');
      select.disabled = busy || account.disabled || isSelected;
      select.setAttribute('aria-label', isSelected ? `Selected account ${account.email}` : `Use account ${account.email}`);
      select.addEventListener('click', () => selectAccount(account));
      const refresh = node('button', 'refresh-quota', loadingQuota.has(account.id) ? 'Checking…' : 'Refresh');
      refresh.disabled = busy || loadingQuota.has(account.id);
      refresh.setAttribute('aria-label', `Refresh quota for ${account.email}`);
      refresh.addEventListener('click', () => refreshQuota(account));
      footer.append(select, refresh);
      card.append(body, footer);
      cards.push(card);
    }
    $('account-grid').replaceChildren(...cards);
  }

  async function loadAccounts() {
    const [list, routing, server] = await Promise.all([api('/credentials'), api('/routing/codex-account'), api('/server/info')]);
    const parsed = codexAccounts(list?.files);
    if (!routing || !['manual', 'automatic'].includes(routing.mode)) throw new Error('The proxy returned an invalid routing choice.');
    accounts = parsed;
    policy = routing;
    if (typeof server?.hostname === 'string' && server.hostname) $('hostname').textContent = server.hostname;
    render();
  }

  async function selectAccount(account) {
    if (busy) return;
    busy = true;
    showError('');
    notify(`Selecting ${account.email}…`);
    render();
    try {
      await api('/routing/codex-account', { method: 'PUT', body: JSON.stringify({ auth_id: account.id }) });
      await loadAccounts();
      if (policy.mode !== 'manual' || policy.auth_id !== account.id || policy.applied === false) {
        throw new Error('The selected account has not reached the running proxy. Refresh accounts and try again.');
      }
      notify(`${account.email} is selected. Your tools keep the same proxy address.`);
    } catch (error) {
      notify('');
      showError(error.message);
      try { if (managementKey) await loadAccounts(); } catch (_) { /* Preserve the original operation error. */ }
    } finally {
      busy = false;
      render();
    }
  }

  async function refreshQuota(account) {
    if (loadingQuota.has(account.id)) return;
    loadingQuota.add(account.id);
    quotaErrors.delete(account.id);
    render();
    try {
      const headers = { Authorization: 'Bearer $TOKEN$', Accept: 'application/json' };
      if (account.accountId) headers['ChatGPT-Account-Id'] = account.accountId;
      const result = await api('/requests/api-call', { method: 'POST', body: JSON.stringify({
        auth_index: account.authIndex,
        method: 'GET',
        url: 'https://chatgpt.com/backend-api/wham/usage',
        header: headers,
      }) });
      if (result?.status_code !== 200) throw new Error(`Quota check returned HTTP ${result?.status_code || 'unknown'}. You can retry or refresh the OAuth login in Advanced settings.`);
      let payload;
      try { payload = JSON.parse(result.body); } catch (_) { throw new Error('The provider returned an invalid quota response.'); }
      const quota = parseQuota(payload, Date.now());
      if (managementKey && accounts.some(current => current.id === account.id)) quotas.set(account.id, quota);
    } catch (error) {
      if (managementKey) {
        quotas.delete(account.id);
        quotaErrors.set(account.id, error.message);
      }
    } finally {
      loadingQuota.delete(account.id);
      render();
    }
  }

  async function refreshModels(account) {
    if (loadingModels.has(account.id)) return;
    loadingModels.add(account.id);
    modelErrors.delete(account.id);
    render();
    try {
      const headers = { Authorization: 'Bearer $TOKEN$', Accept: 'application/json' };
      if (account.accountId) headers['ChatGPT-Account-Id'] = account.accountId;
      const result = await api('/requests/api-call', {method:'POST', body:JSON.stringify({
        auth_index:account.authIndex, method:'GET',
        url:'https://chatgpt.com/backend-api/codex/models?client_version=0.162.1', header:headers,
      })});
      if (result?.status_code !== 200) throw new Error(`Model check returned HTTP ${result?.status_code || 'unknown'}.`);
      const slugs = accountModels(JSON.parse(result.body));
      if (managementKey) models.set(account.id, slugs);
    } catch (error) {
      if (managementKey) modelErrors.set(account.id, error.message);
    } finally {
      loadingModels.delete(account.id);
      render();
    }
  }

  async function connect() {
    const submit = $('login-form').querySelector('button');
    submit.disabled = true;
    submit.textContent = 'Connecting…';
    $('login-error').hidden = true;
    try {
      await loadAccounts();
      $('login-panel').hidden = true;
      $('workspace').hidden = false;
      $('logout').hidden = false;
      if ($('remember').checked) {
        try { sessionStorage.setItem(storageKey, managementKey); } catch (_) { /* Connected without storage. */ }
      }
      $('management-key').value = '';
      await Promise.all([Promise.all(accounts.map(account => refreshQuota(account))), refreshDiagnostics()]);
    } catch (error) {
      $('login-error').textContent = error.message;
      $('login-error').hidden = false;
    } finally {
      submit.disabled = false;
      submit.textContent = 'Connect';
    }
  }

  function metric(value, suffix = '') {
    return typeof value === 'number' && Number.isFinite(value) ? new Intl.NumberFormat().format(Math.round(value)) + suffix : 'Unknown';
  }

  function renderRequests(rows) {
    const cards = rows.slice(0, visibleRequests).map(row => {
      const card = node('article', 'request-row');
      const title = node('div', 'request-title');
      const timestamp = new Date(row.requested_at);
      const time = Number.isFinite(timestamp.getTime()) ? new Intl.DateTimeFormat(undefined, {hour:'2-digit', minute:'2-digit', second:'2-digit'}).format(timestamp) : 'Time unknown';
      title.append(node('strong', '', row.model), node('span', row.success ? 'request-result' : 'request-result failed', row.success ? 'Completed' : 'Failed'), node('span', 'muted small', time));
      const route = node('p', 'request-route');
      const knownAccount = accounts.find(account => account.id === row.auth_id);
      route.textContent = `${knownAccount?.email || row.account || row.auth_id || 'Account unknown'} · ${row.provider || 'Provider unknown'}`;
      const metrics = node('dl', 'request-metrics');
      const pairs = [
        ['Transport', `${row.downstream_transport || 'unknown'} → ${row.upstream_transport || 'unknown'}`],
        ['Duration', metric(row.latency_ms, ' ms')],
        ['First token', metric(row.ttft_ms, ' ms')],
        ['Cached input', row.usage_known ? metric(row.cached_input_tokens) : 'Unknown'],
        ['Input / output', row.usage_known ? `${metric(row.input_tokens)} / ${metric(row.output_tokens)}` : 'Unknown'],
      ];
      for (const [label, value] of pairs) {
        const pair = node('div');
        pair.append(node('dt', '', label), node('dd', '', value));
        metrics.append(pair);
      }
      card.append(title, route, metrics);
      return card;
    });
    $('request-list').replaceChildren(...cards);
    $('requests-empty').hidden = rows.length !== 0;
    $('more-requests').hidden = rows.length <= visibleRequests;
  }

  async function refreshDiagnostics() {
    if (!managementKey || diagnosticsLoading) return;
    diagnosticsLoading = true;
    $('refresh-requests').disabled = true;
    $('requests-error').hidden = true;
    try {
      const payload = await api('/observability/requests/recent');
      if (!managementKey) return;
      diagnosticsRows = recentRequests(payload);
      renderRequests(diagnosticsRows);
      $('requests-host').textContent = payload.host || $('hostname').textContent;
      $('requests-updated').textContent = `Updated ${new Intl.DateTimeFormat(undefined, {hour:'2-digit', minute:'2-digit', second:'2-digit'}).format(Date.now())}`;
    } catch (error) {
      if (managementKey) {
        $('requests-error').textContent = error.message;
        $('requests-error').hidden = false;
      }
    } finally {
      diagnosticsLoading = false;
      $('refresh-requests').disabled = false;
    }
  }

  $('login-form').addEventListener('submit', event => {
    event.preventDefault();
    managementKey = $('management-key').value.trim();
    if (managementKey) connect();
  });
  $('logout').addEventListener('click', disconnect);
  $('refresh-accounts').addEventListener('click', async () => {
    busy = true;
    showError('');
    render();
    try { await loadAccounts(); } catch (error) { showError(error.message); }
    finally { busy = false; render(); }
  });
  $('refresh-quotas').addEventListener('click', () => Promise.all(accounts.map(account => refreshQuota(account))));
  $('refresh-requests').addEventListener('click', refreshDiagnostics);
  $('more-requests').addEventListener('click', () => {
    visibleRequests = Math.min(200, visibleRequests + 20);
    renderRequests(diagnosticsRows);
  });
  setInterval(() => {
    if (managementKey && !document.hidden) {
      renderRecommendation();
      refreshDiagnostics();
    }
  }, 15000);
  try { managementKey = sessionStorage.getItem(storageKey) || ''; } catch (_) { /* Start disconnected. */ }
  if (managementKey) {
    $('remember').checked = true;
    connect();
  }
})();
