# Profiling Guide

This guide covers how to profile Gouroboros for CPU usage, memory allocations, and performance optimization.

## Prerequisites

```bash
# Install benchstat for benchmark comparison
go install golang.org/x/perf/cmd/benchstat@latest

# pprof is included with Go
go tool pprof --help
```

## Quick Start

```bash
# Generate CPU profile
go test -bench=BenchmarkBlockValidation -cpuprofile=cpu.prof ./internal/bench/...

# Generate memory profile
go test -bench=BenchmarkBlockValidation -memprofile=mem.prof ./internal/bench/...

# Interactive web UI (recommended)
go tool pprof -http=localhost:8080 cpu.prof
```

## Generating Profiles

### CPU Profiling

CPU profiles identify hot code paths and functions consuming the most CPU time.

```bash
# Profile specific benchmark
go test -bench=BenchmarkConsensusIsSlotLeader -cpuprofile=cpu.prof ./internal/bench/...

# Profile with longer runtime for more samples
go test -bench=BenchmarkConsensusIsSlotLeader -benchtime=10s -cpuprofile=cpu.prof ./internal/bench/...

# Profile all benchmarks
go test -bench=. -cpuprofile=cpu.prof ./internal/bench/...
```

### Memory Profiling

Memory profiles show allocation counts and bytes per function.

```bash
# Allocation profile (recommended)
go test -bench=BenchmarkConsensusThreshold -memprofile=mem.prof ./internal/bench/...

# Also capture allocation rate
go test -bench=BenchmarkConsensusThreshold -memprofile=mem.prof -memprofilerate=1 ./internal/bench/...
```

### Block Profile

Block profiles show where goroutines block waiting on synchronization.

```bash
go test -bench=BenchmarkBlockPipeline -blockprofile=block.prof ./pipeline/...
```

### Mutex Profile

Mutex profiles show contention on sync primitives.

```bash
go test -bench=BenchmarkParallelVerify -mutexprofile=mutex.prof ./vrf/...
```

### Trace

Execution traces provide detailed timing for goroutines, GC, and syscalls.

```bash
go test -bench=BenchmarkBlockPipeline -trace=trace.out ./pipeline/...

# View trace
go tool trace trace.out
```

## Analyzing Profiles

### Interactive Web UI

The web UI is the most powerful way to explore profiles:

```bash
go tool pprof -http=localhost:8080 cpu.prof
```

Key views:
- **Graph**: Call graph with node sizes proportional to time/memory
- **Flame Graph**: Hierarchical view of call stacks
- **Top**: Functions sorted by resource usage
- **Source**: Annotated source code

### Command Line

For quick analysis or scripting:

```bash
# Top 20 functions by CPU time
go tool pprof -top cpu.prof

# Top 20 by cumulative time
go tool pprof -top -cum cpu.prof

# Show specific function
go tool pprof -focus=CertifiedNatThreshold cpu.prof

# Memory allocations (count)
go tool pprof -alloc_objects mem.prof

# Memory allocations (bytes)
go tool pprof -alloc_space mem.prof

# In-use memory (live objects)
go tool pprof -inuse_objects mem.prof
```

### Interactive Mode

```bash
go tool pprof cpu.prof

# In the pprof prompt:
(pprof) top20              # Top 20 functions
(pprof) top20 -cum         # By cumulative time
(pprof) list functionName  # Show source with annotations
(pprof) web                # Open graph in browser
(pprof) weblist funcName   # Open annotated source in browser
```

## Key Profiling Scenarios

### Finding CPU Hotspots

```bash
# Generate profile
go test -bench=BenchmarkVerifyBlock -cpuprofile=cpu.prof ./ledger/...

# Open web UI
go tool pprof -http=localhost:8080 cpu.prof
```

Look for:
- Wide nodes in the flame graph (high self time)
- Deep stacks (expensive call chains)
- Unexpected functions in hot paths

### Finding Memory Allocations

```bash
# Generate profile
go test -bench=BenchmarkConsensusThreshold -memprofile=mem.prof ./internal/bench/...

# View allocations
go tool pprof -alloc_objects -http=localhost:8080 mem.prof
```

Look for:
- High allocation counts per function
- Large allocations in loops
- Escaped variables (heap vs stack)

### Investigating Allocation Escape

```bash
# See escape analysis decisions
go build -gcflags='-m -m' ./consensus/... 2>&1 | grep -E '(escapes|does not escape)'

# More verbose
go build -gcflags='-m=2' ./consensus/... 2>&1 | head -100
```

Common escape causes:
- Returning pointers to local variables
- Storing pointers in interfaces
- Closures capturing variables
- Slices/maps that grow

### Comparing Before/After

```bash
# Baseline
go test -bench=BenchmarkConsensusThreshold -memprofile=baseline.prof ./internal/bench/...

# After optimization
go test -bench=BenchmarkConsensusThreshold -memprofile=optimized.prof ./internal/bench/...

# Compare (requires go 1.21+)
go tool pprof -base baseline.prof optimized.prof
(pprof) top -diff
```

