'use strict';

// How the shared opener opens a reply on the single-request page: the
// version and nonce headers are checked, then this browser's
// non-extractable private key decrypts the body. The local entry is
// forgotten once the reply is opened or gone. Exposed as
// window.goneRequestDetailSource.
(function requestDetailSourceModule() {
  const util = window.goneUtil;
  const deps = [window.goneRequestDetailActions, window.goneCryptoV3, window.goneCryptoEncoding, window.goneConsumeApi, window.goneConsumeView];
  if (window.goneRequestDetailSource || !util || !util.allPresent(deps)) return;
  const actions = window.goneRequestDetailActions;
  const consumeApi = window.goneConsumeApi;
  const consumeView = window.goneConsumeView;

  const VERIFY_ERROR = 'js.detail.verify';
  const NONCE_BYTES = 12;

  function readNonce(resp) {
    if (resp.headers.get('X-Gone-Version') !== String(window.goneCryptoV3.version)) throw new Error('version');
    const nonce = window.goneCryptoEncoding.b64urlDecode(resp.headers.get('X-Gone-Nonce') || '');
    if (nonce.length !== NONCE_BYTES) throw new Error('nonce');
    return nonce;
  }

  // source tells the shared opener how to open this reply.
  function source(entry) {
    return {
      decrypt: async function (fetched) {
        consumeView.setStatus('js.consume.decrypting');
        try {
          return await window.goneCryptoV3.decryptV3(fetched.body, readNonce(fetched.resp), entry.privateKey, entry.publicKey);
        } catch {
          throw consumeApi.FetchError(VERIFY_ERROR, false);
        } finally {
          fetched.body.fill(0);
        }
      },
      wipe: function () {},
      onOpened: function () { actions.forget(entry.id); },
      onGone: function () { actions.forget(entry.id); }
    };
  }

  window.goneRequestDetailSource = Object.freeze({ source: source });
})();
