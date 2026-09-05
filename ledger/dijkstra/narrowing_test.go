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

package dijkstra

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// wireTxIndexAboveMaxUint32 exceeds the range of a 32-bit platform uint, so
// narrowing it wraps to a small in-range value on a 32-bit build.
const wireTxIndexAboveMaxUint32 = uint64(math.MaxUint32) + 1

// wireRedeemerIndexAboveMaxInt32 is representable in the wire type for a
// redeemer index (uint32) but negative once narrowed to a 32-bit platform int.
const wireRedeemerIndexAboveMaxInt32 = uint32(1) << 31

// TestDijkstraBlockBodyRejectsInvalidTransactionIndexAboveMaxUint32 asserts the
// consensus-visible behavior: a block whose invalid_transactions set names an
// index no transaction has must be rejected, not silently applied to a
// different transaction. Narrowing the wire index before the bounds check wraps
// it to 0 on a 32-bit build, which flips the validity of transaction 0.
func TestDijkstraBlockBodyRejectsInvalidTransactionIndexAboveMaxUint32(
	t *testing.T,
) {
	bodyCbor, err := cbor.Encode([]any{
		[]uint64{wireTxIndexAboveMaxUint32},
		[]any{minimalTxParts()},
		nil,
		nil,
	})
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.Error(t, err)
	require.Empty(t, blockBody.Transactions)
}

// TestDecodeInvalidTransactionsDoesNotNarrowWireIndex asserts the property
// directly on the decoder: an index that does not fit the platform uint must be
// rejected rather than truncated. The assertion holds on every architecture -
// on 64-bit the index fits and round-trips, on 32-bit it must produce an error.
func TestDecodeInvalidTransactionsDoesNotNarrowWireIndex(t *testing.T) {
	raw, err := cbor.Encode([]uint64{wireTxIndexAboveMaxUint32})
	require.NoError(t, err)

	got, err := decodeInvalidTransactions(cbor.RawMessage(raw))
	if err != nil {
		return
	}
	require.Len(t, got, 1)
	require.Equal(t, wireTxIndexAboveMaxUint32, uint64(got[0]))
}

// TestUtxoValidateExtraneousRedeemersRejectsWireIndexAboveMaxInt32 exercises
// the Dijkstra-era copy of the extraneous-redeemer rule, which does not share
// common.ValidateExtraneousRedeemers. Narrowing the index to a 32-bit platform
// int makes it negative, so the check that exists to reject exactly this input
// is bypassed and the redeemer is accepted.
func TestUtxoValidateExtraneousRedeemersRejectsWireIndexAboveMaxInt32(
	t *testing.T,
) {
	tx := &DijkstraTransaction{
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{
						Tag:   common.RedeemerTagMint,
						Index: wireRedeemerIndexAboveMaxInt32,
					}: {},
				},
			},
		},
	}

	err := UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})
}

// dijkstraTxWithOneScriptGuard builds a transaction carrying a single script
// guard credential, so any redeemer index other than 0 is out of range.
func dijkstraTxWithOneScriptGuard() *DijkstraTransaction {
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{
					{CredType: common.CredentialTypeScriptHash},
				},
			},
		},
	}
}

// TestDijkstraGuardingPurposeRejectsWireIndexAboveMaxInt32 covers the TxGuards
// path. A narrowed index passes the bounds check as a negative value and then
// indexes guards.Credentials with the un-narrowed uint32, which panics.
func TestDijkstraGuardingPurposeRejectsWireIndexAboveMaxInt32(t *testing.T) {
	redeemerKey := common.RedeemerKey{
		Tag:   common.RedeemerTagGuarding,
		Index: wireRedeemerIndexAboveMaxInt32,
	}

	var ok bool
	require.NotPanics(t, func() {
		_, ok = dijkstraGuardingPurpose(dijkstraTxWithOneScriptGuard(), redeemerKey)
	})
	require.False(t, ok)
}

// TestDijkstraGuardCredentialAtRejectsWireIndexAboveMaxInt32 covers the same
// bounds check on the index-only helper reached from redeemer validation.
func TestDijkstraGuardCredentialAtRejectsWireIndexAboveMaxInt32(t *testing.T) {
	var ok bool
	require.NotPanics(t, func() {
		_, ok = dijkstraGuardCredentialAt(
			dijkstraTxWithOneScriptGuard(),
			wireRedeemerIndexAboveMaxInt32,
		)
	})
	require.False(t, ok)
}
