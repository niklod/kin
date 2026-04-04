# Decision 003: Base64 token format for invite links

**Date:** 2026-04-04

## What
Invite links are compact base64url-encoded tokens with `kin:` prefix, e.g. `kin:xYz...`.

## Why
- Easy to copy/paste in any messenger or terminal
- No dependency on a web domain (unlike URL-based format)
- Accepted via `kin join <token>` CLI command
- Alternative considered: URL with query params (`https://kin.example/join?pk=...`) — requires owning a domain, adds unnecessary web dependency for a desktop CLI tool

## How
- Binary encoding (not protobuf) for compactness: `[32B pubkey][2B endpoint count][endpoints...][16B nonce][8B expiry unix][64B Ed25519 signature]`
- Signature covers everything before it (signed payload)
- Full blob base64url-encoded (no padding), prefixed with `kin:`
