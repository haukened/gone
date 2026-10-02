# Gone Protocol Specification

Status: **normative** for protocol version 1 and plaintext envelope GONE2.
Reference implementations: the browser client (`web/js/crypto.js`, `envelope.js`, `fileMeta.js`, `consume.js`) and the Go package `internal/envelope`. Both are tested against the shared vectors in [`test/vectors/`](../test/vectors/).

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are used as described in RFC 2119.

## 1. Overview

Gone shares a secret exactly once. The sender's client encrypts everything locally and uploads only ciphertext. The decryption key travels to the recipient inside the URL fragment, which browsers never send to a server. The server stores opaque bytes, hands them out once, and deletes them.

```
sender                          server                       recipient
  | key, nonce <- CSPRNG          |                               |
  | ct = AES-256-GCM(key, pt)     |                               |
  |-- POST ct + version + nonce ->| store                         |
  |<- 201 {id, expires_at} -------|                               |
  | link = /secret/{id}#v1:<key> ---------- out of band --------->|
  |                               |<- GET (claim) ----------------|
  |                               |-- ct + nonce + claim token -->|
  |                               |                    decrypt, decode
  |                               |<- DELETE + claim token -------|
  |                               | delete                        |
```

**Trust boundary.** The server is trusted to enforce TTL, size limits, and single delivery. It is **not** trusted with confidentiality or integrity: it never sees the key or the plaintext, and AES-GCM authentication detects any modification of the ciphertext or nonce.

## 2. Encodings

### 2.1 Base64url

All binary values in headers and fragments use the URL-safe alphabet of RFC 4648 §5 **without padding**. Decoders **MUST** reject input that:

- contains any character outside `A–Z a–z 0–9 - _` (including `=`, `+`, `/`, and whitespace);
- has a length where `len mod 4 == 1`;
- is non-canonical, meaning the unused low bits of the final character are not zero.

Each byte string therefore has exactly one valid encoding. A 32-byte key encodes to 43 characters, and a 12-byte nonce encodes to 16.

### 2.2 Protocol version

The protocol version is an integer from 1 to 255 written as canonical ASCII decimal: digits only, with no sign, whitespace, or leading zeros. Version `0` is invalid.

## 3. Versioning

There are two independent version numbers:

| Number | Where it appears | What it identifies |
| ------ | ---------------- | ------------------ |
| **Protocol version** | `X-Gone-Version` header and link fragment `#v<N>:` | Key format, cipher, nonce size, AAD, fragment payload |
| **Envelope version** | Inside the plaintext (`GONE2` magic and header `"v":2`) | How the decrypted bytes are structured |

Which number changes depends on what changes:

| Change | Requires |
| ------ | -------- |
| Key format, cipher, nonce size, AAD, or fragment payload | A new **protocol version**. Each protocol version has its own AAD (`gone:v<N>`), so ciphertext never authenticates across versions. |
| Plaintext layout or how its fields are interpreted | A new **envelope** magic or `"v"`. The protocol version is unchanged. |
| Tighter validation of an existing layer (rejecting input that used to be accepted) | Only allowed if no conforming encoder ever produced that input. Otherwise the owning layer needs a new version. |

- Protocol version **2 is reserved** for the passphrase-wrapped key (roadmap Phase 5).
- Servers **MUST** reject unsupported protocol versions (§8). Clients **MUST** reject an unsupported protocol version in a link *before* making any request (§7.2).

Currently supported: protocol version **1**, envelope **GONE2**.

## 4. Protocol version 1 cryptography

| Parameter | Value |
| --------- | ----- |
| Cipher | AES-256-GCM |
| Key | 32 bytes from a CSPRNG, unique per secret |
| Nonce | 12 bytes from a CSPRNG, generated for each encryption |
| Tag | 16 bytes, appended to the ciphertext |
| AAD | the 7 ASCII bytes `gone:v1` |
| Ciphertext body | `GCM-Encrypt(key, nonce, plaintext, AAD)`, which is the encrypted bytes followed by the tag |

- Each secret uses a fresh key and is encrypted exactly once, so a (key, nonce) pair is never reused.
- Implementations **MUST NOT** derive keys or nonces from anything other than a CSPRNG.
- A valid ciphertext body is at least 16 bytes long.
- Decryption **MUST** report a single generic error for every failure: wrong key length, wrong nonce length, a ciphertext shorter than the tag, or a failed authentication. The error does not reveal which input was at fault. Validating the link (§7.2) happens earlier and is separate from this.
- Implementations **SHOULD** overwrite key and plaintext buffers once they are no longer needed. They **MUST NOT** log keys, fragments, or plaintext.

## 5. Plaintext formats

Decrypted plaintext is one of the following.

1. **Text.** Raw UTF-8 bytes that do **not** begin with the 8-byte GONE2 magic.
2. **GONE2 envelope.** Any plaintext that begins with the magic.

Rules for encoders and decoders:

