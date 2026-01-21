# Consensus Package

This package provides Cardano consensus primitives for Ouroboros Praos, enabling block production and chain selection in pure Go.

## Why This Package Exists

Cardano nodes require consensus algorithms to:

1. **Leader Election**: Determine when a stake pool is eligible to produce a block using VRF-based lottery
2. **Block Construction**: Build valid block headers with VRF proofs, KES signatures, and operational certificates
3. **Chain Selection**: Choose the preferred chain among competing forks using Praos rules

This package provides these capabilities without external dependencies, enabling Go-native Cardano node implementations.

## Components

### Leader Election (`leader.go`, `threshold.go`)

- `IsSlotLeader()` - Check if a pool is eligible to produce a block in a given slot
- `CertifiedNatThreshold()` - Compute the leadership threshold based on stake ratio
- `FindNextSlotLeadership()` - Look ahead for scheduling block production

The threshold formula is: `T = 2^512 * (1 - (1-f)^σ)` where `f` is the active slot coefficient and `σ` is the pool's relative stake.

### Block Construction (`block.go`)

- `BlockBuilder` - Constructs block headers with proper VRF proofs and KES signatures
- `BuildHeader()` - Creates a complete header for a new block
- Helper types for header body, VRF results, and operational certificates

### Chain Selection (`selection.go`)

- `PraosChainSelector` - Implements Praos chain selection rules:
  1. Prefer longer chains (higher block number)
  2. For equal length, prefer lower VRF output (tiebreaker)
  3. For deep forks (>k slots), compare chain density

### Block Validation (`validate.go`)

- `HeaderValidator` - Validates block headers for consensus correctness
- Validation checks:
  1. Slot strictly increases from previous block
  2. Block number is previous + 1
  3. Previous hash matches
  4. VRF proof is valid for the claimed slot
  5. VRF output satisfies leadership threshold
  6. KES period is within valid range
  7. KES signature is valid
- `ValidationError` - Detailed error types with context for debugging

### Network Configurations (`consensus.go`)

Pre-defined configurations for mainnet, preprod, and preview networks including security parameter (k), active slot coefficient (f), and KES parameters.

## Usage

See the package documentation on [pkg.go.dev](https://pkg.go.dev/github.com/blinklabs-io/gouroboros/consensus) for API details.
