import { el, toast, setSelectedProject } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

function goFiltered(hash, projectId) {
  setSelectedProject(String(projectId));
  router.go(hash);
}

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

    // Projects table
    if (!data.projects || data.projects.length === 0) {
      frag.appendChild(el('div', { className: 'empty' }, 'No projects yet'));
    } else {
      const t = el('table');
      t.appendChild(el('thead', null, el('tr', null,
        el('th', null, 'Project'),
        el('th', null, 'Platform'),
        el('th', { style: 'text-align:right' }, 'Issues'),
        el('th', { style: 'text-align:right' }, 'Unresolved'),
        el('th', { style: 'text-align:right' }, 'Transactions'),
        el('th', null, '')
      )));
      const tb = el('tbody');
      data.projects.forEach(p => {
        tb.appendChild(el('tr', null,
          el('td', null,
            el('a', { href: '#/projects/' + p.id, style: 'font-weight:600' }, p.name)
          ),
          el('td', { style: 'color:var(--muted)' }, p.platform || '-'),
          el('td', { style: 'text-align:right' },
            el('a', { href: '#', onClick: (e) => { e.preventDefault(); goFiltered('/issues', p.id); } }, String(p.issuesTotal))
          ),
          el('td', { style: 'text-align:right' },
            p.issuesUnresolved > 0
              ? el('a', { href: '#', style: 'color:var(--danger)', onClick: (e) => { e.preventDefault(); goFiltered('/issues', p.id); } }, String(p.issuesUnresolved))
              : el('span', { style: 'color:var(--muted)' }, '0')
          ),
          el('td', { style: 'text-align:right' },
            el('a', { href: '#', onClick: (e) => { e.preventDefault(); goFiltered('/transactions', p.id); } }, String(p.transactionsTotal))
          ),
          el('td', null,
            el('a', { href: '#', style: 'color:var(--muted);font-size:12px', onClick: (e) => { e.preventDefault(); goFiltered('/performance', p.id); } }, 'performance')
          )
        ));
      });
      t.appendChild(tb);
      frag.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
    }
  } catch (err) { toast(err.message, 'error'); }
  render(frag);
});
