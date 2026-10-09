'use strict';

// Network side of the requester's pages: create a request, check its status,
// and cancel it (docs/protocol.md section 8.6). The manage token only ever
// travels in the X-Gone-Manage header. URLs are built from an ID that must
// match the 32-hex pattern, on this page's origin. Claiming and acknowledging
// the reply go through consumeApi. Exposed as window.goneRequestApi.
(function requestApiModule() {
  if (window.goneRequestApi) return;

  const API_PATH = '/api/request';
  const ID_RE = /^[0-9a-f]{32}$/;
  const TOKEN_RE = /^[A-Za-z0-9_-]{43}$/;
  const RETRY_AFTER_RE = /^[0-9]{1,6}$/;
  const NETWORK_ERROR = 'Couldn\u2019t reach the server. Check your connection, then try again.';
  const SERVER_ERROR = 'The server had a problem. Try again in a moment.';
  const STATUS_MESSAGES = new Map([
    [400, 'The server rejected this request.'],
    [429, 'Too many requests right now. Wait a moment, then try again.'],
    [503, 'The server is busy right now. Wait a moment, then try again.']
  ]);

  // RequestError carries a user-facing message, whether retrying may help,
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

  function isID(id) {
    return typeof id === 'string' && ID_RE.test(id);
  }

  function isToken(token) {
    return typeof token === 'string' && TOKEN_RE.test(token);
  }

  // actionURL builds /api/request/{id}/{action} on this origin.
  function actionURL(id, action) {
    if (!isID(id)) throw RequestError(SERVER_ERROR, false);
    return new URL(`${API_PATH}/${id}/${action}`, window.location.origin).href;
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

  function checkCreated(body) {
    if (!body || !isID(body.id) || !isToken(body.manage_token) || !isToken(body.fill_token)) throw RequestError(SERVER_ERROR, true);
    return { id: body.id, expiresAt: parseDate(body.expires_at), manageToken: body.manage_token, fillToken: body.fill_token };
  }

  // create opens a request that waits ttl (a Go duration such as "1h").
  //
  // Returns {id, expiresAt, manageToken, fillToken}.
  async function create(ttl) {
    const url = new URL(API_PATH, window.location.origin).href;
    const resp = await send(url, 'POST', { 'X-Gone-TTL': ttl });
    if (resp.status !== 201) throw failure(resp);
    return checkCreated(await readJSON(resp));
  }

  // status reports one request to its requester.
  //
  // Returns {state:'waiting'|'ready', createdAt, expiresAt} or {state:'gone'}.
  async function status(id, token) {
    if (!isToken(token)) return { state: 'gone' };
    const resp = await send(actionURL(id, 'status'), 'GET', { 'X-Gone-Manage': token });
    if (resp.status === 404) return { state: 'gone' };
    if (resp.status !== 200) throw failure(resp);
    const body = await readJSON(resp);
    if (!body || (body.state !== 'waiting' && body.state !== 'ready')) throw RequestError(SERVER_ERROR, true);
    return { state: body.state, createdAt: parseDate(body.created_at), expiresAt: parseDate(body.expires_at) };
  }

  // cancel deletes an open request or an unopened reply.
  //
  // Returns 'cancelled', or 'gone' when there was nothing left to cancel.
  async function cancel(id, token) {
    if (!isToken(token)) return 'gone';
    const resp = await send(actionURL(id, 'revoke'), 'POST', { 'X-Gone-Manage': token });
    if (resp.status === 204) return 'cancelled';
    if (resp.status === 404) return 'gone';
    throw failure(resp);
  }

  window.goneRequestApi = Object.freeze({ RequestError, isRequestError, isID, isToken, create, status, cancel });
})();
