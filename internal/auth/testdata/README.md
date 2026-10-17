# Test data

- `google-iap-public_key-jwk.json`: Google Cloud IAP's real public signing keys, fetched on 2026-10-17 from <https://www.gstatic.com/iap/verify/public_key-jwk>. Public keys only. The tests check that Gone's key parser reads the real format; Google rotates these keys, so they are not used to verify tokens.
