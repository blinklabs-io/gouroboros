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

package babbage_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// TestBabbageUtxoValidateRequiredRedeemersRegistered pins that Babbage
// registers UtxoValidateRequiredRedeemers in its production rule list.
// Babbage never executes Plutus itself, but a reference-script-backed
// script-address input with no redeemer must still be rejected rather than
// silently spent unexecuted (issue #2147: "across all supported eras").
func TestBabbageUtxoValidateRequiredRedeemersRegistered(t *testing.T) {
	want := reflect.ValueOf(babbage.UtxoValidateRequiredRedeemers).Pointer()
	for _, rule := range babbage.UtxoValidationRules {
		if reflect.ValueOf(rule).Pointer() == want {
			return
		}
	}
	t.Fatal("UtxoValidateRequiredRedeemers is not registered in UtxoValidationRules")
}

// TestBabbageUtxoValidateRequiredRedeemers covers a Plutus script-address
// input satisfied by a CIP-33 reference script: missing its spend redeemer
// must be rejected, and a valid redeemer must be accepted.
func TestBabbageUtxoValidateRequiredRedeemers(t *testing.T) {
	v1 := common.PlutusV1Script{0x01, 0x02, 0x03}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		v1.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)

	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
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

	newTx := func() *babbage.BabbageTransaction {
		return &babbage.BabbageTransaction{
			Body: babbage.BabbageTransactionBody{
				TxInputs: shelley.NewShelleyTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				),
			},
			TxIsValid: true,
		}
	}

	t.Run("missing redeemer for reference script rejected", func(t *testing.T) {
		err := babbage.UtxoValidateRequiredRedeemers(
			newTx(),
			0,
			ls,
			&babbage.BabbageProtocolParameters{},
		)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, v1.Hash(), missingErr.ScriptHash)
		require.Equal(t, common.RedeemerTagSpend, missingErr.Tag)
		require.Equal(t, uint32(0), missingErr.Index)
	})

	t.Run("valid redeemer accepted", func(t *testing.T) {
		tx := newTx()
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagSpend,
					Index:   0,
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		require.NoError(t, babbage.UtxoValidateRequiredRedeemers(
			tx,
			0,
			ls,
			&babbage.BabbageProtocolParameters{},
		))
	})
}
