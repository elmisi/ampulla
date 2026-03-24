import { el, toast, timeAgo, getSelectedProject, setSelectedProject } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

router.on('/performance', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Performance'));
  const toolbar = el('div', { className: 'toolbar' });
  let projects = [];
  try { projects = await api.get('/projects'); } catch {}
  const sel = el('select');
  sel.appendChild(el('option', { value: '' }, 'All projects'));
  projects.forEach(p => sel.appendChild(el('option', { value: String(p.id) }, p.name)));
  sel.value = getSelectedProject();
  sel.onchange = () => { setSelectedProject(sel.value); loadPerf(); };
  toolbar.appendChild(sel);
  const timeSel = el('select');
  [{ v: '1', l: 'Last 24 hours' }, { v: '7', l: 'Last 7 days' }, { v: '30', l: 'Last 30 days' }].forEach(o => {
    const opt = el('option', { value: o.v }, o.l);
    if (o.v === '7') opt.selected = true;
    timeSel.appendChild(opt);
  });
  timeSel.onchange = () => loadPerf();
  toolbar.appendChild(timeSel);
  frag.appendChild(toolbar);

  const container = el('div');
  frag.appendChild(container);

  async function loadPerf() {
    container.innerHTML = '';
    try {
      const params = [];
      if (sel.value) params.push('project=' + sel.value);
      params.push('days=' + timeSel.value);
      const data = await api.get('/performance?' + params.join('&'));

      // Summary cards
      const cards = el('div', { className: 'cards' });
      const countCard = el('div', { className: 'card stat' });
      countCard.appendChild(el('div', { className: 'num' }, String(data.totalCount || 0)));
      countCard.appendChild(el('div', { className: 'label' }, 'Total Transactions'));
      cards.appendChild(countCard);
      if (data.oldestTransaction) {
        const oldCard = el('div', { className: 'card stat' });
        oldCard.appendChild(el('div', { className: 'num', style: 'font-size:1.2rem' }, timeAgo(data.oldestTransaction)));
        oldCard.appendChild(el('div', { className: 'label' }, 'Oldest Transaction'));
        cards.appendChild(oldCard);
      }
      container.appendChild(cards);

      // Endpoints table
      const label = timeSel.options[timeSel.selectedIndex].text.toLowerCase();
      container.appendChild(el('h2', { style: 'margin-top:24px' }, 'Endpoints (' + label + ')'));
      if (!data.endpoints || data.endpoints.length === 0) {
        container.appendChild(el('div', { className: 'empty' }, 'No transaction data'));
      } else {
        const t = el('table');
        t.appendChild(el('thead', null, el('tr', null,
          el('th', null, 'Endpoint'), el('th', null, 'Op'), el('th', null, 'Count'),
          el('th', null, 'Avg'), el('th', null, 'P50'), el('th', null, 'P75'),
          el('th', null, 'P95'), el('th', null, 'P99')
        )));
        const tb = el('tbody');
        data.endpoints.forEach(ep => {
          const durStyle = (ms) => {
            if (ms > 1000) return 'color:var(--danger);font-weight:600';
            if (ms > 500) return 'color:var(--warning);font-weight:600';
            return '';
          };
          tb.appendChild(el('tr', null,
            el('td', { style: 'max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap' }, ep.name || '-'),
            el('td', { style: 'color:var(--muted)' }, ep.op || '-'),
            el('td', null, String(ep.count)),
            el('td', null, ep.avgMs + 'ms'),
            el('td', null, ep.p50 + 'ms'),
            el('td', null, ep.p75 + 'ms'),
            el('td', { style: durStyle(ep.p95) }, ep.p95 + 'ms'),
            el('td', { style: durStyle(ep.p99) }, ep.p99 + 'ms')
          ));
        });
        t.appendChild(tb);
        container.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
      }
    } catch (err) { toast(err.message, 'error'); }
  }

  await loadPerf();
  render(frag);
});
