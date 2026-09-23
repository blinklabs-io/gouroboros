package babbage_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// collateralFixtureTxId is the input the collateral fixtures spend as collateral.
const collateralFixtureTxId = "811c9029fc79e6b552f54d857bf0db807c9f0d8b23dc9e1e37f382377018674f"

// productionRule returns the entry of babbage.UtxoValidationRules whose
// function identity matches want.
//
// The rules below must be exercised as they are wired, not as free functions:
// a rule dropped from the production slice would still pass a test that called
// it directly, and the point of these cases is what the node actually runs.
func productionRule(
	t *testing.T,
	name string,
	want common.UtxoValidationRuleFunc,
) common.UtxoValidationRuleFunc {
	t.Helper()
	wantId := reflect.ValueOf(want).Pointer()
	for _, rule := range babbage.UtxoValidationRules {
		if reflect.ValueOf(rule).Pointer() == wantId {
			return rule
		}
	}
	t.Fatalf("%s is not wired into babbage.UtxoValidationRules", name)
	return nil
}

func productionRuleByID(
	t *testing.T,
	id common.UtxoValidationRuleId,
) common.UtxoValidationRuleFunc {
	t.Helper()
	descriptors := babbage.UtxoValidationRuleDescriptors()
	for i, descriptor := range descriptors {
		if descriptor.Id == id {
			return babbage.UtxoValidationRules[i]
		}
	}
	t.Fatalf("%s is not registered in babbage.UtxoValidationRules", id)
	return nil
}

// collateralFixtureLedgerState holds the single collateral UTxO the fixtures
// spend: 100 ADA at an enterprise address with a script payment credential,
// which is the shape that wedged the node.
func collateralFixtureLedgerState(t *testing.T) common.LedgerState {
	t.Helper()
	// The real collateral address: enterprise with a script payment credential.
	scriptAddr, err := common.NewAddress(
		"addr_test1wrpe58z89f3kwtq2cslsqvvw9hzdep2jy2ykx9kty8xnamqycrwy6",
	)
	if err != nil {
		t.Fatalf("fixture address: %v", err)
	}
	return mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(collateralFixtureTxId, 0),
			Output: babbage.BabbageTransactionOutput{
				OutputAddress: scriptAddr,
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: 100_000_000,
				},
			},
		},
	}).Build()
}

// spendRedeemers is a witness-set redeemer map standing for phase-2 execution.
func spendRedeemers() alonzo.AlonzoRedeemers {
	return alonzo.AlonzoRedeemers{
		Redeemers: []alonzo.AlonzoRedeemer{
			{Tag: common.RedeemerTagSpend, Index: 0},
		},
	}
}

// collateralFixtureTx builds a transaction declaring the fixture collateral
// input, optionally with a redeemer and a total_collateral field.
func collateralFixtureTx(
	withRedeemer bool,
	totalCollateral uint64,
) *babbage.BabbageTransaction {
	wits := babbage.BabbageTransactionWitnessSet{}
	wits.VkeyWitnesses = []common.VkeyWitness{
		{Vkey: []byte{0x01}, Signature: []byte{0x02}},
	}
	if withRedeemer {
		wits.WsRedeemers = spendRedeemers()
	}
	return &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxTotalCollateral: totalCollateral,
			TxCollateral: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(collateralFixtureTxId, 0),
				},
				false,
			),
		},
		WitnessSet: wits,
		TxIsValid:  true,
	}
}

// withSubTxRedeemers wraps tx so that it also exposes a sub-transaction witness
// set carrying redeemers, standing in for the Dijkstra shape.
func withSubTxRedeemers(tx *babbage.BabbageTransaction) *subTxCarrier {
	return &subTxCarrier{
		BabbageTransaction: tx,
		subWitnessSets: []common.TransactionWitnessSet{
			babbage.BabbageTransactionWitnessSet{WsRedeemers: spendRedeemers()},
		},
	}
}

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
	ls := collateralFixtureLedgerState(t)
	pp := &babbage.BabbageProtocolParameters{}
	rule := productionRuleByID(t, common.UtxoValidationRuleCollateralKeyLocked)

	t.Run("no phase-2 scripts: script collateral is accepted", func(t *testing.T) {
		if err := rule(collateralFixtureTx(false, 0), 0, ls, pp); err != nil {
			t.Errorf(
				"a transaction running no phase-2 scripts has nothing for "+
					"collateral to cover, so it must not be held to the "+
					"key-locked rule; got: %v", err,
			)
		}
	})

	t.Run("redeemers only in a sub-transaction still count", func(t *testing.T) {
		// A Dijkstra transaction can carry its redeemers in a sub-transaction.
		// Reading only the top-level witness set would report no phase-2
		// execution and skip the rule, which is the one direction this guard
		// must never fail in.
		tx := withSubTxRedeemers(collateralFixtureTx(false, 0))
		if err := rule(tx, 0, ls, pp); err == nil {
			t.Error(
				"redeemers in a sub-transaction mean phase-2 execution, so " +
					"the collateral rule must still apply",
			)
		}
	})

	t.Run("phase-2 scripts: script collateral is still rejected", func(t *testing.T) {
		if err := rule(collateralFixtureTx(true, 0), 0, ls, pp); err == nil {
			t.Error(
				"a transaction that runs phase-2 scripts must still be held " +
					"to the key-locked rule; the guard must not become a way " +
					"to skip it",
			)
		}
	})
}

