// <diagram-view> is an embedded view. Its content waits in a <template>
// child; on connection it moves into an open shadow root, so the page's
// CSS and IDs and the diagram's never meet, and is wired up by mount.
// Chrome ignores @font-face inside shadow roots, so each embed's font
// subsets are registered with the document, once each, under family
// names derived from their bytes.
const fonts = new Set();
function addFonts(tpl) {
  for (const s of tpl.content.querySelectorAll('script[data-family]')) {
    const fam = s.dataset.family, weight = s.dataset.weight || '400';
    if (fonts.has(fam + ' ' + weight)) continue;
    fonts.add(fam + ' ' + weight);
    try {
      const bin = atob(s.textContent.trim()), bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      const face = new FontFace(fam, bytes, {weight});
      document.fonts.add(face);
      face.load().catch(() => {});
    } catch (e) {}
  }
}
customElements.define('diagram-view', class extends HTMLElement {
  connectedCallback() {
    if (this.dgAbort) return;
    const tpl = [...this.children].find(c => c.matches('template[data-diagram]'));
    if (!tpl) {
      // Defined before the parser reached the content: wait for it.
      if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => { if (this.isConnected) this.connectedCallback(); }, {once: true});
      }
      return;
    }
    addFonts(tpl);
    const root = this.shadowRoot || this.attachShadow({mode: 'open'});
    const content = tpl.content.cloneNode(true);
    for (const s of content.querySelectorAll('script')) s.remove();
    root.replaceChildren(content);
    this.dgAbort = new AbortController();
    const view = mount(root, {embed: true, host: this, keys: this, hash: this.getAttribute('hash') || null, signal: this.dgAbort.signal});
    this.getView = view.get;
    this.setView = view.set;
    if (view.linked) {
      // Fragments name no element on the page, so bring this one into
      // view, once the page has laid out.
      const show = () => this.scrollIntoView({block: 'center'});
      if (document.readyState === 'complete') show(); else addEventListener('load', show, {once: true});
    }
  }
  disconnectedCallback() {
    if (this.dgAbort) { this.dgAbort.abort(); this.dgAbort = null; }
  }
});
