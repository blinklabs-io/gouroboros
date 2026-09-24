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

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestConwayDonationRestrictionUsesNeededPlutusScripts(t *testing.T) {
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkTestnet,
		make([]byte, common.AddressHashSize),
		make([]byte, common.AddressHashSize),
	)
	require.NoError(t, err)
	spent := shelley.NewShelleyTransactionInput(
		"0101010101010101010101010101010101010101010101010101010101010101",
		0,
	)
	reference := shelley.NewShelleyTransactionInput(
		"0202020202020202020202020202020202020202020202020202020202020202",
		0,
	)
	unusedScript := common.PlutusV2Script{0x41, 0x01}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		{
			Id: spent,
			Output: babbage.BabbageTransactionOutput{
				OutputAddress: address,
				OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000},
			},
		},
		{
			Id: reference,
			Output: babbage.BabbageTransactionOutput{
				TxOutScriptRef: &common.ScriptRef{Script: unusedScript},
			},
		},
	}).Build()
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(
				[]shelley.ShelleyTransactionInput{spent},
			),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{reference},
				true,
			),
			TxOutputs: []babbage.BabbageTransactionOutput{{
				OutputAddress: address,
				OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1_999_999},
			}},
			TxDonation: 1,
		},
	}
	require.NoError(t, conway.UtxoValidateValueNotConservedUtxo(
		tx,
		0,
		state,
		&conway.ConwayProtocolParameters{},
	), "an unrelated V2 reference script must not prohibit a donation")

	for _, test := range []struct {
		name   string
		script common.Script
		want   string
	}{
		{
			name:   "needed PlutusV1",
			script: common.PlutusV1Script{0x42, 0x01},
			want:   "PlutusV1",
		},
		{
			name:   "needed PlutusV2",
			script: common.PlutusV2Script{0x43, 0x01},
			want:   "PlutusV2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxMint:     conwayTreasuryMint(test.script),
					TxDonation: 1,
				},
				WitnessSet: conwayTreasuryWitnessSet(t, test.script),
			}
			err := conway.UtxoValidateValueNotConservedUtxo(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				&conway.ConwayProtocolParameters{},
			)
			var donationErr conway.TreasuryDonationWithPlutusV1V2Error
			require.ErrorAs(t, err, &donationErr)
			require.Equal(t, test.want, donationErr.PlutusVersion)
		})
	}
}
