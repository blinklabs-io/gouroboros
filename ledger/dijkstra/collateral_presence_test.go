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
	raw, err := cbor.Encode(map[uint]any{17: uint64(0)})
	require.NoError(t, err)
	var body DijkstraTransactionBody
	_, err = cbor.Decode(raw, &body)
	require.NoError(t, err)
	require.True(t, body.TotalCollateralPresent())
	body.TxCollateral = cbor.NewSetType([]shelley.ShelleyTransactionInput{input}, false)
	body.TxCollateralReturn = &DijkstraTransactionOutput{Output: babbage.BabbageTransactionOutput{OutputAmount: mary.MaryTransactionOutputValue{Amount: 3_000_000}}}
	tx := &DijkstraTransaction{Body: body, WitnessSet: DijkstraTransactionWitnessSet{WsRedeemers: DijkstraRedeemers{Redeemers: map[common.RedeemerKey]common.RedeemerValue{{Tag: common.RedeemerTagSpend, Index: 0}: {}}}}}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{Id: input, Output: shelley.ShelleyTransactionOutput{OutputAmount: 5_000_000}}}).Build()
	require.Error(t, conway.UtxoValidateCollateralEqBalance(tx, 0, state, &conway.ConwayProtocolParameters{}))
}
