# TLS Certificates for Chord DHT

This directory contains the TLS certificates used to secure communication between Chord nodes.

## Certificate Details

### Key Sizes
- **RSA Keys**: 4096 bits (very strong, secure for decades)
- **AES Encryption**: 256 bits (industry standard)

### Validity
- **Duration**: 365 days from generation
- **Algorithm**: RSA with SHA-256
- **X.509 Version**: v3


### What TLS Provides

1. **Confidentiality**: Data encrypted in transit
   - Nobody can read the data being transmitted
   - Uses AES-256-GCM cipher

2. **Authentication**: Verify node identities
   - Ensures you're talking to the right node
   - Prevents impersonation attacks

3. **Integrity**: Detect tampering
   - HMAC ensures messages haven't been modified
   - Any changes are immediately detected

4. **Forward Secrecy**: Past sessions stay secure
   - Even if server key is compromised later
   - Past communications remain encrypted



## Protocol Versions

gRPC automatically negotiates the highest TLS version supported by both sides:

- **TLS 1.3** (preferred): Faster handshake, stronger security
- **TLS 1.2** (fallback): Widely supported, still secure

Older versions (TLS 1.0, 1.1) are disabled by default due to security vulnerabilities.

## Cipher Suites

Common cipher suites used (in order of preference):

1. `TLS_AES_256_GCM_SHA384` (TLS 1.3)
2. `TLS_CHACHA20_POLY1305_SHA256` (TLS 1.3)
3. `TLS_AES_128_GCM_SHA256` (TLS 1.3)
4. `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384` (TLS 1.2)

These provide:
- **AES-GCM**: Authenticated encryption (AEAD)
- **ECDHE**: Elliptic Curve Diffie-Hellman (forward secrecy)
- **SHA-256/384**: Strong hash functions


### Certificate Expired
```
Error: x509: certificate has expired
```
### Generate the certificates
start of by generate the ca-cert and ca-key they should be in the sub folder shared.
`genAll.sh`

these two pem should be shared amoung all nodes.

then for each node run.
`generate_certs.sh`
this will give you: server-cert och server-key.

