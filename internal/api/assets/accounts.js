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
  let emailsHidden = false;
  let pageErrorMessage = '';
  let noticeMessage = '';
  try { emailsHidden = sessionStorage.getItem('cliproxy-console-hide-emails') === 'true'; } catch (_) { /* Privacy works without storage. */ }
  const quotas = new Map();
  const loadingQuota = new Set();
  const quotaErrors = new Map();
  const models = new Map();
  const loadingModels = new Set();
  const modelErrors = new Map();
  const expandedModels = new Set();
  const expandedOptions = new Set();
  let pendingFocusKey = '';
  let diagnosticsLoading = false;
  let diagnosticsRows = [];
  let visibleRequests = 20;
  $('hostname').textContent = document.body.dataset.host || location.hostname;

  function setAppearance(value) {
    const theme = ['light', 'dark'].includes(value) ? value : 'system';
    document.documentElement.dataset.theme = theme;
    $('appearance').value = theme;
    try { localStorage.setItem('cliproxy-console-theme', theme); } catch (_) { /* Theme still applies without storage. */ }
  }
  let savedAppearance = 'system';
  try { savedAppearance = localStorage.getItem('cliproxy-console-theme') || 'system'; } catch (_) { /* Follow the device theme. */ }
  setAppearance(savedAppearance);
  $('toggle-privacy').textContent = emailsHidden ? 'Show emails' : 'Hide emails';
  $('toggle-privacy').setAttribute('aria-pressed', String(emailsHidden));

  function accountName(account) {
    return emailsHidden ? `Account ${accounts.findIndex(item => item.id === account.id) + 1}` : account.email;
  }

  function node(tag, className, text) {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text !== undefined) element.textContent = text;
    return element;
  }

  function showError(message) {
    pageErrorMessage = message;
    $('page-error').textContent = privateText(message);
    $('page-error').hidden = !message;
  }

  function notify(message) {
    noticeMessage = message;
    $('notice').textContent = privateText(message);
    $('notice').hidden = !message;
  }

  function privateText(message) {
    if (!emailsHidden) return message;
    return accounts.reduce((text, account) => text.split(account.email).join(accountName(account)), message);
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
    expandedOptions.clear();
    pendingFocusKey = '';
    pageErrorMessage = '';
    noticeMessage = '';
    diagnosticsRows = [];
    visibleRequests = 20;
    try { sessionStorage.removeItem(storageKey); } catch (_) { /* Storage may be unavailable. */ }
    $('management-key').value = '';
    $('workspace').hidden = true;
    $('login-panel').hidden = false;
    $('logout').hidden = true;
    $('toggle-privacy').hidden = true;
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
    const value = node('span', 'quota-value');
    value.append(node('strong', '', `${Math.round(window.remaining)}%`), node('span', '', 'remaining'));
    heading.append(node('span', '', window.label), value);
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
      `${accountName(recommendation.account)} has ${Math.round(recommendation.fiveHour.remaining)}% of 5-hour quota and ${Math.round(recommendation.weekly.remaining)}% weekly quota left. ${dateLabel(recommendation.weekly.resetsAt)}.` :
      'Recommendations need usable 5-hour and weekly quota checked within five minutes. Refresh after a quota reset.';
    $('use-recommended').hidden = !recommendation || recommendation.account.id === selectedId;
    $('use-recommended').disabled = busy || policy?.home_enabled === true;
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
          badge = node('span', 'badge recommendation-badge', 'Recommended');
          badge.title = 'Usable Plus quota with the earliest weekly reset';
          card.querySelector('.account-badges').append(badge);
        }
      }
    }
    return recommendation;
  }

  function render() {
    if (!managementKey || !policy) return;
    const activeElement = document.activeElement;
    const focusKey = activeElement?.dataset?.focusKey || (activeElement === document.body ? pendingFocusKey : '');
    const selected = accounts.find(account => policy.mode === 'manual' && account.id === policy.auth_id);
    $('selected-account').textContent = selected ? accountName(selected) : (policy.mode === 'manual' ? 'Selected account is missing' : 'Chosen automatically');
    const automaticSoonestReset = policy.mode === 'automatic' && policy.strategy === 'soonest-reset';
    const routingLabel = policy.mode === 'manual' ? 'Manual · one account' :
      automaticSoonestReset ? 'Automatic · soonest reset' : 'Automatic · standard routing';
    $('routing-mode').textContent = routingLabel + (policy.applied === false ? ' · pending' : '');
    $('routing-detail').textContent = policy.home_enabled ? 'Account selection is managed by Home. Local automatic and manual selection are unavailable.' :
      policy.applied === false ? 'Saved routing has not reached the running proxy. Refresh accounts before relying on this choice.' :
      policy.mode === 'manual' ? 'Your manual account stays selected until you choose an automatic mode.' :
      automaticSoonestReset ? 'The proxy checks quotas even with this page closed. Choose any account below to return to manual selection.' :
      'Your configured routing strategy chooses accounts. Choose any account below to select it manually.';
    $('use-soonest-reset').disabled = busy || policy.home_enabled === true || automaticSoonestReset;
    $('use-standard-routing').disabled = busy || policy.home_enabled === true || (policy.mode === 'automatic' && !automaticSoonestReset);
    $('use-soonest-reset').setAttribute('aria-pressed', String(automaticSoonestReset));
    $('use-standard-routing').setAttribute('aria-pressed', String(policy.mode === 'automatic' && !automaticSoonestReset));
    $('account-count').textContent = String(accounts.length);
    const checked = Array.from(quotas.values()).map(quota => quota.fetchedAt);
    $('checked-at').textContent = checked.length ? new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(Math.max(...checked)) : 'Not yet';
    $('refresh-accounts').disabled = busy;
    $('toggle-privacy').disabled = busy;
    $('refresh-quotas').disabled = busy || loadingQuota.size > 0 || accounts.length === 0;
    $('empty').hidden = accounts.length !== 0;
    const recommendation = renderRecommendation(false);
    const cards = [];
    for (const account of accounts) {
      const isSelected = selected?.id === account.id;
      const displayName = accountName(account);
      const card = node('article', 'account-card' + (isSelected ? ' selected' : ''));
      card.dataset.accountId = account.id;
      const body = node('div', 'card-body');
      const top = node('div', 'account-top');
      const avatarText = emailsHidden ? String(accounts.indexOf(account) + 1) : account.email.split('@')[0].split(/[._-]/).map(word => word.charAt(0)).slice(0, 2).join('').toUpperCase();
      const badgeText = isSelected ? 'Selected' : account.disabled ? 'Disabled' : account.unavailable ? 'Unavailable' : 'Enabled';
      const badges = node('div', 'account-badges');
      badges.append(node('span', 'badge' + (isSelected ? ' selected' : account.disabled || account.unavailable ? ' warning' : ''), badgeText));
      top.append(node('div', 'avatar', avatarText || 'CX'), badges);
      const quota = quotas.get(account.id);
      if (recommendation?.account.id === account.id) {
        const badge = node('span', 'badge recommendation-badge', 'Recommended');
        badge.title = 'Usable Plus quota with the earliest weekly reset';
        badges.append(badge);
      }
      body.append(top, node('h3', '', displayName), node('p', 'plan', `Codex · ${quota?.plan || account.plan || 'Plan unknown'}`));
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
      const modelSummary = node('summary', '', 'Available models');
      modelSummary.dataset.focusKey = account.id + ':models';
      modelDetails.append(modelSummary);
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
      if ((quota?.plan || account.plan) === 'go') modelDetails.append(node('p', 'status-line', 'Go has a different model catalog. Choose a listed model in your coding tool.'));
      body.append(modelDetails);
      if (account.disabled) body.append(node('p', 'status-line', 'Enable this credential in Advanced settings before selecting it.'));
      else if (account.unavailable) body.append(node('p', 'status-line error', 'The proxy reports this account as unavailable.'));
      const footer = node('div', 'card-footer');
      const select = node('button', 'select-account ' + (isSelected ? 'selected-button' : 'primary'), isSelected ? 'Currently selected' : 'Use this account');
      select.disabled = busy || account.disabled || isSelected || policy.home_enabled === true;
      select.dataset.focusKey = account.id + ':select';
      select.setAttribute('aria-label', isSelected ? `Selected account ${displayName}` : `Use account ${displayName}`);
      select.addEventListener('click', () => selectAccount(account));
      const refresh = node('button', 'refresh-quota', loadingQuota.has(account.id) ? 'Checking…' : 'Refresh');
      refresh.disabled = busy || loadingQuota.has(account.id);
      refresh.dataset.focusKey = account.id + ':refresh';
      refresh.setAttribute('aria-label', `Refresh quota for ${displayName}`);
      refresh.addEventListener('click', () => refreshQuota(account));
      footer.append(select, refresh);
      const remove = node('button', 'remove-account', 'Remove account');
      remove.disabled = busy || isSelected;
      remove.dataset.focusKey = account.id + ':remove';
      remove.setAttribute('aria-label', `Remove account ${displayName}`);
      const options = node('details', 'account-options');
      options.open = expandedOptions.has(account.id);
      const optionsSummary = node('summary', '', 'Account options');
      optionsSummary.dataset.focusKey = account.id + ':options';
      options.append(optionsSummary);
      options.addEventListener('toggle', () => {
        if (!options.isConnected) return;
        if (options.open) expandedOptions.add(account.id);
        else expandedOptions.delete(account.id);
      });
      if (isSelected) {
        remove.title = 'Select another account before removing this one.';
        options.append(node('p', 'status-line', 'Select another account before removing this saved login.'));
      }
      remove.addEventListener('click', () => removeAccount(account));
      options.append(remove);
      body.append(options);
      card.append(body, footer);
      cards.push(card);
    }
    $('account-grid').replaceChildren(...cards);
    pendingFocusKey = '';
    if (focusKey) {
      const replacement = Array.from($('account-grid').querySelectorAll('[data-focus-key]')).find(element => element.dataset.focusKey === focusKey);
      if (replacement && !replacement.disabled) replacement.focus({preventScroll: true});
      else if (replacement) pendingFocusKey = focusKey;
    }
  }

  async function loadAccounts() {
    const [list, routing, server] = await Promise.all([api('/credentials'), api('/routing/codex-account'), api('/server/info')]);
    const parsed = codexAccounts(list?.files);
    if (!routing || !['manual', 'automatic'].includes(routing.mode) ||
        (routing.strategy !== undefined && !['', 'soonest-reset'].includes(routing.strategy))) {
      throw new Error('The proxy returned an invalid routing choice.');
    }
    accounts = parsed;
    policy = routing;
    if (typeof server?.hostname === 'string' && server.hostname) $('hostname').textContent = server.hostname;
    render();
  }

  async function selectAccount(account) {
    if (busy || policy?.home_enabled === true) return;
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

  async function selectAutomaticRouting(strategy) {
    if (busy || policy?.home_enabled === true) return;
    busy = true;
    showError('');
    notify('Updating automatic account selection…');
    render();
    try {
      await api('/routing/codex-account', { method: 'PUT', body: JSON.stringify({ auth_id: null, strategy }) });
      await loadAccounts();
      if (policy.mode !== 'automatic' || (policy.strategy || '') !== strategy || policy.applied === false) {
        throw new Error('Automatic routing has not reached the running proxy. Refresh accounts and try again.');
      }
      notify(strategy === 'soonest-reset' ?
        'Soonest-reset routing is active. New sessions prefer available quota before its reset.' :
        'Standard routing is active. Your configured strategy chooses accounts for new sessions.');
    } catch (error) {
      notify('');
      showError(error.message);
      try { if (managementKey) await loadAccounts(); } catch (_) { /* Preserve the original operation error. */ }
    } finally {
      busy = false;
      render();
    }
  }

  async function removeAccount(account) {
    if (busy || (policy.mode === 'manual' && policy.auth_id === account.id)) return;
    const displayName = accountName(account);
    if (!window.confirm(`Remove ${displayName} from this proxy?\n\nThis deletes its saved login. You will need to sign in again to add it back. Your ChatGPT subscription will not be cancelled.`)) return;
    busy = true;
    showError('');
    notify(`Removing ${displayName}…`);
    render();
    let removed = false;
    try {
      await api('/credentials?name=' + encodeURIComponent(account.name), { method: 'DELETE' });
      removed = true;
      accounts = accounts.filter(item => item.id !== account.id);
      for (const cache of [quotas, quotaErrors, models, modelErrors, expandedModels, expandedOptions]) cache.delete(account.id);
      notify(`${displayName} was removed from this proxy.`);
      await loadAccounts();
    } catch (error) {
      if (!removed) notify('');
      showError(removed ? `The account was removed, but refreshing failed: ${error.message}` : `Could not remove ${displayName}: ${error.message}`);
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
      $('toggle-privacy').hidden = false;
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
    const transportLabel = value => ({websocket: 'WS', http: 'HTTP', sse: 'SSE'}[value] || 'Unknown');
    const cards = rows.slice(0, visibleRequests).map(row => {
      const card = node('article', 'request-row');
      const identity = node('div', 'request-identity');
      const timestamp = new Date(row.requested_at);
      const time = Number.isFinite(timestamp.getTime()) ? new Intl.DateTimeFormat(undefined, {hour:'2-digit', minute:'2-digit', second:'2-digit'}).format(timestamp) : 'Time unknown';
      const knownAccount = accounts.find(account => account.id === row.auth_id);
      const displayName = knownAccount ? accountName(knownAccount) : emailsHidden ? 'Unknown account' : row.account || row.auth_id || 'Unknown account';
      identity.append(node('strong', '', row.model), node('p', 'request-route', `${displayName} · ${time}`));
      card.append(identity, node('span', row.success ? 'request-status request-result' : 'request-status request-result failed', row.success ? 'Completed' : 'Failed'));
      const pairs = [
        ['Transport', `${transportLabel(row.downstream_transport)} → ${transportLabel(row.upstream_transport)}`],
        ['Duration', metric(row.latency_ms, ' ms')],
        ['First token', metric(row.ttft_ms, ' ms')],
        ['Input / output tokens', row.usage_known ? `${metric(row.input_tokens)} / ${metric(row.output_tokens)}` : 'Unknown'],
      ];
      for (const [label, value] of pairs) {
        const pair = node('dl', 'request-cell');
        const detail = node('dd', '', value);
        if (label === 'Input / output tokens' && row.usage_known) detail.append(node('small', '', `${metric(row.cached_input_tokens)} cached input`));
        pair.append(node('dt', 'cell-label', label), detail);
        card.append(pair);
      }
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
  $('appearance').addEventListener('change', event => setAppearance(event.target.value));
  $('display-settings').addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      $('display-settings').open = false;
      $('display-settings').querySelector('summary').focus();
    }
  });
  $('toggle-privacy').addEventListener('click', () => {
    emailsHidden = !emailsHidden;
    $('toggle-privacy').textContent = emailsHidden ? 'Show emails' : 'Hide emails';
    $('toggle-privacy').setAttribute('aria-pressed', String(emailsHidden));
    try { sessionStorage.setItem('cliproxy-console-hide-emails', String(emailsHidden)); } catch (_) { /* Apply for this page only. */ }
    notify(noticeMessage);
    showError(pageErrorMessage);
    render();
    renderRequests(diagnosticsRows);
  });

  $('use-soonest-reset').addEventListener('click', () => selectAutomaticRouting('soonest-reset'));
  $('use-standard-routing').addEventListener('click', () => selectAutomaticRouting(''));
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
