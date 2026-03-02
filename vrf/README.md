# VRF Package

This package provides a pure Go implementation of ECVRF-ED25519-SHA512-Elligator2 (IETF draft-irtf-cfrg-vrf-03) for use in Cardano's Ouroboros Praos consensus protocol.

## Why This Package Exists

Cardano's Praos consensus requires Verifiable Random Functions (VRF) for slot leader election. A stake pool operator proves their right to produce a block by generating a VRF proof that, when combined with their stake ratio, demonstrates they "won" the slot lottery.

This package enables:

1. **Block Production**: Stake pool operators can generate VRF proofs for slot leadership without relying on external C libraries like libsodium.

2. **Block Validation**: Nodes can verify VRF proofs in incoming blocks to ensure the block producer was legitimately elected.

3. **Pure Go**: No CGO dependencies means simpler builds, easier cross-compilation, and better portability.

## Algorithm

The implementation follows IETF draft-irtf-cfrg-vrf-03 using:
- Curve: Ed25519 (via `filippo.io/edwards25519`)
- Hash: SHA-512
- Point encoding: Elligator2

## Security Note: SHA-512 Usage

This implementation uses SHA-512 as **required by the cryptographic specifications**:

- **IETF draft-irtf-cfrg-vrf-03** mandates SHA-512 for this VRF suite
- **RFC 8032 Section 5.1.5** requires SHA-512 for Ed25519 scalar derivation
- **Cardano protocol** requires exact algorithm matching for interoperability

SHA-512 is **not being used for password hashing**. It is used for:

1. Deriving Ed25519 secret scalars from 32-byte cryptographic seeds
2. Generating deterministic nonces for Schnorr-style proofs
3. Computing VRF output hashes

Password-specific algorithms (bcrypt, scrypt, argon2) are inappropriate here because:
- They would break protocol compatibility with Cardano
- Ed25519/VRF specifications explicitly require SHA-512
- The input is a cryptographic seed, not a user password

### References

- [IETF draft-irtf-cfrg-vrf-03](https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-vrf-03)
- [RFC 8032 - Ed25519](https://datatracker.ietf.org/doc/html/rfc8032#section-5.1.5)
- [cardano-crypto-praos reference](https://github.com/IntersectMBO/cardano-base/tree/master/cardano-crypto-praos)

## Usage

See the package documentation on [pkg.go.dev](https://pkg.go.dev/github.com/blinklabs-io/gouroboros/vrf) for API details.
