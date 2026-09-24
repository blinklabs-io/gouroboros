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

package conway

import (
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

type datumWitnessLedgerState struct {
	common.LedgerState
	utxos map[string]common.Utxo
}

func (s datumWitnessLedgerState) UtxoById(
	input common.TransactionInput,
) (common.Utxo, error) {
	utxo, ok := s.utxos[input.String()]
	if !ok {
		return common.Utxo{}, errors.New("utxo not found")
	}
	return utxo, nil
}

func datumSpendingFixture(
	t *testing.T,
	script common.Script,
	withDatumHash bool,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input := shelley.NewShelleyTransactionInput(
		"0101010101010101010101010101010101010101010101010101010101010101",
		0,
	)
	address, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		script.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	output := &alonzo.AlonzoTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1_000_000},
	}
	if withDatumHash {
		datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
		datumHash := datum.Hash()
		output.OutputDatumHash = &datumHash
	}
	return input, common.Utxo{Id: input, Output: output}
}

func TestSupplementalDatumsRequiresV1V2WitnessRegardlessOfValidity(t *testing.T) {
	v1 := common.PlutusV1Script{0x01}
	input, utxo := datumSpendingFixture(t, v1, true)
	tx := &ConwayTransaction{
		TxIsValid: false,
		Body: ConwayTransactionBody{
			TxInputs: NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
		},
		WitnessSet: ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType([]common.PlutusV1Script{v1}, true),
		},
	}
	state := datumWitnessLedgerState{utxos: map[string]common.Utxo{input.String(): utxo}}
	var missing common.MissingDatumForSpendingScriptError
	require.ErrorAs(t, UtxoValidateSupplementalDatums(tx, 0, state, nil), &missing)

	datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	tx.WitnessSet.WsPlutusData = cbor.NewSetType([]common.Datum{datum}, true)
	require.NoError(t, UtxoValidateSupplementalDatums(tx, 0, state, nil))
}

func TestSupplementalDatumsRejectsV1SpendingWithoutUsableDatum(t *testing.T) {
	v1 := common.PlutusV1Script{0x03}
	input, utxo := datumSpendingFixture(t, v1, false)
	tx := &ConwayTransaction{
		TxIsValid: true,
		Body: ConwayTransactionBody{
			TxInputs: NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
		},
		WitnessSet: ConwayTransactionWitnessSet{
			WsPlutusV1Scripts: cbor.NewSetType([]common.PlutusV1Script{v1}, true),
		},
	}
	state := datumWitnessLedgerState{utxos: map[string]common.Utxo{input.String(): utxo}}
	var missing common.MissingDatumForSpendingScriptError
	require.ErrorAs(t, UtxoValidateSupplementalDatums(tx, 0, state, nil), &missing)
}

func TestSupplementalDatumsPreservesPlutusV3NoDatumRule(t *testing.T) {
	v3 := common.PlutusV3Script{0x02}
	input, utxo := datumSpendingFixture(t, v3, false)
	tx := &ConwayTransaction{
		TxIsValid: false,
		Body: ConwayTransactionBody{
			TxInputs: NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
		},
		WitnessSet: ConwayTransactionWitnessSet{
			WsPlutusV3Scripts: cbor.NewSetType([]common.PlutusV3Script{v3}, true),
		},
	}
	state := datumWitnessLedgerState{utxos: map[string]common.Utxo{input.String(): utxo}}
	require.NoError(t, UtxoValidateSupplementalDatums(tx, 0, state, nil))
}

func TestRequiredSpendingDatumsValidatesPlutusV3DatumHash(t *testing.T) {
	v3 := common.PlutusV3Script{0x04}
	input, utxo := datumSpendingFixture(t, v3, true)
	tx := &ConwayTransaction{
		TxIsValid: true,
		Body: ConwayTransactionBody{
			TxInputs: NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{input}),
		},
		WitnessSet: ConwayTransactionWitnessSet{
			WsPlutusV3Scripts: cbor.NewSetType([]common.PlutusV3Script{v3}, true),
		},
	}
	state := datumWitnessLedgerState{utxos: map[string]common.Utxo{input.String(): utxo}}
	var missing common.MissingDatumForSpendingScriptError
	require.ErrorAs(t, common.ValidateRequiredSpendingDatums(tx, state), &missing)

	datum := common.Datum{Data: data.NewInteger(big.NewInt(1))}
	tx.WitnessSet.WsPlutusData = cbor.NewSetType([]common.Datum{datum}, true)
	require.NoError(t, common.ValidateRequiredSpendingDatums(tx, state))
}
