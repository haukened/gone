'use strict';

// HTTP plumbing for window.goneRequestApi: same-origin fetch, status-to-error
// mapping, and response parsing. Errors carry goneI18n message keys. Exposed
// as window.goneRequestApiFetch.
(function requestApiFetchModule() {
  if (window.goneRequestApiFetch) return;

  const RETRY_AFTER_RE = /^[0-9]{1,6}$/;
  const NETWORK_ERROR = 'js.common.networkRetry';
  const SERVER_ERROR = 'js.common.server';
  const STATUS_MESSAGES = new Map([
    [400, 'js.request.rejected'],
    [429, 'js.common.tooMany'],
    [503, 'js.common.busy']
  ]);

  // RequestError carries a user-facing message key, whether retrying may help,
  // and the server's Retry-After in seconds (0 when absent).
  function RequestError(message, retryable, retryAfter) {
    const e = new Error(message);
    e.retryable = retryable;
    e.retryAfter = retryAfter || 0;
    return e;
  }

  function isRequestError(e) {
    return Boolean(e) && e.retryable !== undefined;
  }

  async function send(url, method, headers) {
    try {
      // Browser fetch to a same-origin URL built from a validated ID; not a server-side SSRF sink.
      return await fetch(url, { // nosemgrep
        method: method,
        headers: headers,
        mode: 'same-origin',
        credentials: 'same-origin',
        redirect: 'error',
        cache: 'no-store',
        keepalive: method === 'POST'
      });
    } catch {
      throw RequestError(NETWORK_ERROR, true);
    }
  }

  function readRetryAfter(resp) {
    const raw = resp.headers.get('Retry-After') || '';
    return RETRY_AFTER_RE.test(raw) ? Number(raw) : 0;
  }

  // failure maps an unexpected HTTP status to a RequestError.
  function failure(resp) {
    return RequestError(STATUS_MESSAGES.get(resp.status) || SERVER_ERROR, resp.status !== 400, readRetryAfter(resp));
  }

  async function readJSON(resp) {
    try {
      return await resp.json();
    } catch {
      throw RequestError(SERVER_ERROR, true);
    }
  }

  function parseDate(value) {
    const when = new Date(typeof value === 'string' ? value : NaN);
    if (Number.isNaN(when.getTime())) throw RequestError(SERVER_ERROR, true);
    return when;
  }

  window.goneRequestApiFetch = Object.freeze({
    RequestError, isRequestError, send, failure, readJSON, parseDate, SERVER_ERROR
  });
})();
