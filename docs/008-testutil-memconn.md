# Decision 008: Shared MemConn test helper in testutil

**Date:** 2026-04-04

## What
An in-memory bidirectional message pipe (`testutil.MemConn`) is defined once in `testutil/testutil.go` and imported by all test packages that need a fake transport.

## Why
Without a shared type, `transfer_test` and `protocol_test` each defined their own `memConn`/`chanConn` structs with identical Send/Recv semantics. Duplication meant two places to update if the `kinpb.Envelope` interface changed, and diverging buffer sizes (32 vs 64) that could cause subtly different test behaviour.

Centralising in `testutil` gives a single definition, a consistent API (`NewMemConnPair`, `CloseRecv`, etc.), and makes the interface contract explicit in one place.

## Tradeoff
`testutil` now imports `kinpb`, coupling a utility package to the generated protobuf code. Acceptable because `testutil` is test-only and `kinpb` is an internal package with no external dependency of its own.
