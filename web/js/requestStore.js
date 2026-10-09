/* global indexedDB */
'use strict';

// The requester's local copy of each secret request, in IndexedDB (database
// "gone", object store "requests", keyed by request ID). An entry holds the
// non-extractable private key as a CryptoKey, so the reply can be opened
// after the browser is closed and reopened, while page scripts can never read
// the key's bytes. Nothing here is sent anywhere. Entries are removed once
// the reply is opened, the request is cancelled or it expires. Exposed as
// window.goneRequestStore.
(function requestStoreModule() {
  if (window.goneRequestStore) return;
  const DB_NAME = 'gone';
  const STORE = 'requests';
  const DB_VERSION = 1;
  const PROBE_ID = 'probe';
  const conn = { opening: null };

  // openDB opens (once) the database, creating the store on first use.
  function openDB() {
    if (conn.opening) return conn.opening;
    conn.opening = new Promise(function (resolve, reject) {
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = function () { req.result.createObjectStore(STORE, { keyPath: 'id' }); };
      req.onsuccess = function () { resolve(req.result); };
      req.onerror = function () { reject(req.error || new Error('indexedDB open failed')); };
      req.onblocked = function () { reject(new Error('indexedDB blocked')); };
    });
    conn.opening.catch(function () { conn.opening = null; });
    return conn.opening;
  }

  // run performs op in one transaction and resolves with its request's
  // result once the transaction has committed.
  async function run(mode, op) {
    const db = await openDB();
    return new Promise(function (resolve, reject) {
      const tx = db.transaction(STORE, mode);
      const req = op(tx.objectStore(STORE));
      const fail = function () { reject(tx.error || new Error('indexedDB transaction failed')); };
      tx.oncomplete = function () { resolve(req.result); };
      tx.onerror = fail;
      tx.onabort = fail;
    });
  }

  function put(entry) {
    return run('readwrite', function (s) { return s.put(entry); });
  }

  function get(id) {
    return run('readonly', function (s) { return s.get(id); });
  }

  function remove(id) {
    return run('readwrite', function (s) { return s.delete(id); });
  }

  // list returns live entries, newest first, deleting any whose expiry has
  // passed (their key can never open anything again).
  async function list(now) {
    const entries = (await run('readonly', function (s) { return s.getAll(); })) || [];
    const at = now === undefined ? Date.now() : now;
    const expired = entries.filter(function (e) { return e.id !== PROBE_ID && e.expiresAt <= at; });
    await Promise.all(expired.map(function (e) { return remove(e.id); }));
    return entries
      .filter(function (e) { return e.id !== PROBE_ID && e.expiresAt > at; })
      .sort(function (a, b) { return b.createdAt - a.createdAt; });
  }

  // available reports whether this browser can keep a request: IndexedDB
  // exists, opens, and accepts a write (some private modes refuse).
  async function available() {
    try {
      if (typeof indexedDB === 'undefined' || !indexedDB) return false;
      await put({ id: PROBE_ID, expiresAt: 0, createdAt: 0 });
      await remove(PROBE_ID);
      return true;
    } catch {
      return false;
    }
  }

  // persist asks the browser not to evict this origin's storage under
  // pressure. Best-effort: the answer does not change anything here.
  function persist() {
    try {
      if (navigator.storage && navigator.storage.persist) navigator.storage.persist().catch(function () {});
    } catch {
      // best-effort
    }
  }

  window.goneRequestStore = Object.freeze({ available: available, persist: persist, put: put, get: get, remove: remove, list: list });
})();
