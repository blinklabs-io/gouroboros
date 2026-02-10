# Benchmark Test Data

This directory contains test fixtures for memory profiling benchmarks.

## Directory Structure

```
testdata/
  blocks/           # Block CBOR fixtures (future use)
  transactions/     # Transaction CBOR fixtures (future use)
```

## Current Fixture Source

The benchmark infrastructure currently uses block fixtures from `internal/testdata/`:

| Era     | Source File         | Block Hash                                                         |
|---------|--------------------|--------------------------------------------------------------------|
| Byron   | byron_block.hex    | 1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8   |
| Shelley | shelley_block.hex  | 2308cdd4c0bf8b8bf92523bdd1dd31640c0f42ff079d985fcc07c36cbf915c2b   |
| Allegra | allegra_block.hex  | 8115134ab013f6a5fd88fd2a10825177a2eedcde31cb2f1f35e492df469cf9a8   |
| Mary    | mary_block.hex     | d36ab36f451e9fcbd4247daef45ce5be9a4b918fce5ee97a63b8aeac606fca03   |
| Alonzo  | alonzo_block.hex   | 1d7974cb01cc9e3fbe9dd7594795a36b21cb1deb2f1b70a0625332c91bd7e5a7   |
| Babbage | babbage_block.hex  | db19fcfaba30607e363113b0a13616e6a9da5aa48b86ec2c033786f0a2e13f7d   |
| Conway  | conway_block.hex   | 27807a70215e3e018eec9be8c619c692e06a78ebcb63daf90d7abe823f3bbf47   |

All blocks are real mainnet blocks fetched from [cexplorer.io](https://cexplorer.io).

## Adding New Fixtures

To add a new block fixture:

1. Fetch the block CBOR from a Cardano node or block explorer
2. Save as hex-encoded file: `testdata/blocks/<era>/<name>.hex`
3. Register in `internal/bench/helpers.go` if needed

Example fixture format:
```
# blocks/conway/governance.hex
# Conway block with governance actions
# Slot: 159835207
# Hash: 27807a70215e3e018eec9be8c619c692e06a78ebcb63daf90d7abe823f3bbf47
820685828a1a00...
```

## Transaction Fixtures

Transaction fixtures should follow the same pattern:

```
testdata/transactions/<era>/<name>.hex
```

Categories:
- `simple` - Single input/output, no scripts
- `multisig` - Native multisig scripts
- `plutusv1` - PlutusV1 script transaction
- `plutusv2` - PlutusV2 script transaction
- `plutusv3` - PlutusV3 script transaction
- `governance` - Conway governance actions
- `large` - Maximum inputs/outputs

## Usage

```go
import "github.com/blinklabs-io/gouroboros/internal/bench"

// Load a block fixture
fixture := bench.MustLoadBlockFixture("conway", "default")

// Access the decoded block
block := fixture.Block

// Access raw CBOR
cbor := fixture.Cbor
```
