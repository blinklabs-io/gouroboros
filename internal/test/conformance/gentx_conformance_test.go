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
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
)

func consensusFixtureBytes(
	t *testing.T,
	allFixtures []fixtures.Fixture,
	kind fixtures.Kind,
	fixtureEra string,
	name string,
) []byte {
	t.Helper()
	for _, fixture := range allFixtures {
		if fixture.Repo != fixtures.RepoOuroborosConsensus ||
			fixture.Kind != kind ||
			fixture.Format != fixtures.FormatCBOR ||
			!strings.EqualFold(fixture.Era, fixtureEra) ||
			fixture.Name != name {
			continue
		}
		data, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", fixture.RelPath, err)
		}
		return data
	}
	t.Fatalf("fixture %s (%s, %s) not found", name, kind, fixtureEra)
	return nil
}

// consensusEnvelope decodes the two-element ouroboros-consensus envelope
// [era_id, payload] that carries a GenTx or a GenTxId.
func consensusEnvelope(
	t *testing.T,
	allFixtures []fixtures.Fixture,
	kind fixtures.Kind,
	fixtureEra string,
	name string,
) (uint, cbor.RawMessage) {
	t.Helper()
	data := consensusFixtureBytes(t, allFixtures, kind, fixtureEra, name)
	var envelope []cbor.RawMessage
	if _, err := cbor.Decode(data, &envelope); err != nil {
		t.Fatalf("failed to decode envelope %s: %v", name, err)
	}
	if len(envelope) != 2 {
		t.Fatalf(
			"unexpected envelope width for %s: got %d want 2",
			name,
			len(envelope),
		)
	}
	var era uint
	if _, err := cbor.Decode(envelope[0], &era); err != nil {
		t.Fatalf("failed to decode envelope era for %s: %v", name, err)
	}
	return era, envelope[1]
}

