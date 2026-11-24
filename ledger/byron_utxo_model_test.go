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

package ledger_test

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

// byronModelKey is a Byron spending key and the address it controls.
type byronModelKey struct {
	addressType uint64
	vk          []byte
	sk          ed25519.PrivateKey
	address     common.Address
}

func newByronModelKey(t *testing.T, addressType uint64, seed byte) byronModelKey {
	t.Helper()
	sk := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	pub := sk.Public().(ed25519.PublicKey)
	attributes := common.ByronAddressAttributes{}
	key := byronModelKey{addressType: addressType, sk: sk}
	switch addressType {
	case common.ByronAddressTypePubkey:
		key.vk = append(append([]byte(nil), pub...), bytes.Repeat([]byte{seed}, 32)...)
		rootData, err := cbor.Encode([]any{
			addressType,
			[]any{addressType, key.vk},
			attributes,
		})
		require.NoError(t, err)
		rootDigest := sha3.Sum256(rootData)
		root := common.Blake2b224Hash(rootDigest[:])
		key.address, err = common.NewByronAddressFromParts(addressType, root[:], attributes)
		require.NoError(t, err)
	case common.ByronAddressTypeRedeem:
		key.vk = append([]byte(nil), pub...)
		var err error
		key.address, err = common.NewByronAddressRedeem(pub, attributes)
		require.NoError(t, err)
	default:
		t.Fatalf("unsupported Byron address type %d", addressType)
	}
	return key
}

// witness signs the transaction id as the Byron witness for key.
func (k byronModelKey) witness(t *testing.T, txId common.Blake2b256) cbor.Value {
	t.Helper()
	magicCbor, err := cbor.Encode(byronStateProtocolMagic)
	require.NoError(t, err)
	txIdCbor, err := cbor.Encode(txId[:])
	require.NoError(t, err)
	tag := byte(byron.SignTagTx)
	if k.addressType == common.ByronAddressTypeRedeem {
		tag = byron.SignTagRedeemTx
	}
	signed := append([]byte{tag}, magicCbor...)
	signed = append(signed, txIdCbor...)
	payload, err := cbor.Encode([]any{k.vk, ed25519.Sign(k.sk, signed)})
	require.NoError(t, err)
	witnessCbor, err := cbor.Encode([]any{k.addressType, cbor.WrappedCbor(payload)})
	require.NoError(t, err)
	var witness cbor.Value
	require.NoError(t, witness.UnmarshalCBOR(witnessCbor))
	return witness
}

// byronModelUtxo is one entry of the model's starting UTxO.
type byronModelUtxo struct {
	input byron.ByronTransactionInput
	key   byronModelKey
	value uint64
}

// buildByronModelTx signs a transaction spending ins into outs. Each input
// gets the witness of the key that owns it; wits trims or extends that list
// (extra witnesses are valid signatures by the first input's key).
func buildByronModelTx(
	t *testing.T,
	utxos map[byron.ByronTransactionInput]byronModelUtxo,
	owner byronModelKey,
	ins []byron.ByronTransactionInput,
	outs []uint64,
	wits int,
) *byron.ByronTransaction {
	t.Helper()
	body := byron.ByronTransactionBody{
		TxInputs:   ins,
		Attributes: cbor.RawMessage{0xa0},
	}
	for _, value := range outs {
		body.TxOutputs = append(body.TxOutputs, byron.ByronTransactionOutput{
			OutputAddress: owner.address,
			OutputAmount:  value,
		})
	}
	bodyCbor, err := cbor.Encode(&body)
	require.NoError(t, err)
	body.SetCbor(bodyCbor)
	txId := body.WireId()
	tx := &byron.ByronTransaction{Body: body}
	for i := range wits {
		signer := owner
		if i < len(ins) {
			if entry, ok := utxos[ins[i]]; ok {
				signer = entry.key
			}
		} else if len(ins) > 0 {
			if entry, ok := utxos[ins[0]]; ok {
				signer = entry.key
			}
		}
		tx.Twit = append(tx.Twit, signer.witness(t, txId))
	}
	return tx
}

