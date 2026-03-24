import { el, toast, confirm, timeAgo, levelBadge, statusBadge } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// Issues list
router.on('/issues', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Issues'));
  const toolbar = el('div', { className: 'toolbar' });
  let projects = [];
  try { projects = await api.get('/projects'); } catch {}
  const sel = el('select');
  sel.appendChild(el('option', { value: '' }, 'All projects'));
  projects.forEach(p => sel.appendChild(el('option', { value: String(p.id) }, p.name)));
  sel.onchange = () => loadIssues();
  toolbar.appendChild(sel);
  const envInput = el('input', { type: 'text', placeholder: 'Filter by environment...', style: 'width:200px' });
  let envTimer;
  envInput.oninput = () => { clearTimeout(envTimer); envTimer = setTimeout(() => loadIssues(), 300); };
  toolbar.appendChild(envInput);
  frag.appendChild(toolbar);

  const container = el('div');
  frag.appendChild(container);

  async function loadIssues() {
    container.innerHTML = '';
    try {
      const params = [];
      if (sel.value) params.push('project=' + sel.value);
      if (envInput.value.trim()) params.push('environment=' + encodeURIComponent(envInput.value.trim()));
      const q = params.length ? '?' + params.join('&') : '';
      const issues = await api.get('/issues' + q);
      if (!issues || issues.length === 0) {
        container.appendChild(el('div', { className: 'empty' }, 'No issues'));
        return;
      }
      const t = el('table');
      t.appendChild(el('thead', null, el('tr', null,
        el('th', null, 'Issue'), el('th', null, 'Level'), el('th', null, 'Env'),
        el('th', null, 'Status'), el('th', null, 'Events'), el('th', null, 'Last Seen'), el('th', null, 'Actions')
      )));
      const tb = el('tbody');
      issues.forEach(i => {
        tb.appendChild(el('tr', null,
          el('td', null, el('a', { href: '#/issues/' + i.id }, i.title)),
          el('td', null, levelBadge(i.level)),
          el('td', { style: 'color:var(--muted)' }, i.environment || '-'),
          el('td', null, statusBadge(i.status)),
          el('td', { style: 'color:var(--muted)' }, String(i.count)),
          el('td', { style: 'color:var(--muted)' }, timeAgo(i.lastSeen)),
          el('td', null,
            el('button', { className: 'sm', onClick: async () => {
              const newStatus = i.status === 'resolved' ? 'unresolved' : 'resolved';
              await api.put('/issues/' + i.id, { status: newStatus });
              toast('Issue ' + newStatus);
              loadIssues();
            }}, i.status === 'resolved' ? 'Reopen' : 'Resolve'),
            ' ',
            el('button', { className: 'sm danger', onClick: async () => {
              if (await confirm('Delete this issue and all its events?')) {
                await api.del('/issues/' + i.id);
                toast('Issue deleted');
                loadIssues();
              }
            }}, 'Delete')
          )
        ));
      });
      t.appendChild(tb);
      container.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
    } catch (err) { toast(err.message, 'error'); }
  }
  render(frag);
  loadIssues();
});

