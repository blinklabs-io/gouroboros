// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package byron

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// Byron sign tags, from cardano-crypto's Cardano.Crypto.Signing.Tag. Every
// Byron signature is domain-separated by prefixing the signed buffer with
// its tag byte followed by the CBOR-encoded protocol magic, so a signature
// made for one purpose (or one network) can never be replayed as another.
const (
	SignTagTx             byte = 0x01
	SignTagRedeemTx       byte = 0x02
	SignTagVssCert        byte = 0x03
	SignTagUSProposal     byte = 0x04
	SignTagCommitment     byte = 0x05
	SignTagUSVote         byte = 0x06
	SignTagMainBlock      byte = 0x07
	SignTagMainBlockLight byte = 0x08
	SignTagMainBlockHeavy byte = 0x09
	SignTagCertificate    byte = 0x0a
)

// VerificationKeySize is the size of a Byron extended Ed25519 verification
// key: a 32-byte Ed25519 public key followed by a 32-byte chain code. Only
// the leading 32 bytes take part in signature verification, but the full 64
// bytes are what the wire format carries and what gets hashed and signed
// over, so the distinction matters.
const VerificationKeySize = 64

// ErrInvalidSignature reports a Byron signature that is structurally
// well-formed but does not verify against the key it claims to come from.
var ErrInvalidSignature = errors.New("byron signature verification failed")

// signTag builds the domain-separation prefix for a Byron signature:
// the tag byte followed by the CBOR-encoded protocol magic. This is
// cardano-crypto's `signTag pm tag`.
func signTag(tag byte, protocolMagic uint32) ([]byte, error) {
	magicCbor, err := cbor.Encode(protocolMagic)
	if err != nil {
		return nil, fmt.Errorf("encode protocol magic: %w", err)
	}
	prefix := make([]byte, 0, 1+len(magicCbor))
	prefix = append(prefix, tag)
	return append(prefix, magicCbor...), nil
}

// signedBytes assembles the exact buffer a Byron signature is made over:
// the domain-separation tag, then the already-serialized payload.
//
// Callers pass payload already in its signed-over serialized form, because
// what that form is differs per tag: `safeSign pm tag ss` is
// `safeSignRaw pm (Just tag) ss . serialize'`, so the payload is the CBOR
// encoding of whatever Haskell value was signed. For a ByteString argument
// (delegation certificates) that means the value wrapped in a CBOR
// byte-string header; for a structured record (update proposals, votes)
// it means that record's own CBOR encoding, which must be reproduced from
// preserved bytes rather than re-encoded.
func signedBytes(
	tag byte,
	protocolMagic uint32,
	payload []byte,
) ([]byte, error) {
	prefix, err := signTag(tag, protocolMagic)
	if err != nil {
		return nil, err
	}
	signed := make([]byte, 0, len(prefix)+len(payload))
	signed = append(signed, prefix...)
	return append(signed, payload...), nil
}

// verifyEd25519 checks sig over signed using the Ed25519 half of a Byron
// extended verification key.
func verifyEd25519(verificationKey, signed, sig []byte) bool {
	if len(verificationKey) != VerificationKeySize ||
		len(sig) != ed25519.SignatureSize {
		return false
	}
	// Byron signatures are verified permissively on purpose. The reference is
	// Cardano.Crypto.Signing.Signature.verifySignatureRaw, which calls CC.verify in
	// cardano-crypto-wallet, whose bundled ed25519-donna ed25519_sign_open checks
	// only the high bits of S and performs no small-order test on A or R. It
	// accepts proofs libsodium rejects, and Byron blocks are immutable history, so
	// routing these through internal/ed25519strict would reject chain the node
	// accepts. Do not "fix" these to match the non-Byron boundaries.
	return ed25519.Verify(verificationKey[:32], signed, sig)
}

// EncodeDelegationEpoch returns the CBOR encoding of a delegation
// certificate's epoch for callers whose epoch did not arrive as CBOR in the
// first place -- notably the genesis file's heavyweight delegations, which
// carry it as a JSON number.
//
// Do NOT use this to reconstruct the epoch of a certificate that was
// decoded from CBOR. CBOR admits non-shortest integer encodings and this
// decoder accepts them, so a certificate whose epoch arrived as 0x1807
// decodes to 7 and re-encodes to 0x07 -- different bytes, and the issuer
// signed the ones on the wire. Pass the preserved field encoding to
// VerifyDelegationCertificateSignature instead; ParseDelegationCertificate
// keeps it in DelegationCertificate.EpochCbor for exactly this reason.
func EncodeDelegationEpoch(epoch uint64) ([]byte, error) {
	encoded, err := cbor.Encode(epoch)
	if err != nil {
		return nil, fmt.Errorf("encode delegation epoch: %w", err)
	}
	return encoded, nil
}

// VerifyDelegationCertificateSignature verifies a Byron heavyweight
// delegation certificate signature, reproducing cardano-ledger-byron's
// Cardano.Chain.Delegation.Certificate signing format exactly:
//
//	inner  = "00" || delegateVK || epochCbor
//	signed = 0x0a || CBOR(protocolMagic) || CBOR_bytestring(inner)
//
// "00" is the two ASCII bytes 0x30 0x30, delegateVK is the raw 64-byte
// extended verification key, and the CBOR byte-string wrapping of inner
// comes from safeSign applying serialize' to its ByteString argument.
// certSig is then a plain Ed25519 signature over signed, verifiable with
// the leading 32 bytes of the 64-byte extended issuerVK.
//
// epochCbor must be the epoch field's ORIGINAL wire encoding, not a
// re-encoding of its decoded value: cardano-ledger verifies against the
// annotated bytes, and this decoder accepts non-shortest integer encodings
// that would not survive a round trip. ParseDelegationCertificate preserves
// it; callers whose epoch never was CBOR use EncodeDelegationEpoch.
func VerifyDelegationCertificateSignature(
	protocolMagic uint32,
	issuerVK []byte,
	delegateVK []byte,
	certSig []byte,
	epochCbor []byte,
) error {
	if len(issuerVK) != VerificationKeySize {
		return fmt.Errorf(
			"invalid issuer verification key size: got %d, expected %d",
			len(issuerVK), VerificationKeySize,
		)
	}
	if len(delegateVK) != VerificationKeySize {
		return fmt.Errorf(
			"invalid delegate verification key size: got %d, expected %d",
			len(delegateVK), VerificationKeySize,
		)
	}
	if len(certSig) != ed25519.SignatureSize {
		return fmt.Errorf(
			"invalid certificate signature size: got %d, expected %d",
			len(certSig), ed25519.SignatureSize,
		)
	}
	if len(epochCbor) == 0 {
		return errors.New(
			"delegation certificate epoch encoding is empty",
		)
	}
	inner := make([]byte, 0, 2+len(delegateVK)+len(epochCbor))
	inner = append(inner, '0', '0')
	inner = append(inner, delegateVK...)
	inner = append(inner, epochCbor...)
	innerCbor, err := cbor.Encode(inner)
	if err != nil {
		return fmt.Errorf("encode delegation certificate payload: %w", err)
	}
	signed, err := signedBytes(SignTagCertificate, protocolMagic, innerCbor)
	if err != nil {
		return err
	}
	if !verifyEd25519(issuerVK, signed, certSig) {
		return fmt.Errorf(
			"%w: delegation certificate for epoch %x",
			ErrInvalidSignature, epochCbor,
		)
	}
	return nil
}
