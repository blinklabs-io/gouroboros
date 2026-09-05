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
	"embed"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
)

// consensusGoldenRoot is the CardanoNodeToNodeVersion2 golden directory of
// ouroboros-consensus, mirrored into the ouroboros-mock module and embedded
// there.
const consensusGoldenRoot = "upstream/ouroboros-consensus/" +
	"ouroboros-consensus-cardano/golden/cardano/CardanoNodeToNodeVersion2/"

// consensusEnvelope decodes the two-element ouroboros-consensus envelope
// [era_id, payload] that carries a GenTx or a GenTxId.
func consensusEnvelope(
	t *testing.T,
	fsys embed.FS,
	name string,
) (uint, cbor.RawMessage) {
	t.Helper()
	data, err := fsys.ReadFile(consensusGoldenRoot + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
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
// A GenTx is [era_id, #6.24(bytes .cbor transaction)] and the matching GenTxId
// is [era_id, bytes .size 32], where the identifier is the Blake2b-256 hash of
// the transaction body's own CBOR. The era identifier is the ledger TxType, so
// era 7 selects the Dijkstra decoder, whose transaction is the three-element
// [transaction_body, transaction_witness_set, auxiliary_data / nil] with no
// is_valid flag, against the four-element Alonzo through Conway form.
func TestConsensusGenTxFixtures(t *testing.T) {
	embedded := fixtures.EmbeddedFixtures()
	for _, testCase := range []struct {
		name string
		// era is both the consensus era identifier in the envelope and
		// the ledger transaction type.
		era uint
		// txArrayLen is the width of the era's transaction array.
		txArrayLen int
	}{
		{name: "Shelley", era: ledger.TxTypeShelley, txArrayLen: 3},
		{name: "Allegra", era: ledger.TxTypeAllegra, txArrayLen: 3},
		{name: "Mary", era: ledger.TxTypeMary, txArrayLen: 3},
		{name: "Alonzo", era: ledger.TxTypeAlonzo, txArrayLen: 4},
		{name: "Babbage", era: ledger.TxTypeBabbage, txArrayLen: 4},
		{name: "Conway", era: ledger.TxTypeConway, txArrayLen: 4},
		{name: "Dijkstra", era: ledger.TxTypeDijkstra, txArrayLen: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			era, payload := consensusEnvelope(
				t,
				embedded,
				"GenTx_"+testCase.name,
			)
			if era != testCase.era {
				t.Fatalf(
					"unexpected GenTx era: got %d want %d",
					era,
					testCase.era,
				)
			}
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
			txBytes, ok := tag.Content.([]byte)
			if !ok {
				t.Fatalf(
					"unexpected GenTx payload type %T",
					tag.Content,
				)
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
				embedded,
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
			if _, err := cbor.Decode(idPayload, &txId); err != nil {
				t.Fatalf("failed to decode GenTxId payload: %v", err)
			}
			if !bytes.Equal(tx.Hash().Bytes(), txId) {
				t.Fatalf(
					"transaction id mismatch:\n got %x\nwant %x",
					tx.Hash().Bytes(),
					txId,
				)
			}
		})
	}
}
