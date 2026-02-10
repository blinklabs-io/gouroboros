# Memory Allocation Baselines

**Last Updated**: 2026-02-10
**Go Version**: 1.24+
**Platform**: linux/arm64

## Overview

This document tracks memory allocation baselines for key validation paths in gouroboros. These baselines serve three purposes:

1. **Regression Detection**: CI fails if allocations exceed thresholds by >50%
2. **Optimization Tracking**: Measure impact of performance improvements
3. **Contributor Guidance**: Set expectations for new code

All baseline values were captured after the completion of optimization work in PRs #1496-1503 and #1529.

---

## Current Baselines

### VRF Operations (`vrf/`)

| Operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| VRF KeyGen | 2 | 64 B | ~75us | Seed processing only |
| VRF Prove | 11 | 736 B | ~760us | Scalar multiplication |
| VRF Verify | 11 | 816 B | ~950us | Full verification |
| VRF VerifyAndHash | 11 | 816 B | ~955us | Verify + hash extraction |
| VRF ProofToHash | 2 | 224 B | ~38us | Hash extraction only |
| MkInputVrf | 3 | 464 B | ~1.4us | VRF input creation |

### KES Operations (`kes/`)

| Operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| KES KeyGen (depth=6) | 255 | 8800 B | ~5ms | Cardano standard depth |
| KES Sign (depth=6) | 1 | 448 B | ~180us | Single allocation |
| KES Update (depth=6) | 3 | 736 B | ~78us | Key evolution |
| KES Verify (depth=6) | 6 | 192 B | ~243us | Signature verification |
| KES VerifySignedKES | 12 | 616 B | ~265us | Full verification path |
| KES NewSumKesFromBytes (depth=6) | 6 | 424 B | ~1.1us | Signature deserialization |
| KES HashPair | 1 | 32 B | ~843ns | Blake2b hash |

### Block Validation (`internal/bench/`)

| Operation | Era | Allocs | Bytes | Time | Notes |
|-----------|-----|--------|-------|------|-------|
| Block Validation | Shelley | 461 | 101 KB | ~2.4ms | Full validation |
| Block Validation | Allegra | 1092 | 246 KB | ~3.1ms | |
| Block Validation | Mary | 1136 | 236 KB | ~3.0ms | |
| Block Validation | Alonzo | 1382 | 365 KB | ~3.5ms | |
| Block Validation | Babbage | 5709 | 1014 KB | ~7.0ms | Largest blocks |
| Block Validation | Conway | 2672 | 487 KB | ~4.4ms | |
| Block Validation (pre-parsed) | All Eras | 20 | 1.5 KB | ~0.9ms | Skip decode |
| VRF Verification | All Eras | 10 | 608 B | ~0.9ms | Block VRF check |
| KES Verification | All Eras | 12 | 616 B | ~270us | Block KES check |
| Body Hash | Shelley | 15 | 10 KB | ~42us | |
| Body Hash | Babbage | 19 | 83 KB | ~328us | Largest body |
| Body Hash | Conway | 18 | 39 KB | ~153us | |

### Block Decode (`internal/bench/`)

| Operation | Era | Allocs | Bytes | Throughput | Notes |
|-----------|-----|--------|-------|------------|-------|
| CBOR Decode | Byron | 500 | 89 KB | 5.5 MB/s | |
| CBOR Decode | Shelley | 441 | 100 KB | 8.2 MB/s | |
| CBOR Decode | Allegra | 1072 | 245 KB | 6.7 MB/s | |
| CBOR Decode | Mary | 1116 | 235 KB | 5.2 MB/s | |
| CBOR Decode | Alonzo | 1362 | 363 KB | 6.4 MB/s | |
| CBOR Decode | Babbage | 5689 | 1014 KB | 3.5 MB/s | |
| CBOR Decode | Conway | 2652 | 485 KB | 3.5 MB/s | |
| Parallel Decode | Byron | 500 | 89 KB | 19.6 MB/s | |
| Parallel Decode | Shelley | 441 | 100 KB | 35.8 MB/s | |
| Parallel Decode | Babbage | 5690 | 1005 KB | 25.2 MB/s | |

### Transaction Validation (`internal/bench/`)

| Operation | Era | Allocs | Bytes | Time | Notes |
|-----------|-----|--------|-------|------|-------|
| Tx Validation | Shelley | 64 | 5.3 KB | ~605us | Simple tx |
| Tx Validation | Allegra | 32 | 3.7 KB | ~600us | |
| Tx Validation | Mary | 42 | 4.5 KB | ~553us | |
| Tx Validation | Alonzo | 44 | 5.3 KB | ~371us | |
| Tx Validation | Babbage | 310 | 22.1 KB | ~1.4ms | |
| Tx Validation | Conway | 220 | 18.7 KB | ~1.8ms | |
| Value Balance | Shelley | 9 | 216 B | ~1.2us | |
| Value Balance | Alonzo | 21 | 624 B | ~3.0us | |
| Witness Validation | Shelley | 13 | 624 B | ~3.1us | |
| Witness Validation | Alonzo | 28 | 6.5 KB | ~33us | |

