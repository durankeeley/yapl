// YAPL console UI. Rendering and I/O live here; navigation and input rules
// live in nav.js where they are unit-tested.
import {
  actionForButton, actionForKey, axisDirection, cycle, formatBytes, gridMove,
  initials, jobPercent, listMove, repeatDue, tileHue,
} from './nav.js';

const API = '/api';
const CONSOLE_HEADER = 'X-Yapl-Console';
const JOB_POLL_MS = 1000;
const SESSION_POLL_MS = 2000;
const CLOCK_MS = 15000;
const TOAST_MS = 4500;
const TABS = ['library', 'store', 'system'];
const DIRECTIONS = new Set(['up', 'down', 'left', 'right']);
const ACTIVE_STATES = new Set(['queued', 'running']);

const state = {
  tab: 0,
  library: [],
  store: [],
  storeError: null,
  system: null,
  jobs: new Map(),
  session: { running: false },
  focus: { library: 0, store: 0 },
  overlay: null,
};

// ---------- API ----------

async function api(path, { method = 'GET' } = {}) {
  const headers = method === 'GET' ? {} : { [CONSOLE_HEADER]: '1' };
  const res = await fetch(API + path, { method, headers });
  if (!res.ok) {
    let message = res.statusText;
    try { message = (await res.json()).error || message; } catch { /* not JSON */ }
    throw new Error(message);
  }
  return res.status === 204 ? null : res.json();
}

const enc = encodeURIComponent;

async function refresh() {
  const [library, system] = await Promise.all([api('/library'), api('/system')]);
  state.library = library;
  state.system = system;
  try {
    state.store = await api('/store');
    state.storeError = null;
  } catch (err) {
    state.store = [];
    state.storeError = err.message;
  }
  for (const item of state.store) {
    if (item.job_id && !state.jobs.has(item.job_id)) state.jobs.set(item.job_id, { id: item.job_id, name: item.name, state: 'running', done: 0, total: -1 });
  }
  render();
}

// ---------- DOM helpers ----------

function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'style') {
      for (const [prop, v] of Object.entries(value)) {
        if (prop.startsWith('--')) node.style.setProperty(prop, String(v));
        else node.style[prop] = v;
      }
    }
    else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value === true ? '' : value);
  }
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}

function toast(message, isError = false) {
  const box = document.getElementById('toast');
  box.textContent = message;
  box.className = isError ? 'toast error' : 'toast';
  box.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { box.hidden = true; }, TOAST_MS);
}

// ---------- Views ----------

function currentItems() {
  return TABS[state.tab] === 'store' ? state.store : TABS[state.tab] === 'library' ? state.library : [];
}

function jobFor(name) {
  for (const job of state.jobs.values()) if (job.name === name && ACTIVE_STATES.has(job.state)) return job;
  return null;
}

function storeEntry(name) {
  return state.store.find((s) => s.name === name) || null;
}

function coverURL(item, source) {
  if (!item.has_cover) return null;
  return source === 'store' ? `${API}/store/${enc(item.name)}/cover` : `${API}/library/${enc(item.name)}/cover`;
}

function tile(item, index, source) {
  const focused = state.focus[source] === index && !state.overlay;
  const placeholder = el('div', { class: 'placeholder', style: { '--hue': tileHue(item.name) } }, initials(item.title || item.name));
  const url = coverURL(item, source);
  const cover = url ? el('img', { src: url, alt: '', loading: 'lazy', onerror: (e) => e.target.replaceWith(placeholder) }) : placeholder;

  let badge = null;
  if (source === 'store' && item.upgradable) badge = el('span', { class: 'badge update' }, 'Update');
  else if (source === 'store' && item.installed) badge = el('span', { class: 'badge installed' }, 'Installed');
  else if (source === 'library' && !item.ready) badge = el('span', { class: 'badge not-ready' }, 'Not installed');
  else if (source === 'library' && storeEntry(item.name)?.upgradable) badge = el('span', { class: 'badge update' }, 'Update');

  const job = jobFor(item.name);
  let progress = null;
  if (job) {
    const pct = jobPercent(job);
    progress = el('div', { class: pct === null ? 'progress indeterminate' : 'progress', style: { '--pct': `${pct ?? 0}%` } }, el('i'));
  }
  const version = source === 'store' ? item.latest : item.version;
  const inGrid = index >= 0;
  return el('div', {
    class: ['tile', focused && 'focused', job && 'busy'].filter(Boolean).join(' '),
    'data-index': inGrid ? index : null,
    role: inGrid ? 'button' : null,
    'aria-label': item.title || item.name,
    onmouseenter: inGrid ? () => { if (!state.overlay) { state.focus[source] = index; render(); } } : null,
    onclick: inGrid ? () => { state.focus[source] = index; dispatch('select'); } : null,
  }, cover, badge, el('div', { class: 'caption' }, item.title || item.name, el('small', {}, version ? `v${version}` : '')), progress);
}

