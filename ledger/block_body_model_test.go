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

// A table test of the block-body rule (BBODY with its LEDGERS fold):
// the invalid-transaction list and the validity flags it yields, the order
// in which a block's transactions apply (a later one sees an earlier one's
// outputs; a phase-2-invalid one consumes only its collateral and adds only
// its collateral return), the block's execution-unit total, and its
// reference-script total before and from protocol version 11. The rules
// below restate each era's Rules/Bbody.hs and Rules/Ledgers.hs in
// cardano-ledger.
//
// A test transaction names one spent input, one collateral input and an
// optional reference input relative to itself: "g" is a genesis output
// carrying a reference script, "k" a genesis output without one, "prev"
// the output of the transaction before, "prevret" its collateral return.
// The test enumerates every block over a small transaction space, builds
// each as real Babbage or Conway transactions, runs gOuroboros, and checks
// that it decides as the restated reference rules do.

package ledger

import (
	"bytes"
	"fmt"
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
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

type bbmTx struct {
	in, coll, ref string
	script, ex    int
}

type bbmBlock struct {
	txs     []bbmTx
	invalid []int
}

func (b bbmBlock) String() string {
	return fmt.Sprintf("%+v invalid=%v", b.txs, b.invalid)
}

// bbmOutput identifies a test output: "g" or "k" at 0, or "o" (output)
// and "r" (collateral return) of the 1-based transaction n.
type bbmOutput struct {
	kind string
	n    int
}

var (
	bbmGenesis    = bbmOutput{"g", 0}
	bbmCollateral = bbmOutput{"k", 0}
)

// bbmAligned is the reference's alignedValidFlags, word for word.
func bbmAligned(prev, n int, xs []int) []bool {
	rep := func(k int) []bool {
		ret := []bool{}
		for range max(k, 0) {
			ret = append(ret, true)
		}
		return ret
	}
	if len(xs) == 0 {
		return rep(n)
	}
	x := xs[0]
	ret := append(rep(x-prev-1), false)
	return append(ret, bbmAligned(x, n-(x-prev), xs[1:])...)
}

// bbmFlags are the reference's validity flags: the aligned flags cut to the
// transaction count, as the decoder zips them with the transactions.
func bbmFlags(b bbmBlock) []bool {
	aligned := bbmAligned(-1, len(b.txs), b.invalid)
	flags := make([]bool, len(b.txs))
	for i := range flags {
		flags[i] = i >= len(aligned) || aligned[i]
	}
	return flags
}

func bbmDecodeOk(b bbmBlock) bool {
	for _, idx := range b.invalid {
		if idx >= len(b.txs) {
			return false
		}
	}
	return true
}

// bbmResolve resolves an input name of the 1-based transaction i.
func bbmResolve(i int, name string) bbmOutput {
	switch name {
	case "g":
		return bbmGenesis
	case "k":
		return bbmCollateral
	case "prev":
		return bbmOutput{"o", i - 1}
	case "prevret":
		return bbmOutput{"r", i - 1}
	}
	panic("unknown model input " + name)
}

func bbmOutOf(i int, valid bool) bbmOutput {
	if valid {
		return bbmOutput{"o", i}
	}
	return bbmOutput{"r", i}
}

func bbmNeeds(b bbmBlock, i int) []bbmOutput {
	tx := b.txs[i-1]
	ret := []bbmOutput{bbmResolve(i, tx.in), bbmResolve(i, tx.coll)}
	if tx.ref != "none" {
		ret = append(ret, bbmResolve(i, tx.ref))
	}
	return ret
}

// bbmLedgerOk reports whether the reference's LEDGERS fold accepts the
// block: every transaction finds what it names among what the earlier ones
// left.
func bbmLedgerOk(b bbmBlock) bool {
	flags := bbmFlags(b)
	avail := map[bbmOutput]bool{bbmGenesis: true, bbmCollateral: true}
	for i := 1; i <= len(b.txs); i++ {
		for _, need := range bbmNeeds(b, i) {
			if !avail[need] {
				return false
			}
		}
		tx := b.txs[i-1]
		if flags[i-1] {
			delete(avail, bbmResolve(i, tx.in))
		} else {
			delete(avail, bbmResolve(i, tx.coll))
		}
		avail[bbmOutOf(i, flags[i-1])] = true
	}
	return true
}

// bbmRefTotal is the reference's block reference-script total in script
// units (Conway Rules/Bbody.hs): through protocol
// version 10 each transaction is measured against the start; from 11
// against the start and every earlier transaction's output or return.
func bbmRefTotal(b bbmBlock, accumulates bool) int {
	flags := bbmFlags(b)
	measured := map[bbmOutput]bool{bbmGenesis: true, bbmCollateral: true}
	scriptAt := func(o bbmOutput) int {
		if !measured[o] {
			return 0
		}
		if o == bbmGenesis {
			return 1
		}
		if o.kind == "o" && o.n >= 1 && o.n <= len(b.txs) {
			return b.txs[o.n-1].script
		}
		return 0
	}
	total := 0
	for i := 1; i <= len(b.txs); i++ {
		tx := b.txs[i-1]
		ids := map[bbmOutput]bool{bbmResolve(i, tx.in): true}
		if tx.ref != "none" {
			ids[bbmResolve(i, tx.ref)] = true
		}
		for id := range ids {
			total += scriptAt(id)
		}
		if accumulates {
			measured[bbmOutOf(i, flags[i-1])] = true
		}
	}
	return total
}

func bbmExTotal(b bbmBlock) int {
	total := 0
	for _, tx := range b.txs {
		total += tx.ex
	}
	return total
}

func bbmTxSpace(
	ins, colls, refs []string,
	scripts, exs []int,
) []bbmTx {
	var ret []bbmTx
	for _, in := range ins {
		for _, coll := range colls {
			for _, ref := range refs {
				for _, script := range scripts {
					for _, ex := range exs {
						ret = append(ret, bbmTx{in, coll, ref, script, ex})
					}
				}
			}
		}
	}
	return ret
}

func bbmTxLists(space []bbmTx, maxTxs int) [][]bbmTx {
	ret := [][]bbmTx{{}}
	prev := [][]bbmTx{{}}
	for range maxTxs {
		var next [][]bbmTx
		for _, list := range prev {
			for _, tx := range space {
				next = append(next, append(append([]bbmTx{}, list...), tx))
			}
		}
		ret = append(ret, next...)
		prev = next
	}
	return ret
}

func bbmInvalidLists(maxLen, maxIdx int) [][]int {
	ret := [][]int{{}}
	prev := [][]int{{}}
	for range maxLen {
		var next [][]int
		for _, list := range prev {
			for idx := 0; idx <= maxIdx; idx++ {
				next = append(next, append(append([]int{}, list...), idx))
			}
		}
		ret = append(ret, next...)
		prev = next
	}
	return ret
}

// bbmBlocks lists every block over the given transaction space, with the
// envelope fields fixed.
func bbmBlocks(space []bbmTx, maxTxs, maxInvalidLen int) []bbmBlock {
	var ret []bbmBlock
	for _, txs := range bbmTxLists(space, maxTxs) {
		for _, invalid := range bbmInvalidLists(maxInvalidLen, maxTxs) {
			ret = append(ret, bbmBlock{txs: txs, invalid: invalid})
		}
	}
	return ret
}

// bbmBuilt is a test block built as real transactions.
type bbmBuilt struct {
	block  *babbage.BabbageBlock
	txs    []common.Transaction
	state  common.LedgerState
	script common.PlutusV2Script
}

func bbmInput(t *testing.T, id common.Blake2b256, index uint32) shelley.ShelleyTransactionInput {
	t.Helper()
	input, err := shelley.NewShelleyTransactionInput(id.String(), int(index))
	require.NoError(t, err)
	return input
}

// bbmBuild builds b as Babbage transactions. The genesis outputs are at
// fixed IDs; "prev" and "prevret" of transaction i are outputs 0 and 1 of
// transaction i-1, whose collateral return follows its one output. The
// first transaction's "prev" names an ID nothing produced.
func bbmBuild(t *testing.T, b bbmBlock) bbmBuilt {
	t.Helper()
	addr, err := common.NewAddress(sequentialTestAddress)
	require.NoError(t, err)
	script := common.PlutusV2Script(bytes.Repeat([]byte{0x4d}, 10))
	output := func(withScript bool) babbage.BabbageTransactionOutput {
		ret := babbage.BabbageTransactionOutput{
			OutputAddress: addr,
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: 2_000_000},
		}
		if withScript {
			ret.TxOutScriptRef = &common.ScriptRef{
				Type:   common.ScriptRefTypePlutusV2,
				Script: script,
			}
		}
		return ret
	}
	genesisId := common.Blake2b256(bytes.Repeat([]byte{0x01}, 32))
	collId := common.Blake2b256(bytes.Repeat([]byte{0x02}, 32))
	missingId := common.Blake2b256(bytes.Repeat([]byte{0xee}, 32))
	genesisOut := output(true)
	collOut := output(false)
	// The UtxoState contract: a missing input wraps common.ErrUtxoNotFound,
	// which the reference-script measure reads as "contributes nothing".
	utxos := []common.Utxo{
		{Id: bbmInput(t, genesisId, 0), Output: &genesisOut},
		{Id: bbmInput(t, collId, 0), Output: &collOut},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(id common.TransactionInput) (common.Utxo, error) {
			for _, utxo := range utxos {
				if utxo.Id.Id() == id.Id() && utxo.Id.Index() == id.Index() {
					return utxo, nil
				}
			}
			return common.Utxo{}, fmt.Errorf(
				"input %s: %w", id, common.ErrUtxoNotFound,
			)
		},
	).Build()

	flags := bbmFlags(b)
	built := bbmBuilt{
		block: &babbage.BabbageBlock{
			BlockHeader: &babbage.BabbageBlockHeader{},
		},
		state:  state,
		script: script,
	}
	for _, idx := range b.invalid {
		built.block.InvalidTransactions = append(
			built.block.InvalidTransactions,
			uint(idx), // #nosec G115
		)
	}
	prevId := missingId
	for i, tx := range b.txs {
		resolve := func(name string) shelley.ShelleyTransactionInput {
			switch name {
			case "g":
				return bbmInput(t, genesisId, 0)
			case "k":
				return bbmInput(t, collId, 0)
			case "prev":
				return bbmInput(t, prevId, 0)
			case "prevret":
				return bbmInput(t, prevId, 1)
			}
			t.Fatalf("unknown model input %s", name)
			return shelley.ShelleyTransactionInput{}
		}
		ret := output(false)
		body := babbage.BabbageTransactionBody{
			TxOutputs:          []babbage.BabbageTransactionOutput{output(tx.script == 1)},
			TxFee:              uint64(i + 1), // #nosec G115
			TxCollateralReturn: &ret,
			TxCollateral: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{resolve(tx.coll)},
				false,
			),
		}
		body.TxInputs = shelley.NewShelleyTransactionInputSet(
			[]shelley.ShelleyTransactionInput{resolve(tx.in)},
		)
		if tx.ref != "none" {
			body.TxReferenceInputs = cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{resolve(tx.ref)},
				false,
			)
		}
		witness := babbage.BabbageTransactionWitnessSet{}
		if tx.ex > 0 {
			witness.WsRedeemers = alonzo.AlonzoRedeemers{
				Redeemers: []alonzo.AlonzoRedeemer{{
					Tag: common.RedeemerTagSpend,
					Data: common.Datum{
						Data: data.NewInteger(big.NewInt(0)),
					},
					ExUnits: common.ExUnits{
						Memory: int64(tx.ex),
						Steps:  int64(tx.ex),
					},
				}},
			}
		}
		built.block.TransactionBodies = append(
			built.block.TransactionBodies,
			body,
		)
		built.block.TransactionWitnessSets = append(
			built.block.TransactionWitnessSets,
			witness,
		)
		built.txs = append(built.txs, &babbage.BabbageTransaction{
			Body:       body,
			WitnessSet: witness,
			TxIsValid:  flags[i],
		})
		prevId = body.Id()
	}
	return built
}

