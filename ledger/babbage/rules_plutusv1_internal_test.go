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
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "build key address")
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
	require.NoError(t, err, "build script address")
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
		Type:   scriptRefType(s),
		Script: s,
	}
	return out
}

// scriptRefType tags the ref with the version of the script it carries. The
// rule reads ScriptRef().Script and never the tag, but a fixture whose tag
// contradicts its script would mislead the next reader.
func scriptRefType(s common.Script) uint {
	switch s.(type) {
	case common.PlutusV1Script:
		return common.ScriptRefTypePlutusV1
	case common.PlutusV2Script:
		return common.ScriptRefTypePlutusV2
	case common.PlutusV3Script:
		return common.ScriptRefTypePlutusV3
	case common.PlutusV4Script:
		return common.ScriptRefTypePlutusV4
	default:
		return common.ScriptRefTypeNativeScript
	}
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
	require.NoError(t, runRule(t, tx, []common.Utxo{
		utxo(inlineIn, inlineDatumOutput(key)),
		utxo(refScriptIn, scriptRefOutput(key, v1)),
	}), "expected accept")
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
	require.ErrorAs(t, err, &want)
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
	require.ErrorAs(t, err, &want)
}

// A reference script on a produced output must NOT disqualify a transaction that
// needs a PlutusV1 script. Conway's transTxOutV1 checks only the inline datum;
// unlike the Babbage-era transTxOutV1 it shadows, it carries no
// ReferenceScriptsNotSupported branch
// (eras/conway/impl/src/Cardano/Ledger/Conway/TxInfo.hs). No conformance vector
// exercises this shape, so only this test holds the line.
func TestInlineDatumsPlutusV1_ScriptRefOnProducedOutputAccepted(t *testing.T) {
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
	require.NoError(t, runRule(t, tx, []common.Utxo{
		utxo(in, plainOutput(scriptAddr(t, v1))),
	}), "a reference script on a produced output is not a V1 violation")
}

// The same asymmetry on the input side: a needed PlutusV1 script spending a
// regular input that carries a reference script must be accepted. This is the
// shape of the Conway conformance vector
// "UTXOS/can use regular inputs for reference", which fails outright if the rule
// rejects a reference script on a consumed input.
func TestInlineDatumsPlutusV1_ScriptRefOnConsumedInputAccepted(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x11, 0x12})
	spend := testInput(0xc, 0)
	refCarrier := testInput(0xd, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(spend, refCarrier),
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	require.NoError(t, runRule(t, tx, []common.Utxo{
		utxo(spend, plainOutput(scriptAddr(t, v1))),
		utxo(refCarrier, scriptRefOutput(keyAddr(t), v1)),
	}), "a reference script on a consumed input is not a V1 violation")
}

// An inline datum on a *reference* input is disqualifying, because Conway's V1
// instance translates reference inputs through the same transTxOutV1 as
// consumed inputs (under mapM_) and discards only the result, not the failure.
// The pre-PR implementation missed this: it scanned reference inputs for V1
// script refs and never for datums.
func TestInlineDatumsPlutusV1_DatumOnReferenceInputRejected(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x13, 0x14})
	spend := testInput(0xe, 0)
	ref := testInput(0xf, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(spend),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{ref},
				false,
			),
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{v1},
		},
	}
	err := runRule(t, tx, []common.Utxo{
		utxo(spend, plainOutput(scriptAddr(t, v1))),
		utxo(ref, inlineDatumOutput(keyAddr(t))),
	})
	var want common.InlineDatumsNotSupportedError
	require.ErrorAs(
		t,
		err,
		&want,
		"an inline datum on a reference input must be rejected",
	)
}

// The restriction is keyed on the Plutus *version* of the needed script, not on
// needing a script at all: a needed PlutusV2 script spending an inline-datum
// output is exactly what inline datums were introduced for. This pins the
// NeedsAny version filter against a future widening to all Plutus versions.
func TestInlineDatumsPlutusV2_NeededSpendingScriptAccepted(t *testing.T) {
	v2 := common.PlutusV2Script([]byte{0x15, 0x16})
	in := testInput(0x10, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs:  inputSet(in),
			TxOutputs: []BabbageTransactionOutput{inlineDatumOutput(keyAddr(t))},
		},
		WitnessSet: BabbageTransactionWitnessSet{
			WsPlutusV2Scripts: []common.PlutusV2Script{v2},
		},
	}
	require.NoError(t, runRule(t, tx, []common.Utxo{
		utxo(in, inlineDatumOutput(scriptAddr(t, v2))),
	}), "inline datums are supported for PlutusV2")
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
	require.NoError(t, runRule(t, tx, []common.Utxo{
		utxo(in, plainOutput(scriptAddr(t, v1))),
		utxo(ref, plainOutput(keyAddr(t))),
	}), "expected accept with a reference input present")
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
	require.NoError(t, runRule(t, tx, []common.Utxo{utxo(in, out)}), "expected accept for datum-hash output")
}

// An unresolvable input is UtxoValidateBadInputsUtxo's finding, not this rule's.
func TestInlineDatumsPlutusV1_UnresolvableInputSkipped(t *testing.T) {
	in := testInput(0x9, 0)
	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{TxInputs: inputSet(in)},
	}
	require.NoError(
		t,
		runRule(t, tx, nil),
		"an unresolvable input is BadInputsUtxo's finding, not this rule's",
	)
}

// TestInlineDatumsPlutusV1_NeededScriptFromReferenceScriptRejected settles
// whether a needed PlutusV1 script supplied only by a reference script -- never
// present in the witness set -- is detected. availableScripts collects script
// refs from every resolved input, so it is.
func TestInlineDatumsPlutusV1_NeededScriptFromReferenceScriptRejected(
	t *testing.T,
) {
	v1 := common.PlutusV1Script([]byte{0x0d, 0x0e})
	spend := testInput(0xa, 0)
	ref := testInput(0xb, 0)

	tx := &BabbageTransaction{
		Body: BabbageTransactionBody{
			TxInputs: inputSet(spend),
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{ref},
				false,
			),
		},
		// Deliberately no witness scripts: the only copy of the V1 script is
		// the reference script on the reference input below.
	}
	err := runRule(t, tx, []common.Utxo{
		// Spending a UTxO locked by the V1 script, carrying an inline datum.
		utxo(spend, inlineDatumOutput(scriptAddr(t, v1))),
		utxo(ref, scriptRefOutput(keyAddr(t), v1)),
	})
	var want common.InlineDatumsNotSupportedError
	require.ErrorAs(
		t,
		err,
		&want,
		"a needed PlutusV1 script supplied by a reference script must be detected",
	)
}
