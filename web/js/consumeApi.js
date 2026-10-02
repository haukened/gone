'use strict';

// Network and crypto side of the consume flow: claim (streamed GET) ->
// decrypt -> ack (DELETE). Every request goes through apiFetch, which only
// sends to the single same-origin endpoint registered for this page.
// Requires window.goneCrypto and window.goneUtil. Exposed as
// window.goneConsumeApi.
(function consumeApiModule() {
  if (window.goneConsumeApi || !window.goneCrypto || !window.goneUtil) return;
  const util = window.goneUtil;

  const MAX_FETCH_ATTEMPTS = 3;
  const MAX_ACK_ATTEMPTS = 3;
  const RETRY_BACKOFF_MS = 500;
  const API_PATH = '/api/secret/';
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const GONE_MESSAGE = 'This secret is gone: it never existed, was already opened, has expired, or was opened elsewhere. If your own download was interrupted, ask the sender to share it again.';
  const STATUS_MESSAGES = new Map([
    [400, 'Invalid secret link'],
    [404, GONE_MESSAGE],
    [410, GONE_MESSAGE],
    [429, 'Slow down: too many requests. Please wait and retry.']
  ]);
  const NETWORK_ERROR = 'Network error retrieving secret';
  const VERIFY_ERROR = 'Couldn\u2019t verify this secret. The link may be incomplete or wrong; ask the sender to resend it.';

  // api.endpoint is resolved once by registerEndpoint(); allowedEndpoints is
  // the allowlist every request URL must appear in before fetch is called.
  const api = { endpoint: '' };
  const allowedEndpoints = new Set();

  // FetchError carries a user-facing message and whether retrying may help.
  function FetchError(message, retryable) {
    const e = new Error(message);
    e.retryable = retryable;
    return e;
  }

  // isFetchError reports whether e carries a user-facing message.
  function isFetchError(e) {
    return Boolean(e) && e.retryable !== undefined;
  }

  function statusMessage(status) {
    return STATUS_MESSAGES.get(status) || 'Server error retrieving secret';
  }

  function headerInt(resp, name) {
    return parseInt(resp.headers.get(name) || '0', 10) || 0;
  }

  // secretEndpoint is the only place an API URL is built. It accepts nothing but
  // a 32-char hex id and pins the result to this page's origin, so no
  // user-controlled value can redirect requests elsewhere.
  function secretEndpoint(id) {
    if (typeof id !== 'string' || !SECRET_ID_RE.test(id)) throw FetchError('Invalid secret id', false);
    const origin = window.location.origin;
    const endpoint = new URL(API_PATH + id, origin);
    if (endpoint.origin !== origin || endpoint.pathname !== API_PATH + id) {
      throw FetchError('Invalid secret id', false);
    }
    return endpoint.href;
  }

  // apiInit builds fetch options restricted to same-origin, no redirects, no cache.
  function apiInit(method, headers) {
    return {
      method: method,
      headers: headers,
      mode: 'same-origin',
      credentials: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      keepalive: method === 'DELETE'
    };
  }

  // registerEndpoint validates the secret id, then pins the resulting URL as
  // the single allowlisted API endpoint for this page.
  function registerEndpoint(id) {
    const url = secretEndpoint(id);
    allowedEndpoints.clear();
    allowedEndpoints.add(url);
    api.endpoint = url;
  }

  // apiFetch is the only caller of fetch. It never accepts a URL: it uses the
  // pre-registered endpoint and refuses to send unless it is allowlisted.
  function apiFetch(method, headers) {
    const url = api.endpoint;
    if (allowedEndpoints.has(url)) {
      return fetch(url, apiInit(method, headers));
    }
    return Promise.reject(FetchError('Blocked request to an unexpected URL', false));
  }

  // --- Retrieval -------------------------------------------------------------
  function concatChunks(chunks, length) {
    const out = new Uint8Array(length);
    let offset = 0;
    chunks.forEach(function (c) { out.set(c, offset); offset += c.length; c.fill(0); });
    return out;
  }

  // readBody streams the response body, reporting progress as it goes.
  async function readBody(resp, total, onProgress) {
    if (!resp.body || !resp.body.getReader) {
      return new Uint8Array(await resp.arrayBuffer());
    }
    const reader = resp.body.getReader();
    const chunks = [];
    let received = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      received += value.length;
      onProgress(received, total);
    }
    return concatChunks(chunks, received);
  }

  // requestSecret issues the GET (presenting any claim token) and records the
  // claim token the server returns.
  async function requestSecret(claim) {
    const headers = claim.token ? { 'X-Gone-Claim': claim.token } : {};
    let resp;
    try {
      resp = await apiFetch('GET', headers);
    } catch (_) {
      throw FetchError(NETWORK_ERROR, true);
    }
    if (!resp.ok) throw FetchError(statusMessage(resp.status), resp.status >= 500);
    claim.token = resp.headers.get('X-Gone-Claim') || claim.token;
    return resp;
  }

  // readComplete reads the whole body and rejects a truncated download.
  async function readComplete(resp, onProgress) {
    const total = headerInt(resp, 'Content-Length');
    let body;
    try {
      body = await readBody(resp, total, onProgress);
    } catch (_) {
      throw FetchError(NETWORK_ERROR, true);
    }
    if (total && body.length !== total) throw FetchError('Download was incomplete', true);
    return body;
  }

  async function fetchOnce(claim, onProgress) {
    const resp = await requestSecret(claim);
    const body = await readComplete(resp, onProgress);
    return { resp: resp, body: body };
  }

  // shouldStop reports whether a failed fetch attempt must not be retried:
  // the error is final, or a retry without a claim token would be refused.
  function shouldStop(err, attempt, claim) {
    return !err.retryable || (attempt > 1 && !claim.token);
  }

  // fetchWithRetry retries network failures/truncation, presenting the claim
  // token so the server re-serves the same claim instead of reporting it gone.
  //
  // Returns {resp, body}; onProgress(received, total) reports download progress.
  async function fetchWithRetry(claim, onProgress) {
    let lastErr;
    for (let attempt = 1; attempt <= MAX_FETCH_ATTEMPTS; attempt++) {
      try {
        return await fetchOnce(claim, onProgress);
      } catch (e) {
        lastErr = e;
        if (shouldStop(e, attempt, claim)) break;
        console.warn('[gone] retrieval attempt failed, retrying', attempt);
        await util.sleep(RETRY_BACKOFF_MS * attempt);
      }
    }
    throw lastErr;
  }

  // --- Crypto ------------------------------------------------------------------
  // decrypt authenticates and decrypts ciphertext using the response's version
  // and nonce headers. The key and ciphertext buffers are zeroed afterwards.
  async function decrypt(resp, ciphertext, keyB64) {
    const gc = window.goneCrypto;
    if (headerInt(resp, 'X-Gone-Version') !== gc.version) throw FetchError('Unsupported secret version', false);
    const nonce = gc.b64urlDecode(resp.headers.get('X-Gone-Nonce') || '');
    const keyBytes = gc.importKeyB64(keyB64);
    try {
      return await gc.decrypt(ciphertext, nonce, keyBytes);
    } catch (_) {
      throw FetchError(VERIFY_ERROR, false);
    } finally {
      keyBytes.fill(0);
      ciphertext.fill(0);
    }
  }

  // --- Acknowledge (delete) ---------------------------------------------------
  async function ackOnce(claim) {
    try {
      const resp = await apiFetch('DELETE', { 'X-Gone-Claim': claim.token });
      return resp.status === 204;
    } catch (_) {
      return false;
    }
  }

  // acknowledge asks the server to delete the claimed secret, retrying with
  // backoff. Returns whether the server confirmed deletion.
  async function acknowledge(claim) {
    for (let attempt = 1; attempt <= MAX_ACK_ATTEMPTS; attempt++) {
      if (await ackOnce(claim)) return true;
      await util.sleep(RETRY_BACKOFF_MS * attempt);
    }
    return false;
  }

  window.goneConsumeApi = Object.freeze({
    FetchError: FetchError,
    isFetchError: isFetchError,
    registerEndpoint: registerEndpoint,
    fetchWithRetry: fetchWithRetry,
    decrypt: decrypt,
    acknowledge: acknowledge
  });
})();
