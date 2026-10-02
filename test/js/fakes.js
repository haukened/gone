'use strict';

// Shared fakes for network-facing tests: fetch responses and XMLHttpRequest.

// fakeResponse builds a fetch Response-like object. chunks (Uint8Array[])
// are streamed through body.getReader(); stream:false uses arrayBuffer().
// readError makes the stream reject after the first chunk.
function fakeResponse(opts) {
  const o = Object.assign({ status: 200, headers: {}, chunks: [], stream: true }, opts);
  const headers = new Headers(o.headers);
  const all = () => {
    const total = o.chunks.reduce((n, c) => n + c.length, 0);
    const out = new Uint8Array(total);
    let off = 0;
    o.chunks.forEach((c) => { out.set(c, off); off += c.length; });
    return out;
  };
  const resp = {
    status: o.status,
    ok: o.status >= 200 && o.status < 300,
    headers: headers,
    body: null,
    arrayBuffer: async () => all().buffer
  };
  if (o.stream) {
    resp.body = {
      getReader() {
        const queue = o.chunks.map((c) => c.slice());
        let reads = 0;
        return {
          cancel: async () => { resp.cancelled = true; throw new Error('cancelled'); },
          read: async () => {
            if (o.readError && reads++ > 0) throw new Error('reset');
            if (!queue.length) return { done: true, value: undefined };
            return { done: false, value: queue.shift() };
          }
        };
      }
    };
  }
  return resp;
}

// installFetch replaces fetch with a stub that replays handlers in order
// (functions returning a response or throwing) and records each call.
function installFetch(handlers) {
  const calls = [];
  const queue = handlers.slice();
  globalThis.fetch = async (url, init) => {
    calls.push({ url: url, init: init });
    const next = queue.shift();
    if (!next) throw new Error('unexpected fetch');
    return next(url, init);
  };
  return calls;
}

// FakeXHR records requests; tests drive the outcome via FakeXHR.respond.
class FakeXHR {
  constructor() {
    this.upload = { onprogress: null };
    this.headers = {};
    this.status = 0;
    this.responseText = '';
    FakeXHR.instances.push(this);
    if (FakeXHR.onCreate) FakeXHR.onCreate(this);
  }
  open(method, url) { this.method = method; this.url = url; }
  setRequestHeader(k, v) { this.headers[k] = v; }
  send(body) { this.body = body; if (FakeXHR.onSend) FakeXHR.onSend(this); }
  // respond finishes the request with status and an optional JSON body.
  respond(status, json) {
    this.status = status;
    this.responseText = json === undefined ? '' : JSON.stringify(json);
    this.onload();
  }
}
FakeXHR.instances = [];
FakeXHR.onSend = null;
FakeXHR.onCreate = null;
FakeXHR.reset = () => { FakeXHR.instances = []; FakeXHR.onSend = null; FakeXHR.onCreate = null; };

module.exports = { fakeResponse, installFetch, FakeXHR };