- An encoder **MUST** produce a GONE2 envelope when there are attachments, *or* when the message bytes themselves begin with the magic. Otherwise it **MUST** emit raw text, which keeps the format compatible with older clients.
- A decoder **MUST** decode plaintext that begins with the magic as GONE2, and any GONE2 validation error is fatal. All other plaintext is text.
- Message bytes are UTF-8. Clients that display them use the WHATWG UTF-8 decode, which replaces invalid sequences with U+FFFD and strips a leading byte-order mark.

### 5.1 GONE2 layout

```
offset  size  field
0       8     magic: 47 4F 4E 45 32 00 00 00   ("GONE2\0\0\0")
8       4     H: header length, unsigned 32-bit big-endian
12      H     header: UTF-8 JSON (§5.2)
12+H    msg   message bytes
...     s_1   file 1 bytes
...     ...   ...
...     s_n   file n bytes   (end of plaintext)
```

### 5.2 Header

The header is a JSON object (RFC 8259):

```json
{"v":2,"msg":5,"files":[{"name":"a.txt","type":"text/plain","size":3}]}
```

A **size** is a JSON number whose IEEE-754 binary64 value, rounded to nearest-even, is an integer in `[0, 2^53 − 1]`. That makes `3`, `3.0`, and `3e0` all equal to 3.

### 5.3 Decoding

A decoder **MUST** apply every check below and reject the envelope if any of them fails.

1. The plaintext length `L` is at least 12.
2. `H ≤ 16384` and `H ≤ L − 12`.
3. The header bytes are well-formed UTF-8. Decoding is fatal: there is no replacement, and a leading byte-order mark is **not** removed. A BOM is therefore invalid JSON outside a string.
4. The header is a single valid JSON text whose top-level value is an object. Member names are matched **case-sensitively**. If a name repeats, the **last** value wins. Unknown members are ignored. Within JSON string escapes, an unpaired surrogate decodes to U+FFFD.
5. `v` is a number equal to 2.
6. `msg` is a size.
7. `files` is an array with **at most 10** elements. Each element is an object with:
   - `name`: a string;
   - `type`: a string;
   - `size`: a size.

   Unknown members of each element are ignored.
8. `msg + Σ size_i == L − 12 − H` exactly. The sum **MUST** be computed so that it cannot overflow (for example, with unsigned 64-bit arithmetic that has overflow checks). Any narrowing conversion to a platform index type happens only after the value has been checked against the remaining length.

The message is bytes `[12+H, 12+H+msg)`. Each file's bytes follow in header order. Decoders **MUST** re-apply §6 to every `name` and `type` before exposing them, because a malicious sender can put anything in the header.

### 5.4 Encoding

An encoder takes a message and up to 10 files, each with a name, a MIME type, and bytes.

- If there are more than 10 files, the encoder **MUST** fail.
- It sanitizes each name and type (§6).
- It writes the header in this exact canonical form:

  ```
  {"v":2,"msg":<n>,"files":[<f1>,<f2>,...]}
  <fi> = {"name":<string>,"type":<string>,"size":<n>}
  ```

  - Numbers are shortest-form decimal integers. There is no insignificant whitespace.
  - Strings are escaped exactly as ECMAScript `JSON.stringify` does:
    - `"` becomes `\"` and `\` becomes `\\`;
    - U+0008, U+0009, U+000A, U+000C and U+000D become `\b \t \n \f \r`;
    - every other code point below U+0020 becomes `\u00xx`, with lowercase hex;
    - all other code points are written as literal UTF-8.
- If the header is longer than 16384 bytes, the encoder **MUST** fail.

The canonical form makes encoding byte-for-byte reproducible across implementations, which the test vectors check.

## 6. Sanitization

These rules stop path traversal through attacker-controlled metadata and remove the invisible formatting characters listed below. They do **not** defend against confusable characters (homoglyphs). The rules work on Unicode **code points**:

- In JavaScript, an unpaired surrogate is treated as U+FFFD.
- Header strings are always valid UTF-8 (§5.3).
- Encoders that take byte strings (Go `Pack`) **MUST** reject names and types that are not valid UTF-8, rather than guess at a repair.

**Unsafe code points:**
- U+0000–001F and U+007F–009F (controls);
- U+061C (Arabic letter mark);
- U+180E (Mongolian vowel separator);
- U+200B–200F (zero-width characters and direction marks);
- U+202A–202E (bidi embeddings and overrides);
- U+2060–206F (word joiner, invisible operators, bidi isolates, deprecated format characters);
- U+FEFF (byte-order mark / zero-width no-break space).

**Whitespace** is the set used by ECMAScript `String.prototype.trim`:
- U+0009–000D and U+0020;
- U+00A0 and U+1680;
- U+2000–200A;
- U+2028, U+2029, U+202F, U+205F and U+3000;
- U+FEFF.

### 6.1 File names

1. Split the input on every run of `/` or `\`. Drop empty segments and keep the **last** one, or use an empty string if none remain.
2. Remove every unsafe code point.
3. Keep only the first 255 code points.
4. Trim whitespace from both ends.
5. If the result is empty, `.`, or `..`, it becomes `file`.

The output never contains `/`, `\`, or an unsafe code point, and sanitizing it again does not change it (the function is idempotent).

This sanitization guarantees that a name is a single path segment with no invisible formatting characters. It is **not** a filesystem save-name policy. Software that writes attachments to disk (for example, the Phase 3 CLI) **MUST** also apply a platform-aware policy that:

- creates files exclusively, never overwriting and never following symlinks, inside the chosen directory;
- handles Windows reserved names (`CON`, `NUL`, …), trailing dots and spaces, and `:` (alternate data streams).

### 6.2 MIME types

1. Take the input up to its first `;`.
2. Trim whitespace from both ends.
3. If any code point is outside ASCII, the result is `application/octet-stream`.
4. Convert ASCII letters to lowercase.
5. If the value is on the allowlist, return it. Otherwise return `application/octet-stream`.

The allowlist is:
- `application/pdf`, `application/zip`, `application/gzip`, `application/json`, `application/x-tar`, `application/x-7z-compressed`
- `text/plain`, `text/csv`
- `image/png`, `image/jpeg`, `image/gif`, `image/webp`
- `audio/mpeg`, `video/mp4`

## 7. Links

### 7.1 Format

```
link     = base "/secret/" id "#" fragment
id       = 32 lowercase hex characters
fragment = "v" version ":" payload
version  = canonical decimal 1–255 (§2.2)
payload  = 1*( ALPHA / DIGIT / "-" / "_" )
```

For protocol version 1, the payload is the strict base64url encoding of the 32-byte key, which is exactly 43 characters.

### 7.2 Parsing

- The fragment is the text **after** the `#` delimiter. Browsers take `location.hash` and remove its single leading `#`. Go uses `url.URL.EscapedFragment()`.
- Clients **MUST** match the raw, percent-encoded fragment and **MUST NOT** percent-decode it first. A `%`, a second `#`, or any other character outside the grammar makes the fragment invalid.
- Clients **MUST** reject a malformed fragment, an unsupported version, or a payload that fails that version's validation (for v1: strict base64url decoding to 32 bytes). They must do this **before** sending the claim request, so a bad link never claims a secret.

