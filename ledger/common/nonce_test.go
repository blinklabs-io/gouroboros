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
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestNonceUnmarshalCBOR(t *testing.T) {
	testCases := []struct {
		name        string
		data        []byte
		expectedErr string
	}{
		{
			name: "NonceTypeNeutral",
			data: []byte{0x81, 0x00},
		},
		{
			name: "NonceTypeNonce",
			data: append(
				[]byte{0x82, 0x01, 0x58, 0x20},
				make([]byte, common.Blake2b256Size)...,
			),
		},
		{
			name:        "UnsupportedNonceType",
			data:        []byte{0x82, 0x02},
			expectedErr: "unsupported nonce type 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := &common.Nonce{}
			err := n.UnmarshalCBOR(tc.data)
			if err != nil {
				if tc.expectedErr == "" || err.Error() != tc.expectedErr {
					t.Errorf("unexpected error: %v", err)
				}
			} else if tc.expectedErr != "" {
				t.Errorf("expected error: %v, got nil", tc.expectedErr)
			}
		})
	}
}

// Expected values come from the actual cardano-ledger nonce functions.
// See testdata/nonce_reference.hs for the pinned reference and generator.
func TestNonceReferenceVectors(t *testing.T) {
	previous := common.Nonce{Type: common.NonceTypeNonce}
	copy(previous.Value[:], bytes.Repeat([]byte{1}, 32))
	zeroHash := common.Nonce{Type: common.NonceTypeNonce}
	output := bytes.Repeat([]byte{2}, 64)
	for _, test := range []struct {
		name         string
		derive       func([]byte) (common.Nonce, error)
		contribution string
		evolving     string
		zeroHash     string
	}{
		{
			"TPraos", common.NonceFromTPraosVRF,
			"bfed46301bf63819ba4dae77ae6077d7a9b3950210f30768acd2d110797f4adb",
			"5d2650064135dfe6a5037da1a336225eff2254875d8bfb6cb628dd04d7b236f4",
			"2e7e9716e0ce3050dc615d2e807db4e6c2efa9cfd9ff9d60713b53f396e48398",
		},
		{
			"Praos", common.NonceFromPraosVRF,
			"0e67af6e4bf9ed97bf8d37652dab254ac88fe309c13271a222a91f0a7652a274",
			"4946e15256ad9ae4a721f4cb7024029d0104facac38afb9503c6b63f71cf65c4",
			"1334aea5f7f1cfb71e0510c37b8b100c6c1816670d19200624fd30340445dfe8",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			contribution, err := test.derive(output)
			require.NoError(t, err)
			require.Equal(t, uint(common.NonceTypeNonce), contribution.Type)
			require.Equal(t, test.contribution,
				hex.EncodeToString(contribution.Value[:]))
			for _, combination := range []struct {
				previous common.Nonce
				want     string
			}{
				{previous, test.evolving},
				{zeroHash, test.zeroHash},
			} {
				got, err := common.CombineNonces(
					combination.previous, contribution,
				)
				require.NoError(t, err)
				require.Equal(t, uint(common.NonceTypeNonce), got.Type)
				require.Equal(t, combination.want,
					hex.EncodeToString(got.Value[:]))
				legacy, err := common.CalculateRollingNonce(
					combination.previous.Value[:], contribution.Value[:],
				)
				require.NoError(t, err)
				require.Equal(
					t,
					combination.want,
					hex.EncodeToString(legacy[:]),
				)
			}
			for _, length := range []int{0, 31, 32, 63, 65} {
				_, err := test.derive(make([]byte, length))
				require.Error(t, err, "length %d", length)
			}
		})
	}
	tp, err := common.NonceFromTPraosVRF(output)
	require.NoError(t, err)
	pr, err := common.NonceFromPraosVRF(output)
	require.NoError(t, err)
	rolling, err := common.CombineNonces(previous, tp)
	require.NoError(t, err)
	rolling, err = common.CombineNonces(rolling, pr)
	require.NoError(t, err)
	require.Equal(t,
		"d8d64a5a8294637cb5ae8d8fb780f8384ad422f049dec0eda4b10fde78c51b1a",
		hex.EncodeToString(rolling.Value[:]),
	)
}

func TestCombineNoncesNeutral(t *testing.T) {
	neutral := common.Nonce{Type: common.NonceTypeNeutral}
	zeroHash := common.Nonce{Type: common.NonceTypeNonce}
	tp, err := common.NonceFromTPraosVRF(bytes.Repeat([]byte{2}, 64))
	require.NoError(t, err)
	pr, err := common.NonceFromPraosVRF(bytes.Repeat([]byte{2}, 64))
	require.NoError(t, err)
	for _, nonce := range []common.Nonce{neutral, zeroHash, tp, pr} {
		left, err := common.CombineNonces(neutral, nonce)
		require.NoError(t, err)
		require.Equal(t, nonce, left)
		right, err := common.CombineNonces(nonce, neutral)
		require.NoError(t, err)
		require.Equal(t, nonce, right)
	}
	invalid := common.Nonce{Type: 2}
	for _, nonce := range []common.Nonce{neutral, zeroHash, tp, pr} {
		_, err := common.CombineNonces(invalid, nonce)
		require.Error(t, err)
		_, err = common.CombineNonces(nonce, invalid)
		require.Error(t, err)
	}
}

