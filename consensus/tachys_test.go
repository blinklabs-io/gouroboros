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

package consensus

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

func tachysPool(value byte, stake uint64) TachysPool {
	var id common.PoolKeyHash
	for i := range id {
		id[i] = value
	}
	return TachysPool{ID: id, Stake: stake}
}

func testTachysSelector(t *testing.T) *TachysLeaderSelector {
	t.Helper()
	selector, err := NewTachysLeaderSelector([]TachysPool{
		tachysPool(3, 3),
		tachysPool(1, 1),
		tachysPool(2, 2),
	}, 6)
	require.NoError(t, err)
	return selector
}

func TestTachysSelectionVectors(t *testing.T) {
	t.Parallel()
	selector := testTachysSelector(t)
	tests := []struct {
		name   string
		nonce  string
		slot   uint64
		sample string
		leader byte
	}{
		{
			name:   "zero nonce and slot",
			nonce:  "0000000000000000000000000000000000000000000000000000000000000000",
			slot:   0,
			sample: "4a1931803561f431decab002e7425f0a8531d5e456a1a47fd9998a2530c0f800",
			leader: 2,
		},
		{
			name:   "absolute epoch slot",
			nonce:  "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
			slot:   4_492_800,
			sample: "078c16c737011e1d5e9122ddefad3d733d624ccad3ce84e409303b595fa59a28",
			leader: 1,
		},
		{
			name:   "maximum slot",
			nonce:  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			slot:   ^uint64(0),
			sample: "6002773786391ba081a87d2bf0139514627f79b1b8a4696ad7bee135e1646680",
			leader: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			nonce, err := hex.DecodeString(test.nonce)
			require.NoError(t, err)
			sample, err := TachysSample(nonce, test.slot)
			require.NoError(t, err)
			require.Equal(t, test.sample, hex.EncodeToString(sample[:]))
			leader, err := selector.Leader(nonce, test.slot)
			require.NoError(t, err)
			require.Equal(t, tachysPool(test.leader, 0).ID, leader)
		})
	}
}

func TestTachysSelectionBoundaries(t *testing.T) {
	t.Parallel()
	selector := testTachysSelector(t)
	tests := []struct {
		sample string
		leader byte
	}{
		{"0000000000000000000000000000000000000000000000000000000000000000", 1},
		{"2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1},
		{"2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab", 2},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 2},
		{"8000000000000000000000000000000000000000000000000000000000000000", 3},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 3},
	}
	for _, test := range tests {
		sample, err := hex.DecodeString(test.sample)
		require.NoError(t, err)
		leader, err := selector.LeaderForSample(sample)
		require.NoError(t, err)
		require.Equal(t, tachysPool(test.leader, 0).ID, leader)
	}
}

func TestTachysSelectorRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	valid := tachysPool(1, 1)
	for _, test := range []struct {
		name  string
		pools []TachysPool
		total uint64
	}{
		{"empty", nil, 0},
		{"zero total", []TachysPool{valid}, 0},
		{"zero pool stake", []TachysPool{tachysPool(1, 0)}, 1},
		{"duplicate", []TachysPool{valid, valid}, 2},
		{"total mismatch", []TachysPool{valid}, 2},
		{"overflow", []TachysPool{tachysPool(1, ^uint64(0)), tachysPool(2, 1)}, ^uint64(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTachysLeaderSelector(test.pools, test.total)
			require.Error(t, err)
		})
	}
	selector := testTachysSelector(t)
	_, err := selector.Leader(make([]byte, 31), 0)
	require.Error(t, err)
	_, err = selector.LeaderForSample(make([]byte, 31))
	require.Error(t, err)
	_, err = selector.AuthorizeIssuer(make([]byte, 32), 0, make([]byte, 31))
	require.Error(t, err)
	var zero TachysLeaderSelector
	_, err = zero.Leader(make([]byte, 32), 0)
	require.Error(t, err)
}

func TestTachysSelectorCopiesDistribution(t *testing.T) {
	t.Parallel()
	pools := []TachysPool{
		tachysPool(2, 2),
		tachysPool(1, 1),
	}
	selector, err := NewTachysLeaderSelector(pools, 3)
	require.NoError(t, err)
	pools[0] = tachysPool(9, 3)
	pools[1] = tachysPool(8, 4)
	leader, err := selector.LeaderForSample(make([]byte, 32))
	require.NoError(t, err)
	require.Equal(t, tachysPool(1, 0).ID, leader)
}

