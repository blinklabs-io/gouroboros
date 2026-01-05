// Copyright 2025 Blink Labs Software
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

package common

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"golang.org/x/crypto/sha3"
)

// VerifyVKeySignature verifies an ed25519 signature against the provided public key and message.
func VerifyVKeySignature(pubKey, sig, msg []byte) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d", len(pubKey))
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: %d", len(sig))
	}
	if !ed25519.Verify(ed25519.PublicKey(pubKey), msg, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}

// ValidateVKeyWitnesses verifies that the vkey witnesses in the transaction properly
// sign the transaction body (CBOR). It returns a ValidationError-wrapped failure on the first
// invalid signature encountered.
func ValidateVKeyWitnesses(tx Transaction) error {
	w := tx.Witnesses()
	txHash := tx.Hash()
	msg := txHash[:]
	if w != nil {
		for _, vw := range w.Vkey() {
			if err := VerifyVKeySignature(vw.Vkey, vw.Signature, msg); err != nil {
				return NewValidationError(
					ValidationErrorTypeTransaction,
					"invalid vkey signature",
					map[string]any{"err": err.Error()},
					err,
				)
			}
		}
	}
	return nil
}

// computeByronAddressRoot computes the address root for a Byron address
// using the bootstrap witness public key, chain code, and raw attributes bytes.
// Byron address root = blake2b_224(sha3_256(cbor([0, [0, bytes(pubkey || chainCode)], attributes])))
// The attributes must be used as-is (already CBOR-encoded) to preserve the original encoding.
// This matches the Amaru implementation.
func computeByronAddressRoot(
	pubkey []byte,
	chainCode []byte,
	attrBytes []byte,
) (Blake2b224, error) {
	// Validate input sizes to ensure correct CBOR encoding
	// Byron addresses require exactly 32 bytes for both pubkey and chain code
	if len(pubkey) != 32 {
		return Blake2b224{}, fmt.Errorf("invalid Byron pubkey size: expected 32 bytes, got %d", len(pubkey))
	}
	if len(chainCode) != 32 {
		return Blake2b224{}, fmt.Errorf("invalid Byron chain code size: expected 32 bytes, got %d", len(chainCode))
	}

	// Build the CBOR structure: [0, [0, bytes(pubkey || chainCode)], attributes]
	// Byron addresses always use addrType = 0 for public key addresses
	// CBOR prefix: 0x83 (array of 3) 0x00 (int 0) 0x82 (array of 2) 0x00 (int 0) 0x5840 (bytes of 64)
	var buf []byte
	buf = append(buf, 0x83)       // CBOR array of 3 elements
	buf = append(buf, 0x00)       // First element: addrType = 0
	buf = append(buf, 0x82)       // Second element: array of 2
	buf = append(buf, 0x00)       // addrType = 0
	buf = append(buf, 0x58, 0x40) // CBOR bytes of 64 bytes (0x40 = 64)

	// Append pubkey (32 bytes) and chainCode (32 bytes)
	buf = append(buf, pubkey...)
	buf = append(buf, chainCode...)

	// Third element: raw attributes (already CBOR-encoded)
	buf = append(buf, attrBytes...)

	sha3Sum := sha3.Sum256(buf)
	addrHash := Blake2b224Hash(sha3Sum[:])
	return addrHash, nil
}

