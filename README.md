# gone

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