// Issue detail (enhanced)
router.on('/issues/:id', async (params) => {
  const frag = el('div');
  const back = el('a', { href: '#/issues', style: 'color:var(--muted);font-size:12px' }, '\u2190 Back to issues');
  frag.appendChild(back);

  try {
    const [issue, events] = await Promise.all([
      api.get('/issues/' + params.id),
      api.get('/issues/' + params.id + '/events')
    ]);

    // --- Issue Header ---
    const header = el('div', { className: 'issue-header' });
    header.appendChild(el('h1', null, issue.title || 'Issue ' + params.id));

    const meta = el('div', { className: 'issue-meta' });
    meta.append(
      levelBadge(issue.level),
      statusBadge(issue.status)
    );
    if (issue.environment) {
      meta.append(
        el('span', { className: 'sep' }, '\u00b7'),
        el('span', { className: 'badge badge-info' }, issue.environment)
      );
    }
    meta.append(
      el('span', { className: 'sep' }, '\u00b7'),
      el('span', null, 'First seen ' + timeAgo(issue.firstSeen)),
      el('span', { className: 'sep' }, '\u00b7'),
      el('span', null, 'Last seen ' + timeAgo(issue.lastSeen)),
      el('span', { className: 'sep' }, '\u00b7'),
      el('span', null, issue.count + ' event' + (issue.count !== 1 ? 's' : ''))
    );
    header.appendChild(meta);

    const actions = el('div', { className: 'issue-actions' });
    const toggleBtn = el('button', {
      className: issue.status === 'resolved' ? 'sm' : 'sm primary',
      onClick: async () => {
        const newStatus = issue.status === 'resolved' ? 'unresolved' : 'resolved';
        await api.put('/issues/' + params.id, { status: newStatus });
        toast('Issue ' + newStatus);
        router.dispatch();
      }
    }, issue.status === 'resolved' ? 'Reopen' : 'Resolve');
    actions.appendChild(toggleBtn);

    const deleteBtn = el('button', { className: 'sm danger', onClick: async () => {
      if (await confirm('Delete this issue and all its events?')) {
        await api.del('/issues/' + params.id);
        toast('Issue deleted');
        router.go('/issues');
      }
    }}, 'Delete');
    actions.appendChild(deleteBtn);
    header.appendChild(actions);
    frag.appendChild(header);

    // --- Events ---
    if (!events || events.length === 0) {
      frag.appendChild(el('div', { className: 'empty' }, 'No events for this issue'));
    } else {
      let currentIdx = 0;

      // Event navigator
      const nav = el('div', { className: 'event-nav' });
      const newerBtn = el('button', { className: 'sm', onClick: () => showEvent(currentIdx - 1) }, '\u2190 Newer');
      const olderBtn = el('button', { className: 'sm', onClick: () => showEvent(currentIdx + 1) }, 'Older \u2192');
      const posLabel = el('span', { className: 'event-pos' });
      nav.append(newerBtn, posLabel, olderBtn);
      frag.appendChild(nav);

      const eventContainer = el('div');
      frag.appendChild(eventContainer);

      function showEvent(idx) {
        if (idx < 0 || idx >= events.length) return;
        currentIdx = idx;
        newerBtn.disabled = (idx <= 0);
        olderBtn.disabled = (idx >= events.length - 1);
        posLabel.textContent = 'Event ' + (events.length - idx) + ' of ' + events.length;

        eventContainer.innerHTML = '';
        const ev = events[idx];
        let data;
        try {
          data = typeof ev.context === 'string' ? JSON.parse(ev.context) : (ev.context || {});
        } catch { data = {}; }
        eventContainer.appendChild(renderEventDisplay(ev, data));
      }

      showEvent(0);
    }
  } catch (err) { toast(err.message, 'error'); }
  render(frag);
});

// --- Event Display ---

function renderEventDisplay(ev, data) {
  const wrap = el('div', { className: 'card' });

  // Event meta line
  const meta = el('div', { style: 'display:flex;justify-content:space-between;margin-bottom:16px' });
  meta.appendChild(el('div', null,
    levelBadge(ev.level), ' ',
    el('span', { style: 'color:var(--muted);margin-left:8px' }, ev.eventID?.slice(0, 8)),
    el('span', { style: 'color:var(--muted);margin-left:12px' }, new Date(ev.dateCreated).toLocaleString())
  ));
  meta.appendChild(el('span', { style: 'color:var(--muted)' }, ev.platform || ''));
  wrap.appendChild(meta);

  // Tabs
  const hasException = data?.exception?.values?.length > 0;
  const hasBreadcrumbs = !!(data?.breadcrumbs?.values?.length || (Array.isArray(data?.breadcrumbs) && data.breadcrumbs.length));

  const tabDefs = [
    { id: 'stacktrace', label: 'Stacktrace', render: () => renderStacktrace(data) },
    { id: 'details', label: 'Event Details', render: () => renderEventDetails(data) },
    { id: 'breadcrumbs', label: 'Breadcrumbs', render: () => renderBreadcrumbs(data) },
    { id: 'raw', label: 'Raw JSON', render: () => renderRawJSON(data) },
  ];

  let activeTab = hasException ? 'stacktrace' : 'details';

  const tabs = el('div', { className: 'tabs' });
  const tabContent = el('div');

  function switchTab(id) {
    activeTab = id;
    tabs.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === id));
    tabContent.innerHTML = '';
    const def = tabDefs.find(t => t.id === id);
    if (def) tabContent.appendChild(def.render());
  }

  tabDefs.forEach(t => {
    const btn = el('button', {
      className: 'tab' + (t.id === activeTab ? ' active' : ''),
      onClick: () => switchTab(t.id)
    }, t.label);
    btn.dataset.tab = t.id;
    tabs.appendChild(btn);
  });

  wrap.appendChild(tabs);
  wrap.appendChild(tabContent);

  // Render initial tab
  const def = tabDefs.find(t => t.id === activeTab);
  if (def) tabContent.appendChild(def.render());

  return wrap;
}

// --- Stacktrace ---

