# Benchmark Suite

This package provides comprehensive benchmarks for memory profiling and performance measurement across Gouroboros validation paths.

## Quick Start

```bash
# Run all benchmarks in this package
go test -bench=. -benchmem ./internal/bench/...

# Run specific benchmark
go test -bench=BenchmarkConsensusThreshold -benchmem ./internal/bench/...

# Compare two runs (requires benchstat)
go test -bench=. -benchmem -count=5 ./internal/bench/... | tee new.txt
benchstat old.txt new.txt
```

## Package Structure

```
internal/bench/
  helpers.go              # Benchmark utilities and fixture loaders
  helpers_test.go         # Tests for helpers
  block_bench_test.go     # Block validation benchmarks
  cbor_bench_test.go      # CBOR decode benchmarks
  tx_bench_test.go        # Transaction benchmarks
  consensus_bench_test.go # Consensus/leader election benchmarks
  script_bench_test.go    # Script execution benchmarks
  regression_test.go      # Allocation regression tests
  profile_test.go         # Profiling integration (build tag: profile)
  README.md               # This file
  BASELINES.md            # Memory allocation baselines
  PROFILING.md            # Profiling guide
  testdata/               # Test fixtures (blocks, transactions)
```

## Available Benchmarks

### Block Benchmarks (`block_bench_test.go`)

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkBlockValidation` | Full block validation (VRF, KES, body hash) by era |
| `BenchmarkBlockValidationPreParsed` | Validation with pre-parsed blocks |
| `BenchmarkBlockVRFVerification` | VRF verification isolated by era |
| `BenchmarkBlockKESVerification` | KES verification isolated by era |
| `BenchmarkBlockBodyHash` | Body hash validation by era |
| `BenchmarkBlockDecode` | Block CBOR decoding by era |
| `BenchmarkBlockDecodeWithBodyHash` | Block decoding with body hash validation |
| `BenchmarkMkInputVrf` | VRF input creation for leader election |
| `BenchmarkVerifyKesComponents` | Full KES components verification |
| `BenchmarkHeaderCborEncode` | Header body CBOR encoding for KES |

### CBOR Benchmarks (`cbor_bench_test.go`)

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkCBORDecodeBlock` | Block CBOR decoding by era |
| `BenchmarkCBORDecodeBlockParallel` | Parallel block decoding throughput |
| `BenchmarkCBORDecodeTx` | Transaction CBOR decoding by era |
| `BenchmarkCBORDecodeTxParallel` | Parallel transaction decoding |
| `BenchmarkCBOREncodeBlock` | Block CBOR encoding by era |
| `BenchmarkCBOREncodeTx` | Transaction CBOR encoding by era |
| `BenchmarkCBORDecodeRaw` | Low-level cbor.Decode overhead |
| `BenchmarkCBOREncode` | Low-level cbor.Encode (Small/Medium/Large) |
| `BenchmarkCBORRoundTrip` | Decode + encode cycle by era |
| `BenchmarkCBORDecodeBlockWithOffsets` | Block decoding with TX offset extraction |

### Transaction Benchmarks (`tx_bench_test.go`)

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkTxDecode` | Transaction CBOR decoding by era |
| `BenchmarkTxHash` | Transaction hash calculation |
| `BenchmarkTxInputs` | Transaction input iteration |
| `BenchmarkTxOutputs` | Transaction output iteration |
| `BenchmarkTxFee` | Transaction fee retrieval |
| `BenchmarkTxMetadata` | Transaction metadata retrieval |
| `BenchmarkTxCbor` | Transaction CBOR bytes retrieval |
| `BenchmarkTxUtxorpc` | Transaction Utxorpc conversion |
| `BenchmarkTxProducedUtxos` | Produced UTXO enumeration |
| `BenchmarkTxConsumedUtxos` | Consumed UTXO enumeration |
| `BenchmarkTxBlockIteration` | Iterate all transactions in a block |

### Consensus Benchmarks (`consensus_bench_test.go`)

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkConsensusThreshold` | Certified natural threshold calculation |
| `BenchmarkConsensusThresholdWithMode` | Threshold for CPraos vs TPraos modes |
| `BenchmarkConsensusLeaderValue` | VRF leader value computation (BLAKE2b-256) |
| `BenchmarkConsensusVRFOutputToInt` | VRF output to big.Int conversion |
| `BenchmarkConsensusIsSlotLeader` | Full leader election check |
| `BenchmarkConsensusIsSlotLeaderFromComponents` | Leader check from pre-computed components |
| `BenchmarkConsensusMkInputVrf` | VRF input (nonce) creation |
| `BenchmarkConsensusIsVRFOutputBelowThreshold` | Threshold comparison |
| `BenchmarkConsensusFullLeaderElectionWorkflow` | Complete leader election workflow |
| `BenchmarkConsensusParallelThreshold` | Concurrent threshold calculations |

