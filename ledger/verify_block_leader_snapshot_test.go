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

package ledger_test

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestVerifyBlockStateUsesLeaderSnapshot(t *testing.T) {
	block := newRealConwayBlock(t)
	h := block.BlockHeader
	b := &h.Body
	nonce, err := hex.DecodeString(realConwayHeaderEta0Hex)
	require.NoError(t, err)
	vrfHash := common.Blake2b256Hash(b.VrfKey)
	poolHash := common.Blake2b224Hash(b.IssuerVkey[:])
	validator := consensus.NewHeaderValidator(consensus.NetworkConfig{
		SlotsPerKESPeriod: testSlotsPerKesPeriod, MaxKESEvolutions: 62,
		ActiveSlotCoeff: common.GenesisRat{Rat: big.NewRat(1, 20)},
	})
	result := validator.ValidateHeader(&consensus.ValidateHeaderInput{
		Slot: b.Slot, BlockNumber: b.BlockNumber, PrevHash: b.PrevHash[:],
		IssuerVkey: b.IssuerVkey[:], VrfKey: b.VrfKey,
		VrfProof: b.VrfResult.Proof, VrfOutput: b.VrfResult.Output,
		KesSignature: h.Signature, HeaderBodyCbor: b.Cbor(),
		OpCertHotVkey:        b.OpCert.HotVkey,
		OpCertSequenceNumber: b.OpCert.SequenceNumber,
		OpCertKesPeriod:      b.OpCert.KesPeriod,
		OpCertSignature:      b.OpCert.Signature,
		OpCertCounterState:   &b.OpCert.SequenceNumber,
		PrevSlot:             b.Slot - 1, PrevBlockNumber: b.BlockNumber - 1,
		PrevHeaderHash: b.PrevHash[:], EpochNonce: nonce,
		PoolStake: 1, TotalStake: 1, RegisteredVrfKeyHash: vrfHash[:],
	})
	require.True(t, result.Valid, "%v", result.Errors)

	rotated := vrfHash
	rotated[0] ^= 1
	matching := &common.PoolRegistrationCertificate{
		Operator: poolHash, VrfKeyHash: vrfHash,
	}
	queryErr := errors.New("snapshot unavailable")
	for _, tc := range []struct {
		name            string
		current         *common.PoolRegistrationCertificate
		snapshot        *common.Blake2b256
		lookupErr       error
		missingProvider bool
		skip            bool
		want            common.ValidationErrorType
	}{
		{name: "matching registration", current: matching, snapshot: &vrfHash},
		{name: "retired but still active", snapshot: &vrfHash},
		{name: "rotation not yet active",
			current: &common.PoolRegistrationCertificate{
				Operator: poolHash, VrfKeyHash: rotated,
			}, snapshot: &vrfHash},
		{name: "current registration cannot establish membership",
			current: matching,
			want:    common.ValidationErrorTypeStakePool},
		{name: "current VRF cannot override snapshot", current: matching,
			snapshot: &rotated, want: common.ValidationErrorTypeVRF},
		{name: "lookup failure", current: matching, snapshot: &vrfHash,
			lookupErr: queryErr, want: common.ValidationErrorTypeStakePool},
		{name: "missing provider", current: matching, missingProvider: true,
			want: common.ValidationErrorTypeConfiguration},
		{name: "explicit skip", missingProvider: true, skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			currentQueries := 0
			ls := mockledger.NewLedgerStateBuilder().WithPoolCurrentState(func(
				_ common.PoolKeyHash,
			) (*common.PoolRegistrationCertificate, *uint64, error) {
				currentQueries++
				return tc.current, nil, nil
			}).Build()
			config := common.VerifyConfig{
				LedgerState: ls, SkipTransactionValidation: true,
				SkipBlockLimitsValidation: true,
				SkipStakePoolValidation:   tc.skip,
			}
			snapshotQueries := 0
			if !tc.missingProvider {
				config.LeaderPoolVrfKeyHash = func(
					slot uint64, pool common.PoolKeyHash,
				) (*common.Blake2b256, error) {
					snapshotQueries++
					require.Equal(t, b.Slot, slot)
					require.Equal(t, poolHash, pool)
					return tc.snapshot, tc.lookupErr
				}
			}
			err := ledger.VerifyBlockState(block, config)
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				var validationErr *common.ValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, tc.want, validationErr.Type)
				if tc.lookupErr != nil {
					require.ErrorIs(t, err, queryErr)
				}
			}
			require.Zero(t, currentQueries)
			if tc.missingProvider {
				require.Zero(t, snapshotQueries)
			} else {
				require.Equal(t, 1, snapshotQueries)
			}
		})
	}
}
