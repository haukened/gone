'use strict';

// Endpoint pinning and the only fetch entry point for secret consumption.
(function consumeApiEndpointModule() {
  if (window.goneConsumeApiEndpoint || !window.goneConsumeApiErrors) return;
  const errs = window.goneConsumeApiErrors;
  const API_PATH = '/api/secret/';
  const REQUEST_PATH = '/api/request/';
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const TOKEN_RE = /^[A-Za-z0-9_-]{43}$/;
  const api = { endpoint: '', headers: {} };
  const allowedEndpoints = new Set();

  // pinnedURL builds path on this page's origin and checks it did not move.
  function pinnedURL(path) {
    const origin = window.location.origin;
    const endpoint = new URL(path, origin);
    if (endpoint.origin !== origin || endpoint.pathname !== path) throw errs.FetchError(errs.INVALID_LINK, false);
    return endpoint.href;
  }

  function checkID(id) {
    if (typeof id !== 'string' || !SECRET_ID_RE.test(id)) throw errs.FetchError(errs.INVALID_LINK, false);
  }

  function apiInit(method, headers) {
    return { method: method, headers: headers, mode: 'same-origin', credentials: 'same-origin', redirect: 'error', cache: 'no-store', keepalive: method === 'DELETE' };
  }

  function pin(url, headers) {
    allowedEndpoints.clear();
    allowedEndpoints.add(url);
    api.endpoint = url;
    api.headers = headers;
  }

  // registerEndpoint pins the secret's claim endpoint /api/secret/{id}.
  function registerEndpoint(id) {
    checkID(id);
    pin(pinnedURL(API_PATH + id), {});
  }

  // registerReplyEndpoint pins a request reply's claim endpoint
  // /api/request/{id}/reply; every call also carries the requester's
  // manage token (docs/protocol.md section 8.6).
  function registerReplyEndpoint(id, manageToken) {
    checkID(id);
    if (typeof manageToken !== 'string' || !TOKEN_RE.test(manageToken)) throw errs.FetchError(errs.INVALID_LINK, false);
    pin(pinnedURL(`${REQUEST_PATH}${id}/reply`), { 'X-Gone-Manage': manageToken });
  }

  function apiFetch(method, headers) {
    const url = api.endpoint;
    if (allowedEndpoints.has(url)) return fetch(url, apiInit(method, Object.assign({}, api.headers, headers)));
    return Promise.reject(errs.FetchError('Blocked request to an unexpected URL', false));
  }

  window.goneConsumeApiEndpoint = Object.freeze({ registerEndpoint: registerEndpoint, registerReplyEndpoint: registerReplyEndpoint, apiFetch: apiFetch });
})();
