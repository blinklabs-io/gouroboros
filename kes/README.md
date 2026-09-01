# KES Package

This package provides a pure Go implementation of MMM Sum-Composition Key-Evolving Signatures (KES) as used in Cardano's Ouroboros Praos consensus protocol.

## Why This Package Exists

Cardano uses Key-Evolving Signatures to provide forward-secure block signing. If a stake pool operator's hot key is compromised, the attacker cannot forge signatures for past time periods. The key evolves periodically, and old key material is securely erased.

This package enables:

1. **Block Production**: Stake pool operators can sign block headers with KES signatures without relying on external C libraries.

2. **Block Validation**: Nodes can verify KES signatures in incoming blocks to ensure authenticity and temporal validity.

3. **Key Management**: Generate KES keypairs and evolve keys to new periods while maintaining forward security.

4. **Pure Go**: No CGO dependencies means simpler builds, easier cross-compilation, and better portability.

## Algorithm

The implementation uses MMM Sum-Composition with depth 6 (Sum6Kes):
- 64 total periods (2^6)
- Signature size: 448 bytes (64 + 6*64)
- Base signature: Ed25519
- Tree hashing: Blake2b-256

Each period lasts `slotsPerKESPeriod` slots (129,600 on mainnet, roughly 36 hours).

## Usage

See the package documentation on [pkg.go.dev](https://pkg.go.dev/github.com/blinklabs-io/gouroboros/kes) for API details.