// TestCollateralEqBalanceOnlyForPhase2 covers the remaining member of the
// collateral group.
//
// UtxoValidateCollateralEqBalance is Part 6 of the reference's
// validateTotalCollateral, which feesOK runs only when the redeemer map is
// non-empty. A transaction declaring collateral and a total_collateral field
// but running no phase-2 scripts must not be held to it — the same
// false-rejection shape as the key-locked rule.
//
// The fixture's collateral input holds 100 ADA and there is no collateral
// return, so a total_collateral of 1 ADA is a genuine mismatch: the rule
// rejects it whenever it runs.
func TestCollateralEqBalanceOnlyForPhase2(t *testing.T) {
	const mismatchedTotalCollateral = 1_000_000
	ls := collateralFixtureLedgerState(t)
	pp := &babbage.BabbageProtocolParameters{}
	rule := productionRule(
		t,
		"UtxoValidateCollateralEqBalance",
		babbage.UtxoValidateCollateralEqBalance,
	)

	t.Run("no phase-2 scripts: total_collateral is not checked", func(t *testing.T) {
		tx := collateralFixtureTx(false, mismatchedTotalCollateral)
		if err := rule(tx, 0, ls, pp); err != nil {
			t.Errorf(
				"total_collateral is checked inside feesOK's redeemer guard, "+
					"so a transaction running no phase-2 scripts must not be "+
					"held to it; got: %v", err,
			)
		}
	})

	t.Run("redeemers only in a sub-transaction still count", func(t *testing.T) {
		tx := withSubTxRedeemers(
			collateralFixtureTx(false, mismatchedTotalCollateral),
		)
		if err := rule(tx, 0, ls, pp); err == nil {
			t.Error(
				"redeemers in a sub-transaction mean phase-2 execution, so " +
					"total_collateral must still be checked",
			)
		}
	})

	t.Run("phase-2 scripts: total_collateral is still checked", func(t *testing.T) {
		tx := collateralFixtureTx(true, mismatchedTotalCollateral)
		if err := rule(tx, 0, ls, pp); err == nil {
			t.Error(
				"a transaction that runs phase-2 scripts must still be held " +
					"to the total_collateral rule; the guard must not become " +
					"a way to skip it",
			)
		}
	})
}

// keyLockedCollateralFixtureTxId is the input the key-locked collateral
// fixtures spend as collateral, distinct from collateralFixtureTxId's
// script-locked address.
const keyLockedCollateralFixtureTxId = "2222222222222222222222222222222222222222222222222222222222222222"

// keyLockedCollateralVkey backs the key-locked fixture address's payment
// credential.
var keyLockedCollateralVkey = bytes.Repeat([]byte{0x42}, 32)

func keyLockedCollateralKeyHash() common.Blake2b224 {
	return common.Blake2b224Hash(keyLockedCollateralVkey)
}

// keyLockedCollateralFixtureLedgerState holds a single collateral UTxO at a
// key-locked enterprise address, matching keyLockedCollateralVkey.
func keyLockedCollateralFixtureLedgerState(t *testing.T) common.LedgerState {
	t.Helper()
	keyAddr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		keyLockedCollateralKeyHash().Bytes(),
		nil,
	)
	if err != nil {
		t.Fatalf("fixture address: %v", err)
	}
	return mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(
				keyLockedCollateralFixtureTxId,
				0,
			),
			Output: babbage.BabbageTransactionOutput{
				OutputAddress: keyAddr,
				OutputAmount: mary.MaryTransactionOutputValue{
					Amount: 100_000_000,
				},
			},
		},
	}).Build()
}