// bbmResolveAllInputs resolves every input a transaction names, as the
// reference's BadInputs does whatever IsValid says. A test transaction's
// "valid" means only that.
func bbmResolveAllInputs(
	tx common.Transaction,
	_ uint64,
	ls common.LedgerState,
	_ common.ProtocolParameters,
) error {
	inputs := append([]common.TransactionInput{}, tx.Inputs()...)
	inputs = append(inputs, tx.Collateral()...)
	inputs = append(inputs, tx.ReferenceInputs()...)
	for _, input := range inputs {
		if _, err := ls.UtxoById(input); err != nil {
			return fmt.Errorf("input %s: %w", input, err)
		}
	}
	return nil
}

// TestBlockBodyModelValidityFlags checks gOuroboros' validity flags against
// the reference's alignedValidFlags on every list of up to three indexes
// for up to three transactions, and the block decoder's index range rule.
func TestBlockBodyModelValidityFlags(t *testing.T) {
	for n := 0; n <= 3; n++ {
		for _, list := range bbmInvalidLists(3, 3) {
			b := bbmBlock{txs: make([]bbmTx, n), invalid: list}
			if !bbmDecodeOk(b) {
				continue
			}
			wire := make([]uint, len(list))
			for i, idx := range list {
				wire[i] = uint(idx) // #nosec G115
			}
			require.Equal(
				t,
				bbmFlags(b),
				common.TransactionValidityFlags(n, wire),
				"n=%d list=%v",
				n,
				list,
			)
		}
	}
}

