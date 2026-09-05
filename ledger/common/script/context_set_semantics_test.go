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

package script_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// duplicateRequiredSignerTxHex is a transaction body whose required signer set
// (key 14) lists the same key hash twice, spending one input and producing no
// outputs. Preview transaction
// eab27325c569121613728db87a4d8333ce74cc857bbe4991875ceab8787d0213 at slot
// 41098839 does the same and was accepted by the network.
const duplicateRequiredSignerTxHex = "84a4008182582020f9a5a89ed5da223f992427733c6fbe6e44cf4a35f48ead39b1e8366cd92d94010180021a00030d400e82581c910da2e8649cb8b76ad829a233d08e83cade13c551c0c04294fe175a581c910da2e8649cb8b76ad829a233d08e83cade13c551c0c04294fe175aa0f5f6"

const duplicateRequiredSignerHash = "910da2e8649cb8b76ad829a233d08e83cade13c551c0c04294fe175a"

// TestTxInfoSignatoriesDeduplicated pins txInfoSignatories against
// cardano-ledger's transTxBodyReqSignerHashes, which is
// `transKeyHash <$> Set.toList (txBody ^. reqSignerHashesTxBodyG)`. Set.toList
// deduplicates, so a body that repeats a required signer yields one signatory.
// Every era and every Plutus language version routes through that one function,
// so one deduplication covers every version. Emitting the repeat makes a
// validator that walks the list execute extra reductions, which pushes its
// execution units past the producer-declared budget.
//
// Only V1 and V2 are exercised from the wire: Conway encodes reqSignerHashes
// as a tagged set and its decoder rejects a duplicate member outright, so a
// V3 TxInfo cannot be reached with a repeated required signer.
func TestTxInfoSignatoriesDeduplicated(t *testing.T) {
	txBytes, err := hex.DecodeString(duplicateRequiredSignerTxHex)
	if err != nil {
		t.Fatalf("decode transaction hex: %v", err)
	}
	expected, err := hex.DecodeString(duplicateRequiredSignerHash)
	if err != nil {
		t.Fatalf("decode signer hash: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		build func() ([]lcommon.Blake2b224, error)
	}{
		{
			name: "v1",
			build: func() ([]lcommon.Blake2b224, error) {
				tx, err := alonzo.NewAlonzoTransactionFromCbor(txBytes)
				if err != nil {
					return nil, err
				}
				info, err := script.NewTxInfoV1FromTransaction(
					preprodSlotState,
					tx,
					nil,
					false,
				)
				if err != nil {
					return nil, err
				}
				return info.Signatories, nil
			},
		},
		{
			name: "v2",
			build: func() ([]lcommon.Blake2b224, error) {
				tx, err := babbage.NewBabbageTransactionFromCbor(txBytes)
				if err != nil {
					return nil, err
				}
				info, err := script.NewTxInfoV2FromTransaction(
					preprodSlotState,
					tx,
					nil,
					false,
				)
				if err != nil {
					return nil, err
				}
				return info.Signatories, nil
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			signatories, err := testCase.build()
			if err != nil {
				t.Fatalf("build TxInfo: %v", err)
			}
			if len(signatories) != 1 {
				t.Fatalf(
					"expected 1 signatory, got %d",
					len(signatories),
				)
			}
			if hex.EncodeToString(signatories[0].Bytes()) !=
				hex.EncodeToString(expected) {
				t.Fatalf(
					"expected signatory %x, got %x",
					expected,
					signatories[0].Bytes(),
				)
			}
		})
	}
}

// TestSortInputsDeduplicates pins SortInputs against cardano-ledger, which
// holds inputsTxBodyL and referenceInputsTxBodyL as Set TxIn and renders them
// with Set.toList. A repeated input must therefore contribute one entry and
// must not shift the redeemer index of any later spend.
func TestSortInputsDeduplicates(t *testing.T) {
	const (
		idA = "0000000000000000000000000000000000000000000000000000000000000001"
		idB = "0000000000000000000000000000000000000000000000000000000000000002"
	)
	inputA0 := shelley.NewShelleyTransactionInput(idA, 0)
	inputA1 := shelley.NewShelleyTransactionInput(idA, 1)
	inputB0 := shelley.NewShelleyTransactionInput(idB, 0)
	sorted := script.SortInputs([]lcommon.TransactionInput{
		&inputB0,
		&inputA1,
		&inputA0,
		&inputB0,
		&inputA1,
	})
	expected := []string{
		inputA0.String(),
		inputA1.String(),
		inputB0.String(),
	}
	if len(sorted) != len(expected) {
		t.Fatalf(
			"expected %d inputs, got %d: %v",
			len(expected),
			len(sorted),
			sorted,
		)
	}
	for i, want := range expected {
		if sorted[i].String() != want {
			t.Fatalf(
				"input %d: expected %s, got %s",
				i,
				want,
				sorted[i].String(),
			)
		}
	}
}
