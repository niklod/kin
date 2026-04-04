# Decision 005: ~/Kin as default shared folder

**Date:** 2026-04-04

## What
The shared folder defaults to `~/Kin` in the user's home directory.

## Why
- Visible and intuitive, following Dropbox/Syncthing convention
- Alternatives considered:
  - `~/.local/share/kin/shared` — XDG-correct but hidden, less discoverable for users
  - User-specified at first launch (`kin init /path`) — more flexible but adds setup friction
- Desktop-first MVP prioritizes discoverability over XDG compliance
