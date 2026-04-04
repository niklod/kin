# Decision 007: Use bytes.Equal for TOFU public key comparison

**Date:** 2026-04-04

## What
`peerstore.PutPeer` uses `bytes.Equal(prev.PublicKey, p.PublicKey)` to detect a public key change for a known NodeID (TOFU enforcement).

## Why
The initial implementation used `string(prev.PublicKey) != string(p.PublicKey)`. While functionally equivalent for equality checks on byte slices, `string()` casting reads as string comparison to reviewers and obscures the intent (comparing cryptographic key material). A future change that accidentally treats the result as a string could introduce subtle bugs.

`bytes.Equal` is idiomatic Go for byte-slice equality and makes the security-critical TOFU check immediately legible.
