// Package envelope is the client-side implementation of the Gone protocol
// (docs/protocol.md): v1 AES-256-GCM sealing, link fragments, the GONE2
// plaintext envelope, and metadata sanitization.
//
// It is never imported by the server. It exists so non-browser clients (the
// Phase 3 CLI) and the shared test vectors have a reference implementation
// that matches web/js byte-for-byte.
package envelope
