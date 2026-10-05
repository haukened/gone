'use strict';

// Claim acknowledgement for the consume flow.
(function consumeApiAckModule() {
  if (window.goneConsumeApiAck || !window.goneConsumeApiEndpoint || !window.goneUtil) return;
  const endpoint = window.goneConsumeApiEndpoint;
  const RETRY_BACKOFF_MS = 500;
  const MAX_ACK_ATTEMPTS = 3;

  async function ackOnce(claim) {
    try {
      const resp = await endpoint.apiFetch('DELETE', { 'X-Gone-Claim': claim.token });
      // 404 for a claim we hold means the row is already deleted (the lease
      // lapsed and the janitor ran, or the sender revoked it): still gone.
      return resp.status === 204 || resp.status === 404;
    } catch {
      return false;
    }
  }

  async function acknowledge(claim) {
    for (let attempt = 1; attempt <= MAX_ACK_ATTEMPTS; attempt++) {
      if (await ackOnce(claim)) return true;
      await window.goneUtil.sleep(RETRY_BACKOFF_MS * attempt);
    }
    return false;
  }

  window.goneConsumeApiAck = Object.freeze({ acknowledge: acknowledge });
})();
