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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// nearMissKeyHash returns a 28-byte witness key hash whose last byte is zero.
// Such a hash is reachable by zero-padding its own 27-byte prefix and by
// truncating any longer slice that starts with it, so it is the shape an
// unchecked copy into a fixed-size array conflates with a wrong-length value.
func nearMissKeyHash() common.Blake2b224 {
	var hash common.Blake2b224
	for i := range hash {
		hash[i] = 0xAA
	}
	hash[common.Blake2b224Size-1] = 0x00
	return hash
}

func TestNativeScriptPubkeyRejectsWrongLengthKeyHash(t *testing.T) {
	t.Parallel()
	witness := nearMissKeyHash()
	keyHashes := map[common.Blake2b224]bool{witness: true}
	testCases := []struct {
		name string
		hash []byte
		want bool
	}{
		{
			name: "exact length satisfied by the witness",
			hash: slices.Clone(witness[:]),
			want: true,
		},
		{
			name: "short hash must not zero-pad into the witness",
			hash: slices.Clone(witness[:common.Blake2b224Size-1]),
			want: false,
		},
		{
			name: "long hash must not truncate into the witness",
			hash: slices.Concat(witness[:], []byte{0xFF}),
			want: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			scriptCbor, err := cbor.Encode(
				[]any{uint(0), testCase.hash},
			)
			require.NoError(t, err)
			var script common.NativeScript
			require.NoError(t, script.UnmarshalCBOR(scriptCbor))
			require.Equal(
				t,
				testCase.want,
				script.Evaluate(0, 0, math.MaxUint64, keyHashes),
				"native script pubkey hash of %d bytes",
				len(testCase.hash),
			)
		})
	}
}

func TestMultiAssetUnmarshalJSONRejectsWrongLengthPolicyId(t *testing.T) {
	t.Parallel()
	var policy common.Blake2b224
	for i := range policy {
		policy[i] = 0xBB
	}
	policy[common.Blake2b224Size-1] = 0x00
	testCases := []struct {
		name     string
		policyId []byte
		wantErr  bool
	}{
		{
			name:     "exact length accepted",
			policyId: slices.Clone(policy[:]),
		},
		{
			name:     "short policy id rejected",
			policyId: slices.Clone(policy[:common.Blake2b224Size-1]),
			wantErr:  true,
		},
		{
			name:     "long policy id rejected",
			policyId: slices.Concat(policy[:], []byte{0xFF}),
			wantErr:  true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			raw := fmt.Sprintf(
				`[{"name":"","nameHex":"","policyId":%q,"fingerprint":"","amount":"1"}]`,
				hex.EncodeToString(testCase.policyId),
			)
			var assets common.MultiAsset[uint64]
			err := json.Unmarshal([]byte(raw), &assets)
			if testCase.wantErr {
				require.Error(t, err)
				require.Empty(
					t,
					assets.Policies(),
					"rejected policy id must not be stored",
				)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []common.Blake2b224{policy}, assets.Policies())
		})
	}
}

func TestNewBlake2bCheckedRejectsWrongLength(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		size int
		call func([]byte) error
	}{
		{
			name: "blake2b-256",
			size: common.Blake2b256Size,
			call: func(b []byte) error {
				_, err := common.NewBlake2b256Checked(b)
				return err
			},
		},
		{
			name: "blake2b-224",
			size: common.Blake2b224Size,
			call: func(b []byte) error {
				_, err := common.NewBlake2b224Checked(b)
				return err
			},
		},
		{
			name: "blake2b-160",
			size: common.Blake2b160Size,
			call: func(b []byte) error {
				_, err := common.NewBlake2b160Checked(b)
				return err
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, testCase.call(make([]byte, testCase.size)))
			for _, size := range []int{0, testCase.size - 1, testCase.size + 1} {
				require.Error(
					t,
					testCase.call(make([]byte, size)),
					"expected %d bytes to be rejected",
					size,
				)
			}
		})
	}
}

func TestNewBlake2bCheckedPreservesBytes(t *testing.T) {
	t.Parallel()
	want := nearMissKeyHash()
	got, err := common.NewBlake2b224Checked(want[:])
	require.NoError(t, err)
	require.Equal(t, want, got)
}