// ValidateInputVKeyWitnesses ensures that for each key-locked input, a vkey witness
// exists and the corresponding signature is valid for the transaction body.
func ValidateInputVKeyWitnesses(tx Transaction, ls LedgerState) error {
	// Build a map of provided vkey hashes to witnesses
	w := tx.Witnesses()
	provided := make(map[Blake2b224]VkeyWitness)
	if w != nil {
		for _, vw := range w.Vkey() {
			provided[Blake2b224Hash(vw.Vkey)] = vw
		}
	}

	// Collect bootstrap witnesses for Byron address validation
	var bootstrapWitnesses []BootstrapWitness
	if w != nil {
		bootstrapWitnesses = w.Bootstrap()
	}

	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// BadInputsUtxo will handle missing UTxOs elsewhere
			continue
		}
		if utxo.Output == nil {
			// Treat nil Output like missing UTxO
			continue
		}
		addr := utxo.Output.Address()
		payload := addr.PayloadPayload()
		switch p := payload.(type) {
		case AddressPayloadKeyHash:
			h := p.Hash
			// Check for regular vkey witness (Shelley+ addresses)
			_, ok := provided[h]
			if !ok {
				// Check for bootstrap witness (Byron addresses)
				if addr.Type() == AddressTypeByron {
					found := false
					for _, bw := range bootstrapWitnesses {
						// Compute address root using the witness pubkey, chain code, and attributes
						addrRoot, err := computeByronAddressRoot(bw.PublicKey, bw.ChainCode, bw.Attributes)
						if err != nil {
							// Skip malformed witnesses that can't be processed
							// TODO: Consider logging this for debugging
							continue
						}
						if addrRoot == h {
							found = true
							break
						}
					}
					if !found {
						return NewValidationError(
							ValidationErrorTypeTransaction,
							"missing bootstrap witness for Byron input",
							map[string]any{
								"input":   input.String(),
								"keyhash": h.String(),
							},
							nil,
						)
					}
				} else {
					// Non-Byron address without matching vkey witness
					return NewValidationError(
						ValidationErrorTypeTransaction,
						"missing vkey witness for input",
						map[string]any{
							"input":   input.String(),
							"keyhash": h.String(),
						},
						nil,
					)
				}
			}
		default:
			// script-locked inputs are handled elsewhere
		}
	}
	return nil
}

// ValidateBootstrapWitnesses performs a best-effort validation of bootstrap witnesses.
// For current vectors we validate the signature assuming the PublicKey is a raw ed25519 key.
// Note: Bootstrap witnesses sign the transaction hash (tx.Hash()) for Byron-era compatibility.
func ValidateBootstrapWitnesses(tx Transaction) error {
	w := tx.Witnesses()
	if w == nil {
		return nil
	}
	txHash := tx.Hash()
	msg := txHash[:]
	for _, bw := range w.Bootstrap() {
		// Validate sizes first; reject malformed bootstrap witnesses rather
		// than silently ignoring them. This mirrors the strict behavior of
		// `VerifyVKeySignature` for vkey witnesses.
		if len(bw.PublicKey) != ed25519.PublicKeySize {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"invalid bootstrap public key size",
				map[string]any{"size": len(bw.PublicKey)},
				nil,
			)
		}
		if len(bw.Signature) != ed25519.SignatureSize {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"invalid bootstrap signature size",
				map[string]any{"size": len(bw.Signature)},
				nil,
			)
		}
		if !ed25519.Verify(
			ed25519.PublicKey(bw.PublicKey),
			msg,
			bw.Signature,
		) {
			return NewValidationError(
				ValidationErrorTypeTransaction,
				"invalid bootstrap signature",
				nil,
				nil,
			)
		}
	}
	return nil
}

// UtxoValidateSignatures verifies vkey and bootstrap signatures present in the transaction.
// Parameters slot and pp are unused but included for interface compatibility with UtxoValidationRuleFunc.
// Note: ValidateVKeyWitnesses must be called before ValidateInputVKeyWitnesses since the latter
// only checks witness presence (not cryptographic validity) and relies on the former for validation.
func UtxoValidateSignatures(
	tx Transaction,
	slot uint64,
	ls LedgerState,
	pp ProtocolParameters,
) error {
	if err := ValidateVKeyWitnesses(tx); err != nil {
		return err
	}
	if err := ValidateBootstrapWitnesses(tx); err != nil {
		return err
	}
	if err := ValidateInputVKeyWitnesses(tx, ls); err != nil {
		return err
	}
	return nil
}
