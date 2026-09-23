package babbage_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestCollateralReturnValidationUsesNetCollateral(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	tx := &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxFee:        1_000_000,
			TxCollateral: cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
			TxCollateralReturn: &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{Amount: 9_000_000},
			},
		},
		WitnessSet: babbage.BabbageTransactionWitnessSet{WsRedeemers: alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{}}}},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
		Id:     input,
		Output: shelley.ShelleyTransactionOutput{OutputAmount: 10_000_000},
	}}).Build()
	err := babbage.UtxoValidateInsufficientCollateral(tx, 0, state, &babbage.BabbageProtocolParameters{CollateralPercentage: 150})
	require.ErrorAs(t, err, new(alonzo.InsufficientCollateralError))
}

func TestCollateralReturnCannotCreateAssets(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	assets := common.NewMultiAsset[common.MultiAssetTypeOutput](map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
		common.Blake2b224Hash([]byte("policy")): {cbor.NewByteString([]byte("token")): big.NewInt(1)},
	})
	tx := &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxCollateral: cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
			TxCollateralReturn: &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{Amount: 1_000_000, Assets: &assets},
			},
		},
		WitnessSet: babbage.BabbageTransactionWitnessSet{WsRedeemers: alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{}}}},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
		Id:     input,
		Output: shelley.ShelleyTransactionOutput{OutputAmount: 2_000_000},
	}}).Build()
	err := babbage.UtxoValidateCollateralContainsNonAda(tx, 0, state, &babbage.BabbageProtocolParameters{})
	require.ErrorAs(t, err, new(alonzo.CollateralContainsNonAdaError))
}

func TestBabbageOutputRulesIncludeCollateralReturn(t *testing.T) {
	addr, err := common.NewAddressFromParts(common.AddressTypeKeyKey, common.AddressNetworkMainnet, make([]byte, common.AddressHashSize), make([]byte, common.AddressHashSize))
	require.NoError(t, err)
	output := &babbage.BabbageTransactionOutput{OutputAddress: addr}
	tx := &babbage.BabbageTransaction{Body: babbage.BabbageTransactionBody{TxCollateralReturn: output}}
	state := mockledger.NewLedgerStateBuilder().WithNetworkId(uint(common.AddressNetworkTestnet)).Build()
	require.Error(t, babbage.UtxoValidateWrongNetwork(tx, 0, state, &babbage.BabbageProtocolParameters{}))
	require.Error(t, babbage.UtxoValidateOutputTooBigUtxo(tx, 0, state, &babbage.BabbageProtocolParameters{MaxValueSize: 0}))
	require.Error(t, babbage.UtxoValidateOutputTooSmallUtxo(tx, 0, state, &babbage.BabbageProtocolParameters{AdaPerUtxoByte: 1}))
	byronAddr, err := common.NewByronAddressFromParts(common.ByronAddressTypePubkey, make([]byte, common.AddressHashSize), common.ByronAddressAttributes{Payload: make([]byte, 100)})
	require.NoError(t, err)
	tx.Body.TxCollateralReturn.OutputAddress = byronAddr
	require.Error(t, babbage.UtxoValidateOutputBootAddrAttrsTooBig(tx, 0, state, &babbage.BabbageProtocolParameters{}))
}

func TestBabbageTotalCollateralPresenceDistinguishesExplicitZero(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	bodyBytes, err := cbor.Encode(map[uint]any{17: uint64(0)})
	require.NoError(t, err)
	var body babbage.BabbageTransactionBody
	_, err = cbor.Decode(bodyBytes, &body)
	require.NoError(t, err)
	require.True(t, body.TotalCollateralPresent())
	body.TxCollateral = cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false)
	body.TxCollateralReturn = &babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 3_000_000}}
	tx := &babbage.BabbageTransaction{Body: body, WitnessSet: babbage.BabbageTransactionWitnessSet{WsRedeemers: alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{Tag: common.RedeemerTagSpend}}}}}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 5_000_000}}}).Build()
	require.Error(t, babbage.UtxoValidateCollateralEqBalance(tx, 0, state, &babbage.BabbageProtocolParameters{}))
	body.TxTotalCollateral = 0
	body.TxCollateralReturn.OutputAmount.Amount = 5_000_000
	tx.Body = body
	require.NoError(t, babbage.UtxoValidateCollateralEqBalance(tx, 0, state, &babbage.BabbageProtocolParameters{}))

	var absent babbage.BabbageTransactionBody
	absentBytes, err := cbor.Encode(map[uint]any{})
	require.NoError(t, err)
	_, err = cbor.Decode(absentBytes, &absent)
	require.NoError(t, err)
	require.False(t, absent.TotalCollateralPresent())
	absent.TxCollateral = cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false)
	absent.TxCollateralReturn = &babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 3_000_000}}
	tx.Body = absent
	require.NoError(t, babbage.UtxoValidateCollateralEqBalance(tx, 0, state, &babbage.BabbageProtocolParameters{}))
}