// keyLockedCollateralFixtureTx builds a transaction declaring the key-locked
// fixture collateral input, optionally with a redeemer (phase-2 execution)
// and optionally with a vkey witness matching the fixture's key hash.
func keyLockedCollateralFixtureTx(
	withRedeemer bool,
	withMatchingVkey bool,
) *babbage.BabbageTransaction {
	wits := babbage.BabbageTransactionWitnessSet{}
	if withMatchingVkey {
		wits.VkeyWitnesses = []common.VkeyWitness{
			{Vkey: keyLockedCollateralVkey, Signature: []byte{0x02}},
		}
	}
	if withRedeemer {
		wits.WsRedeemers = spendRedeemers()
	}
	return &babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxCollateral: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{
					shelley.NewShelleyTransactionInput(
						keyLockedCollateralFixtureTxId,
						0,
					),
				},
				false,
			),
		},
		WitnessSet: wits,
	}
}

// TestCollateralVKeyWitnessRequiredWithoutPhase2 is the
// blinklabs-io/dingo#4350 regression.
//
// witsVKeyNeeded (Alonzo adds collateral inputs to that set) is not
// redeemer-gated in the reference -- only the key-locked/script-locked rule
// (validateScriptsNotPaidUTxO, blinklabs-io/dingo#3896) is. A key-locked
// collateral input with no matching vkey witness must therefore be rejected
// even when the transaction runs no phase-2 scripts at all.
func TestCollateralVKeyWitnessRequiredWithoutPhase2(t *testing.T) {
	ls := keyLockedCollateralFixtureLedgerState(t)
	pp := &babbage.BabbageProtocolParameters{}
	rule := productionRule(
		t,
		"UtxoValidateCollateralVKeyWitnesses",
		babbage.UtxoValidateCollateralVKeyWitnesses,
	)

	t.Run("no phase-2, no vkey witness: key-locked collateral is rejected", func(t *testing.T) {
		tx := keyLockedCollateralFixtureTx(false, false)
		if err := rule(tx, 0, ls, pp); err == nil {
			t.Error(
				"witsVKeyNeeded is not redeemer-gated in the reference, so a " +
					"key-locked collateral input with no matching vkey " +
					"witness must be rejected even when the transaction runs " +
					"no phase-2 scripts",
			)
		}
	})

	t.Run("no phase-2, matching vkey witness: key-locked collateral is accepted", func(t *testing.T) {
		tx := keyLockedCollateralFixtureTx(false, true)
		if err := rule(tx, 0, ls, pp); err != nil {
			t.Errorf(
				"a key-locked collateral input with a matching vkey witness "+
					"must be accepted regardless of phase-2 execution; got: %v",
				err,
			)
		}
	})

	t.Run("phase-2, no vkey witness: key-locked collateral is still rejected", func(t *testing.T) {
		tx := keyLockedCollateralFixtureTx(true, false)
		if err := rule(tx, 0, ls, pp); err == nil {
			t.Error(
				"a key-locked collateral input with no matching vkey " +
					"witness must be rejected when the transaction runs " +
					"phase-2 scripts too",
			)
		}
	})

	t.Run("no phase-2: script-locked collateral is accepted with zero vkey witnesses", func(t *testing.T) {
		// Same shape as TestCollateralKeyLockedOnlyForPhase2, but with zero
		// vkey witnesses at all (not even one for an unrelated input), to
		// pin that the vkey-witness membership check never applies to a
		// script-locked collateral input: only the key-locked/script-locked
		// rule is phase-2-gated, and it is not held to needing any witness.
		tx := &babbage.BabbageTransaction{
			Body: babbage.BabbageTransactionBody{
				TxCollateral: cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{
						shelley.NewShelleyTransactionInput(
							collateralFixtureTxId,
							0,
						),
					},
					false,
				),
			},
		}
		if err := rule(tx, 0, collateralFixtureLedgerState(t), pp); err != nil {
			t.Errorf(
				"script-locked collateral with no phase-2 execution and no "+
					"vkey witnesses at all must still be accepted; got: %v",
				err,
			)
		}
	})
}

// subTxCarrier is a Babbage transaction that also exposes sub-transaction
// witness sets, standing in for the Dijkstra shape without pulling that era in.
type subTxCarrier struct {
	*babbage.BabbageTransaction
	subWitnessSets []common.TransactionWitnessSet
}

func (t *subTxCarrier) SubTransactionWitnessSets() []common.TransactionWitnessSet {
	return t.subWitnessSets
}
