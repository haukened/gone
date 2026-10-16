'use strict';

// Network side of the requester's pages: create a request, check its status,
// and cancel it (docs/protocol.md section 8.6). The manage token only ever
// travels in the X-Gone-Manage header. URLs are built from an ID that must
// match the 32-hex pattern, on this page's origin. Claiming and acknowledging
// the reply go through consumeApi. Exposed as window.goneRequestApi.
(function requestApiModule() {
  if (window.goneRequestApi || !window.goneRequestApiFetch || !window.goneUtil) return;

  const API_PATH = '/api/request';
  const ID_RE = /^[0-9a-f]{32}$/;
  const http = window.goneRequestApiFetch;
  const RequestError = http.RequestError;
  const SERVER_ERROR = http.SERVER_ERROR;
  const send = http.send;
  const failure = http.failure;
  const readJSON = http.readJSON;
  const parseDate = http.parseDate;

  function isID(id) {
    return typeof id === 'string' && ID_RE.test(id);
  }

  const isToken = window.goneUtil.isToken;

  // actionURL builds /api/request/{id}/{action} on this origin.
  function actionURL(id, action) {
    if (!isID(id)) throw RequestError(SERVER_ERROR, false);
    return new URL(`${API_PATH}/${id}/${action}`, window.location.origin).href;
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

  window.goneRequestApi = Object.freeze({ RequestError, isRequestError: http.isRequestError, isID, isToken, create, status, cancel });
})();
