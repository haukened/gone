'use strict';

// Shared user-facing errors for the consume API modules.
(function consumeApiErrorsModule() {
  if (window.goneConsumeApiErrors) return;
  const GONE_MESSAGE = 'This secret is gone: it never existed, was already opened, has expired, or was opened elsewhere. If your own download was interrupted, ask the sender to share it again.';
  const INVALID_LINK = 'This link isn\u2019t valid. Check that you copied all of it.';
  const STATUS_MESSAGES = new Map([
    [400, INVALID_LINK],
    [404, GONE_MESSAGE],
    [410, GONE_MESSAGE],
    [429, 'Too many requests right now. Wait a moment, then try again.']
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
    NETWORK_ERROR: 'Couldn\u2019t reach the server. Check your connection.',
    INCOMPLETE_ERROR: 'The download was interrupted.',
    SERVER_ERROR: 'The server had a problem retrieving this secret.',
    VERIFY_ERROR: 'Couldn\u2019t verify this secret. The link may be incomplete or wrong; ask the sender to resend it.'
  });
})();
