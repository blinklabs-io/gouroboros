# Consensus Package

This package provides Cardano consensus primitives for Ouroboros Praos, enabling block production and chain selection in pure Go.

## Why This Package Exists

Cardano nodes require consensus algorithms to:

1. **Leader Election**: Determine when a stake pool is eligible to produce a block using VRF-based lottery
2. **Block Construction**: Build valid block headers with VRF proofs, KES signatures, and operational certificates
3. **Chain Selection**: Choose the preferred chain among competing forks using Praos rules

This package provides these capabilities without external dependencies, enabling Go-native Cardano node implementations.

## Components

### Leader Election (`leader.go`, `eligibility.go`, `e34.go`)

- `NewLeaderEligibility(poolStake, totalStake, f, mode)` prepares one immutable
  evaluator for repeated decisions.
- `CheckCertifiedNatural(v)` evaluates a natural in `[0, M]`.
- `CheckVRFOutput(output)` evaluates exactly 64 raw VRF output bytes, without
  verifying a proof.
- `IsSlotLeader{,WithMode}()` signs a slot input and applies pool policy.
- `FindNextSlotLeadership{,Context}()` searches an inclusive CPraos slot range
  with one preparation and cancellation before preparation and each slot.

The executable rule is the pinned Haskell ledger's finite E34 predicate, not
an ideal real-number cutoff. Values are signed integers scaled by `10^34`;
every multiplication and division floors. Preparation converts `f` to E34
before subtracting from one, preserves the finite active-slot logarithm, and
floors the stake/log multiplication before negation. Only `BELOW` from the
bounded exponential comparator elects. `ABOVE` and exhaustion of its fixed
1000-iteration budget both return false, without a fallback or margin.

| Mode | Certified natural | Maximum `M` |
|------|-------------------|-------------|
| CPraos (Babbage+) | Domain-separated BLAKE2b-256 leader hash | `2^256` |
| TPraos (Shelley–Alonzo) | Unsigned big-endian raw output | `2^512` |

`CheckCertifiedNatural` also accepts the artificial oracle endpoint `v=M`.
Its quotient is `M*10^34`; this is arithmetic support only, never an N-bit
serialization. Representable naturals instead use
`floor(M*10^34/(M-v))`.

The coefficient must be in `[1/10^19, 1]`, with reduced numerator and
denominator fitting `uint64`. Exact rationals such as `1/3` are valid; large
denominators and smaller positive coefficients are rejected, not rounded.
The constructor rejects zero total stake and clamps pool stake above a
positive total as existing gOuroboros policy, not a node-validation claim.
Exact `f=1` accepts every valid low-level natural, including zero pool stake.
Receiver, natural-range and raw-output-length checks still precede acceptance.

Preparation retains no caller-owned rational or mutable scratch. A bounded
4096-entry mutex-protected LRU caches successful private states by mode,
clamped stake tuple and exact reduced coefficient. Computation occurs outside
the lock; concurrent misses may duplicate it. Every constructor call returns
a fresh wrapper, so overwriting one cannot corrupt another or the cache.
Concurrent evaluator reads are supported; concurrent mutation of a caller's
rational during construction or of the same wrapper is not.

The election/component adapters deliberately differ from the primitive:
zero pool or total stake returns false. Full election validates mode, signer,
nonce, a nonnil coefficient, and the domain of positive coefficients before
zero-stake policy; positive-stake calls with
`f<=0` still generate and validate proof/output, then return false. Components
return false for nil/nonpositive coefficients and malformed output, but reject
unsupported positive coefficients even with zero stake or malformed output.
Unknown modes always error; the no-error component wrapper suppresses errors.
Search returns no leadership without signing for zero stake. With positive
stake and a nonpositive coefficient, it still validates each slot and proof.
Header validation rejects zero total/pool stake and invalid coefficients,
then checks the same prepared predicate on the cryptographically verified
output. These are library pool-policy gates, not additional Haskell claims.

Independent shared fixtures under `ouroboros-mock/fixtures/upstream/e34`
record pinned ledger/consensus/package provenance and checksums. Go tests
read the embedded corpus; they neither fetch fixtures nor run Haskell.
The corpus is not an installed-node replay.

### Block Construction (`block.go`)

- `BlockBuilder` - Constructs block headers with proper VRF proofs and KES signatures
- `BuildHeader()` - Creates a complete header for a new block
- Helper types for header body, VRF results, and operational certificates

### Chain Selection (`selection.go`)

- `PraosChainSelector` - Implements Praos chain selection rules:
  1. Ordinary comparison (`Compare`): prefer longer chains (higher block
     number); for equal length, prefer lower VRF output (tiebreaker)
  2. `IsDeepFork(fork, tipBlockNumber)` routes between the two regimes: a
     fork is deep when adopting it would roll back more than k *blocks*
     (k counts blocks, not slots)
  3. Genesis density comparison (`CompareWithDensity` /
     `PreferredWithDensity`): shallow forks use the ordinary comparison,
     while deep forks prefer the chain with more blocks within the genesis
     window (3k/f slots after the fork point) regardless of total length;
     equal density falls through to the ordinary comparison

  Tips that implement the optional `WindowBlockCounter` interface get the
  canonical integer window metric; others fall back to `ChainTip.Density`.

### Block Validation (`validate.go`)

- `HeaderValidator` - Validates block headers for consensus correctness
- Validation checks:
  1. Slot strictly increases from previous block
  2. Block number is previous + 1
  3. Previous hash matches
  4. VRF proof is valid for the claimed slot
  5. Verified VRF output satisfies the finite E34 leader predicate and pool policy
  6. KES period is within valid range
  7. KES signature is valid
- `ValidationError` - Detailed error types with context for debugging

### Network Configurations (`consensus.go`)

Pre-defined configurations for mainnet, preprod, and preview networks including security parameter (k), active slot coefficient (f), and KES parameters.

## Usage

See the package documentation on [pkg.go.dev](https://pkg.go.dev/github.com/blinklabs-io/gouroboros/consensus) for API details.
