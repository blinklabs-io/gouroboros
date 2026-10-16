# Memory Allocation Baselines

**Last Updated**: 2026-06-11
**Go Version**: 1.25.8
**Platform**: linux/arm64

The E34 cutover section has its own Go version and platform metadata.

> The previous snapshot (2026-02-22) was captured on linux/amd64. Time and
> throughput deltas against that snapshot partly reflect the hardware change;
> allocation counts and bytes are comparable across platforms.

## Overview

This document tracks memory allocation baselines for key validation paths in gouroboros. These baselines serve three purposes:

1. **Regression Detection**: Allocation regressions are caught by `TestAllocationRegression` tests; benchmark thresholds (>20% allocs, >20% time, >50% bytes) are aspirational targets for future CI enforcement (see [Regression Thresholds](#regression-thresholds))
2. **Optimization Tracking**: Measure impact of performance improvements
3. **Contributor Guidance**: Set expectations for new code

Values are means of `-count=3` runs of `go test -bench -benchmem` per package.

---

## Current Baselines

### VRF Operations (`vrf/`)

| Operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| VRF KeyGen | 2 | 64 B | ~66us | Seed processing only |
| VRF Prove | 11 | 736 B | ~690us | Scalar multiplication |
| VRF Verify | 11 | 816 B | ~825us | Full verification |
| VRF VerifyAndHash | 11 | 816 B | ~812us | Verify + hash extraction |
| VRF ProofToHash | 2 | 224 B | ~33us | Hash extraction only |
| MkInputVrf | 3 | 464 B | ~1.5us | VRF input creation |

### KES Operations (`kes/`)

| Operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| KES KeyGen (depth=6) | 255 | 8800 B | ~4.4ms | Cardano standard depth |
| KES Sign (depth=6) | 10 | 830 B | ~166us | Was 1 alloc / 448 B in 2026-02-22 snapshot |
| KES Update (depth=6) | 3 | 736 B | ~68us | Key evolution |
| KES Verify (depth=6) | 6 | 192 B | ~211us | Signature verification |
| KES VerifySignedKES | 12 | 616 B | ~217us | Full verification path |
| KES NewSumKesFromBytes (depth=6) | 6 | 424 B | ~1.2us | Signature deserialization |
| KES HashPair | 1 | 32 B | ~918ns | Blake2b hash |

### Block Validation (`internal/bench/`)

| Operation | Era | Allocs | Bytes | Time | Notes |
|-----------|-----|--------|-------|------|-------|
| Block Validation | Shelley | 430 | 94 KB | ~0.8ms | Full validation |
| Block Validation | Allegra | 1031 | 220 KB | ~1.4ms | |
| Block Validation | Mary | 1019 | 200 KB | ~1.4ms | |
| Block Validation | Alonzo | 918 | 299 KB | ~1.4ms | |
| Block Validation | Babbage | 3697 | 771 KB | ~3.9ms | Largest blocks |
| Block Validation | Conway | 1924 | 394 KB | ~2.2ms | |
| Block Validation (pre-parsed) | All Eras | 27-31 | 11-84 KB | ~0.4-0.8ms | Skip decode; bytes track block size |
| VRF Verification | All Eras | 9 | 592 B | ~0.8ms | Block VRF check¹ |
| KES Verification | All Eras | 12 | 616 B | ~215us | Block KES check |
| Body Hash | Shelley | 15 | 10 KB | ~48us | |
| Body Hash | Babbage | 19 | 83 KB | ~314us | Largest body |
| Body Hash | Conway | 18 | 39 KB | ~152us | |

¹ Measured with the BenchmarkBlockVRFVerification fix that tolerates
`vrf.ErrProofVerificationFailed`; the benchmark aborted on every run before
that fix.

### Block Decode (`internal/bench/`)

| Operation | Era | Allocs | Bytes | Throughput | Notes |
|-----------|-----|--------|-------|------------|-------|
| CBOR Decode | Byron | 500 | 89 KB | 5.3 MB/s | |
| CBOR Decode | Shelley | 418 | 93 KB | 8.4 MB/s | |
| CBOR Decode | Allegra | 1019 | 219 KB | 6.9 MB/s | |
| CBOR Decode | Mary | 1007 | 200 KB | 5.5 MB/s | |
| CBOR Decode | Alonzo | 906 | 298 KB | 8.2 MB/s | |
| CBOR Decode | Babbage | 3685 | 771 KB | 5.1 MB/s | |
| CBOR Decode | Conway | 1912 | 393 KB | 4.4 MB/s | |
| Parallel Decode | Byron | 500 | 89 KB | 18.5 MB/s | |
| Parallel Decode | Shelley | 418 | 93 KB | 25.2 MB/s | |
| Parallel Decode | Babbage | 3685 | 771 KB | 28.9 MB/s | |

### Transaction Operations (`internal/bench/`, `ledger/...`)

The per-era "Tx Validation", "Value Balance", and "Witness Validation" rows
from earlier snapshots no longer have corresponding benchmarks in the tree;
the rows below are backed by benchmarks that exist today.

| Operation | Era | Allocs | Bytes | Time | Notes |
|-----------|-----|--------|-------|------|-------|
| Tx Decode | Shelley | 73 | 10.9 KB | ~41us | |
| Tx Decode | Allegra | 179 | 26.8 KB | ~98us | |
| Tx Decode | Mary | 340 | 46.3 KB | ~183us | |
| Tx Decode | Alonzo | 332 | 116.6 KB | ~304us | |
| Tx Decode | Babbage | 241 | 42.9 KB | ~177us | |
| Tx Decode | Conway | 115 | 25.8 KB | ~89us | |
| Tx Hash | All Eras | 4 | 480 B | ~2.3-5.0us | |
| UtxoValidateValueNotConservedUtxo (MultiAsset) | Alonzo | 1080 | 85 KB | ~421us | `ledger/alonzo` |
| ValidateScriptWitnesses (no scripts) | Common | 4166 | 141 KB | ~560us | `ledger/common` |
| ValidateScriptWitnesses (with scripts) | Common | 4169 | 142 KB | ~605us | `ledger/common` |

### Historical pre-E34 consensus / leader election (`internal/bench/`)

These June measurements describe the removed ideal-cutoff implementation, not
finite E34 eligibility. They must not be used as current allocation limits.

| Historical operation | Allocs | Bytes | Time | Notes |
|-----------|--------|-------|------|-------|
| Ideal cutoff preparation | 799-805 | ~135 KB | ~0.87ms | big.Rat arithmetic |
| VrfLeaderValue | 5 | 552 B | ~2.0us | Blake2b hash |
| VRFOutputToInt | 2 | 96 B | ~290ns | big.Int conversion |
| Full leader check | 819-825 | ~137 KB | ~2.3ms | Pre-cutover engine |
| Ideal cutoff comparison | 5 | 592 B | ~2.0us | Pre-cutover engine |
| Full Leader Election Workflow | 824 | 137 KB | ~2.3ms | Pre-cutover engine |

### E34 cutover comparison

Measured on Linux/amd64, Go 1.26.8, AMD Ryzen Threadripper PRO 5975WX,
GOMAXPROCS=64. Values below are medians of five non-race runs with
`-benchmem`. The same compiler and inputs were used before and after the
cutover. E34 fixture-dependent runs used the shared corpus at source revision
`bf44019de46e9bc734e70ebdbbbf6fb7fedb8890`. The current dependency is the upstream
`ouroboros-mock` v0.21.1 release, which preserves those corpus bytes.

The `consensus` preparation and ordinary evaluation tuple is
`500000001/1000000003`, coefficient `1/17`. Boundary rows use the pinned
1/3-stake oracle naturals and their indicated coefficients. A dash means no
corresponding pre-cutover measurement was captured.

| Operation | Mode | Old ns/op | E34 ns/op | Old B/op | E34 B/op | Old allocs/op | E34 allocs/op |
|-----------|------|-----------|-----------|----------|----------|---------------|---------------|
| Uncached preparation | CPraos | 220761 | 85633 | 169211 | 24777 | 1164 | 324 |
| Uncached preparation | TPraos | — | 82921 | — | 24809 | — | 324 |
| Warm public preparation | CPraos | 391.3 | 58.91 | 120 | 8 | 6 | 1 |
| Warm public preparation | TPraos | — | 59.27 | — | 8 | — | 1 |
| Prepared natural, zero | CPraos | — | 1342 | — | 835 | — | 12 |
| Prepared natural, zero | TPraos | — | 1420 | — | 932 | — | 12 |
| Prepared output, zero bytes | CPraos | — | 1934 | — | 1350 | — | 16 |
| Prepared output, zero bytes | TPraos | — | 1560 | — | 1029 | — | 13 |
| Prepared output, `0x5a` bytes | CPraos | — | 1974 | — | 1350 | — | 16 |
| Prepared output, `0x5a` bytes | TPraos | — | 1551 | — | 1028 | — | 13 |
| Pinned natural, `f=1/20` | CPraos | — | 7317 | — | 2362 | — | 35 |
| Pinned natural, `f=1/20` | TPraos | — | 7341 | — | 2459 | — | 35 |
| Pinned raw output, `f=1/20` | TPraos | — | 7580 | — | 2555 | — | 36 |
| Pinned natural, `f=1/100000` | CPraos | — | 3028 | — | 1221 | — | 19 |
| Pinned comparator ABOVE, one iteration (`q=S`, `x=0`) | — | — | 210.5 | — | 144 | — | 3 |
| Pinned comparator BELOW, one iteration (`q=S-1`, `x=0`) | — | — | 226.0 | — | 144 | — | 3 |
| Pinned comparator MAX_REACHED, 1000 iterations | — | — | 6758208 | — | 1932701 | — | 2219 |

The MAX_REACHED comparator fixture deliberately has `x=10^6*S`, outside the
prepared coefficient domain. It measures the fixed-budget slow path, not an
ordinary pool decision. No CPraos boundary natural is passed off as raw VRF
bytes; its raw-output benchmark independently hashes actual output bytes.

The internal comparison uses the original stake tuple
`10000000000/100000000000`, coefficient `1/20`, original deterministic key,
nonce and base slot. Schedule and inbound leadership rows are new workloads:
the complete shared 120-slot interval, or its legitimate signed proof/output
samples, in each mode. Parallel header leadership excludes KES/opcert checks.

| Operation | Mode | Old ns/op | E34 ns/op | Old B/op | E34 B/op | Old allocs/op | E34 allocs/op |
|-----------|------|-----------|-----------|----------|----------|---------------|---------------|
| Warm preparation | CPraos | 380.9 | 56.36 | 120 | 8 | 6 | 1 |
| Warm preparation | TPraos | 395.8 | 57.25 | 152 | 8 | 6 | 1 |
| Prepared raw-output decision | CPraos | 476.6 | 1897 | 512 | 1350 | 4 | 16 |
| Prepared raw-output decision | TPraos | — | 1450 | — | 1028 | — | 13 |
| Signing/election with prepared old cutoff | CPraos | 170759 | 189495 | 1793 | 2580 | 23 | 31 |
| Full signing/election | TPraos | — | 189000 | — | 2292 | — | 29 |
| Full reference schedule, 120 slots | CPraos | — | 22472843 | — | 301034 | — | 3482 |
| Full reference schedule, 120 slots | TPraos | — | 23512833 | — | 266321 | — | 3242 |
| Parallel verified-header leadership | CPraos | — | 9890 | — | 2596 | — | 29 |
| Parallel verified-header leadership | TPraos | — | 9668 | — | 2305 | — | 27 |
| Signing-only throwaway control, original CPraos inputs | CPraos | — | 204616 | — | 1152 | — | 13 |

Preparation improves materially: finite E34 uses less arithmetic than the
removed high-precision ideal computation, and validated integer cache keys
avoid rational-string construction and mutable-result copies. Decisions do
more work than the old single integer comparison: they form the rounded
quotient and run the bounded E34 series. Local scratch is reused; accumulator
and divisor updates do not copy redundant next-state integers.

The old 170759 ns/op signing/election sample reused a prepared threshold and
excluded its preparation cost. It is not the historical 824-allocation full
workflow above, which prepared the ideal cutoff on every iteration. The E34
sample includes a warm preparation lookup.

The prepared decision's increased work and allocations are directly measured
without signing. Full election also returns its proof/output result rather
than the old benchmark's boolean-only sink. The signing-only control is
slower than the full workflow in this shared-host sample; do not attribute
the entire historical workflow wall-time difference to eligibility.
Concurrent Dingo race gates and ordinary host activity were not isolated.
These are measured baselines, not guessed latency or allocation limits.

The obsolete 6/10-allocation cutoff assertions and old leader-check budget
were replaced by finite-E34 guards: warm lookup plus decision allows 22
allocations (measured 17 normally and 19–20 with race instrumentation), and cached preparation allows 2 (measured 1).
Unrelated VRF, KES, CBOR and hash guards remain.

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

# Ledger rule benchmarks (value balance, script witnesses)
go test -bench=. -benchmem ./ledger/... -run=^$ 2>&1 | tee ledger_bench.txt
```

### Compare Against Previous Run

```bash
# Compare old vs new with the repository-pinned tool
go tool -modfile=tools/go.mod benchstat old_bench.txt new_bench.txt
```

### Generate Memory Profile

```bash
# CPU profile
go test -bench=BenchmarkBlockValidation -cpuprofile=cpu.prof ./internal/bench/...

# Memory profile
go test -bench=BenchmarkBlockValidation -memprofile=mem.prof ./internal/bench/...

# Analyze
go tool pprof -http=localhost:8080 mem.prof
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
| Threshold Calc | ~2000 allocs | ~800 allocs | ~60% |

---

## Regression Thresholds

### Aspirational Regression Thresholds

These thresholds guide manual review and future CI enforcement.
Currently, regression detection is handled by `TestAllocationRegression`
tests in `regression_test.go`, not by the benchmark CI workflow.

| Metric | Threshold | Rationale |
|--------|-----------|-----------|
| Allocation Count | +20% | Catches allocation leaks |
| Bytes Allocated | +50% | Allows some flexibility for features |
| Time (ns/op) | +20% | Catches performance regressions |

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
