'use strict';

// Response body reading for the consume flow.
(function consumeApiReadModule() {
  if (window.goneConsumeApiRead || !window.goneConsumeApiErrors) return;
  const errs = window.goneConsumeApiErrors;
  const DECIMAL_RE = /^(0|[1-9][0-9]{0,14})$/;

  function declaredLength(resp) {
    const raw = resp.headers.get('Content-Length');
    if (raw === null) return -1;
    if (!DECIMAL_RE.test(raw)) throw errs.FetchError(errs.INCOMPLETE_ERROR, true);
    return Number(raw);
  }

  function concatChunks(chunks, length) {
    const out = new Uint8Array(length);
    let offset = 0;
    chunks.forEach(function (c) { out.set(c, offset); offset += c.length; c.fill(0); });
    return out;
  }

  function cancelReader(reader) {
    reader.cancel().catch(function () {});
  }

  async function readStream(reader, total, onProgress) {
    const chunks = [];
    let received = 0;
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      chunks.push(next.value);
      received += next.value.length;
      if (total >= 0 && received > total) {
        cancelReader(reader);
        break;
      }
      onProgress(received, total);
    }
    return concatChunks(chunks, received);
  }

  async function readBody(resp, total, onProgress) {
    if (!resp.body || !resp.body.getReader) return new Uint8Array(await resp.arrayBuffer());
    return readStream(resp.body.getReader(), total, onProgress);
  }

  async function readComplete(resp, onProgress) {
    const total = declaredLength(resp);
    const body = await readBody(resp, total, onProgress);
    if (total < 0 || body.length === total) return body;
    body.fill(0);
    throw errs.FetchError(errs.INCOMPLETE_ERROR, true);
  }

  window.goneConsumeApiRead = Object.freeze({ readComplete: readComplete });
})();
