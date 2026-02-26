# release notes

## pending release

- **date:** 2026-02-26
- **version:** (from tag)

This release includes the changes listed below.

```json
{
  "Bug Fixes": [
    "Fixed multiple encoding and test-vector issues to ensure outputs match the expected on-chain representation. Specifically, this corrects certificate amount CBOR encoding and adjusts the script context CBOR in tests to align with the updated serialization rules."
  ],
  "New Features": [
    "Improved how transactions are prepared and checked so that redeemer mappings and datums are handled consistently across different cases. Specifically, the library now exports input sorting for redeemer mapping, treats empty Conway redeemers as an empty CBOR map, extends datum justification to outputs, and uses sorted inputs during Plutus script validation."
  ]
}

```
