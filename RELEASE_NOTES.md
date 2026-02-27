---
title: Release notes
---

# Release notes

## v0.159.2 - transaction validation updates

- **Date:** 2026-02-27
- **Version:** 0.159.2

### Summary

This release includes the changes listed below.

### Breaking Changes

- Updated Conway Plutus transaction validation to require `ScriptDataHash` only when redeemers or witness datums are present (script references are treated as inert) and to return a typed input-resolution error when a UTxO lookup fails.
- Migration: Updated callers to omit `ScriptDataHash` unless a transaction includes Plutus redeemers or witness datums.

### Bug Fixes

- Fixed metadata decoding for generic CBOR maps to fail fast on any key or value decode error by using `*cbor.Value` keys.

### Additional Changes

- Updated developer documentation to reduce ambiguity in delegation and transaction-building workflows.
