'use strict';

// DOM side of the consume flow. The page has three server-rendered views:
// #view-open (the Open button, progress and errors), #view-revealed (the
// decrypted message and files) and #view-gone. Requires window.goneUtil,
// window.goneFileMeta and window.goneIcons. Exposed as window.goneConsumeView.
(function consumeViewModule() {
  if (window.goneConsumeView || !window.goneUtil || !window.goneFileMeta || !window.goneIcons) return;
  const util = window.goneUtil;
  const icons = window.goneIcons;
  const formatBytes = window.goneFileMeta.formatBytes;

  const DOWNLOAD_SPACING_MS = 400;
  const VIEWS = ['open', 'revealed', 'gone'];
  const TITLES = { revealed: 'Gone \u00b7 Here\u2019s your secret', gone: 'Gone \u00b7 This secret is gone' };

  function byId(id) {
    return document.getElementById(id);
  }

  // lookupDom finds the consume page elements; any may be null.
  function lookupDom() {
    const open = byId('open-secret');
    return {
      open: open,
      openLabel: open ? open.querySelector('span') : null,
      status: byId('consume-status'),
      progress: byId('download-progress'),
      errorBox: byId('consume-error'),
      errorText: byId('consume-error-text'),
      revealedHeading: byId('revealed-heading'),
      ackWarning: byId('ack-warning'),
      messagePanel: byId('message-panel'),
      output: byId('secret-output'),
      copy: byId('copy-secret'),
      copyStatus: byId('copy-status'),
      fileSection: byId('file-section'),
      fileList: byId('file-output-list'),
      downloadAll: byId('download-all'),
      passField: byId('open-pass-field'),
      pass: byId('open-passphrase'),
      passToggle: byId('open-pass-toggle'),
      passWarn: byId('open-pass-warn')
    };
  }

  const dom = lookupDom();
  const state = { plaintext: null, files: [], urls: [], pending: 0, busy: false, locked: false };
  state.openLabel = dom.openLabel ? dom.openLabel.textContent : '';

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

  // showError puts msg in the error alert and clears the status line.
  function showError(msg) {
    hideProgress();
    setStatus('');
    util.setText(dom.errorText, msg);
    if (dom.errorBox) dom.errorBox.hidden = false;
  }

  function clearError() {
    if (dom.errorBox) dom.errorBox.hidden = true;
  }

  // syncOpen sets aria-disabled on Open: unavailable while busy, after it
  // has been locked, or while a required passphrase is empty.
  function syncOpen() {
    if (!dom.open) return;
    dom.open.setAttribute('aria-disabled', String(state.busy || state.locked || pass.missing()));
  }

  // setOpening reflects whether an open attempt is running. The button stays
  // focusable; aria-disabled tells assistive tech it won't act.
  function setOpening(busy) {
    state.busy = busy;
    if (dom.pass) dom.pass.readOnly = busy || state.locked;
    if (!dom.open) return;
    syncOpen();
    // aria-busy must be the string "true"; an empty value means false.
    if (busy) dom.open.setAttribute('aria-busy', 'true');
    else dom.open.removeAttribute('aria-busy');
    util.setText(dom.openLabel, busy ? 'Opening\u2026' : state.openLabel);
  }

  // disableOpen marks the Open button (and any passphrase field) permanently unavailable.
  function disableOpen() {
    state.locked = true;
    if (dom.pass) dom.pass.readOnly = true;
    syncOpen();
  }

  // passphraseControls owns the recipient passphrase field for v2 links.
  // Every method is a no-op (or returns '' / false) when the page has none.
  function passphraseControls() {
    let needsPass = false;

    // passphraseMissing reports whether Open needs a passphrase that is still empty.
    function passphraseMissing() {
      return needsPass && !dom.pass.value;
    }

    // setPassShown shows the passphrase as text (shown true) or masks it,
    // and relabels the toggle to match.
    function setPassShown(shown) {
      dom.pass.type = shown ? 'text' : 'password';
      util.setText(dom.passToggle ? dom.passToggle.querySelector('span') : null, shown ? 'Hide' : 'Show');
    }

    // onPassKey makes Enter in the passphrase field press Open.
    function onPassKey(ev) {
      if (ev.key !== 'Enter') return;
      ev.preventDefault();
      if (dom.open) dom.open.click();
    }

    // showPassphrase reveals the passphrase field for a v2 link, wires its
    // show toggle and Enter key, and keeps Open unavailable until it has a
    // value. Returns false when the page has no passphrase field.
    function showPassphrase() {
      if (!dom.pass) return false;
      needsPass = true;
      if (dom.passField) dom.passField.hidden = false;
      if (dom.passWarn) {
        dom.passWarn.hidden = false;
        if (dom.open) dom.open.setAttribute('aria-describedby', 'open-pass-warn open-hint');
      }
      dom.pass.addEventListener('input', function () {
        dom.pass.setAttribute('aria-invalid', 'false');
        syncOpen();
      });
      dom.pass.addEventListener('keydown', onPassKey);
      if (dom.passToggle) dom.passToggle.addEventListener('click', function () { setPassShown(dom.pass.type === 'password'); });
      syncOpen();
      return true;
    }

    // passphrase returns the typed passphrase ('' when there is no field).
    function passphrase() {
      return dom.pass ? dom.pass.value : '';
    }

    // focusPassphrase moves focus to the passphrase field.
    function focusPassphrase() {
      if (dom.pass) dom.pass.focus();
    }

    // passphraseFailed marks the field invalid, relabels Open "Try again" and
    // focuses and selects the field so the recipient can retype it.
    function passphraseFailed() {
      state.openLabel = 'Try again';
      util.setText(dom.openLabel, state.openLabel);
      if (!dom.pass) return;
      dom.pass.setAttribute('aria-invalid', 'true');
      dom.pass.focus();
      dom.pass.select();
    }

    // clearPassphrase empties the passphrase field and hides its text.
    function clearPassphrase() {
      if (!dom.pass) return;
      dom.pass.value = '';
      setPassShown(false);
    }

    return {
      show: showPassphrase, value: passphrase, missing: passphraseMissing,
      focus: focusPassphrase, failed: passphraseFailed, clear: clearPassphrase
    };
  }

  const pass = passphraseControls();

  // onOpen registers handler for Open button presses.
  function onOpen(handler) {
    if (dom.open) dom.open.addEventListener('click', handler);
  }

  // switchTo shows the named view, hides the others, and moves focus to the
  // new view's heading so screen readers announce the change.
  function switchTo(name) {
    VIEWS.forEach(function (v) {
      const node = byId(`view-${v}`);
      if (node) node.hidden = v !== name;
    });
    if (TITLES[name]) document.title = TITLES[name];
    const heading = byId(`${name}-heading`);
    if (heading) heading.focus();
  }

  function wireCopy(text) {
    const btn = dom.copy;
    if (!btn) return;
    btn.addEventListener('click', async function () {
      const ok = await util.copyText(text, selectOutput, dom.copyStatus);
      if (ok) util.flashCopied(btn, dom.copyStatus, 'Message copied to clipboard.');
    });
  }

  // selectOutput selects the message text so it can be copied by hand.
  function selectOutput() {
    const range = document.createRange();
    range.selectNodeContents(dom.output);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
  }

  // showMessage displays the decrypted text and wires its copy button.
  function showMessage(text) {
    if (!text || !dom.output) return;
    dom.output.textContent = text;
    if (dom.messagePanel) dom.messagePanel.hidden = false;
    wireCopy(text);
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
    entry.li.classList.add('is-done');
  }

  function downloadEntry(entry) {
    const link = util.el('a', { href: fileURL(entry), download: entry.file.name, rel: 'noopener' });
    document.body.appendChild(link);
    link.click();
    link.remove();
    markDownloaded(entry);
  }

  function renderFileEntry(file) {
    const btn = util.el('button', { type: 'button', className: 'btn btn-secondary btn-small' }, [
      icons.make('down'),
      util.el('span', { textContent: 'Download' })
    ]);
    btn.setAttribute('aria-label', `Download ${file.name}`);
    const li = util.el('li', {}, [
      icons.make('clip'),
      util.el('span', { className: 'name', textContent: file.name }),
      util.el('span', { className: 'size', textContent: formatBytes(file.size) }),
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
    dom.fileList.replaceChildren(...state.files.map(function (e) { return e.li; }));
    if (dom.fileSection) dom.fileSection.hidden = false;
    showDownloadAll(files.length);
  }

  // headingFor names what was received: a message, files, or both.
  function headingFor(decoded) {
    const n = decoded.files.length;
    const files = n === 1 ? 'a file' : `${n} files`;
    if (decoded.message && n) return `Here\u2019s your secret and ${files}.`;
    if (n) return `Here\u2019s ${files}.`;
    return 'Here\u2019s your secret.';
  }

  // showDecoded switches to the revealed view and renders a decoded
  // envelope ({message, files}).
  function showDecoded(decoded) {
    hideProgress();
    pass.clear();
    util.setText(dom.revealedHeading, headingFor(decoded));
    showMessage(decoded.message);
    showFiles(decoded.files);
    switchTo('revealed');
  }

  // showGone switches to the view explaining the secret no longer exists.
  function showGone() {
    switchTo('gone');
  }

  // showAckResult reveals the deletion warning when the server did not
  // confirm deletion.
  function showAckResult(ok) {
    if (!ok && dom.ackWarning) dom.ackWarning.hidden = false;
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

  // cleanup revokes object URLs, zeroes the decrypted buffer and empties
  // the passphrase field.
  function cleanup() {
    pass.clear();
    state.urls.forEach(function (u) { URL.revokeObjectURL(u); });
    state.urls = [];
    if (state.plaintext) state.plaintext.fill(0);
  }

  window.goneConsumeView = Object.freeze({
    present: Boolean(byId('view-open')),
    setStatus, setProgress, hideProgress, showError, clearError, setOpening, disableOpen, onOpen,
    showPassphrase: pass.show, passphrase: pass.value, passphraseMissing: pass.missing,
    focusPassphrase: pass.focus, passphraseFailed: pass.failed, clearPassphrase: pass.clear,
    showMessage, showDecoded, showGone, showAckResult, guardUnload, keepPlaintext, cleanup, headingFor
  });
})();
