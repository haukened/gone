'use strict';

// Public network and crypto side of the consume flow.
(function consumeApiModule() {
  if (window.goneConsumeApi) return;
  const errs = window.goneConsumeApiErrors;
  const endpoint = window.goneConsumeApiEndpoint;
  const fetching = window.goneConsumeApiFetch;
  const cryptoApi = window.goneConsumeApiCrypto;
  const ack = window.goneConsumeApiAck;
  if (!errs || !endpoint || !fetching || !cryptoApi || !ack) return;

  window.goneConsumeApi = Object.freeze({
    FetchError: errs.FetchError,
    isFetchError: errs.isFetchError,
    isGone: errs.isGone,
    registerEndpoint: endpoint.registerEndpoint,
    registerReplyEndpoint: endpoint.registerReplyEndpoint,
    fetchWithRetry: fetching.fetchWithRetry,
    decrypt: cryptoApi.decrypt,
    decryptV2: cryptoApi.decryptV2,
    acknowledge: ack.acknowledge
  });
})();
