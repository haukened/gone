'use strict';

// File rendering and download helpers for consumed secrets.
(function consumeViewFilesModule() {
  if (window.goneConsumeViewFiles || !window.goneConsumeViewBase) return;
  const ctx = window.goneConsumeViewBase;
  const dom = ctx.dom;
  const state = ctx.state;
  const DOWNLOAD_SPACING_MS = 400;

  function fileURL(entry) {
    if (!entry.url) {
      entry.url = URL.createObjectURL(new Blob([entry.file.bytes], { type: entry.file.type }));
      state.urls.push(entry.url);
    }
    return entry.url;
  }

  function markDownloaded(entry) {
    if (entry.done) return;
    entry.done = true;
    state.pending--;
    entry.li.classList.add('is-done');
  }

  function downloadEntry(entry) {
    const link = ctx.util.el('a', { href: fileURL(entry), download: entry.file.name, rel: 'noopener' });
    document.body.appendChild(link);
    link.click();
    link.remove();
    markDownloaded(entry);
  }

  function renderFileEntry(file) {
    const btn = ctx.util.el('button', { type: 'button', className: 'btn btn-secondary btn-small' }, [ctx.icons.make('down'), ctx.util.el('span', { textContent: 'Download' })]);
    btn.setAttribute('aria-label', `Download ${file.name}`);
    const li = ctx.util.el('li', {}, [ctx.icons.make('clip'), ctx.util.el('span', { className: 'name', textContent: file.name }), ctx.util.el('span', { className: 'size', textContent: ctx.formatBytes(file.size) }), btn]);
    const entry = { file: file, url: '', done: false, li: li };
    btn.addEventListener('click', function () { downloadEntry(entry); });
    return entry;
  }

  async function downloadAll() {
    for (const entry of state.files) {
      downloadEntry(entry);
      await ctx.util.sleep(DOWNLOAD_SPACING_MS);
    }
  }

  function showFiles(files) {
    if (!files.length || !dom.fileList) return;
    state.files = files.map(renderFileEntry);
    state.pending = state.files.length;
    dom.fileList.replaceChildren(...state.files.map(function (e) { return e.li; }));
    if (dom.fileSection) dom.fileSection.hidden = false;
    if (dom.downloadAll) {
      dom.downloadAll.hidden = files.length < 2;
      dom.downloadAll.addEventListener('click', downloadAll);
    }
  }

  window.goneConsumeViewFiles = Object.freeze({ showFiles: showFiles });
})();
