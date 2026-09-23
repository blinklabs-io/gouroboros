package conway_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestExplicitZeroTotalCollateralIsValidated(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	raw, err := cbor.Encode(map[uint]any{
		0:  cbor.NewSetType([]any{}, false),
		1:  []any{},
		2:  uint64(0),
		17: uint64(0),
	})
	require.NoError(t, err)
	var body conway.ConwayTransactionBody
	_, err = cbor.Decode(raw, &body)
	require.NoError(t, err)
	require.True(t, body.TotalCollateralPresent())
	body.TxCollateral = cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false)
	body.TxCollateralReturn = &babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 3_000_000}}
	tx := &conway.ConwayTransaction{Body: body, WitnessSet: conway.ConwayTransactionWitnessSet{WsRedeemers: conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{{Tag: common.RedeemerTagSpend, Index: 0}: {}}}}}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 5_000_000}}}).Build()
	require.Error(t, conway.UtxoValidateCollateralEqBalance(tx, 0, state, &conway.ConwayProtocolParameters{}))
}

func TestConwayCollateralReturnRunsAllOutputPredicates(t *testing.T) {
	addr, err := common.NewAddressFromParts(common.AddressTypeKeyKey, common.AddressNetworkMainnet, make([]byte, common.AddressHashSize), make([]byte, common.AddressHashSize))
	require.NoError(t, err)
	tx := &conway.ConwayTransaction{
		Body:       conway.ConwayTransactionBody{TxCollateralReturn: &babbage.BabbageTransactionOutput{OutputAddress: addr}},
		WitnessSet: conway.ConwayTransactionWitnessSet{WsRedeemers: conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{{Tag: common.RedeemerTagSpend}: {}}}},
	}
	state := mockledger.NewLedgerStateBuilder().WithNetworkId(uint(common.AddressNetworkTestnet)).Build()
	pp := &conway.ConwayProtocolParameters{MaxValueSize: 0, AdaPerUtxoByte: 1}
	require.Error(t, conway.UtxoValidateWrongNetwork(tx, 0, state, pp))
	require.Error(t, conway.UtxoValidateOutputTooBigUtxo(tx, 0, state, pp))
	require.Error(t, conway.UtxoValidateOutputTooSmallUtxo(tx, 0, state, pp))
	byronAddr, err := common.NewByronAddressFromParts(common.ByronAddressTypePubkey, make([]byte, common.AddressHashSize), common.ByronAddressAttributes{Payload: make([]byte, 100)})
	require.NoError(t, err)
	tx.Body.TxCollateralReturn.OutputAddress = byronAddr
	require.Error(t, conway.UtxoValidateOutputBootAddrAttrsTooBig(tx, 0, state, pp))
}

func TestConwayCollateralReturnCannotCreateAssets(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	assets := common.NewMultiAsset[common.MultiAssetTypeOutput](map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
		common.Blake2b224Hash([]byte("policy")): {cbor.NewByteString([]byte("token")): big.NewInt(1)},
	})
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxCollateral:       cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
			TxCollateralReturn: &babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 1_000_000, Assets: &assets}},
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{WsRedeemers: conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{{Tag: common.RedeemerTagSpend}: {}}}},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 2_000_000}}}).Build()
	var collateralErr alonzo.CollateralContainsNonAdaError
	require.ErrorAs(t, conway.UtxoValidateCollateralContainsNonAda(tx, 0, state, &conway.ConwayProtocolParameters{}), &collateralErr)
}

func TestConwayMinimumCollateralUsesNetCollateral(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	tx := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxFee:              1_000_000,
			TxCollateral:       cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
			TxCollateralReturn: &babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 9_000_000}},
		},
		WitnessSet: conway.ConwayTransactionWitnessSet{WsRedeemers: conway.ConwayRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{{Tag: common.RedeemerTagSpend}: {}}}},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 10_000_000}}}).Build()
	var collateralErr alonzo.InsufficientCollateralError
	require.ErrorAs(t, conway.UtxoValidateInsufficientCollateral(tx, 0, state, &conway.ConwayProtocolParameters{CollateralPercentage: 150}), &collateralErr)
}
