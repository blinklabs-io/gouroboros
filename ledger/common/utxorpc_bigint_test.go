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

package common_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// TestBigIntToUtxorpcBigIntBoundaries checks the int64 edges and the CBOR
// bignum encodings: tag 2 carries n, tag 3 carries -1-n.
func TestBigIntToUtxorpcBigIntBoundaries(t *testing.T) {
	t.Parallel()
	pow := func(exp uint) *big.Int { return new(big.Int).Lsh(big.NewInt(1), exp) }
	sub1 := func(v *big.Int) *big.Int { return new(big.Int).Sub(v, big.NewInt(1)) }
	neg := func(v *big.Int) *big.Int { return new(big.Int).Neg(v) }

	t.Run("maxInt64", func(t *testing.T) {
		t.Parallel()
		got := common.BigIntToUtxorpcBigInt(sub1(pow(63)))
		require.Equal(t, int64(1<<63-1), got.GetInt())
	})
	t.Run("minInt64", func(t *testing.T) {
		t.Parallel()
		got := common.BigIntToUtxorpcBigInt(neg(pow(63)))
		require.Equal(t, int64(-1<<63), got.GetInt())
	})
	t.Run("aboveInt64", func(t *testing.T) {
		t.Parallel()
		got := common.BigIntToUtxorpcBigInt(pow(63))
		require.Equal(t, pow(63).Bytes(), got.GetBigUInt())
		require.Nil(t, got.GetBigNInt())
	})
	t.Run("belowInt64", func(t *testing.T) {
		t.Parallel()
		// -2^63-1 is tag 3 over 2^63: -1-2^63 = -2^63-1.
		got := common.BigIntToUtxorpcBigInt(neg(pow(63).Add(pow(63), big.NewInt(1))))
		require.Equal(t, pow(63).Bytes(), got.GetBigNInt())
		require.Nil(t, got.GetBigUInt())
	})
	t.Run("minusTwoToThe64", func(t *testing.T) {
		t.Parallel()
		// -2^64 is tag 3 over 2^64-1, the widest CBOR negative integer.
		got := common.BigIntToUtxorpcBigInt(neg(pow(64)))
		require.Equal(t, sub1(pow(64)).Bytes(), got.GetBigNInt())
	})
}