// TestBlockBodyModelLedgerOrder runs every Alonzo test block through
// verifyBlockTransactions: gOuroboros accepts exactly the blocks in which
// every transaction finds its inputs among what the earlier ones left.
func TestBlockBodyModelLedgerOrder(t *testing.T) {
	space := bbmTxSpace(
		[]string{"g", "prev", "prevret"},
		[]string{"k", "prevret"},
		[]string{"none", "g", "prev"},
		[]int{0},
		[]int{0},
	)
	cells, accepted := 0, 0
	for _, b := range bbmBlocks(space, 2, 2) {
		if !bbmDecodeOk(b) {
			continue
		}
		built := bbmBuild(t, b)
		_, err := verifyBlockTransactions(
			built.txs,
			0,
			built.state,
			&mockledger.MockProtocolParamsRules{},
			[]common.UtxoValidationRuleFunc{bbmResolveAllInputs},
		)
		want := bbmLedgerOk(b)
		require.Equal(t, want, err == nil, "%s: %v", b, err)
		cells++
		if want {
			accepted++
		}
	}
	require.Positive(t, accepted)
	require.Less(t, accepted, cells)
}

// TestBlockBodyModelExUnits checks the block's execution-unit total, over
// valid and invalid transactions alike, against the maximum in
// VerifyBlockState.
func TestBlockBodyModelExUnits(t *testing.T) {
	space := bbmTxSpace(
		[]string{"g"}, []string{"k"}, []string{"none"}, []int{0}, []int{0, 1},
	)
	for _, b := range bbmBlocks(space, 2, 1) {
		if !bbmDecodeOk(b) {
			continue
		}
		built := bbmBuild(t, b)
		for maxEx := 0; maxEx <= 2; maxEx++ {
			err := VerifyBlockState(built.block, common.VerifyConfig{
				SkipTransactionValidation: true,
				SkipStakePoolValidation:   true,
				ProtocolParameters: &babbage.BabbageProtocolParameters{
					MaxBlockExUnits: common.ExUnits{
						Memory: int64(maxEx),
						Steps:  int64(maxEx),
					},
				},
			})
			want := bbmExTotal(b) <= maxEx
			require.Equal(t, want, err == nil, "%s max=%d: %v", b, maxEx, err)
			if !want {
				require.ErrorIs(t, err, common.ErrBlockExUnitsTooBig)
			}
		}
	}
}

