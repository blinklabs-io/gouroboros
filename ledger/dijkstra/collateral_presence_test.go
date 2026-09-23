package dijkstra

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

func TestExplicitZeroTotalCollateralIsValidated(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	raw, err := cbor.Encode(map[uint]any{
		0:  cbor.NewSetType([]any{}, false),
		1:  []any{},
		2:  uint64(0),
		17: uint64(0),
	})
	require.NoError(t, err)
	var body DijkstraTransactionBody
	_, err = cbor.Decode(raw, &body)
	require.NoError(t, err)
	require.True(t, body.TotalCollateralPresent())
	body.TxCollateral = cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false)
	body.TxCollateralReturn = &DijkstraTransactionOutput{Output: babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 3_000_000}}}
	tx := &DijkstraTransaction{
		Body:      body,
		TxIsValid: true,
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {},
			}},
		},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 5_000_000}}}).Build()
	_, ruleIndex := dijkstraValidationRule(
		t,
		"ledger/conway.UtxoValidateCollateralEqBalance",
	)
	require.Error(t, common.VerifyTransaction(
		tx,
		0,
		state,
		&DijkstraProtocolParameters{},
		UtxoValidationRules[ruleIndex:ruleIndex+1],
	))
}

func TestDijkstraTotalCollateralEncodingPreservesExplicitZero(t *testing.T) {
	raw, err := cbor.Encode(map[uint]any{
		0:  cbor.NewSetType([]any{}, false),
		1:  []any{},
		2:  uint64(0),
		17: uint64(0),
	})
	require.NoError(t, err)
	var body DijkstraTransactionBody
	_, err = cbor.Decode(raw, &body)
	require.NoError(t, err)
	require.True(t, body.TotalCollateralPresent())
	body.SetCbor(nil)

	reencoded, err := cbor.Encode(&body)
	require.NoError(t, err)
	var fields map[uint]cbor.RawMessage
	_, err = cbor.Decode(reencoded, &fields)
	require.NoError(t, err)
	require.Contains(t, fields, uint(17))
}

func TestDijkstraCollateralReturnIsCheckedByOutputRules(t *testing.T) {
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkMainnet,
		make([]byte, common.AddressHashSize),
		make([]byte, common.AddressHashSize),
	)
	require.NoError(t, err)
	output := &DijkstraTransactionOutput{Output: &babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1},
	}}
	tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
		TxCollateralReturn: output,
	}}
	state := mockledger.NewLedgerStateBuilder().
		WithNetworkId(uint(common.AddressNetworkTestnet)).
		Build()
	pp := &DijkstraProtocolParameters{ConwayProtocolParameters: conway.ConwayProtocolParameters{
		AdaPerUtxoByte: 1,
		MaxValueSize:   0,
	}}
	require.Error(t, UtxoValidateWrongNetwork(tx, 0, state, pp))
	require.Error(t, UtxoValidateOutputTooSmallUtxo(tx, 0, state, pp))
	require.Error(t, UtxoValidateOutputTooBigUtxo(tx, 0, state, pp))
	byronAddress, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{Payload: make([]byte, 100)},
	)
	require.NoError(t, err)
	output.Output = &babbage.BabbageTransactionOutput{
		OutputAddress: byronAddress,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1},
	}
	require.Error(t, UtxoValidateOutputBootAddrAttrsTooBig(tx, 0, state, pp))
}

func TestDijkstraInsufficientCollateralUsesNetAmount(t *testing.T) {
	input := shelley.NewShelleyTransactionInput("d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22", 0)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxFee:        1_000_000,
			TxCollateral: cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false),
			TxCollateralReturn: &DijkstraTransactionOutput{Output: &babbage.BabbageTransactionOutput{
				OutputAmount: mary.MaryTransactionOutputValue{Amount: 9_000_000},
			}},
		},
		TxIsValid: true,
		WitnessSet: DijkstraTransactionWitnessSet{WsRedeemers: DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {},
			},
		}},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
		Id:     input,
		Output: shelley.ShelleyTransactionOutput{OutputAmount: 10_000_000},
	}}).Build()
	pp := &DijkstraProtocolParameters{ConwayProtocolParameters: conway.ConwayProtocolParameters{
		CollateralPercentage: 150,
	}}
	require.Error(t, UtxoValidateInsufficientCollateral(tx, 0, state, pp))
}
