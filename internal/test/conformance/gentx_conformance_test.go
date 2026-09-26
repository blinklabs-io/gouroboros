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

package conformance

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
)

// TestConsensusGenTxFixtures decodes the ouroboros-consensus GenTx and GenTxId
// goldens through the public fixture harness. Fixture selection and era mapping
// use typed metadata rather than paths that mirror upstream's directory layout.
func TestConsensusGenTxFixtures(t *testing.T) {
	harness := fixtures.NewHarness(fixtures.HarnessConfig{})
	allFixtures, err := harness.Collect()
	if err != nil {
		t.Fatalf("failed to collect upstream fixtures: %v", err)
	}

	transactionFilter := fixtures.Filter{
		Repo:   fixtures.RepoOuroborosConsensus,
		Kind:   fixtures.KindTransaction,
		Format: fixtures.FormatCBOR,
	}
	transactionIDFilter := fixtures.Filter{
		Repo:   fixtures.RepoOuroborosConsensus,
		Kind:   fixtures.KindTransactionID,
		Format: fixtures.FormatCBOR,
	}
	transactions := make(map[string]fixtures.Fixture)
	transactionIDs := make(map[string]fixtures.Fixture)
	for _, fixture := range allFixtures {
		switch {
		case transactionFilter.Matches(fixture):
			if _, exists := transactions[fixture.Era]; exists {
				t.Fatalf(
					"duplicate transaction fixture for "+
						"era %q",
					fixture.Era,
				)
			}
			transactions[fixture.Era] = fixture
		case transactionIDFilter.Matches(fixture):
			if _, exists := transactionIDs[fixture.Era]; exists {
				t.Fatalf(
					"duplicate transaction-id fixture for "+
						"era %q",
					fixture.Era,
				)
			}
			transactionIDs[fixture.Era] = fixture
		}
	}

	for _, testCase := range []struct {
		era                   string
		eraID                 uint
		txArrayLen            int
		pairedID              bool
		expectedTransactionID string
		expectedFixtureID     string
	}{
		{
			era:                   "byron",
			eraID:                 ledger.TxTypeByron,
			txArrayLen:            2,
			expectedTransactionID: "b2d85afe1fcf06dae966187c7312b1734d6fd6d2f110bd321301151b05cd4a97",
			expectedFixtureID:     "4ba839c420b3d2bd439530f891cae9a5d4c4d812044630dac72e8e0962feeecc",
		},
		{
			era:        "shelley",
			eraID:      ledger.TxTypeShelley,
			txArrayLen: 3,
			pairedID:   true,
		},
		{
			era:        "allegra",
			eraID:      ledger.TxTypeAllegra,
			txArrayLen: 3,
			pairedID:   true,
		},
		{
			era:        "mary",
			eraID:      ledger.TxTypeMary,
			txArrayLen: 3,
			pairedID:   true,
		},
		{
			era:        "alonzo",
			eraID:      ledger.TxTypeAlonzo,
			txArrayLen: 4,
			pairedID:   true,
		},
		{
			era:        "babbage",
			eraID:      ledger.TxTypeBabbage,
			txArrayLen: 4,
			pairedID:   true,
		},
		{
			era:        "conway",
			eraID:      ledger.TxTypeConway,
			txArrayLen: 4,
			pairedID:   true,
		},
		{
			era:        "dijkstra",
			eraID:      ledger.TxTypeDijkstra,
			txArrayLen: 3,
			pairedID:   true,
		},
	} {
		t.Run(testCase.era, func(t *testing.T) {
			transactionFixture, ok := transactions[testCase.era]
			if !ok {
				t.Fatalf(
					"missing GenTx fixture for era %q",
					testCase.era,
				)
			}
			idFixture, ok := transactionIDs[testCase.era]
			if !ok {
				t.Fatalf(
					"missing GenTxId fixture for era %q",
					testCase.era,
				)
			}

			transactionEnvelope, err := transactionFixture.
				ConsensusEnvelope()
			if err != nil {
				t.Fatalf(
					"failed to decode GenTx envelope: %v",
					err,
				)
			}
			if transactionEnvelope.Era != testCase.eraID {
				t.Fatalf(
					"unexpected GenTx era: got %d want %d",
					transactionEnvelope.Era,
					testCase.eraID,
				)
			}
			txBytes, err := transactionFixture.
				ConsensusTransactionBytes()
			if err != nil {
				t.Fatalf(
					"failed to extract transaction "+
						"bytes: %v",
					err,
				)
			}
			var txArray []cbor.RawMessage
			if _, err := cbor.Decode(
				txBytes,
				&txArray,
			); err != nil {
				t.Fatalf(
					"failed to decode transaction "+
						"array: %v",
					err,
				)
			}
			if len(txArray) != testCase.txArrayLen {
				t.Fatalf(
					"unexpected transaction array width: "+
						"got %d want %d",
					len(txArray),
					testCase.txArrayLen,
				)
			}

			tx, err := transactionFixture.DecodeLedgerTransaction()
			if err != nil {
				t.Fatalf(
					"failed to decode transaction: %v",
					err,
				)
			}
			if tx.Type() != int(testCase.eraID) {
				t.Fatalf(
					"unexpected transaction type: "+
						"got %d want %d",
					tx.Type(),
					testCase.eraID,
				)
			}
			if !bytes.Equal(tx.Cbor(), txBytes) {
				t.Fatalf(
					"transaction CBOR not preserved:\n"+
						" got %x\nwant %x",
					tx.Cbor(),
					txBytes,
				)
			}
			reencoded, err := cbor.Encode(tx)
			if err != nil {
				t.Fatalf(
					"failed to re-encode transaction: %v",
					err,
				)
			}
			if !bytes.Equal(reencoded, txBytes) {
				t.Fatalf(
					"re-encoded transaction differs:\n"+
						" got %x\nwant %x",
					reencoded,
					txBytes,
				)
			}

			idEnvelope, err := idFixture.ConsensusEnvelope()
			if err != nil {
				t.Fatalf(
					"failed to decode GenTxId envelope: %v",
					err,
				)
			}
			if idEnvelope.Era != testCase.eraID {
				t.Fatalf(
					"unexpected GenTxId era: "+
						"got %d want %d",
					idEnvelope.Era,
					testCase.eraID,
				)
			}
			txID, err := idFixture.ConsensusTransactionIDBytes()
			if err != nil {
				t.Fatalf(
					"failed to extract transaction id: %v",
					err,
				)
			}
			if len(txID) != 32 {
				t.Fatalf(
					"unexpected transaction id size: "+
						"got %d want 32",
					len(txID),
				)
			}
			if testCase.pairedID {
				if !bytes.Equal(tx.Hash().Bytes(), txID) {
					t.Fatalf(
						"transaction id mismatch:\n"+
							" got %x\nwant %x",
						tx.Hash().Bytes(),
						txID,
					)
				}
				return
			}

			// The upstream Byron GenTxId fixture is not paired with this GenTx.
			_, ok = tx.(*ledgerbyron.ByronTransaction)
			if !ok {
				t.Fatalf(
					"unexpected Byron transaction type %T",
					tx,
				)
			}
			expectedHash, err := hex.DecodeString(
				testCase.expectedTransactionID,
			)
			if err != nil {
				t.Fatalf("invalid Byron transaction hash vector: %v", err)
			}
			if !bytes.Equal(tx.Hash().Bytes(), expectedHash) {
				t.Fatalf(
					"Byron transaction hash mismatch: got %x want %x",
					tx.Hash().Bytes(),
					expectedHash,
				)
			}
			expectedFixtureID, err := hex.DecodeString(
				testCase.expectedFixtureID,
			)
			if err != nil {
				t.Fatalf("invalid Byron fixture id vector: %v", err)
			}
			if !bytes.Equal(txID, expectedFixtureID) {
				t.Fatalf(
					"Byron GenTxId fixture mismatch: got %x want %x",
					txID,
					expectedFixtureID,
				)
			}
		})
	}
}
