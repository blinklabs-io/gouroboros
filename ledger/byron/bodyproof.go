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
	"bytes"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ErrBodyProofMismatch reports a Byron block whose body does not match the
// proof carried in its header. The header, and therefore the block hash, can
// be genuine while the body has been substituted, so this is the only check
// that binds the two together.
var ErrBodyProofMismatch = errors.New("byron block body proof mismatch")

// ErrMalformedBodyProof reports a Byron header whose body proof does not
// have the wire shape the format requires, so no body hash can be read out
// of it at all. This is distinct from ErrBodyProofMismatch, which reports a
// well-formed proof that disagrees with the body.
var ErrMalformedBodyProof = errors.New("byron block body proof is malformed")

// Indices into the Byron header's body proof, which is
// [tx_proof, ssc_proof, dlg_proof, upd_proof].
const (
	bodyProofTxIndex  = 0
	bodyProofSscIndex = 1
	bodyProofDlgIndex = 2
	bodyProofUpdIndex = 3
	bodyProofLength   = 4
)

// Indices into the Byron tx proof, which is
// [tx_count, tx_merkle_root, witnesses_hash].
const (
	txProofCountIndex     = 0
	txProofMerkleIndex    = 1
	txProofWitnessesIndex = 2
	txProofLength         = 3
)

// ValidateBodyProof recomputes the transaction, delegation, update, and SSC
// proofs from the block body and checks them against the header.
//
// The transaction proof is the load-bearing one: it covers the count, a
// merkle root over transaction bodies, and a hash over the witness list, so
// altering, adding, or removing any transaction changes it. tx_proof,
// dlg_proof, and upd_proof are always checked by full hash comparison: real
// mainnet blocks going through cardano-node's own decoder for years is a
// safety net for those three fields being both correct and safe to enforce
// unconditionally.
//
// ssc_proof does not have that same safety net (see
// common.VerifyConfig.EnableByronSscProofHashValidation's doc comment and
// checkSscProofCore's for the full reasoning), so by default this function
// checks ssc_proof only structurally -- its declared type, element counts,
// and the wire shape of every field it hashes, via ValidateSscProofShape --
// without comparing any of its hash values against the header. Pass a
// common.VerifyConfig with EnableByronSscProofHashValidation set to true to
// additionally run the full hash comparison (ValidateSscProof) as part of
// this call; NewByronMainBlockFromCbor forwards whatever VerifyConfig it
// was given here, so that same flag controls decode-time behavior too.
//
// The dlg_proof and upd_proof comparisons bind the delegation and update
// payload bytes to the header but say nothing about what those bytes
// decode to. Pass a common.VerifyConfig with EnableByronPayloadValidation
// set to true to additionally validate those payloads structurally and
// verify the signatures they carry, via ValidatePayloads; see that flag's
// doc comment for why it is opt-in.
//
// This was not always the default: an earlier version of this function
// unconditionally ran the full ssc_proof hash comparison, after an even
// earlier version had deliberately skipped ssc_proof's hashes entirely,
// believing that they depended on epoch-wide accumulated state -- real,
// non-empty mainnet vectors disproved that belief, but the unconditional
// hash comparison this replaced was reverted in favor of the opt-in scheme
// here specifically because ssc_proof's construction has no upstream
// reference oracle: see checkSscProofLocal's doc comment (sscstate.go) for
// the vectors and reasoning, and checkSscProofCore's for why the hash
// comparison itself, not the structural check, is what moved behind the
// opt-in flag.
func (b *ByronMainBlock) ValidateBodyProof(
	config ...common.VerifyConfig,
) error {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	proof, err := b.bodyProofArray()
	if err != nil {
		return err
	}
	if err := b.validateTxProof(proof[bodyProofTxIndex]); err != nil {
		return err
	}
	if cfg.EnableByronSscProofHashValidation {
		if err := b.ValidateSscProof(); err != nil {
			return err
		}
	} else if err := b.ValidateSscProofShape(); err != nil {
		return err
	}
	if err := checkPayloadHash(
		"delegation", proof[bodyProofDlgIndex], b.Body.DlgPayloadCbor(),
	); err != nil {
		return err
	}
	if err := checkPayloadHash(
		"update", proof[bodyProofUpdIndex], b.Body.UpdPayloadCbor(),
	); err != nil {
		return err
	}
	if cfg.EnableByronPayloadValidation {
		return b.ValidatePayloads()
	}
	return nil
}

