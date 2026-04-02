import { el, toast, confirm, timeAgo, statusBadge } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// Organizations list
router.on('/orgs', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'Organizations'));
  const toolbar = el('div', { className: 'toolbar' });
  toolbar.appendChild(el('button', { className: 'primary', onClick: () => router.go('/orgs/new') }, '+ New Organization'));
  frag.appendChild(toolbar);
  try {
    const orgs = await api.get('/organizations');
    if (!orgs || orgs.length === 0) {
      frag.appendChild(el('div', { className: 'empty' }, 'No organizations yet'));
    } else {
      const t = el('table');
      t.appendChild(el('thead', null, el('tr', null,
        el('th', null, 'Name'), el('th', null, 'Slug'), el('th', null, 'Created'), el('th', null, 'Actions')
      )));
      const tb = el('tbody');
      orgs.forEach(o => {
        tb.appendChild(el('tr', null,
          el('td', null, el('a', { href: '#/orgs/' + o.id }, o.name)),
          el('td', { style: 'color:var(--muted)' }, o.slug),
          el('td', { style: 'color:var(--muted)' }, timeAgo(o.dateCreated)),
          el('td', null,
            el('button', { className: 'sm danger', onClick: async () => {
              if (await confirm('Delete organization "' + o.name + '" and all its projects?')) {
                await api.del('/organizations/' + o.id);
                toast('Organization deleted');
                router.dispatch();
              }
            }}, 'Delete')
          )
        ));
      });
      t.appendChild(tb);
      frag.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
    }
  } catch (err) { toast(err.message, 'error'); }
  render(frag);
});

// New Organization
router.on('/orgs/new', () => renderOrgForm());

// Edit Organization
router.on('/orgs/:id', async (params) => {
  try {
    const orgs = await api.get('/organizations');
    const org = orgs.find(o => String(o.id) === params.id);
    if (!org) { toast('Organization not found', 'error'); router.go('/orgs'); return; }
    renderOrgForm(org);
  } catch (err) { toast(err.message, 'error'); router.go('/orgs'); }
});

async function renderOrgForm(org) {
  const isEdit = !!org;
  const frag = el('div');
  frag.appendChild(el('h1', null, isEdit ? 'Edit Organization' : 'New Organization'));

  const form = el('form', { className: 'card' });
  const row = el('div', { className: 'form-row' });
  const ng = el('div', { className: 'form-group' }, el('label', null, 'Name'));
  const ni = el('input', { type: 'text', value: org?.name || '', placeholder: 'My Organization' });
  ni.addEventListener('input', () => {
    if (!isEdit || si.dataset.auto !== 'false') {
      si.value = ni.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
      si.dataset.auto = 'true';
    }
  });
  ng.appendChild(ni);
  const sg = el('div', { className: 'form-group' }, el('label', null, 'Slug'));
  const si = el('input', { type: 'text', value: org?.slug || '', placeholder: 'my-organization' });
  si.addEventListener('input', () => { si.dataset.auto = 'false'; });
  sg.appendChild(si);
  row.append(ng, sg);
  form.appendChild(row);
  const actions = el('div', { className: 'toolbar' });
  actions.appendChild(el('button', { type: 'submit', className: 'primary' }, isEdit ? 'Save' : 'Create'));
  actions.appendChild(el('button', { type: 'button', onClick: () => router.go('/orgs') }, 'Cancel'));
  form.appendChild(actions);
  form.onsubmit = async (e) => {
    e.preventDefault();
    try {
      if (isEdit) {
        await api.put('/organizations/' + org.id, { name: ni.value, slug: si.value });
        toast('Organization updated');
      } else {
        await api.post('/organizations', { name: ni.value, slug: si.value });
        toast('Organization created');
      }
      router.go('/orgs');
    } catch (err) { toast(err.message, 'error'); }
  };
  frag.appendChild(form);

  // If editing, show projects
  if (isEdit) {
    frag.appendChild(el('h1', { style: 'margin-top:32px' }, 'Projects'));
    const toolbar = el('div', { className: 'toolbar' });
    toolbar.appendChild(el('button', { className: 'primary', onClick: () => router.go('/projects/new?org=' + org.id) }, '+ New Project'));
    frag.appendChild(toolbar);
    try {
      const projects = await api.get('/organizations/' + org.slug + '/projects');
      if (!projects || projects.length === 0) {
        frag.appendChild(el('div', { className: 'empty' }, 'No projects in this organization'));
      } else {
        const t = el('table');
        t.appendChild(el('thead', null, el('tr', null,
          el('th', null, 'Name'), el('th', null, 'Slug'), el('th', null, 'Platform'), el('th', null, 'Last Transaction'), el('th', null, 'Actions')
        )));
        const tb = el('tbody');
        projects.forEach(p => {
          tb.appendChild(el('tr', null,
            el('td', null, el('a', { href: '#/projects/' + p.id }, p.name)),
            el('td', { style: 'color:var(--muted)' }, p.slug),
            el('td', { style: 'color:var(--muted)' }, p.platform || '-'),
            el('td', { style: 'color:var(--muted)' }, p.lastTransaction ? timeAgo(p.lastTransaction) : '-'),
            el('td', null,
              el('button', { className: 'sm danger', onClick: async () => {
                if (await confirm('Delete project "' + p.name + '"?')) {
                  await api.del('/projects/' + p.id);
                  toast('Project deleted');
                  router.dispatch();
                }
              }}, 'Delete')
            )
          ));
        });
        t.appendChild(tb);
        frag.appendChild(el('div', { className: 'card', style: 'padding:0;overflow:auto' }, t));
      }
    } catch (err) { toast(err.message, 'error'); }
  }
  render(frag);
  ni.focus();
}
