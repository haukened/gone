'use strict';

const { reset, load, fastUtil, captureConsole } = require('./harness');
const { fakeResponse } = require('./fakes');

const ID = '0123456789abcdef0123456789abcdef';
const URL_FOR_ID = 'https://gone.test/api/secret/' + ID;

function setup(t, opts) {
  reset('https://gone.test/secret/' + ID);
  t.mock.method(console, 'log', () => {});
  load('util', 'crypto');
  const delays = fastUtil();
  load('consumeApi');
  const api = window.goneConsumeApi;
  if (!opts || opts.register !== false) api.registerEndpoint(ID);
  return { api, delays, logs: captureConsole(t) };
}

const bytes = (...b) => new Uint8Array(b);
const ok = (extra) => () => fakeResponse(Object.assign({ headers: { 'Content-Length': '3', 'X-Gone-Claim': 'tok' }, chunks: [bytes(1, 2), bytes(3)] }, extra));

module.exports = { ID, URL_FOR_ID, setup, bytes, ok, reset, load };
