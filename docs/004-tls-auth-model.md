# Decision 004: Mutual TLS with TOFU, no CA

**Date:** 2026-04-04

## What
Nodes authenticate via mutual TLS using self-signed Ed25519 certificates. Trust is established through invite-based TOFU (Trust On First Use), not certificate authorities.

## Why
- No CA infrastructure to manage — appropriate for a decentralized P2P system
- Ed25519 keys serve dual purpose: node identity and TLS authentication
- TLS 1.3 has native Ed25519 support; TLS 1.2 does not, which would require custom cipher suite config
- `InsecureSkipVerify: true` is necessary because self-signed certs fail Go's default CA-based verification — actual trust verification happens in custom `VerifyPeerCertificate` callback by checking SHA-256(pubkey) against expected NodeID

## How
- `GenerateSelfSignedCert()` creates x509 cert from Ed25519 key (10-year validity)
- Listener: `ClientAuth: RequireAnyClientCert` — demands peer cert, verifies via callback
- Dialer: verifies peer's NodeID matches expected value
- PeerStore enforces TOFU: if a known NodeID presents a different public key, connection is rejected with `ErrKeyChanged`
- Only the 32-byte Ed25519 seed is stored on disk; full key reconstructed deterministically
