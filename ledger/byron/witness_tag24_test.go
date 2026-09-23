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

package byron_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeSingleWitness(t *testing.T, data []byte) *byron.ByronTransactionWitnessSet {
	t.Helper()
	var v cbor.Value
	require.NoError(t, v.UnmarshalCBOR(data))
	return byron.NewByronTransactionWitnessSet([]cbor.Value{v})
}

func TestByronWitnessRequiresTag24(t *testing.T) {
	pk := []byte{1, 2, 3, 4}
	sig := []byte{5, 6, 7, 8}
	chainCode := []byte{9, 10}
	attrs := []byte{11, 12}

	t.Run("vk witness with tag 24 decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		require.Len(t, ws.Vkey(), 1)
		assert.Equal(t, pk, []byte(ws.Vkey()[0].Vkey))
		assert.Equal(t, sig, []byte(ws.Vkey()[0].Signature))
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("redeem witness with tag 24 decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		require.Len(t, ws.Vkey(), 1)
		assert.Equal(t, pk, []byte(ws.Vkey()[0].Vkey))
		assert.Equal(t, sig, []byte(ws.Vkey()[0].Signature))
	})

	t.Run("bootstrap witness with tag 24 decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig, chainCode, attrs})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(3), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		require.Len(t, ws.Bootstrap(), 1)
		bw := ws.Bootstrap()[0]
		assert.Equal(t, pk, []byte(bw.PublicKey))
		assert.Equal(t, sig, []byte(bw.Signature))
		assert.Equal(t, chainCode, []byte(bw.ChainCode))
		assert.Equal(t, attrs, []byte(bw.Attributes))
		assert.Empty(t, ws.Vkey())
	})

	t.Run("untagged vk witness is rejected", func(t *testing.T) {
		outer, err := cbor.Encode([]any{uint64(0), []any{pk, sig}})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("untagged bootstrap witness is rejected", func(t *testing.T) {
		outer, err := cbor.Encode(
			[]any{uint64(3), []any{pk, sig, chainCode, attrs}},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("wrong semantic tag is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		wrongTag := cbor.RawTag{Number: 25, Content: cbor.RawMessage(inner)}
		wrapped, err := cbor.Encode(&wrongTag)
		require.NoError(t, err)
		outer, err := cbor.Encode(
			[]any{uint64(0), cbor.RawMessage(wrapped)},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("trailing bytes after nested cbor are rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		withTrailingGarbage := append(append([]byte{}, inner...), 0xFF, 0xFF)
		outer, err := cbor.Encode(
			[]any{uint64(0), cbor.WrappedCbor(withTrailingGarbage)},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("unknown constructor is rejected regardless of field count", func(t *testing.T) {
		// A 2-field payload happens to match the VKey shape, and a
		// 4-field payload happens to match the bootstrap shape, but an
		// unrecognized constructor number must still be rejected: the
		// reference TxInWitness sum type has no catch-all case.
		twoFields, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		outerTwo, err := cbor.Encode(
			[]any{uint64(99), cbor.WrappedCbor(twoFields)},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outerTwo)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())

		fourFields, err := cbor.Encode([]any{pk, sig, chainCode, attrs})
		require.NoError(t, err)
		outerFour, err := cbor.Encode(
			[]any{uint64(99), cbor.WrappedCbor(fourFields)},
		)
		require.NoError(t, err)
		ws = decodeSingleWitness(t, outerFour)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})
}

// encodeByronTransactionWithWitnesses builds a minimal but structurally
// valid Byron transaction with an empty body and the given raw witness
// list CBOR, for exercising ByronTransaction.UnmarshalCBOR's decode-time
// witness validation.
func encodeByronTransactionWithWitnesses(t *testing.T, twitCbor []byte) []byte {
	t.Helper()
	body, err := cbor.Encode([]any{[]any{}, []any{}, map[any]any{}})
	require.NoError(t, err)
	tx, err := cbor.Encode(
		[]any{cbor.RawMessage(body), cbor.RawMessage(twitCbor)},
	)
	require.NoError(t, err)
	return tx
}

func TestByronTransactionRejectsInvalidWitness(t *testing.T) {
	pk := []byte{1, 2, 3, 4}
	sig := []byte{5, 6, 7, 8}

	t.Run("valid tag-24 witness decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		witness, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		twit, err := cbor.Encode([]any{cbor.RawMessage(witness)})
		require.NoError(t, err)
		var tx byron.ByronTransaction
		require.NoError(t, tx.UnmarshalCBOR(encodeByronTransactionWithWitnesses(t, twit)))
		require.Len(t, tx.Witnesses().Vkey(), 1)
	})

	t.Run("untagged witness fails the whole transaction", func(t *testing.T) {
		witness, err := cbor.Encode([]any{uint64(0), []any{pk, sig}})
		require.NoError(t, err)
		twit, err := cbor.Encode([]any{cbor.RawMessage(witness)})
		require.NoError(t, err)
		var tx byron.ByronTransaction
		require.Error(t, tx.UnmarshalCBOR(encodeByronTransactionWithWitnesses(t, twit)))
	})

	t.Run("unknown witness constructor fails the whole transaction", func(t *testing.T) {
		inner, err := cbor.Encode([]any{pk, sig})
		require.NoError(t, err)
		witness, err := cbor.Encode(
			[]any{uint64(99), cbor.WrappedCbor(inner)},
		)
		require.NoError(t, err)
		twit, err := cbor.Encode([]any{cbor.RawMessage(witness)})
		require.NoError(t, err)
		var tx byron.ByronTransaction
		require.Error(t, tx.UnmarshalCBOR(encodeByronTransactionWithWitnesses(t, twit)))
	})
}

func TestByronUpdateProposalTxFeePolicyRequiresTag24(t *testing.T) {
	blockVersionModFields := func(txFeePolicy []any) []any {
		return []any{
			[]any{}, // scriptVersion
			[]any{}, // slotDuration
			[]any{}, // maxBlockSize
			[]any{}, // maxHeaderSize
			[]any{}, // maxTxSize
			[]any{}, // maxProposalSize
			[]any{}, // mpcThd
			[]any{}, // heavyDelThd
			[]any{}, // updateVoteThd
			[]any{}, // updateProposalThd
			[]any{}, // updateImplicit
			[]any{}, // softForkRule
			txFeePolicy,
			[]any{}, // unlockStakeEpoch
		}
	}

	t.Run("tag 24 wrapped policy decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{uint64(100), uint64(200)})
		require.NoError(t, err)
		policy := []any{uint64(0), cbor.WrappedCbor(inner)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.NoError(t, mod.UnmarshalCBOR(data))
	})

	t.Run("untagged policy is rejected", func(t *testing.T) {
		policy := []any{uint64(0), []any{uint64(100), uint64(200)}}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})

	t.Run("absent policy decodes", func(t *testing.T) {
		data, err := cbor.Encode(blockVersionModFields([]any{}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.NoError(t, mod.UnmarshalCBOR(data))
	})

	t.Run("wrong semantic tag is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{uint64(100), uint64(200)})
		require.NoError(t, err)
		wrongTag := cbor.RawTag{Number: 25, Content: cbor.RawMessage(inner)}
		wrapped, err := cbor.Encode(&wrongTag)
		require.NoError(t, err)
		policy := []any{uint64(0), cbor.RawMessage(wrapped)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})

	t.Run("trailing bytes after nested TxSizeLinear are rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{uint64(100), uint64(200)})
		require.NoError(t, err)
		withTrailingGarbage := append(append([]byte{}, inner...), 0xFF, 0xFF)
		policy := []any{uint64(0), cbor.WrappedCbor(withTrailingGarbage)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})

	t.Run("nested TxSizeLinear with wrong element count is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{uint64(100)})
		require.NoError(t, err)
		policy := []any{uint64(0), cbor.WrappedCbor(inner)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})

	t.Run("unknown policy constructor is rejected", func(t *testing.T) {
		// The reference TxFeePolicy sum type has a single constructor
		// (0); a two-element, correctly tag-24-wrapped payload under a
		// different constructor number must still be rejected, not
		// accepted because its shape happens to match.
		inner, err := cbor.Encode([]any{uint64(100), uint64(200)})
		require.NoError(t, err)
		policy := []any{uint64(1), cbor.WrappedCbor(inner)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})

	t.Run("nested TxSizeLinear with non-numeric field is rejected", func(t *testing.T) {
		// Both TxSizeLinear fields are Nano (numeric) values; a
		// two-element payload where a field holds an arbitrary CBOR
		// value (here, a nested array) must be rejected rather than
		// accepted because the element count matches.
		inner, err := cbor.Encode([]any{[]any{uint64(1), uint64(2)}, uint64(200)})
		require.NoError(t, err)
		policy := []any{uint64(0), cbor.WrappedCbor(inner)}
		data, err := cbor.Encode(blockVersionModFields([]any{policy}))
		require.NoError(t, err)
		var mod byron.ByronUpdateProposalBlockVersionMod
		require.Error(t, mod.UnmarshalCBOR(data))
	})
}

func TestByronUpdateProposalLovelacePortionBounds(t *testing.T) {
	fields := func() []any {
		return []any{
			[]any{}, // scriptVersion
			[]any{}, // slotDuration
			[]any{}, // maxBlockSize
			[]any{}, // maxHeaderSize
			[]any{}, // maxTxSize
			[]any{}, // maxProposalSize
			[]any{}, // mpcThd
			[]any{}, // heavyDelThd
			[]any{}, // updateVoteThd
			[]any{}, // updateProposalThd
			[]any{}, // updateImplicit
			[]any{}, // softForkRule
			[]any{}, // txFeePolicy
			[]any{}, // unlockStakeEpoch
		}
	}
	thresholdFields := []struct {
		name  string
		index int
	}{
		{"mpcThd", 6},
		{"heavyDelThd", 7},
		{"updateVoteThd", 8},
		{"updateProposalThd", 9},
	}
	for _, field := range thresholdFields {
		t.Run(field.name, func(t *testing.T) {
			for _, value := range []uint64{0, 1, byron.MaxLovelacePortion} {
				t.Run(fmt.Sprintf("accepts_%d", value), func(t *testing.T) {
					proposalFields := fields()
					proposalFields[field.index] = []any{value}
					data, err := cbor.Encode(proposalFields)
					require.NoError(t, err)
					var mod byron.ByronUpdateProposalBlockVersionMod
					require.NoError(t, mod.UnmarshalCBOR(data))
				})
			}
			proposalFields := fields()
			proposalFields[field.index] = []any{byron.MaxLovelacePortion + 1}
			data, err := cbor.Encode(proposalFields)
			require.NoError(t, err)
			var mod byron.ByronUpdateProposalBlockVersionMod
			require.ErrorContains(t, mod.UnmarshalCBOR(data), "maximum LovelacePortion")
		})
	}

	softforkFields := []struct {
		name  string
		index int
	}{
		{"initThd", 0},
		{"minThd", 1},
		{"thdDecrement", 2},
	}
	for _, field := range softforkFields {
		t.Run("softForkRule/"+field.name, func(t *testing.T) {
			portions := []uint64{1, 1, 1}
			portions[field.index] = byron.MaxLovelacePortion
			proposalFields := fields()
			proposalFields[11] = []any{portions}
			data, err := cbor.Encode(proposalFields)
			require.NoError(t, err)
			var mod byron.ByronUpdateProposalBlockVersionMod
			require.NoError(t, mod.UnmarshalCBOR(data))

			portions[field.index] = byron.MaxLovelacePortion + 1
			proposalFields[11] = []any{portions}
			data, err = cbor.Encode(proposalFields)
			require.NoError(t, err)
			require.ErrorContains(t, mod.UnmarshalCBOR(data), "maximum LovelacePortion")
		})
	}

	t.Run(
		"rejects an oversized value in a full update proposal",
		func(t *testing.T) {
			proposalFields := fields()
			proposalFields[6] = []any{byron.MaxLovelacePortion + 1}
			data, err := cbor.Encode([]any{
				[]any{uint64(1), uint64(0), uint64(0)},
				proposalFields,
				[]any{"cardano-sl", uint64(1)},
				[]byte{},
				map[any]any{},
				[]byte{},
				[]byte{},
			})
			require.NoError(t, err)
			var proposal byron.ByronUpdateProposal
			require.ErrorContains(
				t,
				proposal.UnmarshalCBOR(data),
				"maximum LovelacePortion",
			)
		},
	)
}