// TestBlockBodyModelRefScripts checks the block's reference-script total
// at protocol versions 10 and 11 against the reference's: against the start
// only, then also against every earlier transaction's output or return.
func TestBlockBodyModelRefScripts(t *testing.T) {
	space := bbmTxSpace(
		[]string{"g", "k", "prev"},
		[]string{"k"},
		[]string{"none", "g", "prev"},
		[]int{0, 1},
		[]int{0},
	)
	for _, accumulates := range []bool{false, true} {
		for _, b := range bbmBlocks(space, 2, 1) {
			if !bbmDecodeOk(b) {
				continue
			}
			built := bbmBuild(t, b)
			got, err := common.ConsumedReferenceScriptSizePerBlock(
				built.block,
				built.state,
				accumulates,
				nil,
			)
			require.NoError(t, err, "%s", b)
			want := bbmRefTotal(b, accumulates) *
				len(built.script.RawScriptBytes())
			require.Equal(
				t,
				uint64(want), // #nosec G115
				got,
				"%s accumulates=%v",
				b,
				accumulates,
			)
		}
	}
}

// TestBlockBodyModelConwayRefScriptLimit checks the Conway block limit's
// protocol-version switch on the cell that tells the versions apart: a
// script made by the first transaction and read by the second counts from
// version 11.
func TestBlockBodyModelConwayRefScriptLimit(t *testing.T) {
	b := bbmBlock{txs: []bbmTx{
		{in: "g", coll: "k", ref: "none", script: 1},
		{in: "k", coll: "k", ref: "prev"},
	}}
	built := bbmBuild(t, b)
	scriptLen := uint64(len(built.script.RawScriptBytes()))
	conwayBlock := &conway.ConwayBlock{}
	prevId := common.Blake2b256{}
	for i, src := range built.block.TransactionBodies {
		inputs := src.TxInputs.Items()
		refs := src.TxReferenceInputs.Items()
		if i == 1 {
			// The second transaction reads the first one's output.
			refs = []shelley.ShelleyTransactionInput{
				bbmInput(t, prevId, 0),
			}
		}
		body := conway.ConwayTransactionBody{
			TxInputs:           conway.NewConwayTransactionInputSet(inputs),
			TxOutputs:          src.TxOutputs,
			TxFee:              src.TxFee,
			TxCollateral:       src.TxCollateral,
			TxCollateralReturn: src.TxCollateralReturn,
			TxReferenceInputs:  cbor.NewSetType(refs, false),
		}
		prevId = body.Id()
		conwayBlock.TransactionBodies = append(
			conwayBlock.TransactionBodies,
			body,
		)
		conwayBlock.TransactionWitnessSets = append(
			conwayBlock.TransactionWitnessSets,
			conway.ConwayTransactionWitnessSet{},
		)
	}
	for _, pv := range []uint{10, 11} {
		accumulates := pv >= 11
		want := uint64(bbmRefTotal(b, accumulates)) * scriptLen // #nosec G115
		require.Equal(t, accumulates, want > scriptLen)
		pp := &conway.ConwayProtocolParameters{}
		pp.ProtocolVersion.Major = pv
		got, err := common.ConsumedReferenceScriptSizePerBlock(
			conwayBlock,
			built.state,
			pp.ProtocolVersion.Major >= common.ProtocolVersionVanRossem,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, want, got, "pv %d", pv)
		require.NoError(
			t,
			conway.ValidateRefScriptSizePerBlock(conwayBlock, pp, built.state),
		)
	}
}
