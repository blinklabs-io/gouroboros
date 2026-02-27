---
title: Release notes
---

# Release notes

## v0.159.2 - bug fixes and updates

- **Date:** 2026-02-27
- **Version:** 0.159.2

### Summary

This release includes the changes listed below.

### Release note data

```json
{
  "Additional Changes": [
    "Developer documentation has been updated to reduce ambiguity in delegation and transaction-building workflows. `AGENTS.md` now clarifies delegation behavior, documents `TransactionBuilder`/`MockTransaction` usage details, and expands validation and review guidelines."
  ],
  "Breaking Changes": [
    "Transaction validation now only requires a script data hash when scripts actually run. Specifically, Conway Plutus validation treats script references as inert, requires `ScriptDataHash` only when redeemers or witness datums are present, and returns a typed input-resolution error when a UTxO lookup fails."
  ],
  "Bug Fixes": [
    "Metadata decoding for generic CBOR maps is now strict and fails fast instead of partially succeeding. It uses `*cbor.Value` keys and returns an error on any key or value decode failure."
  ]
}

```
