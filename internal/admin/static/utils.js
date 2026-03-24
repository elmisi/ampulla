export function esc(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

export function el(tag, attrs, ...children) {
  const e = document.createElement(tag);
  if (attrs) Object.entries(attrs).forEach(([k, v]) => {
    if (k === 'className') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2).toLowerCase(), v);
    else e.setAttribute(k, v);
  });
  children.flat().forEach(c => { if (c != null) e.append(typeof c === 'string' ? c : c); });
  return e;
}

export function timeAgo(ts) {
  const d = new Date(ts);
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s / 60) + 'm ago';
  if (s < 86400) return Math.floor(s / 3600) + 'h ago';
  return Math.floor(s / 86400) + 'd ago';
}

export function levelBadge(level) {
  const m = { error: 'badge-error', warning: 'badge-warning', info: 'badge-info', fatal: 'badge-error' };
  return el('span', { className: 'badge ' + (m[level] || 'badge-muted') }, level || 'unknown');
}

export function statusBadge(status) {
  const m = { unresolved: 'badge-error', resolved: 'badge-success', ignored: 'badge-muted', active: 'badge-success', inactive: 'badge-muted' };
  return el('span', { className: 'badge ' + (m[status] || 'badge-muted') }, status);
}

export function toast(msg, type = 'success') {
  const e = document.createElement('div');
  e.className = 'toast ' + type;
  e.textContent = msg;
  document.getElementById('toast-container').appendChild(e);
  setTimeout(() => e.remove(), 3000);
}

const PROJECT_KEY = 'ampulla_project';
export function getSelectedProject() { return localStorage.getItem(PROJECT_KEY) || ''; }
export function setSelectedProject(id) { localStorage.setItem(PROJECT_KEY, id); }

export function confirm(msg) {
  return new Promise(resolve => {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `<div class="modal"><h2>Confirm</h2><p>${esc(msg)}</p><div class="actions"><button class="cancel">Cancel</button> <button class="danger confirm-btn">Delete</button></div></div>`;
    document.body.appendChild(overlay);
    overlay.querySelector('.cancel').onclick = () => { overlay.remove(); resolve(false); };
    overlay.querySelector('.confirm-btn').onclick = () => { overlay.remove(); resolve(true); };
  });
}
