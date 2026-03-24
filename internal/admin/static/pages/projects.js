import { el, toast, confirm, statusBadge } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// New Project
router.on('/projects/new', () => renderProjectForm());

// Edit Project
router.on('/projects/:id', async (params) => {
  try {
    const projects = await api.get('/projects');
    const proj = projects.find(p => String(p.id) === params.id);
    if (!proj) { toast('Project not found', 'error'); router.go('/orgs'); return; }
    renderProjectForm(proj);
  } catch (err) { toast(err.message, 'error'); router.go('/orgs'); }
});

async function renderProjectForm(proj) {
  const isEdit = !!proj;
  const frag = el('div');
  frag.appendChild(el('h1', null, isEdit ? 'Edit Project' : 'New Project'));

  let orgs = [];
  try { orgs = await api.get('/organizations'); } catch {}
  const defaultOrgId = isEdit ? proj.organizationId : (new URLSearchParams(location.hash.split('?')[1]).get('org') || '');

  const form = el('form', { className: 'card' });
  if (!isEdit) {
    const og = el('div', { className: 'form-group' }, el('label', null, 'Organization'));
    const os = el('select');
    orgs.forEach(o => {
      const opt = el('option', { value: String(o.id) }, o.name);
      if (String(o.id) === String(defaultOrgId)) opt.selected = true;
      os.appendChild(opt);
    });
    og.appendChild(os);
    form.appendChild(og);
    form._orgSelect = os;
  }
  const row = el('div', { className: 'form-row' });
  const ng = el('div', { className: 'form-group' }, el('label', null, 'Name'));
  const ni = el('input', { type: 'text', value: proj?.name || '' });
  ni.addEventListener('input', () => {
    if (!isEdit || si.dataset.auto !== 'false') {
      si.value = ni.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
    }
  });
  ng.appendChild(ni);
  const sg = el('div', { className: 'form-group' }, el('label', null, 'Slug'));
  const si = el('input', { type: 'text', value: proj?.slug || '' });
  si.addEventListener('input', () => { si.dataset.auto = 'false'; });
  sg.appendChild(si);
  row.append(ng, sg);
  form.appendChild(row);
  const pg = el('div', { className: 'form-group' }, el('label', null, 'Platform'));
  const pi = el('input', { type: 'text', value: proj?.platform || '', placeholder: 'javascript, python, go...' });
  pg.appendChild(pi);
  form.appendChild(pg);

  // ntfy notification config
  if (isEdit) {
    form.appendChild(el('h3', { style: 'margin-top:16px;margin-bottom:8px;color:var(--muted)' }, 'Notifications (ntfy)'));
    const ntfyRow = el('div', { className: 'form-row' });
    const nug = el('div', { className: 'form-group' }, el('label', null, 'ntfy Server URL'));
    const nui = el('input', { type: 'text', value: proj?.ntfyUrl || '', placeholder: 'https://n.elmisi.com' });
    nug.appendChild(nui);
    const ntg = el('div', { className: 'form-group' }, el('label', null, 'Topic'));
    const nti = el('input', { type: 'text', value: proj?.ntfyTopic || '', placeholder: 'my-project-errors' });
    ntg.appendChild(nti);
    ntfyRow.append(nug, ntg);
    form.appendChild(ntfyRow);
    const nkg = el('div', { className: 'form-group' }, el('label', null, 'Token (optional)'));
    const nki = el('input', { type: 'password', value: proj?.ntfyToken || '', placeholder: 'Bearer token' });
    nkg.appendChild(nki);
    form.appendChild(nkg);
    form._ntfyUrl = nui;
    form._ntfyTopic = nti;
    form._ntfyToken = nki;

    // SDK version tracking
    form.appendChild(el('h3', { style: 'margin-top:16px;margin-bottom:8px;color:var(--muted)' }, 'SDK Version'));
    const sdkRow = el('div', { className: 'form-row' });
    const skg = el('div', { className: 'form-group' }, el('label', null, 'Known SDK Version'));
    const ski = el('input', { type: 'text', value: proj?.knownSdkVersion || '', placeholder: 'e.g. sentry.python/1.45.2' });
    skg.appendChild(ski);
    const slg = el('div', { className: 'form-group' }, el('label', null, 'Last Seen'));
    const sli = el('input', { type: 'text', value: proj?.lastSdkVersion || '', disabled: true, style: 'opacity:0.6' });
    slg.appendChild(sli);
    sdkRow.append(skg, slg);
    form.appendChild(sdkRow);
    form._knownSdkVersion = ski;
  }

  const actions = el('div', { className: 'toolbar' });
  actions.appendChild(el('button', { type: 'submit', className: 'primary' }, isEdit ? 'Save' : 'Create'));
  actions.appendChild(el('button', { type: 'button', onClick: () => history.back() }, 'Cancel'));
  form.appendChild(actions);
  form.onsubmit = async (e) => {
    e.preventDefault();
    try {
      if (isEdit) {
        await api.put('/projects/' + proj.id, {
          name: ni.value, slug: si.value, platform: pi.value,
          ntfyUrl: form._ntfyUrl?.value || '', ntfyTopic: form._ntfyTopic?.value || '', ntfyToken: form._ntfyToken?.value || '',
          knownSdkVersion: form._knownSdkVersion?.value || ''
        });
        toast('Project updated');
      } else {
        const orgId = parseInt(form._orgSelect.value);
        await api.post('/projects', { orgId, name: ni.value, slug: si.value, platform: pi.value });
        toast('Project created');
      }
      history.back();
    } catch (err) { toast(err.message, 'error'); }
  };
  frag.appendChild(form);

  // If editing, show DSN keys
  if (isEdit) {
    frag.appendChild(el('h1', { style: 'margin-top:32px' }, 'DSN Keys'));
    const toolbar = el('div', { className: 'toolbar' });
    toolbar.appendChild(el('button', { className: 'primary', onClick: async () => {
      const label = prompt('Key label:', 'Default');
      if (label === null) return;
      try {
        await api.post('/projects/' + proj.id + '/keys', { label });
        toast('Key created');
        router.dispatch();
      } catch (err) { toast(err.message, 'error'); }
    }}, '+ New Key'));
    frag.appendChild(toolbar);
    try {
      const keys = await api.get('/projects/' + proj.id + '/keys');
      if (!keys || keys.length === 0) {
        frag.appendChild(el('div', { className: 'empty' }, 'No DSN keys yet'));
      } else {
        keys.forEach(k => {
          const card = el('div', { className: 'card' });
          const header = el('div', { style: 'display:flex;justify-content:space-between;align-items:center;margin-bottom:12px' });
          header.appendChild(el('div', null,
            el('strong', null, k.label + ' '),
            statusBadge(k.isActive ? 'active' : 'inactive')
          ));
          const btns = el('div', { style: 'display:flex;gap:8px' });
          btns.appendChild(el('button', { className: 'sm', onClick: async () => {
            await api.put('/keys/' + k.id, { isActive: !k.isActive });
            toast(k.isActive ? 'Key disabled' : 'Key enabled');
            router.dispatch();
          }}, k.isActive ? 'Disable' : 'Enable'));
          btns.appendChild(el('button', { className: 'sm danger', onClick: async () => {
            if (await confirm('Delete this key?')) {
              await api.del('/keys/' + k.id);
              toast('Key deleted');
              router.dispatch();
            }
          }}, 'Delete'));
          header.appendChild(btns);
          card.appendChild(header);
          const dsn = el('div', { className: 'dsn-box', onClick: () => {
            navigator.clipboard.writeText(k.dsn);
            toast('DSN copied to clipboard');
          }}, k.dsn);
          card.appendChild(dsn);
          frag.appendChild(card);
        });
      }
    } catch (err) { toast(err.message, 'error'); }
  }
  render(frag);
  ni.focus();
}
