'use strict';

// A small in-memory IndexedDB for requestStore.js: open with upgrade,
// one object store keyed by a keyPath, and put/get/delete/getAll in
// transactions that complete asynchronously. opts.failOpen makes open fail,
// opts.blocked makes it blocked, and opts.failWrites aborts readwrite
// transactions.

function later(fn) {
  setImmediate(fn);
}

function makeRequest(run) {
  const req = { result: undefined, error: null, onsuccess: null, onerror: null };
  later(() => {
    try {
      req.result = run();
      if (req.onsuccess) req.onsuccess();
    } catch (e) {
      req.error = e;
      if (req.onerror) req.onerror();
    }
  });
  return req;
}

class FakeTx {
  constructor(db, mode) {
    this.db = db;
    this.mode = mode;
    this.error = null;
    this.oncomplete = null;
    this.onerror = null;
    this.onabort = null;
    this.pending = 0;
    later(() => this.finish());
  }

  objectStore(name) {
    const rows = this.db.stores.get(name);
    const tx = this;
    const op = (fn) => { tx.pending++; return makeRequest(() => { try { return fn(); } finally { tx.pending--; } }); };
    return {
      put(v) { tx.checkWrite(); return op(() => { rows.set(v[tx.db.keyPaths.get(name)], v); return v.id; }); },
      get(k) { return op(() => rows.get(k)); },
      delete(k) { tx.checkWrite(); return op(() => { rows.delete(k); }); },
      getAll() { return op(() => Array.from(rows.values())); }
    };
  }

  checkWrite() {
    if (this.mode !== 'readwrite') throw new Error('ReadOnlyError');
  }

  finish() {
    if (this.pending > 0) {
      later(() => this.finish());
      return;
    }
    if (this.db.opts.failWrites && this.mode === 'readwrite') {
      this.error = new Error('QuotaExceededError');
      if (this.onabort) this.onabort();
      return;
    }
    if (this.oncomplete) this.oncomplete();
  }
}

class FakeDB {
  constructor(opts) {
    this.opts = opts;
    this.stores = new Map();
    this.keyPaths = new Map();
  }

  createObjectStore(name, o) {
    this.stores.set(name, new Map());
    this.keyPaths.set(name, o.keyPath);
  }

  transaction(name, mode) {
    return new FakeTx(this, mode);
  }
}

function fakeIndexedDB(opts) {
  const o = opts || {};
  const dbs = new Map();
  return {
    dbs: dbs,
    open(name) {
      const req = { result: null, error: null, onupgradeneeded: null, onsuccess: null, onerror: null, onblocked: null };
      later(() => {
        if (o.failOpen) {
          req.error = new Error('open failed');
          if (req.onerror) req.onerror();
          return;
        }
        if (o.blocked) {
          if (req.onblocked) req.onblocked();
          return;
        }
        if (!dbs.has(name)) {
          dbs.set(name, new FakeDB(o));
          req.result = dbs.get(name);
          if (req.onupgradeneeded) req.onupgradeneeded();
        }
        req.result = dbs.get(name);
        if (req.onsuccess) req.onsuccess();
      });
      return req;
    }
  };
}

module.exports = { fakeIndexedDB };
