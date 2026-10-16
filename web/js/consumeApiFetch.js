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
  const RATE_LIMITED = 429;

  // retryable reports whether the person may press Open again after status:
  // a server problem, or a rate limit, which turns the request away before
  // anything is claimed. (The page URL no longer holds the key, so a reload
  // would not help; the button must stay usable.)
  function retryable(status) {
    return status >= 500 || status === RATE_LIMITED;
  }

  async function requestSecret(claim) {
    const headers = claim.token ? { 'X-Gone-Claim': claim.token } : {};
    let resp;
    try {
      resp = await endpoint.apiFetch('GET', headers);
    } catch {
      throw errs.FetchError(errs.NETWORK_ERROR, true);
    }
    if (!resp.ok) throw errs.FetchError(errs.statusMessage(resp.status), retryable(resp.status), resp.status);
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

  // shouldStop ends the automatic retries. A rate limit is left to the
  // person: retrying at once would only spend more of the budget.
  function shouldStop(err, attempt, claim) {
    return !err.retryable || err.status === RATE_LIMITED || (attempt > 1 && !claim.token);
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
