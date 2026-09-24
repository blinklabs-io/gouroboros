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
		name    string
		hash    []byte
		wantErr bool
	}{
		{
			name: "exact length satisfied by the witness",
			hash: slices.Clone(witness[:]),
		},
		{
			name:    "short hash must not zero-pad into the witness",
			hash:    slices.Clone(witness[:common.Blake2b224Size-1]),
			wantErr: true,
		},
		{
			name:    "long hash must not truncate into the witness",
			hash:    slices.Concat(witness[:], []byte{0xFF}),
			wantErr: true,
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
			err = script.UnmarshalCBOR(scriptCbor)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(
				t,
				true,
				script.Evaluate(0, 0, math.MaxUint64, keyHashes),
				"native script pubkey hash of %d bytes",
				len(testCase.hash),
			)
		})
	}
}

func TestNativeScriptPubkeyRejectsWrongLengthKeyHashInNestedScripts(t *testing.T) {
	t.Parallel()
	shortHash := make([]byte, common.Blake2b224Size-1)
	invalidLeaf := []any{uint(0), shortHash}
	testCases := []struct {
		name   string
		script any
	}{
		{name: "all", script: []any{uint(1), []any{invalidLeaf}}},
		{name: "any", script: []any{uint(2), []any{invalidLeaf}}},
		{name: "n of k", script: []any{uint(3), int64(1), []any{invalidLeaf}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			data, err := cbor.Encode(testCase.script)
			require.NoError(t, err)
			var script common.NativeScript
			err = script.UnmarshalCBOR(data)
			require.ErrorContains(t, err, "invalid native script key hash")
		})
	}
}

func TestScriptRefRejectsWrongLengthNativeScriptKeyHash(t *testing.T) {
	t.Parallel()
	leaf, err := cbor.Encode([]any{
		uint(0),
		make([]byte, common.Blake2b224Size+1),
	})
	require.NoError(t, err)
	refPayload, err := cbor.Encode([]any{
		uint(common.ScriptRefTypeNativeScript),
		cbor.RawMessage(leaf),
	})
	require.NoError(t, err)
	ref, err := cbor.Encode(cbor.Tag{Number: 24, Content: refPayload})
	require.NoError(t, err)
	var scriptRef common.ScriptRef
	_, err = cbor.Decode(ref, &scriptRef)
	require.ErrorContains(t, err, "invalid native script key hash")
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

func TestPoolRegistrationUnmarshalJSONRejectsWrongLengthHashes(t *testing.T) {
	t.Parallel()
	vrf := make([]byte, common.Blake2b256Size)
	owner := make([]byte, common.Blake2b224Size)
	for i := range vrf {
		vrf[i] = 0xCC
	}
	for i := range owner {
		owner[i] = 0xDD
	}
	vrf[common.Blake2b256Size-1] = 0x00
	owner[common.Blake2b224Size-1] = 0x00
	operator := hex.EncodeToString(make([]byte, common.Blake2b224Size))
	testCases := []struct {
		name    string
		vrf     []byte
		owner   []byte
		wantErr bool
	}{
		{name: "exact lengths accepted", vrf: vrf, owner: owner},
		{
			name:    "short vrf key hash rejected",
			vrf:     vrf[:common.Blake2b256Size-1],
			owner:   owner,
			wantErr: true,
		},
		{
			name:    "long vrf key hash rejected",
			vrf:     slices.Concat(vrf, []byte{0xFF}),
			owner:   owner,
			wantErr: true,
		},
		{
			name:    "short pool owner rejected",
			vrf:     vrf,
			owner:   owner[:common.Blake2b224Size-1],
			wantErr: true,
		},
		{
			name:    "long pool owner rejected",
			vrf:     vrf,
			owner:   slices.Concat(owner, []byte{0xFF}),
			wantErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			raw := fmt.Sprintf(
				`{"operator":%q,"vrfKeyHash":%q,"pledge":0,"cost":0,`+
					`"margin":{"numerator":0,"denominator":1},`+
					`"poolOwners":[%q],"relays":[]}`,
				operator,
				hex.EncodeToString(testCase.vrf),
				hex.EncodeToString(testCase.owner),
			)
			var cert common.PoolRegistrationCertificate
			err := json.Unmarshal([]byte(raw), &cert)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, vrf, cert.VrfKeyHash[:])
			require.Equal(
				t,
				[]common.AddrKeyHash{common.AddrKeyHash(owner)},
				cert.PoolOwners,
			)
		})
	}
}