function gridView(items, source) {
  if (items.length === 0) return emptyView(source);
  const title = source === 'store' ? `Available on ${state.system?.server || 'server'}` : 'Your games';
  return [el('h1', { class: 'section-title' }, title), el('div', { class: 'grid', id: 'grid' }, items.map((item, i) => tile(item, i, source)))];
}

function emptyView(source) {
  if (source === 'store') {
    return el('div', { class: 'empty' },
      el('h2', {}, state.storeError ? 'No YAPL server found' : 'The server has no games yet'),
      el('p', {}, state.storeError
        ? 'Start one with "yapl serve" on the LAN, then reopen the console. Games already installed are on the Library tab.'
        : 'Publish one from the machine you set it up on with "yapl push <name>".'));
  }
  return el('div', { class: 'empty' },
    el('h2', {}, 'Your library is empty'),
    el('p', {}, 'Press RB to open the Store and install a game from the LAN server.'));
}

function systemView() {
  const sys = state.system;
  if (!sys) return el('div', { class: 'empty' }, 'Loading…');
  const modules = Object.entries(sys.modules || {}).sort(([a], [b]) => a.localeCompare(b));
  const pads = [...(navigator.getGamepads?.() || [])].filter(Boolean);
  return el('div', { class: 'system' },
    el('section', { class: 'card' }, el('h3', {}, 'Server'), el('dl', {},
      el('dt', {}, 'Address'), el('dd', {}, sys.server || 'none (offline)'),
      el('dt', {}, 'Status'), el('dd', {}, el('span', { class: sys.server_reachable ? 'dot on' : 'dot' }), sys.server_reachable ? 'Connected' : 'Not reachable'))),
    el('section', { class: 'card' }, el('h3', {}, 'This machine'), el('dl', {},
      el('dt', {}, 'Library'), el('dd', {}, sys.workdir),
      el('dt', {}, 'Platform'), el('dd', {}, sys.platform),
      el('dt', {}, 'Games'), el('dd', {}, String(state.library.length)),
      el('dt', {}, 'Controllers'), el('dd', {}, pads.length ? pads.map((p) => p.id).join(', ') : 'none detected — press a button'))),
    el('section', { class: 'card' }, el('h3', {}, 'YAPL versions'), el('dl', {},
      el('dt', {}, 'kernel'), el('dd', {}, sys.kernel),
      modules.map(([name, version]) => [el('dt', {}, name), el('dd', {}, version)]))));
}

function detailsDialog(o) {
  const { item, source } = o;
  const libraryItem = state.library.find((g) => g.name === item.name);
  const meta = [item.type, item.players && `${item.players} players`, item.size_bytes && formatBytes(item.size_bytes)].filter(Boolean).join(' · ');
  return el('div', { class: 'dialog', role: 'dialog', 'aria-modal': 'true' },
    el('div', {}, tile(item, -1, source)),
    el('div', {},
      el('h2', {}, item.title || item.name),
      el('div', { class: 'meta' }, meta, (libraryItem?.version || item.latest) && ` · v${libraryItem?.version || item.latest}`),
      item.tags?.length ? el('div', { class: 'tags' }, item.tags.map((t) => el('span', {}, t))) : null,
      el('p', {}, item.description || ''),
      actionButtons(o)));
}

function actionButtons(o) {
  return el('div', { class: 'actions' }, o.buttons.map((b, i) => el('button', {
    type: 'button',
    class: ['action', b.kind, i === o.focus ? 'focused' : ''].filter(Boolean).join(' '),
    onmouseenter: () => { o.focus = i; render(); },
    onclick: () => { o.focus = i; dispatch('select'); },
  }, b.label)));
}

function confirmDialog(o) {
  return el('div', { class: 'dialog compact', role: 'alertdialog', 'aria-modal': 'true' },
    el('h2', {}, o.title), el('p', {}, o.message), actionButtons(o));
}

