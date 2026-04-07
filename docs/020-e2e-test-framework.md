# 020 — E2E Test Framework

## What

Process-level E2E test framework in `e2e/` that launches real `kin` and `relay` binaries on the same machine and verifies their interaction.

## Why

Testing Kin's P2P functionality previously required two machines on different networks — a manual process involving a friend connecting from a different NAT. This was slow, unreliable, and blocked development velocity.

The E2E framework automates this by running two isolated kin instances + a relay on loopback, testing the full stack: binary startup, relay registration, invite/join handshake, catalog exchange.

## Design

### Isolation

Each peer gets its own:
- Config directory (`--config-dir`) — unique identity.key, peers.db, catalog.db
- Shared folder (`--shared-dir`) — unique file storage
- Listen port (`--listen 127.0.0.1:<port>`) — no port conflicts
- The relay is shared across all tests (stateless between sessions)

### Architecture

```
Cluster (framework.go)
├── builds kin + relay binaries once (SetupSuite)
├── starts relay process
├── creates Peer instances with isolated temp dirs
└── cleanup: stops all processes, dumps logs on failure

Relay (relay.go)
├── exec.Command for cmd/relay
├── pre-allocated port via freePort()
└── waits for "relay running" stdout line

Peer (peer.go)
├── exec.Command for cmd/kin
├── Start() — kin run (long-running)
├── Invite() — kin invite (one-shot)
├── Join(token) — kin join (one-shot)
├── Status() — kin status (one-shot, parses output)
└── WriteFile() / HasFile() — shared dir operations

lineCapture (helpers.go)
├── thread-safe stdout/stderr capture via io.Pipe
├── WaitForLine(ctx, pattern) — block until regex matches
├── Contains(pattern) — check if pattern appeared
└── All() — dump all captured output
```

### Build Tag

All files use `//go:build e2e`. Unit tests (`make test`) skip E2E entirely. Run via `make test-e2e`.

### Test Lifecycle

```
SetupSuite    → build binaries, start relay (once)
  per test    → NewPeer() creates fresh isolated dirs
  test body   → Start/Invite/Join/Status/verify
TearDownSuite → stop all processes, dump logs on failure
```

## Alternatives Considered

- **In-process testing** (start relay/peers as goroutines): Faster but doesn't test the actual binaries, CLI flag parsing, process lifecycle, or signal handling.
- **Docker-based**: Heavier, slower startup, harder to debug. Overkill for loopback testing.
- **Bash scripts**: Not integrated with `go test`, no race detection, harder to maintain.

## Limitations

- **No ongoing connection tests**: After `kin join` exits, the connection closes. Testing ongoing file sync between two running peers requires the daemon socket (task U.0).
- **Port allocation TOCTOU**: `freePort()` has a tiny race window. Acceptable for test code.
- **Log-based assertions**: Some catalog tests verify behavior via debug log patterns, which couples to log message strings.

## How to Add New Tests

1. Add a method to `E2ESuite` in an existing or new `*_test.go` file in `e2e/`
2. Create peers via `s.cluster.NewPeer("name")` (auto-tracked for cleanup)
3. Call `peer.Start()`, `peer.Invite()`, `peer.Join(token)`, etc.
4. Assert via `peer.Status()`, `peer.HasFile()`, `peer.StderrContains()`, or `peer.stderr.WaitForLine()`
5. Run: `make test-e2e`
