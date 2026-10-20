// Tabs for [data-tabs] sections, following the WAI-ARIA tabs pattern: arrow
// keys, Home and End move between tabs, and only the selected tab is in the
// tab order. Without this script the tab list stays hidden and every panel
// shows, so nothing depends on it.
(function () {
  'use strict';

  function select(tabs, panels, index, focus) {
    tabs.forEach(function (tab, i) {
      var on = i === index;
      tab.setAttribute('aria-selected', on ? 'true' : 'false');
      tab.tabIndex = on ? 0 : -1;
      panels[i].hidden = !on;
    });
    if (focus) tabs[index].focus();
  }

  // nextIndex maps a key to the tab it moves to, or -1 for other keys.
  function nextIndex(key, current, count) {
    switch (key) {
    case 'ArrowRight': return (current + 1) % count;
    case 'ArrowLeft': return (current - 1 + count) % count;
    case 'Home': return 0;
    case 'End': return count - 1;
    default: return -1;
    }
  }

  function wire(box) {
    var list = box.querySelector('[role="tablist"]');
    var tabs = Array.prototype.slice.call(box.querySelectorAll('[role="tab"]'));
    var panels = tabs.map(function (tab) {
      return document.getElementById(tab.getAttribute('aria-controls'));
    });
    if (!list || tabs.length < 2 || panels.indexOf(null) !== -1) return;
    box.classList.add('is-tabbed');
    list.hidden = false;
    tabs.forEach(function (tab, i) {
      tab.addEventListener('click', function () { select(tabs, panels, i, false); });
      tab.addEventListener('keydown', function (e) {
        var to = nextIndex(e.key, i, tabs.length);
        if (to === -1) return;
        e.preventDefault();
        select(tabs, panels, to, true);
      });
    });
    select(tabs, panels, 0, false);
  }

  function init() {
    document.querySelectorAll('[data-tabs]').forEach(wire);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
