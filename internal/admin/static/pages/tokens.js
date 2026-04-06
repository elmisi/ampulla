import { el, toast, confirm } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

// List API tokens
router.on('/tokens', async () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'API Tokens'));

  const intro = el('p', { className: 'muted' },
    'API tokens authenticate machine clients (MCP server, scripts) without sharing the admin password. ' +
    'The plaintext token is shown only once at creation time.');
  frag.appendChild(intro);

  const toolbar = el('div', { className: 'toolbar' });
  toolbar.appendChild(el('button', { className: 'primary', onClick: () => router.go('/tokens/new') }, '+ New Token'));
  frag.appendChild(toolbar);

  try {
    const tokens = await api.get('/tokens');
    if (!tokens || tokens.length === 0) {
      frag.appendChild(el('div', { className: 'empty' }, 'No API tokens yet'));
    } else {
      const table = el('table');
      const thead = el('thead');
      const hr = el('tr');
      ['Name', 'Prefix', 'Created', 'Last Used', 'Actions'].forEach(h => hr.appendChild(el('th', null, h)));
      thead.appendChild(hr);
      table.appendChild(thead);
      const tbody = el('tbody');
      tokens.forEach(t => {
        const tr = el('tr');
        tr.appendChild(el('td', null, t.name));
        tr.appendChild(el('td', null, el('code', null, t.prefix + '…')));
        tr.appendChild(el('td', null, formatDate(t.createdAt)));
        tr.appendChild(el('td', null, t.lastUsedAt ? formatDate(t.lastUsedAt) : '—'));
        const actions = el('td');
        actions.appendChild(el('button', { className: 'sm danger', onClick: async () => {
          if (await confirm('Revoke this token? Clients using it will lose access immediately.')) {
            try {
              await api.del('/tokens/' + t.id);
              toast('Token revoked');
              router.dispatch();
            } catch (err) { toast(err.message, 'error'); }
          }
        }}, 'Revoke'));
        tr.appendChild(actions);
        tbody.appendChild(tr);
      });
      table.appendChild(tbody);
      frag.appendChild(table);
    }
  } catch (err) { toast(err.message, 'error'); }

  render(frag);
});

// New token
router.on('/tokens/new', () => {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'New API Token'));

  const form = el('form', { className: 'card' });

  const ng = el('div', { className: 'form-group' }, el('label', null, 'Name'));
  const ni = el('input', { type: 'text', placeholder: 'e.g. mcp-server' });
  ng.appendChild(ni);
  form.appendChild(ng);

  const actions = el('div', { className: 'toolbar' });
  actions.appendChild(el('button', { type: 'submit', className: 'primary' }, 'Create Token'));
  actions.appendChild(el('button', { type: 'button', onClick: () => router.go('/tokens') }, 'Cancel'));
  form.appendChild(actions);

  form.onsubmit = async (e) => {
    e.preventDefault();
    if (!ni.value.trim()) {
      toast('Name is required', 'error');
      return;
    }
    try {
      const created = await api.post('/tokens', { name: ni.value.trim() });
      showCreatedToken(created);
    } catch (err) { toast(err.message, 'error'); }
  };

  frag.appendChild(form);
  render(frag);
  ni.focus();
});

function showCreatedToken(token) {
  const frag = el('div');
  frag.appendChild(el('h1', null, 'API Token Created'));

  const warning = el('div', { className: 'card', style: 'border-left: 4px solid #f59e0b; padding: 12px;' });
  warning.appendChild(el('strong', null, 'Save this token now. '));
  warning.appendChild(el('span', null, 'It will not be shown again.'));
  frag.appendChild(warning);

  const tokenBox = el('div', { className: 'card', style: 'margin-top: 12px;' });
  tokenBox.appendChild(el('label', null, 'Token (' + token.name + ')'));
  const tokenInput = el('input', {
    type: 'text',
    value: token.token,
    readOnly: true,
    style: 'width: 100%; font-family: monospace; font-size: 13px;',
    onClick: (e) => e.target.select(),
  });
  tokenBox.appendChild(tokenInput);
  frag.appendChild(tokenBox);

  const actions = el('div', { className: 'toolbar', style: 'margin-top: 12px;' });
  actions.appendChild(el('button', { className: 'primary', onClick: () => {
    navigator.clipboard.writeText(token.token).then(() => toast('Copied to clipboard'));
  }}, 'Copy'));
  actions.appendChild(el('button', { onClick: () => router.go('/tokens') }, 'Done'));
  frag.appendChild(actions);

  render(frag);
}

function formatDate(s) {
  if (!s) return '—';
  const d = new Date(s);
  return d.toLocaleString();
}
