[![Build](https://github.com/haukened/gone/actions/workflows/build.yaml/badge.svg)](https://github.com/haukened/gone/actions/workflows/build.yaml)
[![Security Scan](https://github.com/haukened/gone/actions/workflows/sec.yaml/badge.svg)](https://github.com/haukened/gone/actions/workflows/sec.yaml)
[![Codacy Badge](https://app.codacy.com/project/badge/Grade/f632a2010c7748199f7c2cb8317feffa)](https://app.codacy.com/gh/haukened/gone/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Codacy Badge](https://app.codacy.com/project/badge/Coverage/f632a2010c7748199f7c2cb8317feffa)](https://app.codacy.com/gh/haukened/gone/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage)

![GitHub License](https://img.shields.io/github/license/haukened/gone)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/haukened/gone)
![GitHub last commit](https://img.shields.io/github/last-commit/haukened/gone)


# gone

Go + One = Gone.  Written in Go, there until its __*gone*__.

## Purpose
Gone is a minimal Go service designed for one-time secret sharing. It enables users to securely share sensitive information by ensuring that secrets are encrypted client-side, transmitted safely, and can only be accessed once before being permanently deleted.

## Security Model
Gone prioritizes security through simplicity and strong encryption practices. Secrets are encrypted on the client side, meaning the service never sees the unencrypted data. Each secret can only be read once, preventing unauthorized access or reuse. This one-time read mechanism, combined with the absence of server-side encryption keys, ensures that secrets remain confidential and ephemeral.

## How It Works
1. The client encrypts the message locally before sending it to the Gone service.
2. The encrypted message is stored temporarily on the server.
3. When the recipient accesses the secret link, the encrypted data is retrieved and decrypted client-side.
4. After the secret is accessed once, it is immediately deleted from the server, making it inaccessible thereafter.
5. The server never has access to the plaintext message or any encryption keys, and therefore cannot decrypt the data.

This straightforward design guarantees secure, ephemeral message sharing without the complexity of managing server-side encryption keys or persistent storage.

## Deployment
Gone is designed to be deployed in Docker. It does not accept command line arguments or config files. Instead, it is configured entirely through environment variables.

## Configuration
Gone can be configured using the following environment variables:

| Environment Variable    | Description                                                                   | Default Value     |
|-------------------------|-------------------------------------------------------------------------------|-------------------|
| `GONE_ADDR`             | The address the service listens on.                                           | `:8080`           |
| `GONE_DATA_DIR`         | The directory where secrets are stored.                                       | `/data`           |
| `GONE_INLINE_MAX_BYTES` | Maximum size of a secret to be stored inline in sqlite3 (bytes).              | `8192` (8 KiB)    |
| `GONE_MAX_BYTES`        | Maximum size of a secret (bytes).                                             | `1048576` (1 MiB) |
| `GONE_MIN_TTL`          | Minimum time-to-live for a secret.                                            | `5m`              |
| `GONE_MAX_TTL`          | Maximum time-to-live for a secret.                                            | `24h`             |

>[!NOTE]
> `GONE_MAX_BYTES` can be calculated as `1024 * 1024` for 1 MiB, `1024 * 10` for 10 KiB, etc.
> `1024` bytes is `1KiB`, `1024 * 1024` bytes is `1MiB`, `1024 * 1024 * 1024` bytes is `1GiB`, and so on.