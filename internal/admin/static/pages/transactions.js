import { el, toast, timeAgo, statusBadge } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// Transactions list
router.on('/transactions', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Transactions'));
  const toolbar = el('div', { className: 'toolbar' });
  let projects = [];
  try { projects = await api.get('/projects'); } catch {}
  const sel = el('select');
  sel.appendChild(el('option', { value: '' }, 'All projects'));
  projects.forEach(p => sel.appendChild(el('option', { value: String(p.id) }, p.name)));
  sel.onchange = () => loadTxns();
  toolbar.appendChild(sel);
  frag.appendChild(toolbar);

  const container = el('div');
  frag.appendChild(container);

  async function loadTxns() {
    container.innerHTML = '';
    try {
      const q = sel.value ? '?project=' + sel.value : '';
      const txns = await api.get('/transactions' + q);
      if (!txns || txns.length === 0) {
        container.appendChild(el('div', { className: 'empty' }, 'No transactions'));
      } else {
        const t = el('table');
        t.appendChild(el('thead', null, el('tr', null,
          el('th', null, 'Transaction'), el('th', null, 'Op'), el('th', null, 'Duration'),
          el('th', null, 'Status'), el('th', null, 'Time')
        )));
        const tb = el('tbody');
        txns.forEach(tx => {
          tb.appendChild(el('tr', null,
            el('td', null, el('a', { href: '#/transactions/' + tx.id }, tx.transaction || tx.name || '-')),
            el('td', { style: 'color:var(--muted)' }, tx.op || '-'),
            el('td', null, tx.duration ? tx.duration.toFixed(1) + 'ms' : '-'),
            el('td', null, statusBadge(tx.status || 'ok')),
            el('td', { style: 'color:var(--muted)' }, timeAgo(tx.startTimestamp))
          ));
        });
        t.appendChild(tb);
        container.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
      }
    } catch (err) { toast(err.message, 'error'); }
  }

  await loadTxns();
  render(frag);
});

// Transaction detail
router.on('/transactions/:id', async (params) => {
  const frag = el('div');
  const back = el('a', { href: '#/transactions', style: 'color:var(--muted);font-size:12px' }, '\u2190 Back to transactions');
  frag.appendChild(back);
  const heading = el('h1', { style: 'margin-top:8px' }, 'Transaction ' + params.id);
  frag.appendChild(heading);
  try {
    const [txn, spans] = await Promise.all([
      api.get('/transactions/' + params.id),
      api.get('/transactions/' + params.id + '/spans')
    ]);
    if (txn.transaction || txn.name) heading.textContent = 'Transaction ' + params.id + ' \u2014 ' + (txn.transaction || txn.name);
    // Info card
    const info = el('div', { className: 'card' });
    const grid = el('div', { style: 'display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px;margin-bottom:12px' });
    const field = (label, val) => {
      const d = el('div');
      d.appendChild(el('div', { style: 'color:var(--muted);font-size:11px;margin-bottom:2px' }, label));
      d.appendChild(el('div', null, val || '-'));
      return d;
    };
    grid.appendChild(field('Operation', txn.op));
    grid.appendChild(field('Duration', txn.duration ? txn.duration.toFixed(1) + 'ms' : '-'));
    grid.appendChild(field('Status', txn.status || 'ok'));
    grid.appendChild(field('Trace ID', txn.traceID));
    grid.appendChild(field('Span ID', txn.spanID));
    grid.appendChild(field('Timestamp', txn.startTimestamp ? new Date(txn.startTimestamp).toLocaleString() : '-'));
    info.appendChild(grid);
    frag.appendChild(info);
    // Spans
    if (spans && spans.length > 0) {
      frag.appendChild(el('h2', { style: 'margin:20px 0 12px' }, 'Spans (' + spans.length + ')'));
      const t = el('table');
      t.appendChild(el('thead', null, el('tr', null,
        el('th', null, 'Op'), el('th', null, 'Description'), el('th', null, 'Duration'),
        el('th', null, 'Status'), el('th', null, 'Time')
      )));
      const tb = el('tbody');
      spans.forEach(s => {
        tb.appendChild(el('tr', null,
          el('td', null, s.op || '-'),
          el('td', null, s.description || '-'),
          el('td', null, s.duration ? s.duration.toFixed(1) + 'ms' : '-'),
          el('td', null, statusBadge(s.status || 'ok')),
          el('td', { style: 'color:var(--muted)' }, timeAgo(s.startTimestamp))
        ));
      });
      t.appendChild(tb);
      frag.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
    }
    // Raw JSON
    frag.appendChild(el('h2', { style: 'margin:20px 0 12px' }, 'Raw Transaction'));
    const jsonDiv = el('div', { className: 'json-view' });
    try {
      const ctx = typeof txn.context === 'string' ? JSON.parse(txn.context) : txn.context;
      jsonDiv.textContent = JSON.stringify(ctx, null, 2);
    } catch { jsonDiv.textContent = String(txn.context); }
    frag.appendChild(jsonDiv);
  } catch (err) { toast(err.message, 'error'); }
  render(frag);
});
