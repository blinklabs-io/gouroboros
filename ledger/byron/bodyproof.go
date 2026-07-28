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
func (b *ByronMainBlock) ValidateBodyProof() error {
	proof, ok := b.BlockHeader.BodyProof.([]any)
	if !ok || len(proof) < bodyProofLength {
		return fmt.Errorf(
			"%w: header body proof is not a %d-element array",
			ErrBodyProofMismatch, bodyProofLength,
		)
	}
	if err := b.validateTxProof(proof[bodyProofTxIndex]); err != nil {
		return err
	}
	// NOTE: ssc_proof (index 1) is deliberately not checked. Its hashes are
	// taken over cardano-ledger's own encoding of the SSC sub-payloads, which
	// is not the encoding carried inline in the block: for the mainnet block
	// in internal/testdata the payload part hashes to 25777aca... while the
	// header records d36a2619..., so the two are not the same bytes. Modelling
	// SscPayload well enough to reproduce it is a separate piece of work, and
	// guessing the encoding from the one available fixture -- whose SSC
	// payload is empty -- would give false assurance for the non-empty case.
	//
	// The consequence is that an alteration confined to the SSC payload is not
	// detected here. SSC carries shared-seed material, not transactions, so
	// transaction contents remain fully covered by the tx proof above.
	if err := checkPayloadHash(
		"delegation", proof[bodyProofDlgIndex], b.Body.DlgPayloadCbor(),
	); err != nil {
		return err
	}
	return checkPayloadHash(
		"update", proof[bodyProofUpdIndex], b.Body.UpdPayloadCbor(),
	)
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
