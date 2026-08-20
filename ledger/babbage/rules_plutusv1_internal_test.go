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

package babbage

import (
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

// UtxoValidateInlineDatumsWithPlutusV1 had no test coverage at all, which is
// how it came to reject transactions cardano-node accepts. These pin both
// directions: a PlutusV1 script that is merely reachable must not invalidate a
// transaction, and one that is actually required must, wherever the
// V1-incompatible feature sits.

func testInput(b byte, idx uint32) shelley.ShelleyTransactionInput {
	return shelley.NewShelleyTransactionInput(
		hexRepeat(b),
		int(idx), // #nosec G115 -- test index
	)
}

func hexRepeat(b byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 64)
	for i := range out {
		out[i] = hexDigits[int(b)%16]
	}
	return string(out)
}

func inputSet(
	inputs ...shelley.ShelleyTransactionInput,
) shelley.ShelleyTransactionInputSet {
	return shelley.NewShelleyTransactionInputSet(inputs)
}

func keyAddr(t *testing.T) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		make([]byte, common.AddressHashSize),
		nil,
	)
	if err != nil {
		t.Fatalf("build key address: %v", err)
	}
	return addr
}

func scriptAddr(t *testing.T, s common.Script) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone,
		common.AddressNetworkTestnet,
		s.Hash().Bytes(),
		nil,
	)
	if err != nil {
		t.Fatalf("build script address: %v", err)
	}
	return addr
}

func plainOutput(addr common.Address) BabbageTransactionOutput {
	return BabbageTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 1_000_000},
	}
}

func inlineDatumOutput(addr common.Address) BabbageTransactionOutput {
	datum := common.Datum{Data: data.NewInteger(big.NewInt(42))}
	out := plainOutput(addr)
	out.DatumOption = &BabbageTransactionOutputDatumOption{data: &datum}
	return out
}

func scriptRefOutput(
	addr common.Address,
	s common.Script,
) BabbageTransactionOutput {
	out := plainOutput(addr)
	out.TxOutScriptRef = &common.ScriptRef{
		Type:   common.ScriptRefTypePlutusV1,
		Script: s,
	}
	return out
}

// utxo stores the output as a *pointer*, matching how
// BabbageTransactionBody.Outputs() represents them (&b.TxOutputs[i]). Using a
// value here would make the fixture diverge from real resolution and, worse,
// would silently dodge upstream's *BabbageTransactionOutput type assertion, so
// a comparison against upstream would measure the fixture rather than the rule.
func utxo(
	in shelley.ShelleyTransactionInput,
	out BabbageTransactionOutput,
) common.Utxo {
	return common.Utxo{Id: in, Output: &out}
}

// utxoOnlyLedgerState implements just the UtxoById lookup this rule performs.
// ouroboros-mock's builder cannot be used from an in-package test here: it
// imports this package, so importing it back is an import cycle. The embedded
// interface is nil on purpose -- any other method this rule started calling
// would panic loudly rather than silently returning a zero value.
type utxoOnlyLedgerState struct {
	common.LedgerState
	utxos map[string]common.Utxo
}

func (l utxoOnlyLedgerState) UtxoById(
	id common.TransactionInput,
) (common.Utxo, error) {
	utxo, ok := l.utxos[id.String()]
	if !ok {
		return common.Utxo{}, errors.New("utxo not found")
	}
	return utxo, nil
}

func runRule(
	t *testing.T,
	tx common.Transaction,
	utxos []common.Utxo,
) error {
	t.Helper()
	byId := make(map[string]common.Utxo, len(utxos))
	for _, u := range utxos {
		byId[u.Id.String()] = u
	}
	return UtxoValidateInlineDatumsWithPlutusV1(
		tx,
		0,
		utxoOnlyLedgerState{utxos: byId},
		&BabbageProtocolParameters{},
	)
}

