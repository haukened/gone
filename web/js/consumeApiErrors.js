'use strict';

// Shared user-facing errors for the consume API modules. Messages are
// goneI18n keys; the view renders them in the visitor's language.
(function consumeApiErrorsModule() {
  if (window.goneConsumeApiErrors) return;
  const GONE_MESSAGE = 'js.consume.gone';
  const INVALID_LINK = 'js.consume.invalidLink';
  const STATUS_MESSAGES = new Map([
    [400, INVALID_LINK],
    [404, GONE_MESSAGE],
    [410, GONE_MESSAGE],
    [429, 'js.common.tooMany'],
    [503, 'js.common.busy']
  ]);

  function FetchError(message, retryable, status) {
    const e = new Error(message);
    e.retryable = retryable;
    e.status = status || 0;
    return e;
  }

  function isFetchError(e) {
    return Boolean(e) && e.retryable !== undefined;
  }

  function isGone(e) {
    return isFetchError(e) && (e.status === 404 || e.status === 410);
  }

  function statusMessage(status) {
    return STATUS_MESSAGES.get(status) || window.goneConsumeApiErrors.SERVER_ERROR;
  }

  window.goneConsumeApiErrors = Object.freeze({
    FetchError: FetchError,
    isFetchError: isFetchError,
    isGone: isGone,
    statusMessage: statusMessage,
    INVALID_LINK: INVALID_LINK,
    NETWORK_ERROR: 'js.common.network',
    INCOMPLETE_ERROR: 'js.consume.incomplete',
    SERVER_ERROR: 'js.consume.server',
    VERIFY_ERROR: 'js.consume.verify'
  });
})();
