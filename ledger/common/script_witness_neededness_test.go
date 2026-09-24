package common_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestScriptWitnessRulesUseNeededPurposes(t *testing.T) {
	plutus := common.PlutusV1Script([]byte{0x41, 0x00})
	address, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		plutus.Hash().Bytes(),
		nil,
	)
	require.NoError(t, err)
	spent, err := mockledger.NewUtxoBuilder().
		WithTxId(bytes.Repeat([]byte{0x20}, 32)).
		WithIndex(0).
		WithAddress(address.String()).
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)
	keyAddress, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		make([]byte, common.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	keyInput, err := mockledger.NewUtxoBuilder().
		WithTxId(bytes.Repeat([]byte{0x21}, 32)).
		WithIndex(0).
		WithAddress(keyAddress.String()).
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)
	ledgerState := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(input common.TransactionInput) (common.Utxo, error) {
			switch input.String() {
			case spent.Id.String():
				return spent, nil
			case keyInput.Id.String():
				return keyInput, nil
			default:
				return common.Utxo{}, nil
			}
		},
	).Build()
	inputs := []common.TransactionInput{spent.Id, keyInput.Id}
	needed := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 0}
	extra := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 1}
	unusedTx := mockledger.NewTransactionBuilder()
	unusedTx.WithInputs(keyInput.Id)
	unusedTx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithPlutusV1Scripts(plutus),
	)
	usedVersions, err := common.UsedPlutusVersions(unusedTx, ledgerState)
	require.NoError(t, err)
	require.Empty(t, usedVersions)
	tx := mockledger.NewTransactionBuilder()
	tx.WithInputs(inputs...)
	tx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithPlutusV1Scripts(plutus).
			WithRedeemers(conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				needed: {},
				extra:  {},
			}}),
	)
	var extraErr common.ExtraneousRedeemerError
	require.ErrorAs(t, common.ValidateExactExtraneousRedeemers(tx, ledgerState), &extraErr)
	require.Equal(t, extra, extraErr.RedeemerKey)

	refInput := shelley.NewShelleyTransactionInput(hex.EncodeToString(bytes.Repeat([]byte{0x22}, 32)), 0)
	scriptRef, err := cbor.Encode(&common.ScriptRef{Type: common.ScriptRefTypePlutusV1, Script: plutus})
	require.NoError(t, err)
	reference, err := mockledger.NewUtxoBuilder().
		WithTxId(bytes.Repeat([]byte{0x22}, 32)).
		WithIndex(0).
		WithAddress(keyAddress.String()).
		WithLovelace(2_000_000).
		WithScriptRef(scriptRef).
		Build()
	require.NoError(t, err)
	ledgerState = mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(input common.TransactionInput) (common.Utxo, error) {
			if input.String() == spent.Id.String() {
				return spent, nil
			}
			return reference, nil
		},
	).Build()
	tx.WithReferenceInputs(refInput).
		WithWitnesses(mockledger.NewMockTransactionWitnessSet().
			WithPlutusV1Scripts(plutus).
			WithRedeemers(conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{needed: {}}}))
	// The same script may be explicitly witnessed for the spending purpose and
	// also appear on an unrelated reference input. Neededness is determined by
	// purposes, not by duplicate script availability.
	require.NoError(t, common.ValidateScriptWitnesses(tx, ledgerState))
}
