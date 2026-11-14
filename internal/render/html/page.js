// The full page is one view, with the page's own fragment for deep links
// and keys heard anywhere. Its pan and zoom survive a reload in the same
// tab (diagram watch reloads on every change), keyed by the page title
// and the diagram's viewBox, and window.diagramView gets and sets them
// ({k, x, y, fit}: scale, the diagram origin's place in the window, and
// whether it is fitted to the window).
const viewKey = 'diagram-view:' + document.title + ':' + document.querySelector('#dg-stage > svg').getAttribute('viewBox');
let saved = null;
try {
  const nav = performance.getEntriesByType('navigation')[0];
  if (nav && nav.type === 'reload') saved = JSON.parse(sessionStorage.getItem(viewKey));
} catch (e) {}
const page = mount(document, {keys: document, hash: '', view: saved});
window.diagramView = {get: page.get, set: page.set};
addEventListener('pagehide', () => {
  try { sessionStorage.setItem(viewKey, JSON.stringify(page.get())); } catch (e) {}
});
