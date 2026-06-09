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

package common

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGovAnchor(t *testing.T) {
	dataHash := make([]byte, 32)
	for i := range dataHash {
		dataHash[i] = byte(i)
	}
	anchor, err := NewGovAnchor("https://example.com/proposal.json", dataHash)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/proposal.json", anchor.Url)
	assert.Equal(t, [32]byte(dataHash), anchor.DataHash)

	// Negative: wrong-length data hash
	_, err = NewGovAnchor("https://example.com", make([]byte, 31))
	require.Error(t, err)
	_, err = NewGovAnchor("https://example.com", make([]byte, 33))
	require.Error(t, err)
}

func TestNewGovActionId(t *testing.T) {
	txId := make([]byte, 32)
	for i := range txId {
		txId[i] = byte(i)
	}
	id, err := NewGovActionId(txId, 3)
	require.NoError(t, err)
	assert.Equal(t, [32]byte(txId), id.TransactionId)
	assert.Equal(t, uint32(3), id.GovActionIdx)

	// Negative: wrong-length transaction id
	_, err = NewGovActionId(make([]byte, 16), 0)
	require.Error(t, err)
}

func TestNewHardForkInitiationGovAction(t *testing.T) {
	action, err := NewHardForkInitiationGovAction(nil, 10, 1)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeHardForkInitiation), action.Type)
	assert.Nil(t, action.ActionId)
	assert.Equal(t, uint(10), action.ProtocolVersion.Major)
	assert.Equal(t, uint(1), action.ProtocolVersion.Minor)

	// CBOR round-trip preserves the discriminant and version
	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	assert.NotEmpty(t, cborData)
	var decoded HardForkInitiationGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeHardForkInitiation), decoded.Type)
	assert.Equal(t, uint(10), decoded.ProtocolVersion.Major)
	assert.Equal(t, uint(1), decoded.ProtocolVersion.Minor)

	// Optional parent action id is carried through
	actionId := &GovActionId{GovActionIdx: 7}
	withParent, err := NewHardForkInitiationGovAction(actionId, 11, 0)
	require.NoError(t, err)
	assert.Equal(t, actionId, withParent.ActionId)
}

func TestNewTreasuryWithdrawalGovAction(t *testing.T) {
	addr, err := NewAddress(
		"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
	)
	require.NoError(t, err)
	withdrawals := map[*Address]uint64{&addr: 5_000_000}
	policyHash := make([]byte, Blake2b224Size)

	action, err := NewTreasuryWithdrawalGovAction(withdrawals, policyHash)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeTreasuryWithdrawal), action.Type)

	// CBOR round-trip preserves the discriminant and withdrawals
	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	var decoded TreasuryWithdrawalGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeTreasuryWithdrawal), decoded.Type)
	assert.Len(t, decoded.Withdrawals, 1)

	// Empty policy hash is valid (optional field)
	noPolicy, err := NewTreasuryWithdrawalGovAction(withdrawals, nil)
	require.NoError(t, err)
	assert.Empty(t, noPolicy.PolicyHash)

	// Negative: at least one withdrawal is required
	_, err = NewTreasuryWithdrawalGovAction(nil, policyHash)
	require.Error(t, err)
	// Negative: non-empty policy hash must be 28 bytes
	_, err = NewTreasuryWithdrawalGovAction(withdrawals, []byte{1, 2})
	require.Error(t, err)
	// Negative: a nil address key would panic in ToPlutusData
	_, err = NewTreasuryWithdrawalGovAction(
		map[*Address]uint64{nil: 5_000_000},
		nil,
	)
	require.Error(t, err)
}

func TestNewNoConfidenceGovAction(t *testing.T) {
	action, err := NewNoConfidenceGovAction(nil)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeNoConfidence), action.Type)
	assert.Nil(t, action.ActionId)

	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	var decoded NoConfidenceGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeNoConfidence), decoded.Type)

	// Optional parent action id is carried through
	actionId := &GovActionId{GovActionIdx: 4}
	withParent, err := NewNoConfidenceGovAction(actionId)
	require.NoError(t, err)
	assert.Equal(t, actionId, withParent.ActionId)
}

func TestNewUpdateCommitteeGovAction(t *testing.T) {
	cred := Credential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: NewBlake2b224([]byte("test")),
	}
	creds := []Credential{cred}
	credEpochs := map[*Credential]uint{&cred: 42}

	// Negative: a zero-value Rat has a nil inner value and panics when CBOR
	// encoded, so the constructor must reject it up front.
	_, err := NewUpdateCommitteeGovAction(
		nil, creds, credEpochs, cbor.Rat{},
	)
	require.Error(t, err)

	// Populated Rat: full CBOR round-trip
	action, err := NewUpdateCommitteeGovAction(
		&GovActionId{},
		creds,
		credEpochs,
		cbor.Rat{Rat: big.NewRat(2, 3)},
	)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeUpdateCommittee), action.Type)

	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	var decoded UpdateCommitteeGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeUpdateCommittee), decoded.Type)
	assert.Len(t, decoded.Credentials, 1)

	// Negative: a nil credential key would panic in ToPlutusData
	_, err = NewUpdateCommitteeGovAction(
		nil,
		creds,
		map[*Credential]uint{nil: 1},
		cbor.Rat{Rat: big.NewRat(2, 3)},
	)
	require.Error(t, err)
}

func TestNewNewConstitutionGovAction(t *testing.T) {
	dataHash := make([]byte, 32)
	anchor, err := NewGovAnchor("https://example.com/constitution.json", dataHash)
	require.NoError(t, err)
	scriptHash := make([]byte, Blake2b224Size)

	action, err := NewNewConstitutionGovAction(nil, anchor, scriptHash)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeNewConstitution), action.Type)
	assert.Equal(t, anchor, action.Constitution.Anchor)
	assert.Equal(t, scriptHash, action.Constitution.ScriptHash)

	// CBOR round-trip preserves the discriminant
	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	var decoded NewConstitutionGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeNewConstitution), decoded.Type)

	// Empty script hash is valid (optional guardrail script)
	noScript, err := NewNewConstitutionGovAction(nil, anchor, nil)
	require.NoError(t, err)
	assert.Empty(t, noScript.Constitution.ScriptHash)

	// Negative: non-empty script hash must be 28 bytes
	_, err = NewNewConstitutionGovAction(nil, anchor, []byte{1, 2})
	require.Error(t, err)
}

func TestNewInfoGovAction(t *testing.T) {
	action, err := NewInfoGovAction()
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeInfo), action.Type)

	cborData, err := cbor.Encode(action)
	require.NoError(t, err)
	var decoded InfoGovAction
	_, err = cbor.Decode(cborData, &decoded)
	require.NoError(t, err)
	assert.Equal(t, uint(GovActionTypeInfo), decoded.Type)
}