### Consensus / Leader Election (`internal/bench/`)

| Operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| CertifiedNatThreshold | 1221-1224 | 163-168 KB | ~4.1ms | big.Rat arithmetic |
| VrfLeaderValue | 4 | 528 B | ~1.6us | Blake2b hash |
| VRFOutputToInt | 1 | 64 B | ~139ns | big.Int conversion |
| IsSlotLeader | 1240-1244 | 165-169 KB | ~5.3ms | Full leader check |
| IsVRFOutputBelowThreshold | 5 | 592 B | ~1.8us | Threshold comparison |
| Full Leader Election Workflow | 1242 | 167 KB | ~5.4ms | Complete flow |

---

## How to Update Baselines

### Run All Benchmarks

```bash
# VRF benchmarks
go test -bench=. -benchmem ./vrf/... -run=^$ 2>&1 | tee vrf_bench.txt

# KES benchmarks
go test -bench=. -benchmem ./kes/... -run=^$ 2>&1 | tee kes_bench.txt

# Internal benchmarks (block, tx, consensus, CBOR)
go test -bench=. -benchmem ./internal/bench/... -run=^$ 2>&1 | tee internal_bench.txt
```

### Compare Against Previous Run

```bash
# Install benchstat if needed
go install golang.org/x/perf/cmd/benchstat@latest

# Compare old vs new
benchstat old_bench.txt new_bench.txt
```

### Generate Memory Profile

```bash
# CPU profile
go test -bench=BenchmarkBlockValidation -cpuprofile=cpu.prof ./internal/bench/...

# Memory profile
go test -bench=BenchmarkBlockValidation -memprofile=mem.prof ./internal/bench/...

# Analyze
go tool pprof -http=:8080 mem.prof
```

### Extract Specific Values

```bash
# Get allocation counts for VRF Verify
go test -bench='BenchmarkVerify/Valid' -benchmem ./vrf/... -run=^$ | grep allocs

# Get allocation counts for block validation
go test -bench='BenchmarkBlockValidation/Era_Conway' -benchmem ./internal/bench/... -run=^$
```

---

## Optimization History

### Merged PRs (2026-01)

| PR | Focus Area | Impact |
|----|------------|--------|
| #1496 | KES optimizations | Reduced allocs in key operations |
| #1497 | VRF scalar ops | Improved scalar multiplication |
| #1498 | Block body prealloc | Reduced body decode allocs |
| #1499 | Byron merkle buffers | Fixed buffer reuse in merkle tree |
| #1500 | Fixed nonce buffers | Reduced MkInputVrf allocs |
| #1501 | Plutus context prealloc | Reduced Plutus context building |
| #1502 | VRF leader value | Optimized leader value computation |
| #1503 | big.Rat reuse | Reduced threshold calculation allocs |
| #1529 | CBOR EncMode/DecMode cache | 46-49% faster encode/decode |

### Pre-Optimization Estimates (for reference)

| Operation | Est. Before | Current | Reduction |
|-----------|-------------|---------|-----------|
| VRF Verify | ~15 allocs | 11 allocs | ~27% |
| KES Verify (depth=6) | ~12 allocs | 6 allocs | ~50% |
| MkInputVrf | ~5 allocs | 3 allocs | ~40% |
| Threshold Calc | ~2000 allocs | ~1220 allocs | ~39% |

---

## Regression Thresholds

### CI Failure Criteria

The benchmark CI workflow fails a PR if any of these thresholds are exceeded:

| Metric | Threshold | Rationale |
|--------|-----------|-----------|
| Allocation Count | +50% | Catches allocation leaks |
| Bytes Allocated | +100% | Allows some flexibility for features |
| Time (ns/op) | +50% | Catches performance regressions |

### Critical Paths

These operations are performance-critical and have stricter monitoring:

| Operation | Max Allocs | Rationale |
|-----------|------------|-----------|
| VRF Verify | 15 | Block validation hot path |
| KES VerifySignedKES | 15 | Block validation hot path |
| MkInputVrf | 5 | Called for every slot check |
| Body Hash | 25 | Called for every block |

### How to Request Threshold Increase

If a PR legitimately increases allocations:

1. Document the reason in the PR description
2. Update this file with new baseline values
3. Request reviewer approval for threshold increase

---

## Benchmark Environment Notes

- **CPU Scaling**: Disable CPU frequency scaling for consistent results
- **Parallel Tests**: Use `-p 1` to avoid contention in parallel benchmarks
- **Warmup**: Run benchmarks twice; use second run for baselines
- **Count**: Use `-count=5` and benchstat for statistical significance

```bash
# Recommended benchmark command for baselines
go test -bench=. -benchmem -count=5 -p=1 ./internal/bench/... 2>&1 | tee bench.txt
```

---

## Related Documentation

- [README.md](README.md) - Benchmark package documentation
- [PROFILING.md](PROFILING.md) - Profiling guide