function renderStacktrace(data) {
  const wrap = el('div');
  const exceptions = data?.exception?.values;
  if (!exceptions || exceptions.length === 0) {
    return el('div', { className: 'empty' }, 'No stacktrace available');
  }

  exceptions.forEach((exc) => {
    // Exception header
    const excHeader = el('div', { className: 'exc-header' });
    if (exc.type) excHeader.appendChild(el('div', { className: 'exc-type' }, exc.type));
    if (exc.value) excHeader.appendChild(el('div', { className: 'exc-value' }, exc.value));
    wrap.appendChild(excHeader);

    const frames = exc.stacktrace?.frames;
    if (!frames || frames.length === 0) {
      wrap.appendChild(el('div', { style: 'color:var(--muted);padding:12px 0' }, 'No frames'));
      return;
    }

    const frameList = el('div', { className: 'frame-list' });
    const reversed = [...frames].reverse();

    reversed.forEach(frame => {
      const isInApp = frame.in_app === true;
      const frameEl = el('div', {
        className: 'frame' + (isInApp ? ' in-app' : ' library')
      });

      const filename = frame.filename || frame.abs_path || frame.module || '<unknown>';
      const func = frame.function || '<anonymous>';
      const lineno = frame.lineno ? ':' + frame.lineno : '';
      const colno = frame.colno ? ':' + frame.colno : '';

      const headerEl = el('div', { className: 'frame-header', onClick: () => {
        frameEl.classList.toggle('expanded');
      }});
      headerEl.appendChild(el('span', { className: 'frame-fn' }, func));
      headerEl.appendChild(el('span', { className: 'frame-loc' }, filename + lineno + colno));
      frameEl.appendChild(headerEl);

      // Context (expanded content)
      const contextEl = el('div', { className: 'frame-context' });
      let hasContext = false;

      if (frame.pre_context) {
        hasContext = true;
        frame.pre_context.forEach((line, j) => {
          const ln = (frame.lineno || 0) - frame.pre_context.length + j;
          contextEl.appendChild(el('div', { style: 'color:var(--muted)' }, String(ln).padStart(5) + '  ' + line));
        });
      }
      if (frame.context_line) {
        hasContext = true;
        contextEl.appendChild(el('div', { style: 'color:var(--text);font-weight:600;background:rgba(88,166,255,0.08)' },
          String(frame.lineno || '').padStart(5) + '  ' + frame.context_line));
      }
      if (frame.post_context) {
        hasContext = true;
        frame.post_context.forEach((line, j) => {
          const ln = (frame.lineno || 0) + 1 + j;
          contextEl.appendChild(el('div', { style: 'color:var(--muted)' }, String(ln).padStart(5) + '  ' + line));
        });
      }

      if (!hasContext) {
        contextEl.appendChild(el('div', { style: 'color:var(--muted)' }, 'No source context available'));
      }

      frameEl.appendChild(contextEl);

      // Auto-expand in-app frames
      if (isInApp) frameEl.classList.add('expanded');

      frameList.appendChild(frameEl);
    });

    wrap.appendChild(frameList);
  });

  return wrap;
}

// --- Event Details ---

