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

package shelley_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

const validTxHashHex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestNewShelleyTransactionInputRejectsBadArguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hash string
		idx  int
	}{
		{
			name: "hash is not hex",
			hash: strings.Repeat("zz", 32),
			idx:  0,
		},
		{
			// Converting a short slice to Blake2b256 panics before any
			// check on it can run.
			name: "hash is one byte short",
			hash: strings.Repeat("ab", 31),
			idx:  0,
		},
		{
			name: "hash is empty",
			hash: "",
			idx:  0,
		},
		{
			// A long slice converts without panicking, silently keeping
			// only its first 32 bytes.
			name: "hash is one byte long",
			hash: strings.Repeat("ab", 33),
			idx:  0,
		},
		{
			name: "index is negative",
			hash: validTxHashHex,
			idx:  -1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := shelley.NewShelleyTransactionInput(
				testCase.hash,
				testCase.idx,
			)
			require.Error(t, err)
		})
	}
}

// Neither bound is representable as an int on a 32-bit GOARCH, where a
// positive int can never reach MaxUint32 at all. Both conversions go through
// a variable so they stay runtime conversions rather than constant ones,
// which would overflow int at compile time there.
func TestNewShelleyTransactionInputIndexUpperBound(t *testing.T) {
	t.Parallel()
	if math.MaxInt <= math.MaxUint32 {
		t.Skip("the MaxUint32 bound is unreachable through int here")
	}
	maxIdx := int64(math.MaxUint32)

	atBound, err := shelley.NewShelleyTransactionInput(
		validTxHashHex,
		int(maxIdx),
	)
	require.NoError(t, err)
	require.Equal(t, uint32(math.MaxUint32), atBound.Index())

	_, err = shelley.NewShelleyTransactionInput(validTxHashHex, int(maxIdx+1))
	require.Error(t, err)
}

// The rejections above cannot be satisfied by refusing everything.
func TestNewShelleyTransactionInputAcceptsValidArguments(t *testing.T) {
	t.Parallel()

	input, err := shelley.NewShelleyTransactionInput(validTxHashHex, 7)
	require.NoError(t, err)
	require.Equal(t, validTxHashHex, input.Id().String())
	require.Equal(t, uint32(7), input.Index())
}

// A wrong-length hash must not reach the array conversion through the Must
// form either: it panics with the constructor's own error rather than the
// runtime's conversion message.
func TestMustNewShelleyTransactionInputPanicsOnWrongLengthHash(t *testing.T) {
	t.Parallel()

	require.PanicsWithValue(
		t,
		fmt.Sprintf(
			"invalid shelley transaction input: transaction hash is 31 bytes, expected %d",
			common.Blake2b256Size,
		),
		func() {
			shelley.MustNewShelleyTransactionInput(
				strings.Repeat("ab", 31),
				0,
			)
		},
	)
	require.NotPanics(t, func() {
		shelley.MustNewShelleyTransactionInput(validTxHashHex, 0)
	})
}