// byronRefAccepts transcribes the reference UTxO rule that ByronUtxo.tla
// calls RefAccepts (cardano-ledger 1210d37f, Cardano/Chain/UTxO/Tx.hs and
// Validation.hs validateTx, validateTxAux): non-empty inputs and outputs,
// every input in the UTxO, balances over the input set, a non-negative fee
// of at least a + ceiling(b * size) unless every input is a redeem address.
// The summand is rounded down to lovelace, as the genesis JSON path does.
func byronRefAccepts(
	utxos map[byron.ByronTransactionInput]byronModelUtxo,
	tx *byron.ByronTransaction,
	policy common.ByronTxFeePolicy,
) bool {
	if len(tx.Body.TxInputs) == 0 || len(tx.Body.TxOutputs) == 0 {
		return false
	}
	in := new(big.Int)
	allRedeem := true
	seen := map[byron.ByronTransactionInput]bool{}
	for _, input := range tx.Body.TxInputs {
		entry, ok := utxos[input]
		if !ok {
			return false
		}
		if seen[input] {
			continue
		}
		seen[input] = true
		in.Add(in, new(big.Int).SetUint64(entry.value))
		if entry.key.addressType != common.ByronAddressTypeRedeem {
			allRedeem = false
		}
	}
	out := new(big.Int)
	for _, output := range tx.Body.TxOutputs {
		out.Add(out, new(big.Int).SetUint64(output.OutputAmount))
	}
	fee := new(big.Int).Sub(in, out)
	if fee.Sign() < 0 {
		return false
	}
	if allRedeem {
		return true
	}
	unit := big.NewInt(1_000_000_000)
	perByte := new(big.Int).Mul(
		new(big.Int).SetUint64(policy.Multiplier),
		big.NewInt(int64(len(tx.Cbor()))),
	)
	perByte.Add(perByte, new(big.Int).Sub(unit, big.NewInt(1)))
	perByte.Quo(perByte, unit)
	minFee := new(big.Int).Quo(new(big.Int).SetUint64(policy.Summand), unit)
	minFee.Add(minFee, perByte)
	return fee.Cmp(minFee) >= 0
}

