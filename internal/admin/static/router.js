const router = {
  routes: [],
  on(pattern, handler) { this.routes.push({ pattern, handler }); },
  go(hash) { location.hash = hash; },
  start() {
    window.addEventListener('hashchange', () => this.dispatch());
    this.dispatch();
  },
  dispatch() {
    const full = location.hash.slice(1) || '/';
    const path = full.split('?')[0];
    for (const route of this.routes) {
      const m = this.match(route.pattern, path);
      if (m) { route.handler(m); return; }
    }
    this.go('/');
  },
  match(pattern, path) {
    const pp = pattern.split('/'), hp = path.split('/');
    if (pp.length !== hp.length) return null;
    const params = {};
    for (let i = 0; i < pp.length; i++) {
      if (pp[i].startsWith(':')) params[pp[i].slice(1)] = hp[i];
      else if (pp[i] !== hp[i]) return null;
    }
    return params;
  }
};

export default router;
