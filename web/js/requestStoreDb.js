/* global indexedDB */
'use strict';

// IndexedDB plumbing for window.goneRequestStore: opens database "gone"
// (object store "requests", keyed by request ID) once and runs one operation
// per transaction. Exposed as window.goneRequestStoreDb.
(function requestStoreDbModule() {
  if (window.goneRequestStoreDb) return;
  const DB_NAME = 'gone';
  const STORE = 'requests';
  const DB_VERSION = 1;
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

  window.goneRequestStoreDb = Object.freeze({ run: run });
})();
