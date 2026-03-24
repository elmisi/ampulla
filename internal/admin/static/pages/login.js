import { el, toast } from '../utils.js';
import router from '../router.js';
import api from '../api.js';
import { render } from '../app.js';

router.on('/login', async () => {
  const wrap = el('div', { className: 'login-wrap' });
  const box = el('div', { className: 'login-box card' });
  box.innerHTML = '<h1>Ampulla</h1>';
  const form = el('form');
  const ug = el('div', { className: 'form-group' }, el('label', null, 'Username'));
  const ui = el('input', { type: 'text', name: 'username', autocomplete: 'username' });
  ug.appendChild(ui);
  const pg = el('div', { className: 'form-group' }, el('label', null, 'Password'));
  const pi = el('input', { type: 'password', name: 'password', autocomplete: 'current-password' });
  pg.appendChild(pi);
  const btn = el('button', { type: 'submit', className: 'primary', style: 'width:100%;margin-top:8px' }, 'Sign In');
  form.append(ug, pg, btn);
  form.onsubmit = async (e) => {
    e.preventDefault();
    try {
      await api.post('/login', { username: ui.value, password: pi.value });
      router.go('/');
    } catch (err) { toast(err.message, 'error'); }
  };
  box.appendChild(form);
  wrap.appendChild(box);
  render(wrap, false);
  ui.focus();
});
