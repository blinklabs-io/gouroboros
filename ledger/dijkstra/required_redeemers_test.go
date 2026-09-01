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

package dijkstra

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// TestUtxoValidateRequiredRedeemersRegistered pins that Dijkstra's rule list
// reuses conway.UtxoValidateRequiredRedeemers directly, rather than each era
// re-deriving the reference-script-implies-redeemer check on its own (issue
// #2147's "avoid duplicating era-specific guards" requirement).
func TestUtxoValidateRequiredRedeemersRegistered(t *testing.T) {
	dijkstraValidationRule(t, "ledger/conway.UtxoValidateRequiredRedeemers")
}

// TestUtxoValidateRequiredRedeemersDijkstra exercises issue #2147's scenario
// through the Dijkstra entry point: a script-address input satisfied by a
// CIP-33 reference script, spent with no redeemer at all. Conway and
// Dijkstra must behave consistently here since they share one function.
func TestUtxoValidateRequiredRedeemersDijkstra(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		v1.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)

	input := shelley.NewShelleyTransactionInput(
		"6666666666666666666666666666666666666666666666666666666666666666",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: scriptAddr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1000},
			TxOutScriptRef: &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV1,
				Script: v1,
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			if id.String() == input.String() {
				return utxo, nil
			}
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	newTx := func() *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				),
			},
			TxIsValid: true,
		}
	}

	// No redeemer at all: the reference script satisfies script-presence
	// checks, but the spend is left completely unexecuted -- exactly the
	// gap issue #2147 describes.
	missingTx := newTx()
	err = conway.UtxoValidateRequiredRedeemers(
		missingTx,
		0,
		ls,
		&DijkstraProtocolParameters{},
	)
	var missingErr common.MissingRedeemerForScriptError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, v1.Hash(), missingErr.ScriptHash)
	require.Equal(t, uint32(0), missingErr.Index)

	// With the matching spend redeemer present, the same input passes.
	validTx := newTx()
	validTx.WitnessSet = DijkstraTransactionWitnessSet{
		WsRedeemers: DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		},
	}
	require.NoError(t, conway.UtxoValidateRequiredRedeemers(
		validTx,
		0,
		ls,
		&DijkstraProtocolParameters{},
	))
}