### Script Benchmarks (`script_bench_test.go`)

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkNativeScriptEvaluate` | Native script evaluation (Pubkey/All/Any/NofK/Timelock) |
| `BenchmarkNativeScriptEvaluateDepth` | Nested script evaluation at various depths |
| `BenchmarkNativeScriptEvaluateWorstCase` | Worst-case branch checking |
| `BenchmarkPlutusScriptDeserialize` | Plutus script deserialization (V1/V2/V3) |
| `BenchmarkPlutusScriptHash` | Plutus script hash computation |
| `BenchmarkNativeScriptHash` | Native script hash computation |
| `BenchmarkNativeScriptCBOR` | Native script CBOR encode/decode |

### Other Package Benchmarks

Additional benchmarks are located in their respective packages:

| Package | File | Focus |
|---------|------|-------|
| `vrf/` | `benchmark_test.go` | VRF key generation, prove, verify |
| `kes/` | `benchmark_test.go` | KES key generation, sign, verify, update |
| `pipeline/` | `benchmark_test.go` | Block processing pipeline throughput |

## Running Benchmarks

### Basic Commands

```bash
# Run all benchmarks with memory stats
go test -bench=. -benchmem ./internal/bench/...

# Run benchmarks in all packages
go test -bench=. -benchmem ./...

# Run with verbose output
go test -bench=. -benchmem -v ./internal/bench/...

# Run specific benchmark pattern
go test -bench=BenchmarkConsensus -benchmem ./internal/bench/...
```

### Common Flags

| Flag | Description | Example |
|------|-------------|---------|
| `-bench=<regex>` | Run benchmarks matching pattern | `-bench=Threshold` |
| `-benchmem` | Report memory allocations | Always recommended |
| `-benchtime=<d>` | Run each benchmark for duration | `-benchtime=5s` |
| `-count=<n>` | Run each benchmark n times | `-count=5` |
| `-cpu=<list>` | Specify GOMAXPROCS values | `-cpu=1,2,4,8` |
| `-cpuprofile=<file>` | Write CPU profile | `-cpuprofile=cpu.prof` |
| `-memprofile=<file>` | Write memory profile | `-memprofile=mem.prof` |
| `-timeout=<d>` | Test timeout | `-timeout=30m` |

### Multiple Run Comparison

For reliable performance comparisons, run benchmarks multiple times:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run baseline
go test -bench=. -benchmem -count=5 ./internal/bench/... | tee baseline.txt

# Make changes, then run again
go test -bench=. -benchmem -count=5 ./internal/bench/... | tee new.txt

# Compare results
benchstat baseline.txt new.txt
```

## Interpreting Results

### Output Format

```
BenchmarkConsensusThreshold/1pct-16    876543    1234 ns/op    256 B/op    8 allocs/op
```

| Field | Meaning |
|-------|---------|
| `876543` | Number of iterations completed |
| `1234 ns/op` | Nanoseconds per operation |
| `256 B/op` | Bytes allocated per operation |
| `8 allocs/op` | Heap allocations per operation |

### Aspirational Performance Targets

These are aspirational targets for future optimization work. Current baselines
are documented in `BASELINES.md`.