// ValidateSscProof validates only a Byron main block's ssc_proof, entirely
// from that block's own payload, including a full comparison of every hash
// it carries against the header (see checkSscProofLocal's doc comment).
//
// This is the opt-in, full-hash form: ValidateBodyProof does not call this
// by default (see its own doc comment and
// common.VerifyConfig.EnableByronSscProofHashValidation) -- it is exposed
// separately for callers that specifically want the full check, such as
// consensus/byron's ValidateBodyHash when given that same opt-in flag, or
// a caller that has already validated tx_proof/dlg_proof/upd_proof through
// some other, independently implemented pipeline and wants to add a real
// ssc_proof check without paying for a second, redundant pass over the
// transaction merkle root and the other body components ValidateBodyProof
// would otherwise repeat. See ValidateSscProofShape for the structural-only
// check ValidateBodyProof runs by default instead.
func (b *ByronMainBlock) ValidateSscProof() error {
	return b.checkSscProof(true)
}

// ValidateSscProofShape validates only a Byron main block's ssc_proof
// structurally -- its declared type, element counts, and the wire shape of
// every field it would hash -- without comparing any hash value against
// the header. This is what ValidateBodyProof runs by default; see its doc
// comment, checkSscProofCore's, and ValidateSscProof for the opt-in form
// that additionally compares hash values.
func (b *ByronMainBlock) ValidateSscProofShape() error {
	return b.checkSscProof(false)
}

// checkSscProof implements both ValidateSscProof (verifyHashes=true) and
// ValidateSscProofShape (verifyHashes=false).
func (b *ByronMainBlock) checkSscProof(verifyHashes bool) error {
	if b == nil || b.BlockHeader == nil {
		return fmt.Errorf(
			"%w: block or block header is nil", ErrBodyProofMismatch,
		)
	}
	proof, err := b.bodyProofArray()
	if err != nil {
		return err
	}
	payloadType, rest, err := decodeSscPayloadParts(b.Body.SscPayload)
	if err != nil {
		return fmt.Errorf("%w: ssc payload: %w", ErrBodyProofMismatch, err)
	}
	if verifyHashes {
		return checkSscProofLocal(proof[bodyProofSscIndex], payloadType, rest)
	}
	return checkSscProofShape(proof[bodyProofSscIndex], payloadType, rest)
}

// bodyProofArray returns the header's body proof as a validated array,
// guarding against a nil receiver or nil BlockHeader.
func (b *ByronMainBlock) bodyProofArray() ([]any, error) {
	if b == nil || b.BlockHeader == nil {
		return nil, fmt.Errorf(
			"%w: block or block header is nil", ErrBodyProofMismatch,
		)
	}
	proof, ok := b.BlockHeader.BodyProof.([]any)
	if !ok || len(proof) < bodyProofLength {
		return nil, fmt.Errorf(
			"%w: header body proof is not a %d-element array",
			ErrBodyProofMismatch, bodyProofLength,
		)
	}
	return proof, nil
}

func (b *ByronMainBlock) validateTxProof(rawProof any) error {
	txProof, ok := rawProof.([]any)
	if !ok || len(txProof) < txProofLength {
		return fmt.Errorf(
			"%w: tx proof is not a %d-element array",
			ErrBodyProofMismatch, txProofLength,
		)
	}

	expectedCount, err := asUint(txProof[txProofCountIndex])
	if err != nil {
		return fmt.Errorf("%w: tx count: %w", ErrBodyProofMismatch, err)
	}
	actualCount := uint64(len(b.Body.TxPayload))
	if expectedCount != actualCount {
		return fmt.Errorf(
			"%w: header declares %d transactions, body carries %d",
			ErrBodyProofMismatch, expectedCount, actualCount,
		)
	}

	// Merkle leaves are the transaction bodies; witnesses are covered
	// separately by a hash over the encoded witness list.
	bodies := make([][]byte, 0, len(b.Body.TxPayload))
	witnesses := make([][]byte, 0, len(b.Body.TxPayload))
	for i, tx := range b.Body.TxPayload {
		body := tx.Body.Cbor()
		if len(body) == 0 {
			return fmt.Errorf(
				"%w: transaction %d has no preserved body CBOR",
				ErrBodyProofMismatch, i,
			)
		}
		bodies = append(bodies, body)
		witness := tx.WitnessesCbor()
		if len(witness) == 0 {
			return fmt.Errorf(
				"%w: transaction %d has no preserved witness CBOR",
				ErrBodyProofMismatch, i,
			)
		}
		witnesses = append(witnesses, witness)
	}

	if err := checkHash(
		"tx merkle root", txProof[txProofMerkleIndex], MerkleRoot(bodies),
	); err != nil {
		return err
	}

	return checkHash(
		"witnesses hash",
		txProof[txProofWitnessesIndex],
		common.Blake2b256Hash(encodeWitnessList(witnesses)),
	)
}