function playingDialog(o) {
  const name = state.session.name;
  const game = state.library.find((g) => g.name === name);
  return el('div', { class: 'dialog compact playing', role: 'dialog', 'aria-modal': 'true' },
    el('div', { class: 'pulse' }), el('div', { class: 'meta' }, 'Now playing'), el('h2', {}, game?.title || name),
    el('p', {}, 'Quit the game to return here.'), actionButtons(o));
}

function renderOverlay() {
  const box = document.getElementById('overlay');
  box.replaceChildren();
  if (state.session.running && state.overlay?.kind !== 'confirm') {
    if (state.overlay?.kind !== 'playing') {
      state.overlay = { kind: 'playing', focus: 0, buttons: [{ label: 'Stop game', kind: 'danger', run: confirmStop }] };
    }
  } else if (state.overlay?.kind === 'playing') {
    state.overlay = null;
  }
  const o = state.overlay;
  box.hidden = !o;
  if (!o) return;
  box.append(o.kind === 'details' ? detailsDialog(o) : o.kind === 'confirm' ? confirmDialog(o) : playingDialog(o));
}

function render() {
  document.querySelectorAll('.tab').forEach((t, i) => t.classList.toggle('active', i === state.tab));
  const view = document.getElementById('view');
  const name = TABS[state.tab];
  view.replaceChildren(...[].concat(name === 'system' ? systemView() : gridView(name === 'store' ? state.store : state.library, name)));
  renderOverlay();
  document.querySelector('.tile.focused')?.scrollIntoView({ block: 'nearest' });
}

// ---------- Actions ----------

function openDetails(item, source) {
  const buttons = [];
  const lib = state.library.find((g) => g.name === item.name);
  const remote = storeEntry(item.name);
  if (lib?.ready) buttons.push({ label: 'Play', kind: 'primary', run: () => launch(item.name) });
  if (!lib && source === 'store') buttons.push({ label: 'Install', kind: 'primary', run: () => startJob('install', item.name) });
  if (lib && remote?.upgradable) buttons.push({ label: `Update to v${remote.latest}`, run: () => startJob('upgrade', item.name) });
  if (lib) buttons.push({ label: 'Uninstall', kind: 'danger', run: () => confirmUninstall(lib) });
  buttons.push({ label: 'Back', run: closeOverlay });
  state.overlay = { kind: 'details', item, source, buttons, focus: 0 };
  render();
}

function closeOverlay() {
  state.overlay = null;
  render();
}

function confirmUninstall(game) {
  state.overlay = {
    kind: 'confirm',
    title: `Uninstall ${game.title || game.name}?`,
    message: 'This deletes the game and its saves from this machine.',
    focus: 0,
    buttons: [
      { label: 'Cancel', kind: 'primary', run: closeOverlay },
      { label: 'Uninstall', kind: 'danger', run: () => uninstall(game.name) },
    ],
  };
  render();
}

function confirmStop() {
  state.overlay = {
    kind: 'confirm',
    title: 'Stop the game?',
    message: 'Unsaved progress will be lost.',
    focus: 0,
    buttons: [
      { label: 'Keep playing', kind: 'primary', run: closeOverlay },
      { label: 'Stop game', kind: 'danger', run: stopGame },
    ],
  };
  render();
}

async function launch(name) {
  try {
    state.overlay = null;
    state.session = await api(`/library/${enc(name)}/launch`, { method: 'POST' });
  } catch (err) {
    toast(err.message, true);
  }
  render();
}

async function stopGame() {
  state.overlay = null;
  try { await api('/session/stop', { method: 'POST' }); } catch (err) { toast(err.message, true); }
  await pollSession();
}

async function uninstall(name) {
  state.overlay = null;
  try {
    await api(`/library/${enc(name)}?confirm=${enc(name)}`, { method: 'DELETE' });
    toast(`${name} uninstalled`);
    state.focus.library = 0;
  } catch (err) {
    toast(err.message, true);
  }
  await refresh();
}

async function startJob(kind, name) {
  state.overlay = null;
  try {
    const path = kind === 'install' ? `/store/${enc(name)}/install` : `/library/${enc(name)}/upgrade`;
    const job = await api(path, { method: 'POST' });
    state.jobs.set(job.id, job);
    toast(kind === 'install' ? `Installing ${name}…` : `Updating ${name}…`);
  } catch (err) {
    toast(err.message, true);
  }
  render();
}