| Operation | Target Allocs | Notes |
|-----------|---------------|-------|
| VRF Verify | 4-8 | After opt/vrf-combined |
| KES Verify | 6-12 | After opt/kes-combined |
| Threshold Calc | 10-15 | After opt/threshold-allocs |
| Leader Check | 15-20 | Composite of above |
| MkInputVrf | 0 | After opt/nonce-buffers |

### benchstat Output

```
name                  old time/op    new time/op    delta
ConsensusThreshold    1.23us +-2%    0.89us +-1%   -27.64%  (p=0.008 n=5+5)

name                  old alloc/op   new alloc/op   delta
ConsensusThreshold     256B +- 0%     128B +- 0%   -50.00%  (p=0.008 n=5+5)
```

- `delta` shows percentage change
- `+-` shows variance
- `p=0.008` is statistical significance (lower is better)
- `n=5+5` means 5 runs of each version

## Helper Functions

The `helpers.go` file provides utilities for benchmarks:

### Ledger State

```go
// Basic mock ledger state
ls := bench.BenchLedgerState()

// With specific UTXOs
ls := bench.BenchLedgerStateWithUtxos(utxos)

// With custom UTXO lookup
ls := bench.BenchLedgerStateWithUtxoFunc(func(input common.TransactionInput) (common.Utxo, error) {
    return myUtxo, nil
})
```

### Block Fixtures

```go
// Load block fixture for an era
fixture, err := bench.LoadBlockFixture("conway", "default")

// Must variant (panics on error, use in init/setup)
fixture := bench.MustLoadBlockFixture("conway", "default")

// Access fixture data
block := fixture.Block
cbor := fixture.Cbor
blockType := fixture.BlockType
```

### Transaction Fixtures

```go
// Load transaction fixture
txFixture, err := bench.LoadTxFixture("conway", "default")

// Access transaction
tx := txFixture.Tx
```

### Era Utilities

```go
// Get all era names
eras := bench.EraNames()  // ["byron", "shelley", ..., "conway"]

// Get post-Byron eras (for Praos benchmarks)
eras := bench.PostByronEraNames()  // ["shelley", ..., "conway"]

// Convert era name to block type
blockType, err := bench.BlockTypeFromEra("conway")
```

## Adding New Benchmarks

### Structure

1. Create a new file `<category>_bench_test.go`
2. Use sub-benchmarks for variants (`b.Run()`)
3. Always call `b.ReportAllocs()`
4. Reset timer after setup (`b.ResetTimer()`)

### Template

```go
// Copyright 2026 Blink Labs Software
// ... license header ...

package bench

import (
    "testing"
)

// BenchmarkMyOperation benchmarks [description].
func BenchmarkMyOperation(b *testing.B) {
    // Setup (not measured)
    fixture := MustLoadBlockFixture("conway", "default")

    variants := []struct {
        name string
        // variant parameters
    }{
        {"Small", /* ... */},
        {"Large", /* ... */},
    }

    for _, v := range variants {
        b.Run(v.name, func(b *testing.B) {
            b.ReportAllocs()
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                // Operation to benchmark
                _ = myOperation(fixture.Block)
            }
        })
    }
}
```

### Naming Conventions

- Top-level: `Benchmark<Component><Operation>` (e.g., `BenchmarkConsensusThreshold`)
- Sub-benchmarks: descriptive variant names (e.g., `1pct`, `10pct`, `Large`, `Small`)
- Use underscores for hierarchical names (e.g., `Era_Conway`, `TxType_Simple`)

### Best Practices

1. **Isolate setup from measurement**: Do expensive setup before `b.ResetTimer()`
2. **Avoid allocations in loop**: Pre-allocate test data outside the benchmark loop
3. **Use realistic data**: Load fixtures from real blocks/transactions
4. **Test multiple variants**: Include edge cases and typical cases
5. **Document allocations**: Note expected allocation counts in comments
6. **Prevent compiler optimization**: Use result (e.g., assign to package-level var)

### Parallel Benchmarks

For concurrency testing:

```go
func BenchmarkParallelOperation(b *testing.B) {
    b.ReportAllocs()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _ = myOperation()
        }
    })
}
```