func TestTachysHeaderValidationKeepsCryptographicChecks(t *testing.T) {
	t.Parallel()
	vrfSeed := common.Blake2b256Hash([]byte("Tachys VRF fixture seed"))
	vrfSigner, err := NewSimpleVRFSigner(vrfSeed[:])
	require.NoError(t, err)
	kesSeed := common.Blake2b256Hash([]byte("Tachys KES fixture seed"))
	kesSecret, kesPublic, err := kes.KeyGen(
		kes.CardanoKesDepth,
		kesSeed[:],
	)
	require.NoError(t, err)
	coldSeed := common.Blake2b256Hash([]byte("Tachys cold fixture seed"))
	coldPrivate := ed25519.NewKeyFromSeed(coldSeed[:])
	coldPublic := coldPrivate.Public().(ed25519.PublicKey)
	poolID := common.PoolKeyHash(common.Blake2b224Hash(coldPublic))
	selector, err := NewTachysLeaderSelector(
		[]TachysPool{{ID: poolID, Stake: 1}},
		1,
	)
	require.NoError(t, err)

	config := testNetworkConfig()
	denominator := new(big.Int).SetUint64(10_000_000_000_000_000_000)
	config.ActiveSlotCoeff.Rat = new(big.Rat).SetFrac(big.NewInt(1), denominator)
	tachysValidator, err := NewTachysHeaderValidator(config, selector)
	require.NoError(t, err)
	praosValidator := NewHeaderValidator(config)

	epochNonce := common.Blake2b256Hash([]byte("tachys epoch nonce"))
	const slot = uint64(1)
	vrfInput, err := vrf.MkInputVrf(int64(slot), epochNonce[:])
	require.NoError(t, err)
	vrfProof, vrfOutput, err := vrfSigner.Prove(vrfInput)
	require.NoError(t, err)
	message := []byte("Tachys signed header validation body")
	kesSignature, err := kes.Sign(kesSecret, 0, message)
	require.NoError(t, err)
	opCertBody := common.OpCertSignableBytes(kesPublic, 0, 0)
	opCertSignature := ed25519.Sign(coldPrivate, opCertBody)
	previousHash := make([]byte, 32)
	input := &ValidateHeaderInput{
		Slot:                 slot,
		BlockNumber:          1,
		PrevHash:             previousHash,
		IssuerVkey:           coldPublic,
		VrfKey:               vrfSigner.PublicKey(),
		VrfProof:             vrfProof,
		VrfOutput:            vrfOutput,
		KesSignature:         kesSignature,
		HeaderBodyCbor:       message,
		OpCertHotVkey:        kesPublic,
		OpCertSequenceNumber: 0,
		OpCertKesPeriod:      0,
		OpCertSignature:      opCertSignature,
		OpCertCounterState:   opCertSequence(0),
		PrevSlot:             0,
		PrevBlockNumber:      0,
		PrevHeaderHash:       previousHash,
		EpochNonce:           epochNonce[:],
		PoolStake:            1,
		TotalStake:           1,
		RegisteredVrfKeyHash: common.Blake2b256Hash(vrfSigner.PublicKey()).Bytes(),
	}

	praosResult := praosValidator.ValidateHeader(input)
	require.False(t, praosResult.Valid)
	require.ErrorContains(t, praosResult.Errors[0], "leader eligibility")
	tachysResult := tachysValidator.ValidateHeader(input)
	require.True(t, tachysResult.Valid, "%v", tachysResult.Errors)

	wrongSeed := common.Blake2b256Hash([]byte("unscheduled Tachys cold seed"))
	wrongPrivate := ed25519.NewKeyFromSeed(wrongSeed[:])
	wrongIssuer := wrongPrivate.Public().(ed25519.PublicKey)
	wrongProducer := *input
	wrongProducer.IssuerVkey = wrongIssuer
	wrongProducer.OpCertSignature = ed25519.Sign(wrongPrivate, opCertBody)
	wrongResult := tachysValidator.ValidateHeader(&wrongProducer)
	require.False(t, wrongResult.Valid)
	require.ErrorContains(
		t,
		wrongResult.Errors[0],
		"issuer is not the scheduled Tachys leader",
	)

	invalidVRF := *input
	invalidVRF.VrfProof = append([]byte(nil), input.VrfProof...)
	invalidVRF.VrfProof[0] ^= 0xff
	invalidVRFResult := tachysValidator.ValidateHeader(&invalidVRF)
	require.False(t, invalidVRFResult.Valid)
	require.ErrorContains(t, invalidVRFResult.Errors[0], "VRF verification failed")

	invalidKES := *input
	invalidKES.KesSignature = append([]byte(nil), input.KesSignature...)
	invalidKES.KesSignature[0] ^= 0xff
	invalidKESResult := tachysValidator.ValidateHeader(&invalidKES)
	require.False(t, invalidKESResult.Valid)
	require.ErrorContains(t, invalidKESResult.Errors[0], "KES signature verification failed")

	invalidOpCert := *input
	invalidOpCert.OpCertSignature = append([]byte(nil), input.OpCertSignature...)
	invalidOpCert.OpCertSignature[0] ^= 0xff
	invalidOpCertResult := tachysValidator.ValidateHeader(&invalidOpCert)
	require.False(t, invalidOpCertResult.Valid)
	require.ErrorContains(
		t,
		invalidOpCertResult.Errors[0],
		"OpCert signature verification failed",
	)
}

