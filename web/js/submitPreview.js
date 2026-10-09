'use strict';

// Preview-only result panel rendering for UI snapshots.
(function submitPreviewModule() {
  if (window.goneSubmitPreview || !window.goneSubmitDom) return;
  const ctx = window.goneSubmitDom;

  function previewResult() {
    const params = new URLSearchParams(location.search);
    if (params.get('preview') !== 'result' || !window.goneResultPanel) return;
    const mockID = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const mockKey = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
    const v2 = params.has('passphrase');
    const version = v2 ? window.goneCrypto.versionV2 : window.goneCrypto.version;
    const mockURL = `${location.origin}/secret/${mockID}#v${version}:${mockKey}`;
    const manageURL = ctx.uploader.buildManageURL(mockID, 'B'.repeat(43));
    const future = new Date(Date.now() + 30 * 60 * 1000).toISOString();
    window.goneResultPanel.show({ shareURL: mockURL, manageURL: manageURL, expiresAt: future, passphrase: v2, focus: false });
  }

  window.goneSubmitPreview = Object.freeze({ previewResult: previewResult });
})();