## 8. HTTP exchange

The full endpoint reference, including status codes, is in [docs/README.md](README.md) and [openapi.yaml](openapi.yaml). This section lists only what is protocol-relevant.

### 8.1 Create: `POST /api/secret`

| Header | Rule |
| ------ | ---- |
| `X-Gone-Version` | Canonical decimal (§2.2). Must be a supported protocol version. |
| `X-Gone-Nonce` | Strict base64url (§2.1) that decodes to exactly that version's nonce size: 12 bytes for v1. |
| `X-Gone-TTL` | A Go duration inside the server's configured `[MinTTL, MaxTTL]`. |
| `Content-Length` | Required. The body is the ciphertext. |

- Each `X-Gone-*` header **MUST** appear exactly once.
- If the version or nonce header breaks a rule, the server **MUST** respond with `400` and `invalid version` or `invalid nonce`. The application service repeats the same checks, so only valid metadata is ever stored.
- The server never inspects the body beyond its length. It **MAY** reject bodies shorter than the tag.

### 8.2 Claim: `GET /api/secret/{id}`

On success the server returns `200` with the ciphertext and these headers:
- `X-Gone-Version` and `X-Gone-Nonce`, exactly as they were stored;
- `X-Gone-Claim`, the claim token;
- `X-Gone-Claim-Expires`, the lease deadline.

The client:
1. **MUST** check that `X-Gone-Version` is exactly the canonical decimal string of the link's version. Otherwise it reports an unsupported version.
2. **MUST** decode the nonce strictly. A malformed nonce is reported with the same generic error as a failed decryption (§4).
3. **MUST** check, when `Content-Length` is present, that it received exactly that many bytes. AES-GCM detects truncation even when `Content-Length` is absent (for example, when a proxy re-chunks the response).
4. **SHOULD** stop reading once it has received more than its own ciphertext limit. Non-browser clients **MUST** enforce such a limit.
5. Then decrypts.

### 8.3 Acknowledge: `DELETE /api/secret/{id}`

The request carries `X-Gone-Claim`. Clients **SHOULD** acknowledge only after decryption and decoding both succeed. Otherwise the lease expires and the janitor removes the secret.

## 9. Test vectors

`test/vectors/*.json` are the conformance suite. Every implementation **MUST** pass all of them.

| File | Covers |
| ---- | ------ |
| `aead_v1.json` | §4: encryption with a fixed nonce, plus decryption failures |
| `envelope_gone2.json` | §5: canonical encoding, decoding, rejection cases |
| `fragment_v1.json` | §2, §7: fragment parsing |
| `sanitize.json` | §6: file names and MIME types |
| `server_headers.json` | §8.1: version and nonce header acceptance |

Binary fields are lowercase hex. All key material in the vectors is synthetic. Go regenerates the vectors with `go test ./internal/envelope -run TestVectors -update`. A regenerated file must be reviewed like any other change.

## 10. Changing the protocol

1. Edit this document first.
2. Add or update vectors.
3. Change the Go and JS implementations together in the same pull request, until both pass every vector.
4. Use the versioning matrix in §3 to decide whether the change needs a new protocol version, a new envelope version, or neither.