function activate(source) {
  const items = currentItems();
  const item = items[state.focus[source]];
  if (!item) return;
  if (source === 'library' && item.ready) return launch(item.name);
  return openDetails(item, source);
}

function columns() {
  const grid = document.getElementById('grid');
  if (!grid) return 1;
  return getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean).length;
}

function dispatch(action) {
  const o = state.overlay;
  if (o) {
    if (DIRECTIONS.has(action)) {
      o.focus = listMove(o.focus, o.buttons.length, action);
    } else if (action === 'select') {
      return o.buttons[o.focus]?.run();
    } else if (action === 'back' && o.kind !== 'playing') {
      return closeOverlay();
    }
    return render();
  }
  const source = TABS[state.tab];
  if (action === 'prevTab' || action === 'nextTab') {
    state.tab = cycle(state.tab, TABS.length, action === 'nextTab' ? 1 : -1);
    if (TABS[state.tab] !== 'system') refresh().catch((err) => toast(err.message, true));
    return render();
  }
  if (source === 'system') return undefined;
  if (DIRECTIONS.has(action)) {
    state.focus[source] = gridMove(state.focus[source], currentItems().length, columns(), action);
    return render();
  }
  const item = currentItems()[state.focus[source]];
  if (action === 'select') return activate(source);
  if (action === 'details' && item) return openDetails(item, source);
  if (action === 'play' && item) {
    const lib = state.library.find((g) => g.name === item.name);
    return lib?.ready ? launch(item.name) : openDetails(item, source);
  }
  return undefined;
}

// ---------- Input ----------

const held = new Map();

function pollGamepads(now) {
  const pads = [...(navigator.getGamepads?.() || [])].filter(Boolean);
  const active = new Set();
  for (const pad of pads) {
    pad.buttons.forEach((button, index) => {
      const action = button.pressed ? actionForButton(index) : null;
      if (action) active.add(action);
    });
    const direction = axisDirection(pad.axes[0] || 0, pad.axes[1] || 0);
    if (direction) active.add(direction);
  }
  for (const action of active) {
    const h = held.get(action);
    if (!h) {
      held.set(action, { pressedAt: now, lastFired: now });
      dispatch(action);
    } else if (DIRECTIONS.has(action) && repeatDue(h.pressedAt, h.lastFired, now)) {
      h.lastFired = now;
      dispatch(action);
    }
  }
  for (const action of [...held.keys()]) if (!active.has(action)) held.delete(action);
  document.getElementById('pad-status').textContent = pads.length ? '🎮' : '';
  requestAnimationFrame(pollGamepads);
}

document.addEventListener('keydown', (event) => {
  const action = actionForKey(event.key);
  if (!action) return;
  event.preventDefault();
  dispatch(action);
});

document.querySelectorAll('.tab').forEach((t) => t.addEventListener('click', () => {
  state.tab = Number(t.dataset.tab);
  render();
}));

// ---------- Polling ----------

async function pollJobs() {
  const active = [...state.jobs.values()].filter((j) => ACTIVE_STATES.has(j.state));
  for (const job of active) {
    try {
      const latest = await api(`/jobs/${enc(job.id)}`);
      state.jobs.set(job.id, latest);
      if (latest.state === 'done') toast(`${latest.name} is ready to play`);
      if (latest.state === 'failed') toast(`${latest.name}: ${latest.error}`, true);
      if (!ACTIVE_STATES.has(latest.state)) await refresh();
    } catch {
      state.jobs.delete(job.id);
    }
  }
  if (active.length) render();
}

async function pollSession() {
  try {
    const before = state.session.running;
    state.session = await api('/session');
    if (before !== state.session.running) {
      if (!state.session.running && state.session.last?.code) toast(`${state.session.last.name} exited with code ${state.session.last.code}`, true);
      render();
    }
  } catch { /* console restarting */ }
}

function tickClock() {
  document.getElementById('clock').textContent = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

tickClock();
setInterval(tickClock, CLOCK_MS);
setInterval(pollJobs, JOB_POLL_MS);
setInterval(pollSession, SESSION_POLL_MS);
window.addEventListener('gamepadconnected', () => { if (TABS[state.tab] === 'system') render(); });
requestAnimationFrame(pollGamepads);
refresh().then(pollSession).catch((err) => toast(err.message, true));
