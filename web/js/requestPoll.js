'use strict';

// Polls a check function while the page is visible: every 20 seconds, 60
// seconds (or the server's Retry-After) after an error, paused while the tab
// is hidden and resumed with an immediate check when it is shown again. The
// check resolves 'stop' when there is nothing left to wait for. Exposed as
// window.goneRequestPoll.
(function requestPollModule() {
  if (window.goneRequestPoll) return;
  const INTERVAL_MS = 20000;
  const BACKOFF_MS = 60000;

  function errorDelay(e) {
    const after = e && e.retryAfter ? e.retryAfter * 1000 : 0;
    return Math.max(BACKOFF_MS, after);
  }

  // create returns {start, stop, now} for check. now runs a check at once
  // (for "Check again") and resets the timer.
  function create(check) {
    const st = { timer: 0, stopped: false, paused: false, inFlight: false };

    function stop() {
      st.stopped = true;
      clearTimeout(st.timer);
      document.removeEventListener('visibilitychange', onVisibility);
    }

    function schedule(ms) {
      clearTimeout(st.timer);
      if (!st.stopped) st.timer = setTimeout(tick, ms);
    }

    async function runCheck() {
      try {
        return (await check()) === 'stop' ? 0 : INTERVAL_MS;
      } catch (e) {
        return errorDelay(e);
      }
    }

    async function tick() {
      if (st.stopped || st.inFlight) return;
      if (document.visibilityState === 'hidden') {
        st.paused = true;
        return;
      }
      st.inFlight = true;
      const next = await runCheck();
      st.inFlight = false;
      if (next === 0) stop();
      else schedule(next);
    }

    function onVisibility() {
      if (document.visibilityState !== 'visible' || !st.paused) return;
      st.paused = false;
      tick();
    }

    document.addEventListener('visibilitychange', onVisibility);
    return { start: tick, stop: stop, now: tick };
  }

  window.goneRequestPoll = Object.freeze({ create: create, INTERVAL_MS: INTERVAL_MS, BACKOFF_MS: BACKOFF_MS });
})();