## Profiling Integration

### Using the Profile Script

The easiest way to generate profiles is with the helper script:

```bash
# Make the script executable (first time only)
chmod +x scripts/profile.sh

# Profile block validation (CPU + memory)
./scripts/profile.sh block both

# Profile CBOR decode with web UI
./scripts/profile.sh cbor both --view

# Profile all components
./scripts/profile.sh all both

# Show help
./scripts/profile.sh --help
```

### Available Profile Tests

Profile tests are guarded by the `profile` build tag (see `profile_test.go`):

| Component | Test Name | Description |
|-----------|-----------|-------------|
| block | `TestProfileBlockValidation` | Full block validation (VRF, KES, body hash) |
| cbor | `TestProfileCBORDecode` | CBOR deserialization for all eras |
| tx | `TestProfileTxValidation` | Transaction validation rules |
| vrf | `TestProfileVRF` | VRF prove/verify operations |
| consensus | `TestProfileConsensus` | Leader election computations |
| bodyhash | `TestProfileBodyHash` | Block body hash validation |
| script | `TestProfileNativeScript` | Native script evaluation |

### Running Profile Tests Directly

```bash
# Generate CPU and memory profiles for block validation
go test -tags=profile -run=TestProfileBlockValidation \
    -cpuprofile=cpu_block.prof -memprofile=mem_block.prof \
    ./internal/bench/...

# Analyze with pprof web UI
go tool pprof -http=localhost:8080 cpu_block.prof
go tool pprof -http=localhost:8081 mem_block.prof
```

### Generate Profiles from Benchmarks

```bash
# CPU profile
go test -bench=BenchmarkConsensusIsSlotLeader -cpuprofile=cpu.prof ./internal/bench/...
go tool pprof -http=localhost:8080 cpu.prof

# Memory profile
go test -bench=BenchmarkConsensusIsSlotLeader -memprofile=mem.prof ./internal/bench/...
go tool pprof -http=localhost:8081 mem.prof
```

### Comparing Profiles

```bash
# Compare two profiles (before/after optimization)
go tool pprof -base=old.prof new.prof

# Text-based top functions
go tool pprof -top cpu.prof
go tool pprof -top -alloc_space mem.prof
```

See `PROFILING.md` in this directory for detailed profiling guidance.

## Regression Testing

Allocation regression tests ensure optimization gains are maintained:

```go
func TestAllocationRegression(t *testing.T) {
    allocs := testing.AllocsPerRun(100, func() {
        _ = myOperation()
    })
    if allocs > 10 {
        t.Errorf("allocations regressed: %.0f > 10", allocs)
    }
}
```

## CI Benchmark Results

### Workflow Artifacts

Benchmark results from CI runs are stored as GitHub Actions workflow artifacts:

1. **Location**: GitHub Actions > Workflows > Benchmarks > Run > Artifacts
2. **Naming**: `benchmark-<commit-sha>-<run-number>`
3. **Retention**: 30 days
4. **Contents**:
   - `benchmark-current.txt` - Benchmark results for the current commit
   - `benchmark-base.txt` - Benchmark results for the base branch (PRs only)
   - `benchmark-comparison.txt` - Comparison output from benchstat (PRs only)

### Accessing Artifacts

1. Navigate to the repository on GitHub
2. Click "Actions" tab
3. Select the "Benchmarks" workflow
4. Click on the specific run you're interested in
5. Scroll down to "Artifacts" section
6. Download the benchmark results

### Regression Thresholds

The CI workflow flags regressions when:
- Time increases by more than 50%
- Allocations increase by more than 50%

### GitHub Pages (Optional)

When enabled, historical benchmark data is available with interactive trend charts.
See `.github/workflows/benchmark.yml` for setup instructions.

## Related Documentation

- `PROFILING.md` - Detailed profiling guide (in this directory)
- `BASELINES.md` - Memory allocation baselines (in this directory)
- `vrf/README.md` - VRF package documentation
- `kes/README.md` - KES package documentation
- `consensus/README.md` - Consensus package documentation