// TestByronUtxoModelTable checks ValidateAgainstUtxo on every transaction
// cell of ByronUtxo.tla's starting state: inputs are every list of one or
// two of {pay-to-key, redeem, missing}; outputs every list of one or two
// values; witnesses one short, exact, or one over. gOuroboros must accept
// exactly what the reference accepts, apart from its two deliberate
// tightenings (GoComplete): a repeated input and a witness count that is
// not the input count, which it always rejects.
//
// Empty input or output lists are not cells here: the reference cannot
// decode them, and gOuroboros on main accepts them (ByronUtxoEmptyTxBug).
// maxTxSize is not a cell either: gOuroboros has no parameter for it
// (ByronUtxoNoSizeLimitBug).
func TestByronUtxoModelTable(t *testing.T) {
	t.Parallel()

	payKey := newByronModelKey(t, common.ByronAddressTypePubkey, 0x41)
	redeemKey := newByronModelKey(t, common.ByronAddressTypeRedeem, 0x52)
	pkIn := byron.ByronTransactionInput{TxId: common.Blake2b256{0xa1}}
	redeemIn := byron.ByronTransactionInput{TxId: common.Blake2b256{0xa2}, OutputIndex: 1}
	missingIn := byron.ByronTransactionInput{TxId: common.Blake2b256{0xa3}}
	utxos := map[byron.ByronTransactionInput]byronModelUtxo{
		pkIn:     {input: pkIn, key: payKey, value: 1000},
		redeemIn: {input: redeemIn, key: redeemKey, value: 600},
	}
	var entries []common.Utxo
	for _, entry := range utxos {
		entries = append(entries, common.Utxo{
			Id: entry.input,
			Output: &byron.ByronTransactionOutput{
				OutputAddress: entry.key.address,
				OutputAmount:  entry.value,
			},
		})
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos(entries).Build()
	// 100 lovelace plus one lovelace per byte: a pay-to-key spend of about
	// 250 bytes needs a fee of about 350, so the output values below put
	// cells on both sides of the minimum.
	policy := common.ByronTxFeePolicy{
		Summand:    100_000_000_000,
		Multiplier: 1_000_000_000,
	}

	refs := []byron.ByronTransactionInput{pkIn, redeemIn, missingIn}
	var inputLists [][]byron.ByronTransactionInput
	for _, a := range refs {
		inputLists = append(inputLists, []byron.ByronTransactionInput{a})
		for _, b := range refs {
			inputLists = append(inputLists, []byron.ByronTransactionInput{a, b})
		}
	}
	values := []uint64{0, 250, 500, 900}
	var outputLists [][]uint64
	for _, a := range values {
		outputLists = append(outputLists, []uint64{a})
		for _, b := range values {
			outputLists = append(outputLists, []uint64{a, b})
		}
	}

	counts := map[string]int{}
	for _, ins := range inputLists {
		for _, outs := range outputLists {
			for _, wits := range []int{len(ins) - 1, len(ins), len(ins) + 1} {
				tx := buildByronModelTx(t, utxos, payKey, ins, outs, wits)
				duplicate := len(ins) == 2 && ins[0] == ins[1]
				ref := byronRefAccepts(utxos, tx, policy)
				want := ref && !duplicate && wits == len(ins)
				err := tx.ValidateAgainstUtxo(byronStateProtocolMagic, state, &policy)
				name := fmt.Sprintf("ins=%v outs=%v wits=%d", ins, outs, wits)
				if want {
					require.NoError(t, err, name)
				} else {
					require.Error(t, err, name)
				}
				switch {
				case ref && !want:
					counts["reference accepts, gOuroboros tightens"]++
				case want:
					counts["accepted"]++
				default:
					counts["rejected"]++
				}
			}
		}
	}
	// Every cell group the model distinguishes is reached.
	for _, group := range []string{
		"accepted",
		"rejected",
		"reference accepts, gOuroboros tightens",
	} {
		require.Positive(t, counts[group], group)
	}
}

// TestByronUtxoModelChained covers the model's ProbeChained and the block
// fold: within one block a transaction may spend an output made by an
// earlier one, and two transactions may not spend the same output.
func TestByronUtxoModelChained(t *testing.T) {
	t.Parallel()

	payKey := newByronModelKey(t, common.ByronAddressTypePubkey, 0x61)
	pkIn := byron.ByronTransactionInput{TxId: common.Blake2b256{0xb1}}
	utxos := map[byron.ByronTransactionInput]byronModelUtxo{
		pkIn: {input: pkIn, key: payKey, value: 1000},
	}
	state := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
		Id: pkIn,
		Output: &byron.ByronTransactionOutput{
			OutputAddress: payKey.address,
			OutputAmount:  1000,
		},
	}}).Build()
	config := byronStateConfig(state)

	first := buildByronModelTx(t, utxos, payKey, []byron.ByronTransactionInput{pkIn}, []uint64{900}, 1)
	made := byron.ByronTransactionInput{TxId: first.Id(), OutputIndex: 0}
	utxos[made] = byronModelUtxo{input: made, key: payKey, value: 900}
	second := buildByronModelTx(t, utxos, payKey, []byron.ByronTransactionInput{made}, []uint64{800}, 1)
	require.NoError(t, ledger.VerifyBlockState(byronStateBlock(*first, *second), config))

	again := buildByronModelTx(t, utxos, payKey, []byron.ByronTransactionInput{pkIn}, []uint64{800}, 1)
	require.Error(t, ledger.VerifyBlockState(byronStateBlock(*first, *again), config))
}
