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

// ValidateBodyProof recomputes the transaction, delegation, and update proofs
// from the block body and checks them against the header.
//
// The transaction proof is the load-bearing one: it covers the count, a
// merkle root over transaction bodies, and a hash over the witness list, so
// altering, adding, or removing any transaction changes it.
//
// This is a block-local, structural validator: for ssc_proof (index 1) it
// only checks that the header body proof parses as a 4-element array --
// i.e. that a ssc_proof entry is present at all -- and does not parse the
// entry's own shape (proof type, hash count) or check any of its hashes
// (see the NOTE below). It is exposed as the day-to-day validator because
// tx/dlg/upd proofs cover everything a single block can attest to on its
// own. For full validation including the actual ssc_proof hashes,
// accumulate the epoch's blocks into a ByronEpochSscState and call
// ValidateBodyProofWithSscState instead.
func (b *ByronMainBlock) ValidateBodyProof() error {
	proof, err := b.bodyProofArray()
	if err != nil {
		return err
	}
	if err := b.validateTxProof(proof[bodyProofTxIndex]); err != nil {
		return err
	}
	// NOTE: ssc_proof (index 1) is deliberately not checked here beyond the
	// bodyProofArray call above confirming it is present. Its hashes are
	// computed over epoch-wide accumulated state (see ByronEpochSscState's
	// NOTE), not this block's payload in isolation, so no amount of
	// per-block encoding fidelity would make this check block-local. Use
	// ValidateBodyProofWithSscState, which takes that state explicitly, for
	// a real check of ssc_proof.
	//
	// The consequence of skipping it here is that an alteration confined to
	// the SSC payload is not detected by this validator. SSC carries
	// shared-seed material, not transactions, so transaction contents remain
	// fully covered by the tx proof above.
	if err := checkPayloadHash(
		"delegation", proof[bodyProofDlgIndex], b.Body.DlgPayloadCbor(),
	); err != nil {
		return err
	}
	return checkPayloadHash(
		"update", proof[bodyProofUpdIndex], b.Body.UpdPayloadCbor(),
	)
}

// ValidateBodyProofWithSscState performs the same transaction, delegation,
// and update proof checks as ValidateBodyProof, and additionally verifies
// the block's ssc_proof against sscState's epoch-accumulated hashes.
//
// Unlike ValidateBodyProof, this is a stateful, epoch-aware validator: the
// caller must have folded every main block of the current epoch, up to and
// including this one, into sscState via ByronEpochSscState.AccumulateBlock,
// in block order, before calling this method. See ByronEpochSscState's NOTE
// for why the SSC proof cannot be checked from a single block alone.
//
// Before checking sscState's hashes, this also confirms the proof's own
// declared SSC type (its first element) matches the discriminant of the
// block's own SscPayload: a header whose ssc_proof structurally claims one
// SSC type (e.g. CertificatesProof) while the body actually carries a
// different payload type (e.g. CommitmentsPayload) is rejected here, rather
// than silently checking hashes against the wrong shape of accumulated
// state.
func (b *ByronMainBlock) ValidateBodyProofWithSscState(
	sscState *ByronEpochSscState,
) error {
	if b == nil || b.BlockHeader == nil {
		return fmt.Errorf(
			"%w: block or block header is nil", ErrBodyProofMismatch,
		)
	}
	if err := b.ValidateBodyProof(); err != nil {
		return err
	}
	if sscState == nil {
		return fmt.Errorf(
			"%w: ssc state is required for full ssc_proof validation",
			ErrBodyProofMismatch,
		)
	}
	proof, err := b.bodyProofArray()
	if err != nil {
		return err
	}
	payloadType, _, err := decodeSscPayloadParts(b.Body.SscPayload)
	if err != nil {
		return fmt.Errorf("%w: ssc payload: %w", ErrBodyProofMismatch, err)
	}
	return sscState.checkProof(proof[bodyProofSscIndex], payloadType)
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
	bodyCbor := b.BodyCbor()
	if len(bodyCbor) == 0 {
		return fmt.Errorf(
			"%w: epoch boundary block has no preserved body CBOR",
			ErrBodyProofMismatch,
		)
	}
	return checkHash(
		"body hash",
		b.BlockHeader.BodyProof,
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
	expectedBytes, ok := expected.([]byte)
	if !ok || len(expectedBytes) != common.Blake2b256Size {
		return fmt.Errorf(
			"%w: %s in header is not a %d-byte hash",
			ErrBodyProofMismatch, label, common.Blake2b256Size,
		)
	}
	if !bytes.Equal(expectedBytes, actual[:]) {
		return fmt.Errorf(
			"%w: %s is %x, computed %x",
			ErrBodyProofMismatch, label, expectedBytes, actual[:],
		)
	}
	return nil
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