func TestTachysBlockBuilderUsesPublicSchedule(t *testing.T) {
	t.Parallel()
	seed := common.Blake2b256Hash([]byte("Tachys builder VRF seed"))
	signer, err := NewSimpleVRFSigner(seed[:])
	require.NoError(t, err)
	issuerSeed := common.Blake2b256Hash([]byte("Tachys builder issuer seed"))
	issuerPrivate := ed25519.NewKeyFromSeed(issuerSeed[:])
	issuer := issuerPrivate.Public().(ed25519.PublicKey)
	poolID := common.PoolKeyHash(common.Blake2b224Hash(issuer))
	selector, err := NewTachysLeaderSelector(
		[]TachysPool{{ID: poolID, Stake: 1}},
		1,
	)
	require.NoError(t, err)
	builder, err := NewTachysBlockBuilder(
		signer,
		nil,
		nil,
		issuer,
		selector,
	)
	require.NoError(t, err)
	nonce := common.Blake2b256Hash([]byte("Tachys builder epoch nonce"))
	result, err := builder.CheckSlotLeadership(42, nonce[:], 0, 0)
	require.NoError(t, err)
	require.True(t, result.Eligible)
	require.Len(t, result.Proof, vrf.ProofSize)
	require.Len(t, result.Output, vrf.OutputSize)
	kesSeed := common.Blake2b256Hash([]byte("Tachys builder KES seed"))
	kesSigner, err := NewSimpleKESSigner(kesSeed[:])
	require.NoError(t, err)
	fullBuilder, err := NewTachysBlockBuilder(
		signer,
		kesSigner,
		&OperationalCert{
			HotVkey:   kesSigner.PublicKey(),
			Signature: make([]byte, ed25519.SignatureSize),
		},
		issuer,
		selector,
	)
	require.NoError(t, err)
	previousHash := common.Blake2b256Hash([]byte("Tachys builder predecessor"))
	bodyHash := common.Blake2b256Hash([]byte("Tachys builder body"))
	header, builtResult, err := fullBuilder.BuildHeader(BuildHeaderInput{
		Slot:          42,
		BlockNumber:   1,
		PrevHash:      previousHash[:],
		EpochNonce:    nonce[:],
		BlockBodyHash: bodyHash[:],
		ProtoMajor:    9,
	})
	require.NoError(t, err)
	require.True(t, builtResult.Eligible)
	require.NotNil(t, header)

	wrongIssuer := append([]byte(nil), issuer...)
	wrongIssuer[0] ^= 0xff
	wrongBuilder, err := NewTachysBlockBuilder(
		signer,
		nil,
		nil,
		wrongIssuer,
		selector,
	)
	require.NoError(t, err)
	wrongResult, err := wrongBuilder.CheckSlotLeadership(42, nonce[:], 0, 0)
	require.NoError(t, err)
	require.False(t, wrongResult.Eligible)
	require.Empty(t, wrongResult.Proof)
}