// encodeWitnessList frames the preserved per-transaction witness CBOR as the
// indefinite-length array Byron hashes.
//
// The array is assembled by hand rather than through the encoder because the
// element bytes must survive untouched, and because the length must stay
// indefinite: a definite-length array over the same elements hashes
// differently and would reject every real block.
func encodeWitnessList(witnesses [][]byte) []byte {
	const (
		indefiniteArrayStart byte = 0x9f
		indefiniteBreak      byte = 0xff
	)
	size := 2
	for _, w := range witnesses {
		size += len(w)
	}
	encoded := make([]byte, 0, size)
	encoded = append(encoded, indefiniteArrayStart)
	for _, w := range witnesses {
		encoded = append(encoded, w...)
	}
	return append(encoded, indefiniteBreak)
}

// ValidateBodyProof checks an epoch boundary block against the hash carried in
// its header. EBBs have no transactions, so the whole body is covered by a
// single hash.
func (b *ByronEpochBoundaryBlock) ValidateBodyProof() error {
	// Read the header hash through the checked accessor so a body proof
	// that is not a hash at all is reported as malformed, rather than
	// reaching checkHash's comparison and being reported as a mismatch
	// against a value the header never carried.
	expected, err := b.BlockBodyHashChecked()
	if err != nil {
		return err
	}
	bodyCbor := b.BodyCbor()
	if len(bodyCbor) == 0 {
		return fmt.Errorf(
			"%w: epoch boundary block has no preserved body CBOR",
			ErrBodyProofMismatch,
		)
	}
	return checkHash(
		"body hash",
		expected[:],
		common.Blake2b256Hash(bodyCbor),
	)
}

// checkPayloadHash compares a proof entry against the hash of the payload's
// original CBOR.
func checkPayloadHash(label string, expected any, payload []byte) error {
	if len(payload) == 0 {
		return fmt.Errorf(
			"%w: %s payload has no preserved CBOR",
			ErrBodyProofMismatch, label,
		)
	}
	return checkHash(label, expected, common.Blake2b256Hash(payload))
}

// checkHash compares a proof entry, which decodes as raw bytes, against a
// locally computed hash.
func checkHash(
	label string,
	expected any,
	actual common.Blake2b256,
) error {
	expectedBytes, err := checkHashShapeBytes(label, expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedBytes, actual[:]) {
		return fmt.Errorf(
			"%w: %s is %x, computed %x",
			ErrBodyProofMismatch, label, expectedBytes, actual[:],
		)
	}
	return nil
}

// checkHashShape validates that a proof entry has the wire shape of a
// blake2b-256 hash (32 raw bytes) without asserting anything about its
// value. See checkHashOrShape for why some callers (ssc_proof by default)
// need this shape-only form instead of checkHash's full comparison.
func checkHashShape(label string, expected any) error {
	_, err := checkHashShapeBytes(label, expected)
	return err
}

// checkHashShapeBytes is checkHashShape's implementation, returning the
// validated bytes so checkHash can reuse it instead of duplicating the
// type/length assertion.
func checkHashShapeBytes(label string, expected any) ([]byte, error) {
	expectedBytes, ok := expected.([]byte)
	if !ok || len(expectedBytes) != common.Blake2b256Size {
		return nil, fmt.Errorf(
			"%w: %s in header is not a %d-byte hash",
			ErrBodyProofMismatch, label, common.Blake2b256Size,
		)
	}
	return expectedBytes, nil
}

// checkHashOrShape validates that a proof entry has the shape of a
// blake2b-256 hash and, only when verify is true, additionally compares it
// against a locally computed value. This is what lets checkSscProofCore
// implement both the opt-in, full-hash form (checkSscProofLocal) and the
// always-on, structural-only form (checkSscProofShape) that
// ValidateBodyProof runs by default -- see checkSscProofCore's doc comment
// (sscstate.go) for why ssc_proof's hash comparison specifically is not
// applied unconditionally the way tx_proof/dlg_proof/upd_proof's are.
func checkHashOrShape(
	label string,
	expected any,
	actual common.Blake2b256,
	verify bool,
) error {
	if !verify {
		return checkHashShape(label, expected)
	}
	return checkHash(label, expected, actual)
}

// asUint normalises the integer types the CBOR decoder may produce for a
// count field.
func asUint(v any) (uint64, error) {
	switch n := v.(type) {
	case uint64:
		return n, nil
	case uint:
		return uint64(n), nil
	case int64:
		if n < 0 {
			return 0, fmt.Errorf("negative count %d", n)
		}
		return uint64(n), nil
	case int:
		if n < 0 {
			return 0, fmt.Errorf("negative count %d", n)
		}
		return uint64(n), nil
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}