// TestConsensusGenTxFixtures decodes the ouroboros-consensus GenTx and GenTxId
// goldens through the ledger transaction decoders.
//
// The era identifier is the ledger TxType. Shelley and later GenTx values use
// tag 24 around transaction CBOR. Byron uses a constructor-tagged payload.
func TestConsensusGenTxFixtures(t *testing.T) {
	fixturesRoot, err := fixtures.ExtractEmbeddedFixtures(t.TempDir())
	if err != nil {
		t.Fatalf("failed to extract ouroboros-mock fixtures: %v", err)
	}
	harness := fixtures.NewHarness(fixtures.HarnessConfig{
		FixturesRoot: fixturesRoot,
	})
	allFixtures, err := harness.Collect()
	if err != nil {
		t.Fatalf("failed to collect ouroboros-mock fixtures: %v", err)
	}
	for _, testCase := range []struct {
		name       string
		fixtureEra string
		// era is both the consensus era identifier in the envelope and
		// the ledger transaction type.
		era uint
		// txArrayLen is the width of the era's transaction array.
		txArrayLen int
	}{
		{
			name: "Byron", fixtureEra: "byron",
			era: ledger.TxTypeByron, txArrayLen: 2,
		},
		{
			name: "Shelley", fixtureEra: "shelley",
			era: ledger.TxTypeShelley, txArrayLen: 3,
		},
		{
			name: "Allegra", fixtureEra: "allegra",
			era: ledger.TxTypeAllegra, txArrayLen: 3,
		},
		{
			name: "Mary", fixtureEra: "mary",
			era: ledger.TxTypeMary, txArrayLen: 3,
		},
		{
			name: "Alonzo", fixtureEra: "alonzo",
			era: ledger.TxTypeAlonzo, txArrayLen: 4,
		},
		{
			name: "Babbage", fixtureEra: "babbage",
			era: ledger.TxTypeBabbage, txArrayLen: 4,
		},
		{
			name: "Conway", fixtureEra: "conway",
			era: ledger.TxTypeConway, txArrayLen: 4,
		},
		{
			name: "Dijkstra", fixtureEra: "dijkstra",
			era: ledger.TxTypeDijkstra, txArrayLen: 3,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			era, payload := consensusEnvelope(
				t,
				allFixtures,
				fixtures.KindTransaction,
				testCase.fixtureEra,
				"GenTx_"+testCase.name,
			)
			if era != testCase.era {
				t.Fatalf(
					"unexpected GenTx era: got %d want %d",
					era,
					testCase.era,
				)
			}
			var txBytes []byte
			if testCase.era == ledger.TxTypeByron {
				var byronGenTx []cbor.RawMessage
				if _, err := cbor.Decode(payload, &byronGenTx); err != nil {
					t.Fatalf("failed to decode Byron GenTx constructor: %v", err)
				}
				if len(byronGenTx) != 2 || !bytes.Equal(byronGenTx[0], []byte{0}) {
					t.Fatalf("unexpected Byron GenTx constructor: %x", payload)
				}
				txBytes = byronGenTx[1]
			} else {
				var tag cbor.Tag
				if _, err := cbor.Decode(payload, &tag); err != nil {
					t.Fatalf("failed to decode GenTx payload tag: %v", err)
				}
				if tag.Number != 24 {
					t.Fatalf(
						"unexpected GenTx payload tag: got %d want 24",
						tag.Number,
					)
				}
				var ok bool
				txBytes, ok = tag.Content.([]byte)
				if !ok {
					t.Fatalf("unexpected GenTx payload type %T", tag.Content)
				}
			}

			var txArray []cbor.RawMessage
			if _, err := cbor.Decode(txBytes, &txArray); err != nil {
				t.Fatalf("failed to decode transaction array: %v", err)
			}
			if len(txArray) != testCase.txArrayLen {
				t.Fatalf(
					"unexpected transaction array width: got %d want %d",
					len(txArray),
					testCase.txArrayLen,
				)
			}

			tx, err := ledger.NewTransactionFromCbor(era, txBytes)
			if err != nil {
				t.Fatalf("failed to decode transaction: %v", err)
			}
			if tx.Type() != int(era) {
				t.Fatalf(
					"unexpected transaction type: got %d want %d",
					tx.Type(),
					era,
				)
			}

			// The decoder must preserve the transaction's own bytes.
			if !bytes.Equal(tx.Cbor(), txBytes) {
				t.Fatalf(
					"transaction CBOR not preserved:\n got %x\nwant %x",
					tx.Cbor(),
					txBytes,
				)
			}
			reencoded, err := cbor.Encode(tx)
			if err != nil {
				t.Fatalf("failed to re-encode transaction: %v", err)
			}
			if !bytes.Equal(reencoded, txBytes) {
				t.Fatalf(
					"re-encoded transaction differs:\n got %x\nwant %x",
					reencoded,
					txBytes,
				)
			}

			idEra, idPayload := consensusEnvelope(
				t,
				allFixtures,
				fixtures.KindTransactionID,
				testCase.fixtureEra,
				"GenTxId_"+testCase.name,
			)
			if idEra != testCase.era {
				t.Fatalf(
					"unexpected GenTxId era: got %d want %d",
					idEra,
					testCase.era,
				)
			}
			var txId []byte
			if testCase.era == ledger.TxTypeByron {
				var byronGenTxID []cbor.RawMessage
				if _, err := cbor.Decode(idPayload, &byronGenTxID); err != nil {
					t.Fatalf("failed to decode Byron GenTxId constructor: %v", err)
				}
				if len(byronGenTxID) != 2 || !bytes.Equal(byronGenTxID[0], []byte{0}) {
					t.Fatalf("unexpected Byron GenTxId constructor: %x", idPayload)
				}
				if _, err := cbor.Decode(byronGenTxID[1], &txId); err != nil {
					t.Fatalf("failed to decode Byron GenTxId value: %v", err)
				}
			} else if _, err := cbor.Decode(idPayload, &txId); err != nil {
				t.Fatalf("failed to decode GenTxId payload: %v", err)
			}
			if testCase.era == ledger.TxTypeByron {
				inputs := tx.Inputs()
				if len(inputs) != 1 {
					t.Fatalf("unexpected Byron fixture input count: %d", len(inputs))
				}
				// The upstream Byron GenTxId golden contains the transaction's
				// referenced input ID, not this GenTx's body hash. Keep both
				// fixtures under typed metadata and check that documented shape.
				if !bytes.Equal(inputs[0].Id().Bytes(), txId) {
					t.Fatalf(
						"Byron GenTxId fixture no longer matches its input ID:\n got %x\nwant %x",
						inputs[0].Id().Bytes(),
						txId,
					)
				}
			} else if !bytes.Equal(tx.Hash().Bytes(), txId) {
				t.Fatalf(
					"transaction id mismatch:\n got %x\nwant %x",
					tx.Hash().Bytes(),
					txId,
				)
			}
		})
	}
}
