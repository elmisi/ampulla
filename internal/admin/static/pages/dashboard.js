import { el, toast, timeAgo } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

router.on('/', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Dashboard'));
  try {
    const data = await api.get('/dashboard');
    // SDK alerts
    if (data.sdkAlerts && data.sdkAlerts.length > 0) {
      data.sdkAlerts.forEach(a => {
        const alert = el('div', { className: 'card', style: 'border-color:var(--warn);background:rgba(210,153,34,0.08);margin-bottom:12px' });
        alert.appendChild(el('div', { style: 'color:var(--warn);font-weight:700;margin-bottom:4px' }, 'SDK version mismatch \u2014 ' + a.projectName));
        alert.appendChild(el('div', { style: 'color:var(--muted);font-size:12px' },
          'Receiving events from ' + a.lastVersion + ' (known: ' + a.knownVersion + ')'
        ));
        frag.appendChild(alert);
      });
    }
    // Stats cards
    const cards = el('div', { className: 'cards' });
    for (const [key, val] of Object.entries(data.counts)) {
      const c = el('div', { className: 'card stat' });
      c.appendChild(el('div', { className: 'num' }, String(val)));
      c.appendChild(el('div', { className: 'label' }, key));
      cards.appendChild(c);
    }
    frag.appendChild(cards);
  } catch (err) { toast(err.message, 'error'); }
  render(frag);
});
