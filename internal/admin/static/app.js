import { el } from './utils.js';
import router from './router.js';
import api from './api.js';

// Import page modules (each registers routes on import)
import './pages/login.js';
import './pages/dashboard.js';
import './pages/orgs.js';
import './pages/projects.js';
import './pages/issues.js';
import './pages/transactions.js';
import './pages/performance.js';

// Cached version info (fetched once at init)
let versionInfo = null;

export function render(content, showNav = true) {
  const app = document.getElementById('app');
  app.innerHTML = '';
  if (showNav) {
    const logoDiv = el('div', { className: 'logo' }, 'ampulla');
    if (versionInfo?.version) {
      logoDiv.appendChild(el('span', { style: 'font-size:11px;color:var(--muted);font-weight:400;margin-left:8px' }, 'v' + versionInfo.version));
    }
    const n = el('nav', null,
      logoDiv,
      navLink('/', 'Dashboard'),
      navLink('/orgs', 'Organizations'),
      navLink('/issues', 'Issues'),
      navLink('/transactions', 'Transactions'),
      navLink('/performance', 'Performance'),
      el('a', { href: '#', style: 'margin-top:auto;color:var(--danger)', onClick: async (e) => {
        e.preventDefault(); await api.post('/logout'); router.go('/login');
      }}, 'Logout')
    );
    app.appendChild(n);
  }
  const m = el('main');
  if (typeof content === 'string') m.innerHTML = content;
  else if (content instanceof Node) m.appendChild(content);
  app.appendChild(m);
}

function navLink(hash, label) {
  const active = location.hash.slice(1) === hash || (hash !== '/' && location.hash.slice(1).startsWith(hash));
  return el('a', { href: '#' + hash, className: active ? 'active' : '' }, label);
}

// Init
(async function init() {
  // Fetch version + Sentry DSN (unauthenticated endpoint)
  try {
    const res = await fetch('/api/version');
    versionInfo = await res.json();
    if (versionInfo.sentryDsn && window.Sentry) {
      window.Sentry.init({
        dsn: versionInfo.sentryDsn,
        release: 'ampulla-admin@' + (versionInfo.version || 'unknown'),
        integrations: [window.Sentry.browserTracingIntegration()],
        tracesSampleRate: 1.0,
      });
    }
  } catch {}

  try {
    await api.get('/me');
    if (!location.hash || location.hash === '#/login') location.hash = '#/';
  } catch {
    location.hash = '#/login';
  }
  router.start();
})();
