import { el, toast, confirm } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// List ntfy configurations
router.on('/ntfy', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Ntfy Configurations'));

  const toolbar = el('div', { className: 'toolbar' });
  toolbar.appendChild(el('button', { className: 'primary', onClick: () => router.go('/ntfy/new') }, '+ New Configuration'));
  frag.appendChild(toolbar);

  try {
    const configs = await api.get('/ntfy-configs');
    if (!configs || configs.length === 0) {
      frag.appendChild(el('div', { className: 'empty' }, 'No ntfy configurations yet'));
    } else {
      const table = el('table');
      const thead = el('thead');
      const hr = el('tr');
      ['Name', 'URL', 'Topic', 'Token', 'Projects', 'Actions'].forEach(h => hr.appendChild(el('th', null, h)));
      thead.appendChild(hr);
      table.appendChild(thead);
      const tbody = el('tbody');
      configs.forEach(c => {
        const tr = el('tr');
        tr.appendChild(el('td', null, c.Name || c.name));
        tr.appendChild(el('td', null, c.URL || c.url));
        tr.appendChild(el('td', null, c.Topic || c.topic));
        tr.appendChild(el('td', null, (c.Token || c.token) ? '****' : '—'));
        tr.appendChild(el('td', null, String(c.projectCount ?? 0)));
        const actions = el('td', { style: 'display:flex;gap:6px' });
        actions.appendChild(el('button', { className: 'sm', onClick: () => router.go('/ntfy/' + (c.ID || c.id)) }, 'Edit'));
        actions.appendChild(el('button', { className: 'sm', onClick: async () => {
          try {
            const res = await api.post('/ntfy-configs/' + (c.ID || c.id) + '/test');
            if (res.success) toast('Test notification sent!');
            else toast('Test failed: ' + (res.error || 'status ' + res.status), 'error');
          } catch (err) { toast(err.message, 'error'); }
        }}, 'Test'));
        actions.appendChild(el('button', { className: 'sm danger', onClick: async () => {
          if (await confirm('Delete this configuration? Projects using it will lose ntfy.')) {
            try {
              await api.del('/ntfy-configs/' + (c.ID || c.id));
              toast('Configuration deleted');
              router.dispatch();
            } catch (err) { toast(err.message, 'error'); }
          }
        }}, 'Delete'));
        tr.appendChild(actions);
        tbody.appendChild(tr);
      });
      table.appendChild(tbody);
      frag.appendChild(table);
    }
  } catch (err) { toast(err.message, 'error'); }

  render(frag);
});

// New ntfy config
router.on('/ntfy/new', () => renderNtfyForm());

// Edit ntfy config
router.on('/ntfy/:id', async (params) => {
  try {
    const configs = await api.get('/ntfy-configs');
    const cfg = configs.find(c => String(c.ID || c.id) === params.id);
    if (!cfg) { toast('Configuration not found', 'error'); router.go('/ntfy'); return; }
    renderNtfyForm(cfg);
  } catch (err) { toast(err.message, 'error'); router.go('/ntfy'); }
});

function renderNtfyForm(cfg) {
  const isEdit = !!cfg;
  const frag = el('div');
  frag.appendChild(el('h1', null, isEdit ? 'Edit Ntfy Configuration' : 'New Ntfy Configuration'));

  const form = el('form', { className: 'card' });

  const ng = el('div', { className: 'form-group' }, el('label', null, 'Name'));
  const ni = el('input', { type: 'text', value: cfg?.Name || cfg?.name || '' });
  ng.appendChild(ni);
  form.appendChild(ng);

  const row = el('div', { className: 'form-row' });
  const ug = el('div', { className: 'form-group' }, el('label', null, 'Server URL'));
  const ui = el('input', { type: 'text', value: cfg?.URL || cfg?.url || '', placeholder: 'https://n.elmisi.com' });
  ug.appendChild(ui);
  const tg = el('div', { className: 'form-group' }, el('label', null, 'Topic'));
  const ti = el('input', { type: 'text', value: cfg?.Topic || cfg?.topic || '', placeholder: 'ampulla-errors' });
  tg.appendChild(ti);
  row.append(ug, tg);
  form.appendChild(row);

  const kg = el('div', { className: 'form-group' }, el('label', null, 'Token (optional)'));
  const ki = el('input', { type: 'password', value: cfg?.Token || cfg?.token || '', placeholder: 'Bearer token' });
  kg.appendChild(ki);
  form.appendChild(kg);

  const actions = el('div', { className: 'toolbar' });
  actions.appendChild(el('button', { type: 'submit', className: 'primary' }, isEdit ? 'Save' : 'Create'));
  actions.appendChild(el('button', { type: 'button', onClick: () => router.go('/ntfy') }, 'Cancel'));
  form.appendChild(actions);

  form.onsubmit = async (e) => {
    e.preventDefault();
    const data = { name: ni.value, url: ui.value, topic: ti.value, token: ki.value };
    try {
      if (isEdit) {
        await api.put('/ntfy-configs/' + (cfg.ID || cfg.id), data);
        toast('Configuration updated');
      } else {
        await api.post('/ntfy-configs', data);
        toast('Configuration created');
      }
      router.go('/ntfy');
    } catch (err) { toast(err.message, 'error'); }
  };

  frag.appendChild(form);
  render(frag);
  ni.focus();
}
