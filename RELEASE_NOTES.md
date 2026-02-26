# release notes

## (from tag) - transaction handling and CBOR conformance

- **date:** 2026-02-26
- **version:** (from tag)

### Summary

This release includes updates to transaction preparation and `CBOR` serialization conformance.

### New Features

- Improved transaction preparation so `redeemer` mappings and `datum` handling remain consistent across cases.

### Bug Fixes

- Fixed `CBOR` encoding and test-vector mismatches to ensure outputs match expected on-chain representation.
