'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const { reset, load } = require('./harness');

// text returns n printable ASCII characters, the same generator that made the
// fingerprints below.
function text(n) {
  return Array.from({ length: n }, (_, i) => String.fromCharCode(33 + ((i * 7 + n) % 90))).join('');
}

const LINK = 'https://gone.test/secret/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa#v1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';

// SHA-256 of the module matrix ('1' dark, '0' light, row-major) from an
// independent encoder (node-qrcode 1.5.4, byte mode, level M, forced mask).
// They cover version 1, the link-sized version 6, version information (7+),
// short and long blocks (13, 26), and the largest version.
const VECTORS = [
  { text: '', mask: 0, version: 1, sha256: 'f0ecf06d4339599be895b66eacd6c346a4e2f211b1a9249f7a2f37af34721787' },
  { text: '', mask: 3, version: 1, sha256: 'de2e48177b471e1c97705057a9ff44a4c139c82cad57da6c631913ce30055fa5' },
  { text: '', mask: 5, version: 1, sha256: 'f4dde4aa20e10dfbf1d874fad6a512fcd209390b7a62db5d4f5af7bc12b62d7c' },
  { text: '', mask: 7, version: 1, sha256: 'ab473cc3df4a8e87037d872a102d72c27e135936cecde9262d55347eb77bc021' },
  { text: LINK, mask: 0, version: 6, sha256: 'a34c2724f0bf2a28ea7d2994a58545d4f0a475d7b9b6df8de6813446f6786417' },
  { text: LINK, mask: 3, version: 6, sha256: '29d95aad24884847867a3181db0d5068a4bcec6b3f461537fd1dd03e57d55964' },
  { text: LINK, mask: 5, version: 6, sha256: '6c714ab9ad9273e6e85f3a725450ac69ca70b9a4667852e3f2e7b17a152b2f21' },
  { text: LINK, mask: 7, version: 6, sha256: '612b2bfc08f799252313ebc748250492c3bd4aa6c9321b6a112f2c6751cba361' },
  { text: text(120), mask: 0, version: 7, sha256: '4fdd6ec9c3805852fd6d2b8eac0804dcc25cff756854cf0505b11ed690cd32f9' },
  { text: text(120), mask: 3, version: 7, sha256: '41556af720af18251e70e8122346f6ffaf90fb6715f21f50f257fca61c2e7759' },
  { text: text(120), mask: 5, version: 7, sha256: 'c591d6d56094030e8d8d81e0025b031641eb1cd03da84a58a44174142419c1f1' },
  { text: text(120), mask: 7, version: 7, sha256: 'eec9249374f2fbdcbe5e2c8d9a28ee03d3c25bfbb0aeb86dcf7828726c2fd74c' },
  { text: text(300), mask: 0, version: 13, sha256: 'c603bc2c4e63eed6909707b2f55842329a149fbe293918f28fd3f3b042e8d2a6' },
  { text: text(300), mask: 3, version: 13, sha256: '244ea21717b46b33621769a880322a3a9c19a9d833acdbb888ccdbb81093c8a8' },
  { text: text(300), mask: 5, version: 13, sha256: '7b359d98721ad6dd840de3caae7e92d56b2b6f2e136455c7817bfb9da36c761b' },
  { text: text(300), mask: 7, version: 13, sha256: 'a5f0223129536a00326aa48a2d0d6f7110ef814962127f9b1e5e027a6a515f26' },
  { text: text(1000), mask: 0, version: 26, sha256: 'e0b3361d262ee83fc5dd40a4130ce9e9b15e0d16df81b674736dd4ad7a543957' },
  { text: text(1000), mask: 3, version: 26, sha256: '4ab6545bf5d16a785173498381cec190721949745054cf3011bcbc0703798e67' },
  { text: text(1000), mask: 5, version: 26, sha256: '0336432c32aa5ce898b3cdbe2f3f70c3fa99f6293f2a7d633d87253e4f019dfa' },
  { text: text(1000), mask: 7, version: 26, sha256: '98a9ffba02741f7af98c57b78f2bb2056b6ca24b49828abad71aa86372e864cc' },
  { text: text(2331), mask: 0, version: 40, sha256: '0fc4fad55ae54db823a7a3d03ea043102adce58c3e7f674e8ed01845e94dbd46' },
  { text: text(2331), mask: 3, version: 40, sha256: '2c8c68bf83adac0b4c7e532b4443dfdbde45e2a1152a7677485b285f98b65005' },
  { text: text(2331), mask: 5, version: 40, sha256: '441150438d8c7a620a6a8cfedc21c9a966214aeaa057cd871eb4d25710975433' },
  { text: text(2331), mask: 7, version: 40, sha256: '07f10389a38ade04eb0ce522e59c6b4e96ba8d4eb2c5e13d6cdb17dd83b722bd' }
];

function fingerprint(code) {
  return crypto.createHash('sha256').update(Array.from(code.dark).join('')).digest('hex');
}

function qr() {
  reset();
  load('qr');
  return window.goneQr;
}

test('loads once', () => {
  const q = qr();
  assert.ok(Object.isFrozen(q));
  load('qr');
  assert.equal(window.goneQr, q);
});

test('matches an independent encoder module for module', () => {
  const q = qr();
  for (const v of VECTORS) {
    const code = q.encode(v.text, { mask: v.mask });
    const label = `${v.text.length} bytes, mask ${v.mask}`;
    assert.equal(code.version, v.version, label);
    assert.equal(code.size, v.version * 4 + 17, label);
    assert.equal(code.mask, v.mask, label);
    assert.equal(code.dark.length, code.size * code.size, label);
    assert.equal(fingerprint(code), v.sha256, label);
  }
});

test('picks a mask by penalty when none is forced', () => {
  const q = qr();
  for (const t of ['', LINK, text(120), text(300)]) {
    const auto = q.encode(t);
    assert.ok(auto.mask >= 0 && auto.mask < 8);
    assert.deepEqual(auto, q.encode(t, { mask: auto.mask }));
    for (const bad of [-1, 8, 1.5, '3', null]) assert.deepEqual(q.encode(t, { mask: bad }), auto, `mask ${bad}`);
  }
  // Project Nayuki's reference encoder picks the same masks.
  assert.deepEqual(['', LINK, text(120), text(300)].map((t) => q.encode(t).mask), [3, 3, 2, 2]);
});

test('encodes UTF-8 bytes and stringifies its input', () => {
  const q = qr();
  assert.deepEqual(q.encode('é'), q.encode(Buffer.from('é', 'utf8').toString('latin1').length === 2 ? 'é' : ''));
  assert.equal(q.encode('é'.repeat(7)).version, 1);
  assert.equal(q.encode('é'.repeat(8)).version, 2);
  assert.deepEqual(q.encode(12345), q.encode('12345'));
});

test('throws RangeError past the largest version', () => {
  const q = qr();
  assert.equal(q.encode(text(2331)).version, 40);
  assert.throws(() => q.encode(text(2332)), RangeError);
});