## Common Optimization Patterns

Based on performance work in Gouroboros, these patterns have proven effective:

### 1. Object Reuse with sync.Pool

Effective for frequently allocated/deallocated objects:

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 4096)
    },
}

func process() {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf)
    // Use buf...
}
```

**When to use**: High allocation rate, short-lived objects, consistent size.

**Caution**: Don't use for objects that vary significantly in size.

### 2. Pre-allocated Slices

Avoid repeated slice growth:

```go
// Before (multiple allocations)
var result []Item
for _, x := range inputs {
    result = append(result, process(x))
}

// After (single allocation)
result := make([]Item, 0, len(inputs))
for _, x := range inputs {
    result = append(result, process(x))
}
```

### 3. Fixed-Size Buffers

For predictable-size operations:

```go
// Before (heap allocation)
buf := make([]byte, 32)

// After (stack allocation - may be optimized by compiler)
var buf [32]byte
```

### 4. big.Int/big.Rat Reuse

Mathematical operations create many intermediates:

```go
// Before (new allocation each call)
func threshold(a, b *big.Int) *big.Int {
    result := new(big.Int)
    return result.Mul(a, b)
}

// After (reuse provided buffer)
func threshold(a, b, result *big.Int) *big.Int {
    return result.Mul(a, b)
}
```

### 5. CBOR Mode Caching

CBOR encoding/decoding modes are expensive to create:

```go
// Before (created per call)
func encode(v interface{}) ([]byte, error) {
    em, _ := cbor.EncOptions{}.EncMode()
    return em.Marshal(v)
}

// After (cached)
var encMode cbor.EncMode
var encModeOnce sync.Once

func encode(v interface{}) ([]byte, error) {
    encModeOnce.Do(func() {
        encMode, _ = cbor.EncOptions{}.EncMode()
    })
    return encMode.Marshal(v)
}
```

### 6. Avoiding Interface Conversions

Interface boxing causes allocations:

```go
// Before (allocates for interface{})
func process(v interface{}) { ... }
process(myInt)

// After (generic, no allocation)
func process[T any](v T) { ... }
process(myInt)
```

## Profiling in Production

### HTTP Endpoint

Add pprof HTTP handlers:

```go
import _ "net/http/pprof"

func main() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    // ... rest of application
}
```

Access profiles:
- `http://localhost:6060/debug/pprof/` - Index
- `http://localhost:6060/debug/pprof/profile?seconds=30` - CPU
- `http://localhost:6060/debug/pprof/heap` - Memory
- `http://localhost:6060/debug/pprof/goroutine` - Goroutines

### Continuous Profiling

For production monitoring, consider:
- [Pyroscope](https://pyroscope.io/)
- [Parca](https://www.parca.dev/)
- [Google Cloud Profiler](https://cloud.google.com/profiler)

## Gouroboros-Specific Notes

### Key Validation Paths

Priority paths for profiling (highest impact):

| Path | Package | Entry Point |
|------|---------|-------------|
| Block validation | `ledger/` | `VerifyBlock()` |
| Transaction validation | `ledger/*/rules.go` | `VerifyTransaction()` |
| VRF verification | `vrf/` | `Verify()`, `VerifyAndHash()` |
| KES verification | `kes/` | `VerifySignedKES()` |
| Leader election | `consensus/` | `IsSlotLeader()`, `CertifiedNatThreshold()` |
| CBOR decode | `cbor/` | `Decode()`, `NewBlockFromCbor()` |

### Historical Optimizations

Merged optimization branches and their impact:

| Optimization | PR | Reduction |
|--------------|-----|-----------|
| big.Rat reuse | #1503 | ~40 allocs -> ~10 |
| Fixed nonce buffers | #1500 | 3 allocs -> 0 |
| Plutus context prealloc | #1501 | Variable |
| Block body prealloc | #1498 | Variable |
| VRF leader value | #1502 | ~5 allocs -> 1 |
| CBOR mode caching | #1529 | 46-49% faster |

### Benchmark Comparison

Always compare against baseline before optimization:

```bash
# Establish baseline
git checkout main
go test -bench=. -benchmem -count=5 ./internal/bench/... | tee baseline.txt

# Test optimization
git checkout my-optimization-branch
go test -bench=. -benchmem -count=5 ./internal/bench/... | tee optimized.txt

# Compare
benchstat baseline.txt optimized.txt
```

## Resources

### Go Documentation
- [Diagnostics](https://go.dev/doc/diagnostics)
- [pprof](https://pkg.go.dev/net/http/pprof)
- [runtime/pprof](https://pkg.go.dev/runtime/pprof)
- [Profiling Go Programs (blog)](https://go.dev/blog/pprof)

### Tools
- [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- [go-torch](https://github.com/uber-archive/go-torch) (flame graphs, older tool)
- [pprof](https://github.com/google/pprof) (standalone)

### Related Project Documentation
- `internal/bench/README.md` - Benchmark suite documentation
- `internal/bench/BASELINES.md` - Memory allocation baselines
