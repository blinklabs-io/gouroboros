// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package byron

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func deterministicPBFTVerificationKey(
	seedByte byte,
) ([]byte, ed25519.PrivateKey) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat(
		[]byte{seedByte},
		ed25519.SeedSize,
	))
	verificationKey := make([]byte, 64)
	copy(verificationKey, privateKey.Public().(ed25519.PublicKey))
	copy(verificationKey[32:], bytes.Repeat([]byte{seedByte ^ 0xff}, 32))
	return verificationKey, privateKey
}

func signedPBFTDelegationCertificate(
	t *testing.T,
	protocolMagic uint32,
	epoch uint64,
	issuerKey []byte,
	issuerPrivateKey ed25519.PrivateKey,
	delegateKey []byte,
) cbor.RawMessage {
	t.Helper()
	epochCbor, err := cbor.Encode(epoch)
	require.NoError(t, err)
	inner := make([]byte, 0, 2+len(delegateKey)+len(epochCbor))
	inner = append(inner, '0', '0')
	inner = append(inner, delegateKey...)
	inner = append(inner, epochCbor...)
	innerCbor, err := cbor.Encode(inner)
	require.NoError(t, err)
	protocolMagicCbor, err := cbor.Encode(protocolMagic)
	require.NoError(t, err)
	signed := []byte{byron.SignTagCertificate}
	signed = append(signed, protocolMagicCbor...)
	signed = append(signed, innerCbor...)
	encoded, err := cbor.Encode([]any{
		epoch,
		append([]byte(nil), issuerKey...),
		append([]byte(nil), delegateKey...),
		ed25519.Sign(issuerPrivateKey, signed),
	})
	require.NoError(t, err)
	return encoded
}

func TestPBFTDelegationStateActivationAndRevocation(t *testing.T) {
	const (
		protocolMagic = uint32(42)
		securityParam = uint64(10)
	)
	issuerKey, issuerPrivateKey := deterministicPBFTVerificationKey(0x11)
	initialDelegateKey, _ := deterministicPBFTVerificationKey(0x22)
	replacementDelegateKey, _ := deterministicPBFTVerificationKey(0x33)
	issuerHash, err := PBFTVerificationKeyHash(issuerKey)
	require.NoError(t, err)
	initialDelegateHash, err := PBFTVerificationKeyHash(initialDelegateKey)
	require.NoError(t, err)
	replacementDelegateHash, err := PBFTVerificationKeyHash(
		replacementDelegateKey,
	)
	require.NoError(t, err)
	state, err := NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    protocolMagic,
		SecurityParam:    securityParam,
		GenesisKeyHashes: [][]byte{issuerHash.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerHash: initialDelegateHash,
		},
	})
	require.NoError(t, err)
	require.Equal(t, initialDelegateHash, state.ActiveDelegations()[issuerHash])

	activationCertificate := signedPBFTDelegationCertificate(
		t,
		protocolMagic,
		1,
		issuerKey,
		issuerPrivateKey,
		replacementDelegateKey,
	)
	state, err = state.ApplyPayloadCbor(
		1, 101, []cbor.RawMessage{activationCertificate},
	)
	require.NoError(t, err)
	require.Equal(
		t,
		initialDelegateHash,
		state.ActiveDelegations()[issuerHash],
		"a certificate must not activate before slot current+2k",
	)
	state = state.Tick(1, 120)
	require.Equal(t, initialDelegateHash, state.ActiveDelegations()[issuerHash])
	state = state.Tick(1, 121)
	require.Equal(
		t,
		replacementDelegateHash,
		state.ActiveDelegations()[issuerHash],
	)

	revocationCertificate := signedPBFTDelegationCertificate(
		t,
		protocolMagic,
		2,
		issuerKey,
		issuerPrivateKey,
		issuerKey,
	)
	state, err = state.ApplyPayloadCbor(
		2, 201, []cbor.RawMessage{revocationCertificate},
	)
	require.NoError(t, err)
	state = state.Tick(2, 220)
	require.Equal(
		t,
		replacementDelegateHash,
		state.ActiveDelegations()[issuerHash],
	)
	state = state.Tick(2, 221)
	require.Equal(
		t,
		issuerHash,
		state.ActiveDelegations()[issuerHash],
		"self-delegation must revoke the prior delegate",
	)
}

func TestNewPBFTDelegationStateUsesFinalGenesisView(t *testing.T) {
	issuerAKey, _ := deterministicPBFTVerificationKey(0x31)
	issuerBKey, _ := deterministicPBFTVerificationKey(0x32)
	delegateKey, _ := deterministicPBFTVerificationKey(0x33)
	issuerA, err := PBFTVerificationKeyHash(issuerAKey)
	require.NoError(t, err)
	issuerB, err := PBFTVerificationKeyHash(issuerBKey)
	require.NoError(t, err)
	delegate, err := PBFTVerificationKeyHash(delegateKey)
	require.NoError(t, err)
	config := ByronConfig{
		ProtocolMagic:    42,
		SecurityParam:    10,
		GenesisKeyHashes: [][]byte{issuerA.Bytes(), issuerB.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerA: issuerB,
			issuerB: delegate,
		},
	}

	for range 100 {
		state, err := NewPBFTDelegationState(config)
		require.NoError(t, err)
		require.Equal(t, config.GenesisDelegations, state.ActiveDelegations())
	}
}

