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
	"runtime"
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
	indexOf := func(rule common.UtxoValidationRuleFunc) int {
		t.Helper()
		want := reflect.ValueOf(rule).Pointer()
		for idx, r := range babbage.UtxoValidationRules {
			if reflect.ValueOf(r).Pointer() == want {
				return idx
			}
		}
		t.Fatalf(
			"rule %s is not registered in UtxoValidationRules",
			runtime.FuncForPC(want).Name(),
		)
		return -1
	}

	requiredRedeemersIdx := indexOf(babbage.UtxoValidateRequiredRedeemers)
	badInputsIdx := indexOf(babbage.UtxoValidateBadInputsUtxo)
	scriptWitnessesIdx := indexOf(babbage.UtxoValidateScriptWitnesses)

	require.Greater(
		t,
		requiredRedeemersIdx,
		badInputsIdx,
		"UtxoValidateRequiredRedeemers must run after UtxoValidateBadInputsUtxo "+
			"so an unresolvable input surfaces as BadInputsUtxo, not a raw "+
			"input-resolution error",
	)
	require.Greater(
		t,
		requiredRedeemersIdx,
		scriptWitnessesIdx,
		"UtxoValidateRequiredRedeemers must run after UtxoValidateScriptWitnesses "+
			"so a missing script is reported by the latter, not masked as a "+
			"missing redeemer",
	)
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

// TestBabbageUtxoValidateRequiredRedeemersDuplicateCertificates pins the
// Alonzo/Babbage duplicate-certificate redeemer index. Babbage encodes
// certificates as a list, so two logically identical entries are decodable;
// Conway onward encodes them as a set and common.ValidateCertificateSet
// rejects a duplicate at decode time, so this case only exists here.
//
// cardano-ledger's getAlonzoScriptsNeeded gives the second occurrence the
// first's index rather than its own -- addUniqueTxCertPurpose in
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/UTxO.hs, whose comment works the
// example through -- so one certificate redeemer at index 0 covers both.
// Assigning the duplicate its own positional index would demand a redeemer
// index 1 that cardano-ledger never asks for, and would reject a Babbage
// block during sync.
func TestBabbageUtxoValidateRequiredRedeemersDuplicateCertificates(t *testing.T) {
	v2 := common.PlutusV2Script{0x04, 0x05, 0x06}
	cert := func() common.CertificateWrapper {
		return common.CertificateWrapper{
			Type: 1,
			Certificate: &common.StakeDeregistrationCertificate{
				StakeCredential: common.Credential{
					CredType:   common.CredentialTypeScriptHash,
					Credential: common.Blake2b224(v2.Hash()),
				},
			},
		}
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(id common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, errors.New("not found")
		}).
		Build()

	newTx := func() *babbage.BabbageTransaction {
		return &babbage.BabbageTransaction{
			Body: babbage.BabbageTransactionBody{
				// The same certificate twice: index 0 and index 1.
				TxCertificates: []common.CertificateWrapper{cert(), cert()},
			},
			WitnessSet: babbage.BabbageTransactionWitnessSet{
				WsPlutusV2Scripts: []common.PlutusV2Script{v2},
			},
			TxIsValid: true,
		}
	}

	t.Run("one redeemer at index 0 covers both", func(t *testing.T) {
		tx := newTx()
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagCert,
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

	t.Run("no redeemer still rejected at index 0", func(t *testing.T) {
		err := babbage.UtxoValidateRequiredRedeemers(
			newTx(),
			0,
			ls,
			&babbage.BabbageProtocolParameters{},
		)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, common.RedeemerTagCert, missingErr.Tag)
		require.Equal(t, uint32(0), missingErr.Index)
	})

	t.Run("distinct certificates each need their own", func(t *testing.T) {
		other := common.PlutusV2Script{0x07, 0x08, 0x09}
		tx := newTx()
		tx.Body.TxCertificates = []common.CertificateWrapper{
			cert(),
			{
				Type: 1,
				Certificate: &common.StakeDeregistrationCertificate{
					StakeCredential: common.Credential{
						CredType:   common.CredentialTypeScriptHash,
						Credential: common.Blake2b224(other.Hash()),
					},
				},
			},
		}
		tx.WitnessSet.WsPlutusV2Scripts = []common.PlutusV2Script{v2, other}
		tx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{
			Redeemers: []alonzo.AlonzoRedeemer{
				{
					Tag:     common.RedeemerTagCert,
					Index:   0,
					ExUnits: common.ExUnits{Steps: 1, Memory: 1},
				},
			},
		}
		err := babbage.UtxoValidateRequiredRedeemers(
			tx,
			0,
			ls,
			&babbage.BabbageProtocolParameters{},
		)
		var missingErr common.MissingRedeemerForScriptError
		require.ErrorAs(t, err, &missingErr)
		require.Equal(t, other.Hash(), missingErr.ScriptHash)
		require.Equal(t, common.RedeemerTagCert, missingErr.Tag)
		require.Equal(t, uint32(1), missingErr.Index)
	})
}
