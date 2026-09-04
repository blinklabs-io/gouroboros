package babbage_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// TestCollateralKeyLockedOnlyForPhase2 is the blinklabs-io/dingo#3896
// regression.
//
// Collateral pays for phase-2 script execution that fails, so a transaction
// running no phase-2 scripts has nothing for it to cover. Preview transaction
// 9ce59ee0dc6abee0 at slot 15148509 carries two vkey witnesses, one native
// script, no Plutus scripts and no redeemers, and declares a collateral input
// at an enterprise-script address. Holding it to the key-locked rule rejected a
// canonical block and wedged the node.
func TestCollateralKeyLockedOnlyForPhase2(t *testing.T) {
	const txId = "811c9029fc79e6b552f54d857bf0db807c9f0d8b23dc9e1e37f382377018674f"
	// The real collateral address: enterprise with a script payment credential.
	scriptAddr, err := common.NewAddress(
		"addr_test1wrpe58z89f3kwtq2cslsqvvw9hzdep2jy2ykx9kty8xnamqycrwy6",
	)
	if err != nil {
		t.Fatalf("fixture address: %v", err)
	}
	input := shelley.NewShelleyTransactionInput(txId, 0)
	utxos := []common.Utxo{
		{
			Id: input,
			Output: babbage.BabbageTransactionOutput{
				OutputAddress: scriptAddr,
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: 100_000_000,
				},
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()

	newTx := func(withRedeemer bool) *babbage.BabbageTransaction {
		wits := babbage.BabbageTransactionWitnessSet{}
		wits.VkeyWitnesses = []common.VkeyWitness{
			{Vkey: []byte{0x01}, Signature: []byte{0x02}},
		}
		if withRedeemer {
			wits.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{
					{Tag: common.RedeemerTagSpend, Index: 0},
				},
			}
		}
		return &babbage.BabbageTransaction{
			Body: babbage.BabbageTransactionBody{
				TxCollateral: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{input}, false,
				),
			},
			WitnessSet: wits,
		}
	}

	t.Run("no phase-2 scripts: script collateral is accepted", func(t *testing.T) {
		if err := babbage.UtxoValidateCollateralVKeyWitnesses(
			newTx(false), 0, ls, &babbage.BabbageProtocolParameters{},
		); err != nil {
			t.Errorf(
				"a transaction running no phase-2 scripts has nothing for "+
					"collateral to cover, so it must not be held to the "+
					"key-locked rule; got: %v", err,
			)
		}
	})

	t.Run("phase-2 scripts: script collateral is still rejected", func(t *testing.T) {
		err := babbage.UtxoValidateCollateralVKeyWitnesses(
			newTx(true), 0, ls, &babbage.BabbageProtocolParameters{},
		)
		if err == nil {
			t.Error(
				"a transaction that runs phase-2 scripts must still be held " +
					"to the key-locked rule; the guard must not become a way " +
					"to skip it",
			)
		}
	})
}
