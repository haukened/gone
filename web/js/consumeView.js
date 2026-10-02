'use strict';

// DOM side of the consume flow: status/progress, the decrypted message with
// its copy button, downloadable file entries, and the deletion banner.
// Requires window.goneUtil, window.goneFileMeta and window.goneIcons.
// Exposed as window.goneConsumeView.
(function consumeViewModule() {
  if (window.goneConsumeView || !window.goneUtil || !window.goneFileMeta || !window.goneIcons) return;
  const util = window.goneUtil;
  const icons = window.goneIcons;
  const formatBytes = window.goneFileMeta.formatBytes;

  const MAX_TEXTAREA_PX = 40 * 16;
  const DOWNLOAD_SPACING_MS = 400;
  const ACK_FAIL_TITLE = 'Couldn\u2019t confirm deletion';
  const ACK_FAIL_TEXT = 'The server did not confirm this secret was deleted. It will still be deleted automatically within a few minutes and cannot be opened again. Save what you need now.';

  function byId(id) {
    return document.getElementById(id);
  }

  // lookupDom finds the consume page elements; any may be null.
  function lookupDom() {
    return {
      status: byId('secret-heading'),
      output: byId('secret-output'),
      copy: byId('copy-secret'),
      downloadAll: byId('download-all'),
      progress: byId('download-progress'),
      fileSection: byId('file-section'),
      fileList: byId('file-output-list'),
      ackBanner: byId('ack-banner'),
      ackCard: byId('ack-card'),
      ackTitle: byId('ack-title'),
      ackText: byId('ack-text')
    };
  }

  const dom = lookupDom();
  const state = { plaintext: null, files: [], urls: [], pending: 0 };

  function setStatus(msg) {
    util.setText(dom.status, msg);
  }

  // setProgress shows download progress as a percentage, or as bytes received
  // when the total size is unknown.
  function setProgress(received, total) {
    const bar = dom.progress;
    if (!bar) return;
    bar.hidden = false;
    if (total > 0) {
      bar.max = total;
      bar.value = Math.min(received, total);
      setStatus(`Retrieving\u2026 ${Math.floor((received / total) * 100)}%`);
      return;
    }
    bar.removeAttribute('value');
    setStatus(`Retrieving\u2026 ${formatBytes(received)}`);
  }

  function hideProgress() {
    if (dom.progress) dom.progress.hidden = true;
  }

  function autoGrow(ta) {
    ta.style.height = 'auto';
    ta.style.height = Math.min(ta.scrollHeight, MAX_TEXTAREA_PX) + 'px';
    ta.style.overflowY = ta.scrollHeight > MAX_TEXTAREA_PX ? 'auto' : 'hidden';
  }

  function attachCopyHandler(text) {
    const btn = dom.copy;
    if (!btn) return;
    btn.addEventListener('click', async function () {
      if (await util.copyText(text)) {
        util.flashCopied(btn, ['Copied! ', icons.make('check', '24')]);
      }
    });
  }

  // showMessage displays the decrypted text and wires its copy button.
  function showMessage(text) {
    if (!text || !dom.output) return;
    dom.output.value = text;
    dom.output.hidden = false;
    autoGrow(dom.output);
    if (dom.copy) dom.copy.hidden = false;
    attachCopyHandler(text);
  }

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
    entry.li.classList.add('downloaded');
  }

  function downloadEntry(entry) {
    const link = util.el('a', { href: fileURL(entry), download: entry.file.name, rel: 'noopener' });
    document.body.appendChild(link);
    link.click();
    link.remove();
    markDownloaded(entry);
  }

  function renderFileEntry(file) {
    const btn = util.el('button', { type: 'button', className: 'secondary-btn', textContent: 'Download' });
    btn.setAttribute('aria-label', `Download ${file.name}`);
    const li = util.el('li', { className: 'file-item' }, [
      util.el('span', { className: 'file-item-name', textContent: file.name }),
      util.el('span', { className: 'file-item-meta', textContent: formatBytes(file.size) }),
      btn
    ]);
    const entry = { file: file, url: '', done: false, li: li };
    btn.addEventListener('click', function () { downloadEntry(entry); });
    return entry;
  }

  async function downloadAll() {
    for (const entry of state.files) {
      downloadEntry(entry);
      // Space out downloads so browsers don't drop or batch-block them.
      await util.sleep(DOWNLOAD_SPACING_MS);
    }
  }

  function showDownloadAll(count) {
    if (!dom.downloadAll) return;
    dom.downloadAll.hidden = count < 2;
    dom.downloadAll.addEventListener('click', downloadAll);
  }

  // showFiles renders a download entry per decrypted file.
  function showFiles(files) {
    if (!files.length || !dom.fileList) return;
    state.files = files.map(renderFileEntry);
    state.pending = state.files.length;
    state.files.forEach(function (e) { dom.fileList.appendChild(e.li); });
    if (dom.fileSection) dom.fileSection.hidden = false;
    showDownloadAll(files.length);
  }

  function headingFor(decoded) {
    const n = decoded.files.length;
    const files = n === 1 ? '1 file' : `${n} files`;
    if (decoded.message && n) return `Decrypted Secret + ${files}:`;
    if (n) return `Decrypted ${files}:`;
    return 'Decrypted Secret:';
  }

  // showDecoded renders a decoded envelope ({message, files}).
  function showDecoded(decoded) {
    hideProgress();
    setStatus(headingFor(decoded));
    showMessage(decoded.message);
    showFiles(decoded.files);
  }

  // showAckResult reveals the deletion banner, switching it to a warning when
  // the server did not confirm deletion.
  function showAckResult(ok) {
    if (!dom.ackBanner) return;
    if (!ok) {
      if (dom.ackCard) dom.ackCard.classList.add('danger');
      util.setText(dom.ackTitle, ACK_FAIL_TITLE);
      util.setText(dom.ackText, ACK_FAIL_TEXT);
    }
    dom.ackBanner.hidden = false;
  }

  // guardUnload warns before leaving while files remain undownloaded.
  function guardUnload(ev) {
    if (state.pending <= 0) return;
    ev.preventDefault();
    ev.returnValue = '';
  }

  // keepPlaintext retains the decrypted buffer so cleanup() can zero it.
  function keepPlaintext(bytes) {
    state.plaintext = bytes;
  }

  // cleanup revokes object URLs and zeroes the decrypted buffer.
  function cleanup() {
    state.urls.forEach(function (u) { URL.revokeObjectURL(u); });
    state.urls = [];
    if (state.plaintext) state.plaintext.fill(0);
  }

  window.goneConsumeView = Object.freeze({
    present: Boolean(byId('secret-consume')),
    setStatus: setStatus,
    setProgress: setProgress,
    hideProgress: hideProgress,
    showMessage: showMessage,
    showDecoded: showDecoded,
    showAckResult: showAckResult,
    guardUnload: guardUnload,
    keepPlaintext: keepPlaintext,
    cleanup: cleanup,
    headingFor: headingFor
  });
})();
