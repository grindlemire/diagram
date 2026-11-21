// mount wires up one diagram view: the full page (R is the document) or
// an embed (R is the <diagram-view> element's shadow root). Everything it
// finds, it finds in R, so views never see each other's elements.
//
// cfg: embed (an embedded view: page-relative tooltips, keys only
// while focused), host (an embed's element, whose theme
// attribute holds its theme), keys (where key presses are heard), hash (deep links: '' for the page's own fragment,
// a name for '#<name>:…', null for none), signal (aborted when the view
// goes away), view (a pan and zoom to start from).
function mount(R, cfg) {
'use strict';
const doc = document, root = doc.documentElement, $ = id => R.getElementById(id);
const stage = $('dg-stage');
const svg = stage.querySelector('svg');
const tip = $('dg-tip');
const q = $('dg-q');
const count = $('dg-count');
const zoomBtn = $('dg-zoom');
const themeBtn = $('dg-theme');
const bar = $('dg-bar');
const banner = $('dg-path');
const help = $('dg-help');
const helpBtn = $('dg-keys');
const NS = 'http://www.w3.org/2000/svg';
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
const modKey = mac ? '⌘' : 'Ctrl';
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
const vb = svg.viewBox.baseVal;
const W0 = vb.width, H0 = vb.height;
const on$ = (t, ev, f, o) => t.addEventListener(ev, f, {signal: cfg.signal, ...o});
const iconCheck = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';
const iconLink = banner.querySelector('.p-copy svg').outerHTML;
const linking = cfg.hash != null;
if (!linking) banner.querySelector('.p-copy').hidden = true;

// ---- Index ----

// items: id -> {id, el, kind, parent, label, desc, tooltip, link, from, to, edges, labelEl}
const items = new Map();
const edges = [], nodes = [];
for (const el of svg.querySelectorAll('[data-kind]')) {
  const d = el.dataset;
  if (d.kind === 'edge-label') continue;
  const it = {id: d.id, el, kind: d.kind, parent: d.parent || '', label: d.label || '', desc: d.desc || '',
    tooltip: d.tooltip || '', link: d.link || '', from: d.from, to: d.to, edges: [], labelEl: null,
    hidden: !!d.hidden};
  items.set(it.id, it);
  if (it.kind === 'edge') edges.push(it);
  if (it.kind === 'node') nodes.push(it);
}
for (const el of svg.querySelectorAll('[data-kind="edge-label"]')) {
  const e = items.get(el.dataset.for);
  if (e) e.labelEl = el;
}
for (const e of edges) {
  for (const end of new Set([e.from, e.to])) {
    const it = items.get(end);
    if (it) it.edges.push(e);
  }
}
const itemOf = el => {
  const g = el && el.closest && el.closest('[data-kind]');
  if (!g || !svg.contains(g)) return null;
  const d = g.dataset;
  return items.get(d.kind === 'edge-label' ? d.for : d.id) || null;
};
const ancestors = id => {
  const out = [];
  for (let it = items.get(id); it && it.parent; it = items.get(it.parent)) out.push(it.parent);
  return out;
};
const within = (id, gid) => id === gid || ancestors(id).includes(gid);
const nameOf = id => { const it = items.get(id); return it ? (it.label || it.id).replace(/\s*\n\s*/g, ' ') : id; };
// A vertex can end a traced path: a visible node or group.
const vertex = it => !!it && !it.hidden && (it.kind === 'node' || it.kind === 'group');

// Element geometry in diagram units, measured once.
const boxes = new Map();
function boxOf(el) {
  let b = boxes.get(el);
  if (!b) { const r = el.getBBox(); b = {x: r.x, y: r.y, w: r.width, h: r.height}; boxes.set(el, b); }
  return b;
}
const union = bs => {
  const x = Math.min(...bs.map(b => b.x)), y = Math.min(...bs.map(b => b.y));
  return {x, y, w: Math.max(...bs.map(b => b.x + b.w)) - x, h: Math.max(...bs.map(b => b.y + b.h)) - y};
};

// Wide invisible strokes make thin edges easy to point at.
for (const e of edges) {
  const line = e.el.querySelector('path');
  e.line = line;
  if (!line) continue;
  const hit = doc.createElementNS(NS, 'path');
  hit.setAttribute('d', line.getAttribute('d'));
  hit.setAttribute('class', 'dg-hit');
  e.el.insertBefore(hit, e.el.firstChild);
}

// A small marker marks linked elements, at the top right of their
// outline: the node's shape, or for a shapeless (Text) node what it
// draws, leaving out its badges and status dot. Those straddle the top
// corners, so with badges the marker sits just left of them, or failing
// room there, at the top left (unless a status dot is there) or else the
// bottom right (right of the badges for a shapeless node).
const SHAPES = 'rect, path, ellipse, circle, polygon, use';
function outline(el) {
  const shape = [...el.children].find(c => c.matches(SHAPES));
  if (shape) { const b = shape.getBBox(); return {x: b.x, y: b.y, w: b.width, h: b.height, shape: true}; }
  const parts = [...el.children].filter(c => c.getBBox && !c.classList.contains('marks') && !c.classList.contains('dg-hit'));
  if (!parts.length) return null;
  return union(parts.map(c => { const b = c.getBBox(); return {x: b.x, y: b.y, w: b.width, h: b.height}; }));
}
function markerAt(it) {
  const b = outline(it.el);
  if (!b) return null;
  const marks = [...it.el.children].find(c => c.classList.contains('marks'));
  const pills = marks ? [...marks.querySelectorAll('rect')].map(r => r.getBBox()) : [];
  if (!pills.length) return {x: b.x + b.w - 3, y: b.y + 3};
  const R = 7, left = pills.reduce((a, p) => p.x < a.x ? p : a);
  const dots = [...marks.querySelectorAll('circle, ellipse')].map(c => c.getBBox());
  const clear = (x, y) => dots.every(d => Math.hypot(d.x + d.width / 2 - x, d.y + d.height / 2 - y) > R + d.width / 2 + 1);
  const at = {x: left.x - 1 - R, y: left.y + left.height / 2};
  if (at.x - R >= b.x + 2 && clear(at.x, at.y)) return at;
  if (clear(b.x + 3, b.y + 3)) return {x: b.x + 3, y: b.y + 3};
  if (b.shape) return {x: b.x + b.w - 3, y: b.y + b.h - 3};
  // A shapeless node's corners are its text, so go right of the badges.
  const right = pills.reduce((a, p) => p.x + p.width > a.x + a.width ? p : a);
  return {x: right.x + right.width + 1 + R, y: right.y + right.height / 2};
}
for (const it of items.values()) {
  if (!it.link || it.hidden) continue;
  const at = markerAt(it);
  if (!at) continue;
  const g = doc.createElementNS(NS, 'g');
  g.setAttribute('class', 'dg-badge');
  g.setAttribute('aria-hidden', 'true');
  g.setAttribute('transform', `translate(${at.x} ${at.y})`);
  g.innerHTML = '<circle r="7"/><path d="M-2.2 2.2 2.2-2.2M-1.4-2.2h3.6v3.6"/>';
  it.el.appendChild(g);
}

// ---- View: k screen px per diagram unit; (x, y) is where the diagram's
// origin sits in the stage. The viewBox always matches the stage's aspect.

const view = {k: 1, x: 0, y: 0};
let fitted = true, anim = 0;
const size = () => ({w: stage.clientWidth || 1, h: stage.clientHeight || 1});
// The stage's top left on screen, to turn pointer positions into stage ones.
const origin = () => { const r = stage.getBoundingClientRect(); return {x: r.left, y: r.top}; };
const local = ev => { const o = origin(); return {x: ev.clientX - o.x, y: ev.clientY - o.y}; };
const clampK = k => Math.min(8, Math.max(0.05, k));
// Room kept clear for the page's toolbar; an embed's floats over it.
const TOP = cfg.embed ? 8 : 64;
function apply() {
  const {w, h} = size();
  svg.setAttribute('viewBox', `${-view.x / view.k} ${-view.y / view.k} ${w / view.k} ${h / view.k}`);
  if (zoomBtn) zoomBtn.textContent = Math.round(view.k * 100) + '%';
  placeTip();
}
function fitView() {
  const {w, h} = size();
  if (cfg.embed) {
    // An embed is sized to the diagram, so it fits edge to edge.
    const k = clampK(Math.min(w / W0, h / H0));
    return {k, x: (w - W0 * k) / 2, y: (h - H0 * k) / 2};
  }
  const pad = Math.min(48, w * 0.06), top = Math.min(TOP, pad + 40);
  const k = clampK(Math.min((w - 2 * pad) / W0, (h - top - pad) / H0, 1.5));
  return {k, x: (w - W0 * k) / 2, y: top + (h - top - pad - H0 * k) / 2};
}
function go(to, animate) {
  cancelAnimationFrame(anim);
  if (!animate || reduced.matches) { Object.assign(view, to); apply(); return; }
  const from = {...view}, t0 = performance.now(), dur = 220;
  const step = now => {
    const t = Math.min(1, (now - t0) / dur), e = 1 - Math.pow(1 - t, 3);
    // Interpolate the scale geometrically so zooming feels even.
    view.k = from.k * Math.pow(to.k / from.k, e);
    const s = (view.k - from.k) / ((to.k - from.k) || 1);
    view.x = to.k === from.k ? from.x + (to.x - from.x) * e : from.x + (to.x - from.x) * s;
    view.y = to.k === from.k ? from.y + (to.y - from.y) * e : from.y + (to.y - from.y) * s;
    apply();
    if (t < 1) anim = requestAnimationFrame(step);
  };
  anim = requestAnimationFrame(step);
}
function zoomAt(px, py, f, animate) {
  const k = clampK(view.k * f), r = k / view.k;
  fitted = false;
  go({k, x: px - (px - view.x) * r, y: py - (py - view.y) * r}, animate);
}
function zoomCenter(f) { const {w, h} = size(); zoomAt(w / 2, h / 2, f, true); }
function panBy(dx, dy, animate) { fitted = false; go({k: view.k, x: view.x + dx, y: view.y + dy}, animate); }
function fit(animate) { fitted = true; go(fitView(), animate); }
const toScreen = (x, y) => ({x: view.x + x * view.k, y: view.y + y * view.k});
const toDiagram = (x, y) => ({x: (x - view.x) / view.k, y: (y - view.y) / view.k});

// Pans an element into view, centring it if it is outside the margins.
function reveal(it) {
  const b = it.el.getBBox(), {w, h} = size(), m = cfg.embed ? 12 : 32;
  const a = toScreen(b.x, b.y), z = toScreen(b.x + b.width, b.y + b.height);
  if (a.x >= m && a.y >= TOP && z.x <= w - m && z.y <= h - m) return;
  const cx = b.x + b.width / 2, cy = b.y + b.height / 2;
  fitted = false;
  go({k: view.k, x: w / 2 - cx * view.k, y: h / 2 - cy * view.k}, true);
}

// Centres a box (diagram units) below the toolbar and above the path
// banner, at 100% (or the fitted scale, if larger) unless that would not
// fit it.
function frame(b, animate) {
  const {w, h} = size(), m = Math.min(cfg.embed ? 16 : 48, w * 0.06), bottom = banner.hidden ? m : 64;
  const k = clampK(Math.min(Math.max(fitView().k, 1), (w - 2 * m) / (b.w || 1), (h - TOP - bottom) / (b.h || 1)));
  fitted = false;
  go({k, x: w / 2 - (b.x + b.w / 2) * k, y: TOP + (h - TOP - bottom) / 2 - (b.y + b.h / 2) * k}, animate);
}

// ---- Highlighting ----

// focus is the clicked (or Enter-ed) element, hover the one under the
// pointer or keyboard focus, path a traced route between two vertices.
// Focus and paths dim everything else strongly, hover a little; an active
// search dims everything but its matches.
const st = {focus: null, hover: null, kbd: false, matches: [], idx: -1, path: null};
let lit = [], on = new Set();
const searching = () => !!(q && q.value.trim());

function related(it) {
  const on = new Set([it.id]);
  const add = e => { on.add(e.id); on.add(e.from); on.add(e.to); };
  if (it.kind === 'edge') add(it);
  else if (it.kind === 'node') it.edges.forEach(add);
  else {
    // A group: the edges crossing its border and their ends, and the
    // elements inside it, but not the edges within it.
    for (const e of edges) if (within(e.from, it.id) !== within(e.to, it.id)) add(e);
    for (const o of items.values()) if (within(o.id, it.id)) on.add(o.id);
  }
  for (const id of [...on]) ancestors(id).forEach(a => on.add(a));
  return on;
}

function paint() {
  for (const el of lit) el.classList.remove('dg-on', 'dg-primary', 'dg-match', 'dg-trace');
  lit = [];
  const mark = (it, cls) => {
    for (const el of [it.el, it.labelEl]) if (el) { el.classList.add(...cls); lit.push(el); }
  };
  let set = null, strong = false, primary = [];
  if (st.path) {
    const p = st.path;
    set = new Set([p.from.id, p.to.id]);
    if (p.route) {
      p.route.ids.forEach(id => set.add(id));
      p.route.edges.forEach(e => { set.add(e.id); mark(e, ['dg-trace']); });
    }
    for (const id of [...set]) ancestors(id).forEach(a => set.add(a));
    strong = true; primary = [p.from, p.to];
  } else if (st.focus) { set = related(st.focus); strong = true; primary = [st.focus]; }
  else if (st.matches.length) {
    set = new Set();
    for (const m of st.matches) { set.add(m.id); ancestors(m.id).forEach(a => set.add(a)); }
    strong = true;
    if (st.matches[st.idx]) primary = [st.matches[st.idx]];
  } else if (searching()) { set = new Set(); strong = true; }
  else if (st.hover) { set = related(st.hover); primary = [st.hover]; }
  on = set || new Set();
  svg.classList.toggle('dg-dim', !!set);
  svg.classList.toggle('dg-strong', strong);
  for (const id of on) { const it = items.get(id); if (it) mark(it, ['dg-on']); }
  for (const m of st.matches) if (!primary.includes(m)) mark(m, ['dg-match']);
  for (const p of primary) mark(p, ['dg-primary']);
  for (const n of nodes) n.el.setAttribute('aria-pressed', String(st.focus === n));
  syncHash();
}

// ---- Deep links ----

// The URL's fragment names what is focused, #<id>, or the traced path,
// #from=<id>&to=<id>, with IDs percent-encoded ('/', ':' and '@' are kept
// for readability). An embed's fragments start with its name and a colon,
// #<name>:<id>, so several embeds can share a page. Changes replace the
// history entry rather than add one. A fragment that names nothing here is
// left alone.
const enc = s => encodeURIComponent(s).replace(/%(2F|3A|40)/gi, decodeURIComponent);
const dec = s => { try { return decodeURIComponent(s); } catch (e) { return null; } };
const prefix = cfg.hash ? '#' + enc(cfg.hash) + ':' : '#';
let synced = '';
function wantedHash() {
  if (!linking) return '';
  if (st.path) return prefix + 'from=' + enc(st.path.from.id) + '&to=' + enc(st.path.to.id);
  return st.focus ? prefix + enc(st.focus.id) : '';
}
function syncHash() {
  const h = wantedHash();
  if (h === synced) return;
  // Clearing removes the fragment only if it is still ours.
  const mine = !h && location.hash !== synced && location.hash !== dec(synced);
  synced = h;
  if (mine) return;
  try { history.replaceState(history.state, '', h || location.pathname + location.search); } catch (e) {}
}
function readHash() {
  if (!linking) return null;
  let raw = location.hash;
  if (!raw || raw === '#') return synced ? {} : null;
  if (!raw.startsWith(prefix)) {
    const d = dec(raw);
    if (!d || !d.startsWith(dec(prefix))) return null;
    raw = d;
  }
  raw = raw.slice(prefix.length);
  const it = items.get(dec(raw));
  if (it && !it.hidden) return {focus: it};
  const p = {};
  for (const part of raw.split('&')) {
    const i = part.indexOf('=');
    if (i > 0) p[part.slice(0, i)] = dec(part.slice(i + 1));
  }
  const a = items.get(p.from), b = items.get(p.to);
  return vertex(a) && vertex(b) && a !== b ? {path: [a, b]} : null;
}
function applyHash(animate) {
  const h = readHash();
  if (!h) return false;
  if (q && q.value) { q.value = ''; searchBox.classList.remove('dg-has'); st.matches = []; st.idx = -1; count.textContent = ''; }
  if (h.path) {
    trace(h.path[0], h.path[1]);
    const r = st.path.route;
    frame(union((r ? r.ids.map(id => items.get(id)).concat(r.edges) : h.path).map(it => boxOf(it.el))), animate);
  } else if (h.focus) {
    clearPath(true);
    setFocus(h.focus);
    frame(boxOf(h.focus.el), animate);
  } else {
    clearPath(true);
    setFocus(null);
  }
  synced = wantedHash();
  return !!(h.path || h.focus);
}

// Copies the page's URL, which names what is focused or traced, and says
// so on btn.
async function copyLink(btn) {
  const url = location.href;
  let ok = false;
  try { await navigator.clipboard.writeText(url); ok = true; } catch (e) {}
  if (!ok) {
    const ta = doc.createElement('textarea'), was = R.activeElement;
    ta.value = url;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;left:-9999px;top:0;opacity:0';
    btn.parentNode.appendChild(ta);
    ta.select();
    try { ok = doc.execCommand('copy'); } catch (e) {}
    ta.remove();
    if (was && was.focus) was.focus({preventScroll: true});
  }
  if (!ok) { try { prompt('Copy this link:', url); ok = null; } catch (e) {} }
  if (ok === null) return;
  const label = btn.querySelector('.c-label');
  clearTimeout(btn.dgT);
  btn.classList.toggle('dg-copied', ok);
  btn.querySelector('svg').outerHTML = ok ? iconCheck : iconLink;
  label.textContent = ok ? 'Copied' : 'Copy failed';
  btn.dgT = setTimeout(() => {
    btn.classList.remove('dg-copied');
    btn.querySelector('svg').outerHTML = iconLink;
    label.textContent = 'Copy link';
  }, 1400);
}
function copyButton() {
  const b = doc.createElement('button');
  b.type = 'button'; b.className = 'c-copy';
  b.innerHTML = iconLink + '<span class="c-label" aria-live="polite">Copy link</span>';
  b.addEventListener('click', () => copyLink(b));
  return b;
}

// ---- Path tracing ----

// Paths follow edges between vertices (nodes and groups). An edge to or
// from a group connects to the group and to everything inside it, so a
// path can go in through an edge to a group and on from a node inside.
// The shortest path by hop count along edge directions wins; failing that,
// the shortest ignoring direction. Ties go to edges declared first.
const members = id => {
  const it = items.get(id), out = [id];
  if (it && it.kind === 'group') for (const o of items.values()) if (o.id !== id && vertex(o) && within(o.id, id)) out.push(o.id);
  return out;
};
const adj = new Map();
const link = (a, b, e, fwd) => { if (!adj.has(a)) adj.set(a, []); adj.get(a).push({e, to: b, fwd}); };
for (const e of edges) {
  if (e.from === e.to) continue;
  const tos = members(e.to);
  for (const a of members(e.from)) for (const b of tos) if (a !== b) { link(a, b, e, true); link(b, a, e, false); }
}
function route(a, b, directed) {
  const prev = new Map([[a.id, null]]), queue = [a.id];
  for (let i = 0; i < queue.length && !prev.has(b.id); i++) {
    for (const s of adj.get(queue[i]) || []) {
      if ((directed && !s.fwd) || prev.has(s.to)) continue;
      prev.set(s.to, {id: queue[i], e: s.e});
      queue.push(s.to);
    }
  }
  if (!prev.has(b.id)) return null;
  const ids = [b.id], es = [];
  for (let p = prev.get(b.id); p; p = prev.get(p.id)) { ids.unshift(p.id); es.unshift(p.e); }
  return {ids, edges: es};
}
function trace(a, b) {
  let r = route(a, b, true), directed = true;
  if (!r) { r = route(a, b, false); directed = false; }
  st.path = {from: a, to: b, route: r, directed};
  st.focus = null;
  hideTip();
  showBanner();
  paint();
}
function clearPath(quiet) {
  if (!st.path) return;
  st.path = null;
  banner.hidden = true;
  if (!quiet) { paint(); hideTip(); }
}
function showBanner() {
  const p = st.path, text = banner.querySelector('.p-route'), note = banner.querySelector('.p-note');
  const b = s => { const e = doc.createElement('b'); e.textContent = s; return e; };
  if (p.route) {
    const n = p.route.edges.length;
    text.replaceChildren(b(nameOf(p.from.id)), ' → ', b(nameOf(p.to.id)), ` · ${n} ${n === 1 ? 'hop' : 'hops'}`);
    note.textContent = p.directed ? '' : 'No directed path; showing undirected';
  } else {
    text.replaceChildren('No path between ', b(nameOf(p.from.id)), ' and ', b(nameOf(p.to.id)));
    note.textContent = '';
  }
  banner.hidden = false;
}
banner.querySelector('.p-x').addEventListener('click', () => clearPath());
const bannerCopy = banner.querySelector('.p-copy');
bannerCopy.addEventListener('click', () => copyLink(bannerCopy));
// The vertex a Shift-click or Shift-Enter traces from.
const traceFrom = () => st.path ? st.path.from : vertex(st.focus) ? st.focus : null;

// ---- Tooltip ----

// The tooltip describes the focused element (pinned, until focus clears)
// or else the hovered one. It is anchored to a node's box, or to the
// point where an edge or group was pointed at, in diagram coordinates so
// it follows pan and zoom. Pointing at the tooltip keeps it open, so its
// links and buttons can be clicked.
let tipItem = null, tipPinned = false, anchor = null, slot = -1, showT = 0, hideT = 0;

function tipFor(it, pinned) {
  const t = (cls, s) => { const d = doc.createElement('div'); d.className = cls; d.textContent = s; return d; };
  const parts = [];
  if (it.kind === 'edge') {
    parts.push(t('t-title', nameOf(it.from) + ' → ' + nameOf(it.to)));
    if (it.label) parts.push(t('t-desc', it.label));
  } else {
    parts.push(t('t-title', it.label || it.id));
    if (it.desc) parts.push(t('t-desc', it.desc));
  }
  if (it.tooltip) parts.push(t('t-text', it.tooltip));
  if (it.link) {
    const row = doc.createElement('div');
    row.className = 't-link';
    const a = doc.createElement('a');
    a.href = it.link; a.target = '_blank'; a.rel = 'noopener noreferrer'; a.textContent = 'Open ↗';
    const u = t('t-url', it.link.replace(/^https?:\/\//, '').replace(/^mailto:/, ''));
    row.append(a, u);
    parts.push(row, t('t-hint', modKey + '-click or ' + modKey + '-Enter opens directly'));
  }
  if (pinned && linking) {
    const row = doc.createElement('div');
    row.className = 't-actions';
    const k = doc.createElement('kbd');
    k.textContent = 'C';
    row.append(copyButton(), k);
    parts.push(row);
  }
  return parts;
}

// anchorFor returns a box in diagram units, or a point (w = h = 0) where
// an edge or group was pointed at. Without a pointer an edge is anchored
// to its label or midpoint, and a group to its header.
function anchorFor(it, ev) {
  if (it.kind !== 'node' && ev) {
    const l = local(ev), p = toDiagram(l.x, l.y);
    return {x: p.x, y: p.y, w: 0, h: 0, point: true};
  }
  if (it.kind === 'edge') {
    if (it.labelEl) return {...boxOf(it.labelEl)};
    if (it.line) { const l = it.line, p = l.getPointAtLength(l.getTotalLength() / 2); return {x: p.x, y: p.y, w: 0, h: 0, point: true}; }
  }
  const b = boxOf(it.el);
  return it.kind === 'group' ? {x: b.x, y: b.y, w: b.w, h: Math.min(b.h, 44)} : {...b};
}

function showTip(it, ev, pinned = st.focus === it) {
  clearTimeout(hideT); clearTimeout(showT);
  hideT = 0;
  if (tipItem === it && tipPinned === pinned && ev == null && anchor) { placeTip(); return; }
  if (tipItem !== it) slot = -1;
  tipItem = it; tipPinned = pinned;
  anchor = anchorFor(it, ev);
  tip.replaceChildren(...tipFor(it, pinned));
  tip.hidden = false;
  placeTip();
  requestAnimationFrame(() => tip.classList.add('dg-show'));
}
function hideTip() {
  clearTimeout(showT); clearTimeout(hideT);
  hideT = 0;
  tipItem = null; anchor = null; slot = -1;
  tip.classList.remove('dg-show');
  tip.hidden = true;
}

// Sample points along an edge, every few units, for testing what the
// tooltip would cover.
function samples(e) {
  if (e.pts) return e.pts;
  e.pts = [];
  if (!e.line) return e.pts;
  const len = e.line.getTotalLength(), n = Math.min(400, Math.ceil(len / 5));
  for (let i = 0; i <= n; i++) { const p = e.line.getPointAtLength(len * i / n); e.pts.push(p.x, p.y); }
  return e.pts;
}
const hits = (b, r) => b.x <= r.x + r.w && b.x + b.w >= r.x && b.y <= r.y + r.h && b.y + b.h >= r.y;
const screenBox = el => { const r = el.getBoundingClientRect(); return {x: r.left, y: r.top, w: r.width, h: r.height}; };

// The tooltip goes beside its anchor (right, left, below, above, then the
// same further out), inside the viewport, wherever it covers least:
// highlighted elements and edge labels above all, the toolbar and path
// banner, never its anchor, and faintly anything else so ties go to open
// space. It stays put while its place is still as good as any. Places are
// worked out in the stage's coordinates; the tooltip itself is fixed to
// the viewport, so it can overhang a small embed.
function placeTip() {
  if (!tipItem || !anchor) return;
  const {w, h} = size(), o = origin(), vw = innerWidth, vh = innerHeight;
  // An embed scrolled out of sight takes its tooltip with it.
  tip.style.visibility = o.x + w < 0 || o.y + h < 0 || o.x > vw || o.y > vh ? 'hidden' : '';
  const tw = tip.offsetWidth, th = tip.offsetHeight, k = view.k;
  const a = toScreen(anchor.x, anchor.y), z = toScreen(anchor.x + anchor.w, anchor.y + anchor.h);
  const cands = [];
  for (const d of anchor.point ? [14, 44, 88] : [8, 40, 80]) {
    if (anchor.point) {
      cands.push([a.x + d, a.y + d], [a.x + d, a.y - d - th], [a.x - d - tw, a.y + d], [a.x - d - tw, a.y - d - th]);
      continue;
    }
    const ys = [a.y, (a.y + z.y - th) / 2, z.y - th], xs = [(a.x + z.x - tw) / 2, a.x, z.x - tw];
    for (const y of ys) cands.push([z.x + d, y]);
    for (const y of ys) cands.push([a.x - d - tw, y]);
    for (const x of xs) cands.push([x, z.y + d]);
    for (const x of xs) cands.push([x, a.y - d - th]);
  }

  // What is near enough to matter, in diagram units.
  const reach = (Math.max(tw, th) + 100) / k;
  const near = {x: anchor.x - reach, y: anchor.y - reach, w: anchor.w + 2 * reach, h: anchor.h + 2 * reach};
  const obs = [];
  for (const it of items.values()) {
    if (it.hidden || it === tipItem && it.kind !== 'edge') continue;
    const lit = on.has(it.id);
    if (it.kind === 'group') {
      // Only a group's header, where its name is.
      const b = boxOf(it.el), hd = {x: b.x, y: b.y, w: b.w, h: Math.min(b.h, 44)};
      if (hits(hd, near)) obs.push({b: hd, w: lit ? 1 : 0.5});
      continue;
    }
    if (hits(boxOf(it.el), near)) obs.push({b: boxOf(it.el), pts: it.kind === 'edge' ? samples(it) : null, w: lit ? 3 : 0.25});
    if (it.labelEl && hits(boxOf(it.labelEl), near)) obs.push({b: boxOf(it.labelEl), w: lit ? 4 : 2});
  }
  const r = anchor.point ? 6 / k : 0;
  obs.push({b: {x: anchor.x - r, y: anchor.y - r, w: anchor.w + 2 * r, h: anchor.h + 2 * r}, w: 20});
  const chrome = [bar, banner].filter(el => el && !el.hidden && el.offsetWidth).map(screenBox);

  const m = 3; // breathing room around the tooltip, in px
  let best = -1, bestCost = Infinity;
  const placed = cands.map(([x, y], i) => {
    // Keep it inside the viewport.
    x = Math.max(8 - o.x, Math.min(vw - tw - 8 - o.x, x));
    y = Math.max(8 - o.y, Math.min(vh - th - 8 - o.y, y));
    const s = {x: x - m, y: y - m, w: tw + 2 * m, h: th + 2 * m};
    const p = toDiagram(s.x, s.y), d = {x: p.x, y: p.y, w: s.w / k, h: s.h / k};
    let c = 0;
    for (const ob of obs) {
      if (!hits(ob.b, d)) continue;
      if (ob.pts) {
        let inside = false;
        for (let j = 0; j < ob.pts.length && !inside; j += 2) {
          const px = ob.pts[j], py = ob.pts[j + 1];
          inside = px >= d.x && px <= d.x + d.w && py >= d.y && py <= d.y + d.h;
        }
        if (!inside) continue;
      }
      c += ob.w;
    }
    const sv = {x: s.x + o.x, y: s.y + o.y, w: s.w, h: s.h};
    for (const b of chrome) if (hits(b, sv)) c += 10;
    // An embed's tooltip may overhang it, but would rather not cover the page.
    if (cfg.embed && (s.x < 0 || s.y < 0 || s.x + s.w > w || s.y + s.h > h)) c += 1.5;
    if (c < bestCost) { best = i; bestCost = c; }
    return {x, y, c};
  });
  if (slot >= 0 && placed[slot] && placed[slot].c <= bestCost) best = slot;
  slot = best;
  tip.style.transform = '';
  tip.style.left = Math.round(placed[best].x + o.x) + 'px';
  tip.style.top = Math.round(placed[best].y + o.y) + 'px';
}
// Hover tooltips wait until the pointer rests, unless one is already
// showing, and linger briefly so the pointer can move onto them.
// A plain group's label and description are in its header already, so
// hovering it shows a tooltip only for its Tooltip text or link.
const tipWorthy = it => it && (it.kind !== 'group' || it.tooltip || it.link) ? it : null;
function hoverTip(it, ev) {
  if (st.focus) return;
  it = tipWorthy(it);
  if (it && it === tipItem) { clearTimeout(hideT); hideT = 0; return; }
  clearTimeout(showT);
  if (!it) { if (tipItem && !hideT) hideT = setTimeout(hideTip, 150); return; }
  clearTimeout(hideT); hideT = 0;
  const e = {clientX: ev.clientX, clientY: ev.clientY};
  if (tipItem) showTip(it, e); else showT = setTimeout(() => showTip(it, e), 280);
}
tip.addEventListener('pointerenter', () => { clearTimeout(hideT); hideT = 0; });
tip.addEventListener('pointerleave', ev => {
  if (stage.contains(ev.relatedTarget) || st.focus || st.kbd) return;
  if (st.hover) { st.hover = null; paint(); }
  hideT = setTimeout(hideTip, 150);
});

// ---- Focus ----

function setFocus(it, ev) {
  if (it) clearPath(true);
  st.focus = it;
  paint();
  if (it) showTip(it, ev, true); else if (st.hover && st.kbd) showTip(st.hover); else hideTip();
}
function toggleFocus(it, ev) { setFocus(st.focus === it ? null : it, ev); }
function open(it) {
  if (!it || !it.link) return false;
  window.open(it.link, '_blank', 'noopener,noreferrer');
  return true;
}
// Selects it: with Shift and a focused vertex or traced path, traces to
// it; otherwise toggles its focus.
function select(it, shift, ev) {
  const from = shift && traceFrom();
  if (from && vertex(it) && it !== from) trace(from, it);
  else if (!(shift && st.path && it === st.path.from)) toggleFocus(it, ev);
}
function clearAll() {
  if (st.path) clearPath();
  else if (st.focus) setFocus(null);
}

// ---- Pointer: drag to pan, pinch to zoom, tap to focus ----

const pts = new Map();
let drag = null, pinch = null;
stage.addEventListener('pointerdown', ev => {
  if (ev.pointerType === 'mouse' && ev.button !== 0) return;
  pts.set(ev.pointerId, local(ev));
  if (pts.size === 1) {
    drag = {id: ev.pointerId, sx: ev.clientX, sy: ev.clientY, x: view.x, y: view.y, moved: false, item: itemOf(ev.target)};
  } else if (pts.size === 2) {
    const [a, b] = [...pts.values()];
    pinch = {d: Math.hypot(a.x - b.x, a.y - b.y) || 1, k: view.k, cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2, x: view.x, y: view.y};
    if (drag) drag.moved = true;
  }
});
stage.addEventListener('pointermove', ev => {
  if (pts.has(ev.pointerId)) pts.set(ev.pointerId, local(ev));
  if (pinch && pts.size >= 2) {
    const [a, b] = [...pts.values()];
    const k = clampK(pinch.k * Math.hypot(a.x - b.x, a.y - b.y) / pinch.d), r = k / pinch.k;
    const cx = (a.x + b.x) / 2, cy = (a.y + b.y) / 2;
    fitted = false;
    go({k, x: cx - (pinch.cx - pinch.x) * r, y: cy - (pinch.cy - pinch.y) * r});
    return;
  }
  if (drag && ev.pointerId === drag.id) {
    const dx = ev.clientX - drag.sx, dy = ev.clientY - drag.sy;
    if (!drag.moved && Math.hypot(dx, dy) > 4) {
      drag.moved = true;
      stage.setPointerCapture(ev.pointerId);
      stage.classList.add('dg-dragging');
      if (!st.focus) hideTip();
    }
    if (drag.moved) { fitted = false; go({k: view.k, x: drag.x + dx, y: drag.y + dy}); }
    return;
  }
  if (!drag && ev.pointerType === 'mouse') {
    const it = itemOf(ev.target);
    if (it !== st.hover || st.kbd) {
      st.hover = it; st.kbd = false;
      paint();
    }
    hoverTip(it, ev);
  }
});
function endPointer(ev, cancelled) {
  pts.delete(ev.pointerId);
  if (pts.size < 2) pinch = null;
  if (!drag || ev.pointerId !== drag.id) return;
  const d = drag;
  drag = null;
  stage.classList.remove('dg-dragging');
  if (cancelled || d.moved) return;
  if (d.item && (ev.metaKey || ev.ctrlKey) && open(d.item)) return;
  if (d.item) select(d.item, ev.shiftKey, ev);
  else clearAll();
}
stage.addEventListener('pointerup', ev => endPointer(ev, false));
stage.addEventListener('pointercancel', ev => endPointer(ev, true));
stage.addEventListener('pointerleave', ev => {
  if (ev.pointerType !== 'mouse' || drag || tip.contains(ev.relatedTarget)) return;
  if (st.hover && !st.kbd) { st.hover = null; paint(); }
  if (!st.focus) hoverTip(null, ev);
});
stage.addEventListener('contextmenu', ev => { if (drag) ev.preventDefault(); });

// A plain wheel, or a two-finger trackpad drag, pans; Shift turns a
// mouse wheel sideways. Ctrl or ⌘ with the wheel zooms around the
// pointer, and so does a pinch, which arrives as a wheel event with
// ctrlKey set, in finer steps. Embeds behave the same.
stage.addEventListener('wheel', ev => {
  ev.preventDefault();
  const unit = ev.deltaMode === 1 ? 16 : ev.deltaMode === 2 ? size().h : 1;
  if (!ev.ctrlKey && !ev.metaKey) {
    let dx = ev.deltaX * unit, dy = ev.deltaY * unit;
    if (ev.shiftKey && !dx) { dx = dy; dy = 0; }
    panBy(-dx, -dy);
    return;
  }
  const dy = Math.max(-120, Math.min(120, ev.deltaY * unit)), p = local(ev);
  zoomAt(p.x, p.y, Math.exp(-dy * (ev.ctrlKey ? 0.012 : 0.002)));
}, {passive: false});
// Safari reports trackpad pinches as gesture events.
let gScale = 1;
stage.addEventListener('gesturestart', ev => { ev.preventDefault(); gScale = 1; });
stage.addEventListener('gesturechange', ev => {
  ev.preventDefault();
  const p = local(ev);
  zoomAt(p.x, p.y, ev.scale / gScale);
  gScale = ev.scale;
});
// Focusing an element must never scroll the stage under the view.
stage.addEventListener('scroll', () => { stage.scrollTop = 0; stage.scrollLeft = 0; });

// ---- Keyboard ----

// Tab and Shift-Tab move between nodes in declaration order (their DOM
// order). A node with keyboard focus is highlighted like a hovered one.
stage.addEventListener('focusin', ev => {
  const it = itemOf(ev.target);
  if (!it || !ev.target.matches(':focus-visible')) return;
  st.hover = it; st.kbd = true;
  paint();
  reveal(it);
  if (!st.focus) showTip(it);
});
stage.addEventListener('focusout', ev => {
  if (!st.kbd) return;
  if (ev.relatedTarget && svg.contains(ev.relatedTarget)) return;
  st.hover = null; st.kbd = false;
  paint();
  if (!st.focus) hideTip();
});

const typing = el => el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
on$(cfg.keys, 'keydown', ev => {
  const mod = ev.metaKey || ev.ctrlKey;
  if (help.open) {
    if (ev.key === 'Escape' || (ev.key === '?' && !mod)) { ev.preventDefault(); toggleHelp(false); }
    return;
  }
  if (typing(ev.composedPath()[0])) return;
  const activeEl = R.activeElement, active = itemOf(activeEl);
  if (ev.key === 'Enter' || (ev.key === ' ' && active)) {
    if (!active) return;
    ev.preventDefault();
    if (mod && open(active)) return;
    select(active, ev.shiftKey);
    return;
  }
  if (ev.key === 'Escape') {
    if (st.path || st.focus) clearAll();
    else if (q && q.value) clearSearch();
    else { hideTip(); if (active) activeEl.blur(); }
    return;
  }
  if (mod || ev.altKey) return;
  const step = ev.shiftKey ? 240 : 60;
  switch (ev.key) {
    case '/': if (!q) return; ev.preventDefault(); q.focus(); q.select(); return;
    case '?': toggleHelp(true); break;
    case 'c': case 'C': {
      const btn = !linking ? null : st.path ? bannerCopy : st.focus && tip.querySelector('.c-copy');
      if (!btn) return;
      copyLink(btn);
      break;
    }
    case '+': case '=': zoomCenter(1.25); break;
    case '-': case '_': zoomCenter(1 / 1.25); break;
    case '0': fit(true); break;
    case 'ArrowLeft': panBy(step, 0, true); break;
    case 'ArrowRight': panBy(-step, 0, true); break;
    case 'ArrowUp': panBy(0, step, true); break;
    case 'ArrowDown': panBy(0, -step, true); break;
    default: return;
  }
  ev.preventDefault();
});

// ---- Search ----

const searchBox = q && q.closest('.dg-search');
function runSearch() {
  const s = q.value.trim().toLowerCase();
  searchBox.classList.toggle('dg-has', !!q.value);
  st.matches = s ? nodes.filter(n => n.label.toLowerCase().includes(s)) : [];
  st.idx = st.matches.length ? 0 : -1;
  if (s) { st.focus = null; clearPath(true); }
  count.textContent = !s ? '' : st.matches.length ? `1/${st.matches.length}` : 'No matches';
  paint();
  if (st.matches.length) reveal(st.matches[0]);
  if (!st.focus) hideTip();
}
function stepSearch(d) {
  const n = st.matches.length;
  if (!n) return;
  st.idx = (st.idx + d + n) % n;
  count.textContent = `${st.idx + 1}/${n}`;
  paint();
  reveal(st.matches[st.idx]);
  showTip(st.matches[st.idx]);
}
function clearSearch() {
  q.value = '';
  runSearch();
}
if (q) {
  q.addEventListener('input', runSearch);
  q.addEventListener('keydown', ev => {
    if (ev.key === 'Enter' || ev.key === 'ArrowDown') { ev.preventDefault(); stepSearch(ev.shiftKey && ev.key === 'Enter' ? -1 : 1); }
    else if (ev.key === 'ArrowUp') { ev.preventDefault(); stepSearch(-1); }
    else if (ev.key === 'Escape') {
      ev.preventDefault();
      if (q.value) clearSearch(); else q.blur();
      hideTip();
    }
  });
}

// ---- Toolbar ----

const click = (id, f) => { const b = $(id); if (b) b.addEventListener('click', f); };
click('dg-in', () => zoomCenter(1.25));
click('dg-out', () => zoomCenter(1 / 1.25));
click('dg-fit', () => fit(true));
click('dg-zoom', () => zoomCenter(1 / view.k));

// Theme: a light/dark toggle that starts on the viewer's system theme. A
// page's choice is its root's data-theme, remembered per viewer; an
// embed's is its element's theme attribute, which the fragment or the
// page may also set. Toggling back to the system's theme follows the
// system again.
if (themeBtn) {
  const el = cfg.host || root, attr = cfg.host ? 'theme' : 'data-theme';
  const sys = matchMedia('(prefers-color-scheme: dark)');
  const system = () => sys.matches ? 'dark' : 'light';
  const mode = () => { const m = el.getAttribute(attr); return m === 'light' || m === 'dark' ? m : system(); };
  const show = () => {
    const dark = mode() === 'dark';
    themeBtn.setAttribute('aria-pressed', String(dark));
    themeBtn.title = dark ? 'Dark theme (click for light)' : 'Light theme (click for dark)';
  };
  if (cfg.host) {
    const mo = new MutationObserver(show);
    mo.observe(cfg.host, {attributes: true, attributeFilter: ['theme']});
    if (cfg.signal) cfg.signal.addEventListener('abort', () => mo.disconnect());
  } else {
    try { const m = localStorage.getItem('diagram-theme'); if (m === 'light' || m === 'dark') el.setAttribute(attr, m); } catch (e) {}
  }
  on$(sys, 'change', show);
  show();
  themeBtn.addEventListener('click', () => {
    const m = mode() === 'dark' ? 'light' : 'dark', follow = m === system();
    if (follow) el.removeAttribute(attr); else el.setAttribute(attr, m);
    if (!cfg.host) try { if (follow) localStorage.removeItem('diagram-theme'); else localStorage.setItem('diagram-theme', m); } catch (e) {}
    show();
  });
}

// Shortcut help: a modal dialog, toggled by ? or the toolbar button and
// closed by Esc, its close button or a click outside it.
for (const k of help.querySelectorAll('.dg-mod')) k.textContent = modKey;
function toggleHelp(show) {
  if (show && !help.open) {
    hideTip();
    if (help.showModal) help.showModal(); else help.setAttribute('open', '');
  } else if (!show && help.open) {
    if (help.close) help.close(); else help.removeAttribute('open');
  }
  if (helpBtn) helpBtn.setAttribute('aria-expanded', String(help.open));
}
if (helpBtn) helpBtn.addEventListener('click', () => toggleHelp(!help.open));
help.querySelector('.h-x').addEventListener('click', () => toggleHelp(false));
help.addEventListener('click', ev => { if (ev.target === help) toggleHelp(false); });
help.addEventListener('close', () => { if (helpBtn) helpBtn.setAttribute('aria-expanded', 'false'); });

const ro = new ResizeObserver(() => { if (fitted) fit(false); else apply(); });
ro.observe(stage);
if (cfg.signal) cfg.signal.addEventListener('abort', () => { ro.disconnect(); cancelAnimationFrame(anim); clearTimeout(hintT); hideTip(); });
on$(window, 'hashchange', () => applyHash(true));
if (cfg.embed) on$(window, 'scroll', placeTip, {passive: true, capture: true});
fit(false);
const linked = applyHash(false);
if (cfg.view && cfg.view.k > 0) {
  if (cfg.view.fit) fit(false);
  else { fitted = false; go({k: clampK(cfg.view.k), x: +cfg.view.x || 0, y: +cfg.view.y || 0}); }
}

// The view, for tools: get() returns the pan and zoom, set() restores it.
return {
  linked,
  get: () => ({k: view.k, x: view.x, y: view.y, fit: fitted}),
  set: v => { if (!v || !(v.k > 0)) return; if (v.fit) { fit(false); return; } fitted = false; go({k: clampK(v.k), x: +v.x || 0, y: +v.y || 0}); },
};
}