// A PlutusV1 script reachable through a spent UTxO's reference script, with no
// purpose requiring it, must not invalidate the transaction. Rejecting here is
// the false positive that halted a node's sync on a real Preview transaction.
func TestInlineDatumsPlutusV1_UnneededReferenceScriptAccepted(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x01, 0x02})
	key := keyAddr(t)
	inlineIn := testInput(0x1, 0)
	refScriptIn := testInput(0x2, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(inlineIn, refScriptIn),
		},
	}
	if err := runRule(t, tx, []common.Utxo{
		utxo(inlineIn, inlineDatumOutput(key)),
		utxo(refScriptIn, scriptRefOutput(key, v1)),
	}); err != nil {
		t.Fatalf("expected accept, got %v", err)
	}
}

// The same script, now actually required to spend its own script-locked UTxO.
func TestInlineDatumsPlutusV1_NeededSpendingScriptRejected(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x03, 0x04})
	in := testInput(0x3, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{TxInputs: inputSet(in)},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	err := runRule(t, tx, []common.Utxo{
		utxo(in, inlineDatumOutput(scriptAddr(t, v1))),
	})
	var want common.InlineDatumsNotSupportedError
	if !errors.As(err, &want) {
		t.Fatalf("expected InlineDatumsNotSupportedError, got %v", err)
	}
}

// The inline datum sits on a produced output rather than a spent one.
func TestInlineDatumsPlutusV1_DatumOnProducedOutputRejected(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x05, 0x06})
	in := testInput(0x4, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs:  inputSet(in),
			TxOutputs: []BabbageTransactionOutput{inlineDatumOutput(keyAddr(t))},
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	err := runRule(t, tx, []common.Utxo{
		utxo(in, plainOutput(scriptAddr(t, v1))),
	})
	var want common.InlineDatumsNotSupportedError
	if !errors.As(err, &want) {
		t.Fatalf("expected InlineDatumsNotSupportedError, got %v", err)
	}
}

// A reference script on a produced output is equally unrepresentable in V1.
func TestInlineDatumsPlutusV1_ScriptRefOnProducedOutputRejected(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x07, 0x08})
	in := testInput(0x5, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(in),
			TxOutputs: []BabbageTransactionOutput{
				scriptRefOutput(keyAddr(t), v1),
			},
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	err := runRule(t, tx, []common.Utxo{
		utxo(in, plainOutput(scriptAddr(t, v1))),
	})
	var want common.InlineDatumsNotSupportedError
	if !errors.As(err, &want) {
		t.Fatalf("expected InlineDatumsNotSupportedError, got %v", err)
	}
}

// The mere presence of a reference input must NOT disqualify a transaction that
// requires a PlutusV1 script. The Conway conformance vector
// "UTXOS/can use reference scripts" (tx 3) expects exactly this to succeed, and
// an earlier revision of this rule failed it by rejecting on reference-input
// presence.
func TestInlineDatumsPlutusV1_ReferenceInputPresentAccepted(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x09, 0x0a})
	in := testInput(0x6, 0)
	ref := testInput(0x7, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(in),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{ref},
				false,
			),
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	if err := runRule(t, tx, []common.Utxo{
		utxo(in, plainOutput(scriptAddr(t, v1))),
		utxo(ref, plainOutput(keyAddr(t))),
	}); err != nil {
		t.Fatalf("expected accept with a reference input present, got %v", err)
	}
}

// A datum *hash* output is not an inline datum and must be accepted.
func TestInlineDatumsPlutusV1_DatumHashOutputAccepted(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x0b, 0x0c})
	in := testInput(0x8, 0)
	hash := common.Blake2b256{}
	out := plainOutput(scriptAddr(t, v1))
	out.DatumOption = &BabbageTransactionOutputDatumOption{hash: &hash}

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{TxInputs: inputSet(in)},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	if err := runRule(t, tx, []common.Utxo{utxo(in, out)}); err != nil {
		t.Fatalf("expected accept for datum-hash output, got %v", err)
	}
}

// An unresolvable input is UtxoValidateBadInputsUtxo's finding, not this rule's.
func TestInlineDatumsPlutusV1_UnresolvableInputSkipped(t *testing.T) {
	in := testInput(0x9, 0)
	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{TxInputs: inputSet(in)},
	}
	if err := runRule(t, tx, nil); err != nil {
		t.Fatalf("expected nil for unresolvable input, got %v", err)
	}
}