func TestCalculateRollingNonceRejectsInvalidInputs(t *testing.T) {
	for _, length := range []int{0, 16, 31, 33, 64, 65} {
		t.Run(fmt.Sprintf("previous_%d", length), func(t *testing.T) {
			_, err := common.CalculateRollingNonce(
				make([]byte, length), make([]byte, 32),
			)
			require.Error(t, err)
		})
		t.Run(fmt.Sprintf("contribution_%d", length), func(t *testing.T) {
			_, err := common.CalculateRollingNonce(
				make([]byte, 32), make([]byte, length),
			)
			require.ErrorContains(
				t,
				err,
				"NonceFromTPraosVRF or NonceFromPraosVRF",
			)
		})
	}
}

func TestCalculateEpochNonce(t *testing.T) {
	testDefs := []struct {
		stableBlockNonce        string
		prevEpochFirstBlockHash string
		extraEntropy            string
		expectedNonce           string
	}{
		{
			stableBlockNonce:        "e86e133bd48ff5e79bec43af1ac3e348b539172f33e502d2c96735e8c51bd04d",
			prevEpochFirstBlockHash: "d7a1ff2a365abed59c9ae346cba842b6d3df06d055dba79a113e0704b44cc3e9",
			expectedNonce:           "e536a0081ddd6d19786e9d708a85819a5c3492c0da7349f59c8ad3e17e4acd98",
		},
		{
			stableBlockNonce:        "d1340a9c1491f0face38d41fd5c82953d0eb48320d65e952414a0c5ebaf87587",
			prevEpochFirstBlockHash: "ee91d679b0a6ce3015b894c575c799e971efac35c7a8cbdc2b3f579005e69abd",
			extraEntropy:            "d982e06fd33e7440b43cefad529b7ecafbaa255e38178ad4189a37e4ce9bf1fa",
			expectedNonce:           "0022cfa563a5328c4fb5c8017121329e964c26ade5d167b1bd9b2ec967772b60",
		},
	}
	for _, testDef := range testDefs {
		stableBlockNonce, err := hex.DecodeString(testDef.stableBlockNonce)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		prevEpochFirstBlockHash, err := hex.DecodeString(
			testDef.prevEpochFirstBlockHash,
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var extraEntropy []byte
		if testDef.extraEntropy != "" {
			tmpEntropy, err := hex.DecodeString(testDef.extraEntropy)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			extraEntropy = tmpEntropy
		}
		tmpNonce, err := common.CalculateEpochNonce(
			stableBlockNonce,
			prevEpochFirstBlockHash,
			extraEntropy,
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpNonceHex := hex.EncodeToString(tmpNonce.Bytes())
		if tmpNonceHex != testDef.expectedNonce {
			t.Fatalf(
				"did not get expected epoch nonce: got %s, wanted %s",
				tmpNonceHex,
				testDef.expectedNonce,
			)
		}
	}
}

func TestCalculateEpochNonceInvalidExtraEntropyLength(t *testing.T) {
	stable := make([]byte, 32)
	prevHash := make([]byte, 32)
	// 16 bytes: not 0 or 32
	_, err := common.CalculateEpochNonce(stable, prevHash, make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for 16-byte extraEntropy, got nil")
	}
	// 64 bytes: not 0 or 32
	_, err = common.CalculateEpochNonce(stable, prevHash, make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for 64-byte extraEntropy, got nil")
	}
}

// Expected values follow the reference tick, ηc ⭒ ηh ⭒ ηe
// (cardano-ledger Cardano.Protocol.TPraos.Rules.Tickn tickTransition), with
// NeutralNonce as the identity of ⭒ (Cardano.Ledger.BaseTypes). The hashes
// are blake2b-256 of the concatenated operands, computed with b2sum -l 256,
// not with the reference itself.
func TestEpochNonceFromNonces(t *testing.T) {
	nonce := func(b byte) common.Nonce {
		n := common.Nonce{Type: common.NonceTypeNonce}
		copy(n.Value[:], bytes.Repeat([]byte{b}, 32))
		return n
	}
	hashed := func(h string) common.Nonce {
		n := common.Nonce{Type: common.NonceTypeNonce}
		raw, err := hex.DecodeString(h)
		require.NoError(t, err)
		copy(n.Value[:], raw)
		return n
	}
	neutral := common.Nonce{Type: common.NonceTypeNeutral}
	candidate, lab, entropy := nonce(1), nonce(2), nonce(3)
	for _, test := range []struct {
		name                    string
		candidate, lab, entropy common.Nonce
		want                    common.Nonce
	}{
		{"neutral lab", candidate, neutral, neutral, candidate},
		{
			"neutral lab with extra entropy", candidate, neutral, entropy,
			hashed("725d60bce59829bc8127bb1551be4b065b3105277343bf405a0c8ab43b69ed7b"),
		},
		{
			"neutral extra entropy", candidate, lab, neutral,
			hashed("30b600fb1f0cc0b3f0fc28cdcb7389405a6659be81c7d5c5905725aa3a5119ce"),
		},
		{
			"all nonneutral", candidate, lab, entropy,
			hashed("d1d371ee5b8ba2283151d7817ec46afd6c32b09c4acec3735de7ca2540272031"),
		},
		{"neutral candidate", neutral, lab, neutral, lab},
		{"all neutral", neutral, neutral, neutral, neutral},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := common.EpochNonceFromNonces(
				test.candidate, test.lab, test.entropy,
			)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
	invalid := common.Nonce{Type: 2}
	for _, args := range [][3]common.Nonce{
		{invalid, lab, entropy},
		{candidate, invalid, entropy},
		{candidate, lab, invalid},
	} {
		_, err := common.EpochNonceFromNonces(args[0], args[1], args[2])
		require.Error(t, err)
	}
}
