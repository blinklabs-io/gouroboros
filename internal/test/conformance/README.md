# Conformance tests

This package checks Gouroboros ledger, cryptographic, consensus, and Byron
block code against independent inputs. The suites are deliberately reported
separately: passing ledger vectors do not imply protocol-wire or consensus
conformance.

## Ledger reference coverage

The ledger suite consumes the pinned Cardano Blueprint archive in the nested
`internal/test/cardano-blueprint` submodule:

| Provenance | Value |
| --- | --- |
| Blueprint revision | `0f0c17e1ca24b062c868d216ae50708fc19c83ab` |
| Archive SHA-256 | `574ff7a17857dfc1f0cf477f7eb9eba1c2a0f901453396a779de4b2392ef6863` |
| Ledger vector files | 2,574 |
| Protocol-parameter files | 78 |
| Reference-ledger export | `cardano-ledger-conformance-tests` fork commit `34365e427e6507442fd8079ddece9f4a565bf1b9` |

Initialize submodules before running the suite:

```bash
git submodule update --init --recursive
go test -v ./internal/test/conformance/...
```

The test verifies the submodule revision and archive checksum, extracts the
archive into a temporary directory, and runs every archive file through the
shared `ouroboros-mock/conformance` harness. It walks the extracted files
itself so the shared collector cannot mistake the real
`can_use_reference_scripts` directory for a generated helper directory.
The transaction bytes, ledger states, and expected results remain the
Blueprint inputs; no generated corpus is committed.

The verbose ledger test emits one line for every observed
`era/rule-family/expected-result` category. It also requires both accepted and
rejected cases and reports reference-input transactions. A failed vector is a
failed subtest, so an aggregate success message cannot hide a defect.

This layer proves:

- decoding and validating the imported transaction/state pairs through the
  Gouroboros ledger entry points selected by the shared harness;
- expected acceptance and rejection for the represented Shelley, Allegra,
  Mary, Alonzo, Babbage, and Conway rule families;
- the imported reference-input and reference-script cases, including negative
  overlap and script-use cases; and
- the repository-local synthetic rollback behavior supplied by the shared
  harness.

It does not prove:

- complete coverage of every rule, era, protocol parameter, or newer era not
  represented by this pinned export;
- consensus leader election, KES/VRF cryptography, block validation, or
  mini-protocol wire behavior. Those claims belong to the dedicated tests in
  this directory and are reported independently;
- node synchronization, networking, deployment, or live-chain behavior; or
- differential agreement with a separately generated current Intersect
  `cardano-ledger` or formal-ledger-specifications vector set. This change has
  no such differential vector set; the Blueprint export provenance above is
  the reference revision for the imported set. Adding portable differential
  vectors requires recording their exact reference revision here and tracking
  unsupported rules as an explicit gap.

Unsupported or intentionally excluded rules are not counted as passing
coverage. If a future corpus update contains an input that the parser or
ledger implementation cannot model, the vector must fail or be listed here
with a follow-up issue; it must not disappear through a collector filter.

## Other conformance layers

The remaining tests use independent fixtures and have different claims:

- `vrf_conformance_test.go` checks VRF proofs and outputs against
  cardano-crypto-praos vectors.
- `kes_conformance_test.go` checks KES signing, verification, and key
  evolution against input-output-hk/kes vectors.
- `consensus_conformance_test.go` checks consensus calculations and selected
  real-block fields.
- `byron_conformance_test.go` checks selected real Byron mainnet, testnet, and
  epoch-boundary blocks.

Run a layer or all layers with:

```bash
go test -v ./internal/test/conformance/... -run TestRulesConformanceVectors
go test -v ./internal/test/conformance/... -run VRF
go test -v ./internal/test/conformance/... -run KES
go test -v ./internal/test/conformance/... -run Consensus
go test -v ./internal/test/conformance/... -run Byron
go test -v ./internal/test/conformance/...
```

## Troubleshooting

- If the Blueprint revision or checksum is wrong, update the submodule pointer
  and the provenance constants together only after reviewing the new corpus
  inventory and reference revision.
- If a vector fails, use its subtest path and rule-family line to locate the
  corresponding implementation in `ledger/<era>/rules.go` and inspect the
  reference-input consumer before changing a boundary.
- Phase-2-invalid transactions can have an expected successful ledger event:
  `success=true` means the ledger accepted the transaction as an event even
  when its `IsValid` flag causes script effects to be rolled back.