func TestNewPBFTDelegationStateRejectsFinalDelegateCollision(t *testing.T) {
	issuerAKey, _ := deterministicPBFTVerificationKey(0x34)
	issuerBKey, _ := deterministicPBFTVerificationKey(0x35)
	delegateKey, _ := deterministicPBFTVerificationKey(0x36)
	issuerA, err := PBFTVerificationKeyHash(issuerAKey)
	require.NoError(t, err)
	issuerB, err := PBFTVerificationKeyHash(issuerBKey)
	require.NoError(t, err)
	delegate, err := PBFTVerificationKeyHash(delegateKey)
	require.NoError(t, err)

	_, err = NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    42,
		SecurityParam:    10,
		GenesisKeyHashes: [][]byte{issuerA.Bytes(), issuerB.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerA: delegate,
			issuerB: delegate,
		},
	})
	require.ErrorContains(t, err, "is active for both")
}

func TestPBFTDelegationStateRejectsInvalidPayloadAtomically(t *testing.T) {
	const protocolMagic = uint32(42)
	issuerKey, issuerPrivateKey := deterministicPBFTVerificationKey(0x41)
	initialDelegateKey, _ := deterministicPBFTVerificationKey(0x42)
	replacementDelegateKey, _ := deterministicPBFTVerificationKey(0x43)
	issuerHash, err := PBFTVerificationKeyHash(issuerKey)
	require.NoError(t, err)
	initialDelegateHash, err := PBFTVerificationKeyHash(initialDelegateKey)
	require.NoError(t, err)
	state, err := NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    protocolMagic,
		SecurityParam:    10,
		GenesisKeyHashes: [][]byte{issuerHash.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerHash: initialDelegateHash,
		},
	})
	require.NoError(t, err)
	certificate := signedPBFTDelegationCertificate(
		t,
		protocolMagic,
		1,
		issuerKey,
		issuerPrivateKey,
		replacementDelegateKey,
	)
	malformed, err := cbor.Encode([]any{})
	require.NoError(t, err)
	_, err = state.ApplyPayloadCbor(
		1, 101, []cbor.RawMessage{certificate, malformed},
	)
	require.ErrorContains(
		t, err, "delegation certificate is not a 4-element array",
	)
	require.Equal(
		t,
		initialDelegateHash,
		state.ActiveDelegations()[issuerHash],
		"a rejected payload must not mutate the input state",
	)
	require.Empty(t, state.scheduledDelegations)
}

func TestPBFTDelegationStateRejectsDuplicateEpoch(t *testing.T) {
	const protocolMagic = uint32(42)
	issuerKey, issuerPrivateKey := deterministicPBFTVerificationKey(0x51)
	initialDelegateKey, _ := deterministicPBFTVerificationKey(0x52)
	replacementDelegateKey, _ := deterministicPBFTVerificationKey(0x53)
	issuerHash, err := PBFTVerificationKeyHash(issuerKey)
	require.NoError(t, err)
	initialDelegateHash, err := PBFTVerificationKeyHash(initialDelegateKey)
	require.NoError(t, err)
	state, err := NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    protocolMagic,
		SecurityParam:    10,
		GenesisKeyHashes: [][]byte{issuerHash.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerHash: initialDelegateHash,
		},
	})
	require.NoError(t, err)
	certificate := signedPBFTDelegationCertificate(
		t,
		protocolMagic,
		1,
		issuerKey,
		issuerPrivateKey,
		replacementDelegateKey,
	)
	state, err = state.ApplyPayloadCbor(1, 101, []cbor.RawMessage{certificate})
	require.NoError(t, err)
	_, err = state.ApplyPayloadCbor(1, 102, []cbor.RawMessage{certificate})
	require.ErrorContains(t, err, "already delegated for epoch 1")
}

// TestPBFTDelegationStateApplyPayloadDecodedCompat pins the deprecated
// ApplyPayload entry point, which downstream callers reach with a decoded
// ByronMainBlockBody.DlgPayload ([]any) rather than preserved CBOR.
func TestPBFTDelegationStateApplyPayloadDecodedCompat(t *testing.T) {
	const protocolMagic = uint32(42)
	issuerKey, issuerPrivateKey := deterministicPBFTVerificationKey(0x61)
	initialDelegateKey, _ := deterministicPBFTVerificationKey(0x62)
	replacementDelegateKey, _ := deterministicPBFTVerificationKey(0x63)
	issuerHash, err := PBFTVerificationKeyHash(issuerKey)
	require.NoError(t, err)
	initialDelegateHash, err := PBFTVerificationKeyHash(initialDelegateKey)
	require.NoError(t, err)
	replacementHash, err := PBFTVerificationKeyHash(replacementDelegateKey)
	require.NoError(t, err)
	state, err := NewPBFTDelegationState(ByronConfig{
		ProtocolMagic:    protocolMagic,
		SecurityParam:    10,
		GenesisKeyHashes: [][]byte{issuerHash.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuerHash: initialDelegateHash,
		},
	})
	require.NoError(t, err)

	certificate := signedPBFTDelegationCertificate(
		t,
		protocolMagic,
		1,
		issuerKey,
		issuerPrivateKey,
		replacementDelegateKey,
	)
	// Decode the certificate the way a block body would present it, so the
	// payload has the []any shape the deprecated entry point takes.
	var decoded []any
	_, err = cbor.Decode(certificate, &decoded)
	require.NoError(t, err)

	state, err = state.ApplyPayload(1, 101, []any{decoded})
	require.NoError(t, err)
	state = state.Tick(1, 121)
	require.Equal(
		t,
		replacementHash,
		state.ActiveDelegations()[issuerHash],
		"the decoded payload path must still schedule and activate",
	)

	_, err = state.ApplyPayload(1, 101, []any{"not a certificate"})
	require.ErrorContains(
		t, err, "delegation certificate is not a 4-element array",
	)
}
