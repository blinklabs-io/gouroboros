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
	"bytes"
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
	// Reference field lengths: VKWitness's key is CC.xpub (64 canonical
	// bytes) and its signature is XSignature (64 bytes); RedeemWitness's
	// key is a plain Ed25519 PublicKey (32 bytes) and its signature is 64
	// bytes.
	vkKey := bytes.Repeat([]byte{0xAB}, 64)
	vkSig := bytes.Repeat([]byte{0xCD}, 64)
	redeemKey := bytes.Repeat([]byte{0xEF}, 32)
	redeemSig := bytes.Repeat([]byte{0x12}, 64)
	shortPk := []byte{1, 2, 3, 4}
	shortSig := []byte{5, 6, 7, 8}
	chainCode := []byte{9, 10}
	attrs := []byte{11, 12}

	t.Run("vk witness with tag 24 decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey, vkSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		require.Len(t, ws.Vkey(), 1)
		assert.Equal(t, vkKey, []byte(ws.Vkey()[0].Vkey))
		assert.Equal(t, vkSig, []byte(ws.Vkey()[0].Signature))
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("redeem witness with tag 24 decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{redeemKey, redeemSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		require.Len(t, ws.Vkey(), 1)
		assert.Equal(t, redeemKey, []byte(ws.Vkey()[0].Vkey))
		assert.Equal(t, redeemSig, []byte(ws.Vkey()[0].Signature))
	})

	t.Run("constructor 3 is rejected: no such Byron TxInWitness variant", func(t *testing.T) {
		// The reference TxInWitness sum type (Cardano.Chain.UTxO.TxWitness)
		// has exactly two live constructors, VKWitness (0) and
		// RedeemWitness (2). A four-field "bootstrap witness" shape under
		// tag 3 belongs to the separate Shelley BootstrapWitness encoding
		// used to spend legacy Byron UTxOs from a Shelley-era transaction,
		// not to Byron's own TxInWitness, and must be rejected here.
		inner, err := cbor.Encode([]any{vkKey, vkSig, chainCode, attrs})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(3), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("untagged vk witness is rejected", func(t *testing.T) {
		outer, err := cbor.Encode([]any{uint64(0), []any{vkKey, vkSig}})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("untagged bootstrap witness is rejected", func(t *testing.T) {
		outer, err := cbor.Encode(
			[]any{uint64(3), []any{vkKey, vkSig, chainCode, attrs}},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("wrong semantic tag is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey, vkSig})
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
		inner, err := cbor.Encode([]any{vkKey, vkSig})
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
		twoFields, err := cbor.Encode([]any{vkKey, vkSig})
		require.NoError(t, err)
		outerTwo, err := cbor.Encode(
			[]any{uint64(99), cbor.WrappedCbor(twoFields)},
		)
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outerTwo)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())

		fourFields, err := cbor.Encode([]any{vkKey, vkSig, chainCode, attrs})
		require.NoError(t, err)
		outerFour, err := cbor.Encode(
			[]any{uint64(99), cbor.WrappedCbor(fourFields)},
		)
		require.NoError(t, err)
		ws = decodeSingleWitness(t, outerFour)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("constructor 1 is rejected: ScriptWitness has no reachable decoder", func(t *testing.T) {
		// ScriptWitness is a defined constructor in the reference sum type,
		// but cardano-ledger has no decoder path that ever produces one on
		// a real chain. It must be rejected the same as any other
		// unrecognized constructor, not silently accepted or skipped.
		inner, err := cbor.Encode([]any{vkKey, vkSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(1), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-0 field count is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-0 field type is rejected", func(t *testing.T) {
		// Both VKWitness fields are byte strings; a numeric field must be
		// rejected rather than accepted because the element count matches.
		inner, err := cbor.Encode([]any{uint64(1), vkSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-0 short key is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{shortPk, vkSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-0 short signature is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey, shortSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-0 redeem-length key is rejected", func(t *testing.T) {
		// A 32-byte key is the correct length for RedeemWitness, not
		// VKWitness: constructor-specific lengths must not cross over.
		inner, err := cbor.Encode([]any{redeemKey, vkSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-2 field count is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{redeemKey, redeemSig, chainCode})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-2 field type is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{redeemKey, uint64(7)})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-2 short key is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{shortPk, redeemSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-2 short signature is rejected", func(t *testing.T) {
		inner, err := cbor.Encode([]any{redeemKey, shortSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
		assert.Empty(t, ws.Vkey())
		assert.Empty(t, ws.Bootstrap())
	})

	t.Run("malformed constructor-2 vk-length key is rejected", func(t *testing.T) {
		// A 64-byte key is the correct length for VKWitness, not
		// RedeemWitness: constructor-specific lengths must not cross over.
		inner, err := cbor.Encode([]any{vkKey, redeemSig})
		require.NoError(t, err)
		outer, err := cbor.Encode([]any{uint64(2), cbor.WrappedCbor(inner)})
		require.NoError(t, err)
		ws := decodeSingleWitness(t, outer)
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
	vkKey := bytes.Repeat([]byte{0xAB}, 64)
	vkSig := bytes.Repeat([]byte{0xCD}, 64)

	t.Run("valid tag-24 witness decodes", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey, vkSig})
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
		witness, err := cbor.Encode([]any{uint64(0), []any{vkKey, vkSig}})
		require.NoError(t, err)
		twit, err := cbor.Encode([]any{cbor.RawMessage(witness)})
		require.NoError(t, err)
		var tx byron.ByronTransaction
		require.Error(t, tx.UnmarshalCBOR(encodeByronTransactionWithWitnesses(t, twit)))
	})

	t.Run("unknown witness constructor fails the whole transaction", func(t *testing.T) {
		inner, err := cbor.Encode([]any{vkKey, vkSig})
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

	t.Run("valid witness plus a malformed extra witness fails the whole transaction", func(t *testing.T) {
		// A transaction with one genuinely valid, required witness must
		// still be rejected outright if it carries a second, unrecognized
		// witness: the valid entry does not excuse the invalid one, and the
		// two are hashed together in the witness proof regardless.
		//
		// Constructor 3 is included specifically (not just an arbitrary
		// unknown value like 1 or 255, both of which the decoder already
		// rejected before this change): it is the shape this change stops
		// treating as a valid bootstrap witness, so this pins the
		// transaction-level rejection to the actual behavior this change
		// adds, not to a case that already passed beforehand.
		for _, ctor := range []uint64{1, 3, 255} {
			t.Run(fmt.Sprintf("constructor %d", ctor), func(t *testing.T) {
				validInner, err := cbor.Encode([]any{vkKey, vkSig})
				require.NoError(t, err)
				validWitness, err := cbor.Encode(
					[]any{uint64(0), cbor.WrappedCbor(validInner)},
				)
				require.NoError(t, err)

				extraInner, err := cbor.Encode(
					[]any{vkKey, vkSig, vkKey, vkSig},
				)
				require.NoError(t, err)
				extraWitness, err := cbor.Encode(
					[]any{ctor, cbor.WrappedCbor(extraInner)},
				)
				require.NoError(t, err)

				twit, err := cbor.Encode(
					[]any{
						cbor.RawMessage(validWitness),
						cbor.RawMessage(extraWitness),
					},
				)
				require.NoError(t, err)
				var tx byron.ByronTransaction
				require.Error(
					t,
					tx.UnmarshalCBOR(encodeByronTransactionWithWitnesses(t, twit)),
				)
			})
		}
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
