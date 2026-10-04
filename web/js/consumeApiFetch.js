'use strict';

// GET-with-retry side of the consume flow.
(function consumeApiFetchModule() {
  if (window.goneConsumeApiFetch || !window.goneConsumeApiErrors || !window.goneConsumeApiEndpoint || !window.goneConsumeApiRead || !window.goneUtil) return;
  const errs = window.goneConsumeApiErrors;
  const endpoint = window.goneConsumeApiEndpoint;
  const reader = window.goneConsumeApiRead;
  const util = window.goneUtil;
  const MAX_FETCH_ATTEMPTS = 3;
  const RETRY_BACKOFF_MS = 500;

  async function requestSecret(claim) {
    const headers = claim.token ? { 'X-Gone-Claim': claim.token } : {};
    let resp;
    try {
      resp = await endpoint.apiFetch('GET', headers);
    } catch {
      throw errs.FetchError(errs.NETWORK_ERROR, true);
    }
    if (!resp.ok) throw errs.FetchError(errs.statusMessage(resp.status), resp.status >= 500, resp.status);
    claim.token = resp.headers.get('X-Gone-Claim') || claim.token;
    return resp;
  }

  async function fetchOnce(claim, onProgress) {
    const resp = await requestSecret(claim);
    try {
      const body = await reader.readComplete(resp, onProgress);
      return { resp: resp, body: body };
    } catch (e) {
      throw errs.isFetchError(e) ? e : errs.FetchError(errs.NETWORK_ERROR, true);
    }
  }

  function shouldStop(err, attempt, claim) {
    return !err.retryable || (attempt > 1 && !claim.token);
  }

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

  window.goneConsumeApiFetch = Object.freeze({ fetchWithRetry: fetchWithRetry });
})();
