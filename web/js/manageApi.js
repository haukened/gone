'use strict';

// Network side of the sender's manage page: status (GET) and revoke (POST)
// for one secret, authorised by the manage token in the X-Gone-Manage
// header. Every request goes through apiFetch, which only sends to the two
// same-origin endpoints registered for this page. No retries: the page
// offers "Check again" and "Try again" instead. Exposed as
// window.goneManageApi.
(function manageApiModule() {
  if (window.goneManageApi) return;

  const API_PATH = '/api/secret/';
  const SECRET_ID_RE = /^[0-9a-f]{32}$/;
  const TOKEN_RE = /^[A-Za-z0-9_-]{43}$/;
  const INVALID_LINK = 'js.manage.invalidLink';
  const NETWORK_ERROR = 'js.common.networkRetry';
  const SERVER_ERROR = 'js.common.server';
  const STATUS_MESSAGES = new Map([
    [400, INVALID_LINK],
    [429, 'js.common.tooMany'],
    [503, 'js.common.busy']
  ]);

  // endpoints holds the URLs pinned by registerEndpoint; allowed is the
  // allowlist every request URL must appear in before fetch is called.
  const endpoints = { status: '', revoke: '' };
  const allowed = new Set();

  // ManageError carries a user-facing message key and whether retrying may
  // help.
  function ManageError(message, retryable) {
    const e = new Error(message);
    e.retryable = retryable;
    return e;
  }

  // isManageError reports whether e carries a user-facing message.
  function isManageError(e) {
    return Boolean(e) && e.retryable !== undefined;
  }

  // isToken reports whether token is a well-formed manage token.
  function isToken(token) {
    return typeof token === 'string' && TOKEN_RE.test(token);
  }

  // actionURL builds /api/secret/{id}/{action} on this page's origin. id
  // must already match SECRET_ID_RE, so it cannot alter the path.
  function actionURL(id, action) {
    return new URL(`${API_PATH}${id}/${action}`, window.location.origin).href;
  }

  // registerEndpoint validates the secret id, then pins the status and
  // revoke URLs as the only allowlisted endpoints for this page.
  function registerEndpoint(id) {
    if (typeof id !== 'string' || !SECRET_ID_RE.test(id)) throw ManageError(INVALID_LINK, false);
    const statusURL = actionURL(id, 'status');
    const revokeURL = actionURL(id, 'revoke');
    allowed.clear();
    allowed.add(statusURL);
    allowed.add(revokeURL);
    endpoints.status = statusURL;
    endpoints.revoke = revokeURL;
  }

  // apiFetch is the only caller of fetch. It takes an endpoint name, never a
  // URL, and refuses to send unless the token and endpoint are valid.
  async function apiFetch(name, method, token) {
    const url = endpoints[name];
    if (!allowed.has(url)) throw ManageError('js.consume.blocked', false);
    if (!isToken(token)) throw ManageError(INVALID_LINK, false);
    try {
      // Browser fetch to an allowlisted same-origin URL; not a server-side SSRF sink.
      return await fetch(url, { // nosemgrep
        method: method,
        headers: { 'X-Gone-Manage': token },
        mode: 'same-origin',
        credentials: 'same-origin',
        redirect: 'error',
        cache: 'no-store',
        keepalive: method === 'POST'
      });
    } catch {
      throw ManageError(NETWORK_ERROR, true);
    }
  }

  // failure maps an unexpected HTTP status to a ManageError.
  function failure(httpStatus) {
    return ManageError(STATUS_MESSAGES.get(httpStatus) || SERVER_ERROR, httpStatus !== 400);
  }

  // parseDate returns a Date for a timestamp string, or throws.
  function parseDate(value) {
    const when = new Date(typeof value === 'string' ? value : NaN);
    if (Number.isNaN(when.getTime())) throw ManageError(SERVER_ERROR, true);
    return when;
  }

  // readPending validates a 200 status body.
  async function readPending(resp) {
    let body;
    try {
      body = await resp.json();
    } catch {
      throw ManageError(SERVER_ERROR, true);
    }
    if (!body || body.state !== 'pending') throw ManageError(SERVER_ERROR, true);
    return { state: 'pending', createdAt: parseDate(body.created_at), expiresAt: parseDate(body.expires_at) };
  }

  // status asks whether the secret is still waiting to be opened.
  //
  // Returns {state:'pending', createdAt, expiresAt} or {state:'gone'};
  // rejects with a ManageError on any other outcome.
  async function status(token) {
    const resp = await apiFetch('status', 'GET', token);
    if (resp.status === 404) return { state: 'gone' };
    if (resp.status !== 200) throw failure(resp.status);
    return readPending(resp);
  }

  // revoke deletes the secret before anyone opens it.
  //
  // Returns 'deleted' when this call removed it, or 'gone' when it was
  // already opened, deleted or expired; rejects with a ManageError otherwise.
  async function revoke(token) {
    const resp = await apiFetch('revoke', 'POST', token);
    if (resp.status === 204) return 'deleted';
    if (resp.status === 404) return 'gone';
    throw failure(resp.status);
  }

  window.goneManageApi = Object.freeze({
    ManageError, isManageError, isToken, registerEndpoint, status, revoke, INVALID_LINK
  });
})();