function renderEventDetails(data) {
  const wrap = el('div');

  function section(title, contentFn) {
    const content = contentFn();
    if (!content) return null;
    const container = el('div', { style: 'border-bottom:1px solid var(--border)' });
    const toggle = el('button', { className: 'section-toggle', onClick: () => {
      toggle.classList.toggle('open');
      body.classList.toggle('open');
    }}, title);
    const body = el('div', { className: 'section-body' });
    body.appendChild(content);
    container.append(toggle, body);
    return container;
  }

  function kvTable(pairs) {
    if (!pairs || pairs.length === 0) return null;
    const filtered = pairs.filter(([, v]) => v !== undefined && v !== null && v !== '');
    if (filtered.length === 0) return null;
    const t = el('table', { className: 'kv-table' });
    filtered.forEach(([k, v]) => {
      const val = typeof v === 'object' ? JSON.stringify(v, null, 2) : String(v);
      t.appendChild(el('tr', null,
        el('td', null, k),
        el('td', null, val)
      ));
    });
    return t;
  }

  // Tags
  const tagsSection = section('Tags', () => {
    let tags = data.tags;
    if (!tags) return null;
    let pairs;
    if (Array.isArray(tags)) {
      pairs = tags.map(t => Array.isArray(t) ? t : [t.key || t[0], t.value || t[1]]);
    } else {
      pairs = Object.entries(tags);
    }
    return kvTable(pairs);
  });
  if (tagsSection) wrap.appendChild(tagsSection);

  // User
  const userSection = section('User', () => {
    const u = data.user;
    if (!u) return null;
    return kvTable([
      ['ID', u.id],
      ['Email', u.email],
      ['Username', u.username],
      ['IP Address', u.ip_address],
    ]);
  });
  if (userSection) wrap.appendChild(userSection);

  // Request
  const requestSection = section('Request', () => {
    const req = data.request;
    if (!req) return null;
    const pairs = [
      ['Method', req.method],
      ['URL', req.url],
      ['Query String', req.query_string],
    ];
    if (req.headers) {
      const headers = Array.isArray(req.headers)
        ? req.headers.map(([k, v]) => k + ': ' + v).join('\n')
        : JSON.stringify(req.headers, null, 2);
      pairs.push(['Headers', headers]);
    }
    if (req.data) pairs.push(['Body', typeof req.data === 'object' ? JSON.stringify(req.data, null, 2) : req.data]);
    return kvTable(pairs);
  });
  if (requestSection) wrap.appendChild(requestSection);

  // Contexts
  const ctxSection = section('Contexts', () => {
    const ctx = data.contexts;
    if (!ctx) return null;
    const pairs = [];
    if (ctx.browser) pairs.push(['Browser', (ctx.browser.name || '') + ' ' + (ctx.browser.version || '')]);
    if (ctx.os) pairs.push(['OS', (ctx.os.name || '') + ' ' + (ctx.os.version || '')]);
    if (ctx.runtime) pairs.push(['Runtime', (ctx.runtime.name || '') + ' ' + (ctx.runtime.version || '')]);
    if (ctx.device) pairs.push(['Device', (ctx.device.name || ctx.device.model || '') + ' ' + (ctx.device.family || '')]);
    Object.entries(ctx).forEach(([k, v]) => {
      if (!['browser', 'os', 'runtime', 'device', 'trace'].includes(k)) {
        pairs.push([k, typeof v === 'object' ? JSON.stringify(v) : String(v)]);
      }
    });
    if (pairs.length === 0) return null;
    return kvTable(pairs);
  });
  if (ctxSection) wrap.appendChild(ctxSection);

  // SDK
  const sdkSection = section('SDK', () => {
    const sdk = data.sdk;
    if (!sdk) return null;
    return kvTable([
      ['Name', sdk.name],
      ['Version', sdk.version],
    ]);
  });
  if (sdkSection) wrap.appendChild(sdkSection);

  // Release / Environment
  const envSection = section('Release / Environment', () => {
    const pairs = [];
    if (data.release) pairs.push(['Release', data.release]);
    if (data.environment) pairs.push(['Environment', data.environment]);
    if (data.server_name) pairs.push(['Server', data.server_name]);
    if (pairs.length === 0) return null;
    return kvTable(pairs);
  });
  if (envSection) wrap.appendChild(envSection);

  if (!wrap.children.length) {
    wrap.appendChild(el('div', { className: 'empty' }, 'No structured event details available'));
  }

  return wrap;
}

// --- Breadcrumbs ---

function renderBreadcrumbs(data) {
  const crumbs = data?.breadcrumbs?.values || (Array.isArray(data?.breadcrumbs) ? data.breadcrumbs : null);
  if (!crumbs || crumbs.length === 0) {
    return el('div', { className: 'empty' }, 'No breadcrumbs');
  }

  const t = el('table');
  t.appendChild(el('thead', null, el('tr', null,
    el('th', null, 'Time'),
    el('th', null, 'Category'),
    el('th', null, 'Message'),
    el('th', null, 'Level')
  )));
  const tb = el('tbody');
  crumbs.forEach(bc => {
    let tsStr = '-';
    if (bc.timestamp) {
      const ts = Number(bc.timestamp);
      if (!isNaN(ts)) {
        tsStr = new Date(ts > 1e15 ? ts : ts > 1e12 ? ts : ts * 1000).toLocaleTimeString();
      } else {
        tsStr = new Date(bc.timestamp).toLocaleTimeString();
      }
    }
    tb.appendChild(el('tr', { className: 'breadcrumb-row' },
      el('td', { className: 'breadcrumb-ts' }, tsStr),
      el('td', { className: 'breadcrumb-cat' }, bc.category || '-'),
      el('td', { className: 'breadcrumb-msg' }, bc.message || (bc.data ? JSON.stringify(bc.data) : '-')),
      el('td', null, bc.level ? levelBadge(bc.level) : el('span', { style: 'color:var(--muted)' }, '-'))
    ));
  });
  t.appendChild(tb);
  return el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t);
}

// --- Raw JSON ---

function renderRawJSON(data) {
  const jsonDiv = el('div', { className: 'json-view' });
  try {
    jsonDiv.textContent = JSON.stringify(data, null, 2);
  } catch {
    jsonDiv.textContent = String(data);
  }
  return jsonDiv;
}
