# Release notes

## (from tag) - Transaction handling and CBOR conformance

- **date:** 2026-02-26
- **version:** (from tag)

### Summary

This release includes updates to transaction preparation and `CBOR` serialization conformance.

### New Features

- Updated transaction preparation to export input sorting for `redeemer` mapping and apply consistent `datum` handling across validation paths.

### Bug Fixes

- Fixed certificate amount `CBOR` encoding and script context serialization in test vectors to match expected on-chain representation.
