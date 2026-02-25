---
title: Release Notes
---

# Release Notes

## version TBD - ledger validation and protocol pipelining

**Date:** 2026-02-25

**Version:** 0.158.4

**Summary:** This release expands ledger validation safety checks and improves protocol pipelining behavior.

### New Features

- Added overflow-safe `ExUnits` accumulation, a no-duplicate-inputs UTxO rule across all eras, and centralized Shelley withdrawal validation with an `IsValid` short-circuit.

### Breaking Changes

- None.

### Bug Fixes

- Fixed an issue where protocol pipelining state transitions could be applied incorrectly by queuing transitions in the core protocol send loop, removing `ChainSync`-specific pipelining state tracking, adding a `ChainSync` client pipelining test, and updating `ouroboros-mock` to v0.9.1.

### Performance

- None.

### Security

- None.

### Deprecations

- None.

### Additional Changes

- Updated `AGENTS.md` to demonstrate creating mock ledgers using `ouroboros-mock` ledger builders instead of `MockLedgerState`.
- Updated `filippo.io/edwards25519` from v1.1.1 to v1.2.0.
