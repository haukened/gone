'use strict';

// Endpoint pinning and the only fetch entry point for secret consumption.
(function consumeApiEndpointModule() {
  if (window.goneConsumeApiEndpoint || !window.goneConsumeApiErrors) return;
  const errs = window.goneConsumeApiErrors;
  const API_PATH = '/api/secret/';
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const api = { endpoint: '' };
  const allowedEndpoints = new Set();

  function secretEndpoint(id) {
    if (typeof id !== 'string' || !SECRET_ID_RE.test(id)) throw errs.FetchError(errs.INVALID_LINK, false);
    const origin = window.location.origin;
    const endpoint = new URL(API_PATH + id, origin);
    if (endpoint.origin !== origin || endpoint.pathname !== API_PATH + id) throw errs.FetchError(errs.INVALID_LINK, false);
    return endpoint.href;
  }

  function apiInit(method, headers) {
    return { method: method, headers: headers, mode: 'same-origin', credentials: 'same-origin', redirect: 'error', cache: 'no-store', keepalive: method === 'DELETE' };
  }

  function registerEndpoint(id) {
    const url = secretEndpoint(id);
    allowedEndpoints.clear();
    allowedEndpoints.add(url);
    api.endpoint = url;
  }

  function apiFetch(method, headers) {
    const url = api.endpoint;
    if (allowedEndpoints.has(url)) return fetch(url, apiInit(method, headers));
    return Promise.reject(errs.FetchError('Blocked request to an unexpected URL', false));
  }

  window.goneConsumeApiEndpoint = Object.freeze({ registerEndpoint: registerEndpoint, apiFetch: apiFetch });
})();
