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

package ledger

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

const (
	certRbTestAnnouncingSlot = 1000
	// certRbTestGapSlots is the certification gap for certRbTestParams over
	// one-second slots: (3*1000 + 4000 + 2500) ms is 9.5 slots, rounded up.
	certRbTestGapSlots = 10
	certRbTestSlot     = certRbTestAnnouncingSlot + certRbTestGapSlots
)

// certRbTestState provides the Leios header and certificate state for a
// CertRB, with a one-seat committee that signs in the legacy domain.
type certRbTestState struct {
	common.LedgerState
	header common.DijkstraLeiosHeaderContext
	cert   common.DijkstraLeiosCertificateContext
}

func (s certRbTestState) DijkstraLeiosHeaderContext(
	common.BlockHeader,
) (common.DijkstraLeiosHeaderContext, error) {
	return s.header, nil
}

func (s certRbTestState) DijkstraLeiosCertificateContext(
	common.BlockHeader,
) (common.DijkstraLeiosCertificateContext, error) {
	return s.cert, nil
}

func (s certRbTestState) DijkstraBlsProofDomain() common.DijkstraBlsProofDomain {
	return common.DijkstraBlsProofDomainLegacySig
}

func certRbTestParams() *dijkstra.DijkstraProtocolParameters {
	pp := &dijkstra.DijkstraProtocolParameters{
		LeiosAnnouncementPeriodLength:  1000,
		LeiosVotePeriodLength:          4000,
		LeiosDiffusionPeriodLength:     2500,
		MaxEndorserBlockReferencesSize: 1000,
		MaxEndorserBlockTxsSize:        12_500_000,
		LeiosCommitteeSize:             1,
		LeiosQuorumStakeThreshold:      &cbor.Rat{Rat: big.NewRat(1, 1)},
	}
	pp.ProtocolVersion.Major = dijkstra.MinProtocolVersionDijkstra
	return pp
}

func certRbTestHeaderBody(slot uint64) babbage.BabbageBlockHeaderBody {
	return babbage.BabbageBlockHeaderBody{
		Slot:   slot,
		VrfKey: make([]byte, 32),
		VrfResult: common.VrfResult{
			Output: make([]byte, 64),
			Proof:  make([]byte, 80),
		},
		OpCert: babbage.BabbageOpCert{
			HotVkey:   make([]byte, 32),
			Signature: make([]byte, 64),
		},
		ProtoVersion: babbage.BabbageProtoVersion{
			Major: dijkstra.MinProtocolVersionDijkstra,
		},
	}
}

// newCertRbTestBlock builds a CertRB as LocalChainSync sends it: its header
// signs the on-chain body (the certificate alone), and its body also carries
// txs. It returns the decoded block, its state, and the on-chain body size.
func newCertRbTestBlock(
	t *testing.T,
	txs []dijkstra.DijkstraTransaction,
) (*dijkstra.DijkstraBlock, certRbTestState, int) {
	t.Helper()
	announcement, err := cbor.Encode([]any{
		make([]byte, common.Blake2b256Size),
		uint32(10),
	})
	require.NoError(t, err)
	predecessor := &dijkstra.DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body:      certRbTestHeaderBody(certRbTestAnnouncingSlot),
			Signature: make([]byte, 448),
		},
		LeiosHeaderExtension: []cbor.RawMessage{{0xf4}, announcement},
	}
	predecessorCbor, err := predecessor.MarshalCBOR()
	require.NoError(t, err)
	predecessor.SetCbor(predecessorCbor)
	announcingHash := predecessor.Hash()

	// One committee member signs the announcing block hash.
	const legacyDST = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
	sign := func(message []byte, secret *big.Int) []byte {
		hash, err := bls12381.HashToG1(message, []byte(legacyDST))
		require.NoError(t, err)
		var sig bls12381.G1Affine
		sig.ScalarMultiplication(&hash, secret)
		encoded := sig.Bytes()
		return encoded[:]
	}
	secret := big.NewInt(23)
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(secret)
	encodedPublic := public.Bytes()
	message := append([]byte{0x58, 0x20}, announcingHash[:]...)
	cert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: sign(message, secret),
	}

	onChainCbor, err := cbor.Encode(
		dijkstra.DijkstraBlockBody{LeiosCertificate: cert},
	)
	require.NoError(t, err)
	headerBody := certRbTestHeaderBody(certRbTestSlot)
	headerBody.PrevHash = announcingHash
	headerBody.BlockBodySize = uint64(len(onChainCbor))
	headerBody.BlockBodyHash = common.Blake2b256Hash(onChainCbor)
	blockCbor, err := cbor.Encode(&dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body:      headerBody,
				Signature: make([]byte, 448),
			},
			// leios_certified = true, leios_announcement = null
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}, {0xf6}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions:     txs,
			LeiosCertificate: cert,
		},
	})
	require.NoError(t, err)
	block, err := dijkstra.NewDijkstraBlockFromCbor(
		blockCbor,
		common.VerifyConfig{AllowCertifiedBlockInlinedTransactions: true},
	)
	require.NoError(t, err)

	state := certRbTestState{
		header: common.DijkstraLeiosHeaderContext{
			Predecessor: predecessor,
			SlotLength:  time.Second,
		},
		cert: common.DijkstraLeiosCertificateContext{
			AnnouncingBlockHash: announcingHash,
			TotalActiveStake:    1,
			Committee: []common.DijkstraLeiosCommitteeMember{{
				Stake: 1,
				Key: &common.LeiosKey{
					PublicKey:       encodedPublic[:],
					PossessionProof: sign(encodedPublic[:], secret),
				},
			}},
		},
	}
	return block, state, len(onChainCbor)
}

func certRbTestTxs() []dijkstra.DijkstraTransaction {
	return []dijkstra.DijkstraTransaction{
		{Body: dijkstra.DijkstraTransactionBody{TxFee: 1}, TxIsValid: true},
	}
}

func TestVerifyBlockStateInlinedCertRb(t *testing.T) {
	t.Parallel()

	t.Run("rejected by default", func(t *testing.T) {
		t.Parallel()
		block, state, _ := newCertRbTestBlock(t, certRbTestTxs())
		err := VerifyBlockState(block, common.VerifyConfig{
			ProtocolParameters:        certRbTestParams(),
			LedgerState:               state,
			SkipBlockLimitsValidation: true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		})
		var both *dijkstra.LeiosCertifiedBlockTransactionsError
		require.ErrorAs(t, err, &both)
	})

	// The RB body size limit applies to the on-chain body: the limit is set
	// to exactly its size, which the full body exceeds.
	t.Run("accepted on node-to-client", func(t *testing.T) {
		t.Parallel()
		block, state, onChainSize := newCertRbTestBlock(t, certRbTestTxs())
		pp := certRbTestParams()
		pp.MaxBlockBodySize = uint(onChainSize)
		fullSize, err := common.BlockBodySizeFromCbor(block.Cbor())
		require.NoError(t, err)
		require.Greater(t, fullSize, uint64(onChainSize))
		require.NoError(t, VerifyBlockState(block, common.VerifyConfig{
			ProtocolParameters:                     pp,
			LedgerState:                            state,
			SkipTransactionValidation:              true,
			SkipStakePoolValidation:                true,
			AllowCertifiedBlockInlinedTransactions: true,
		}))
	})

	t.Run("on-chain body over the limit rejected", func(t *testing.T) {
		t.Parallel()
		block, state, onChainSize := newCertRbTestBlock(t, certRbTestTxs())
		pp := certRbTestParams()
		pp.MaxBlockBodySize = uint(onChainSize - 1)
		err := VerifyBlockState(block, common.VerifyConfig{
			ProtocolParameters:                     pp,
			LedgerState:                            state,
			SkipTransactionValidation:              true,
			SkipStakePoolValidation:                true,
			AllowCertifiedBlockInlinedTransactions: true,
		})
		require.ErrorIs(t, err, common.ErrBlockBodySizeTooBig)
	})

	// The inlined EB transactions are bounded by maxEndorserBlockTxsSize:
	// a total equal to the maximum is accepted, one byte more is rejected.
	t.Run("EB transactions bounded by maxEndorserBlockTxsSize", func(t *testing.T) {
		t.Parallel()
		block, state, _ := newCertRbTestBlock(t, certRbTestTxs())
		var total uint64
		for _, tx := range block.Transactions() {
			size, err := common.TxSize(tx)
			require.NoError(t, err)
			total += uint64(size)
		}
		verify := func(maximum uint64) error {
			pp := certRbTestParams()
			pp.MaxEndorserBlockTxsSize = uint32(maximum)
			return VerifyBlockState(block, common.VerifyConfig{
				ProtocolParameters:                     pp,
				LedgerState:                            state,
				SkipTransactionValidation:              true,
				SkipStakePoolValidation:                true,
				AllowCertifiedBlockInlinedTransactions: true,
			})
		}
		require.NoError(t, verify(total))
		err := verify(total - 1)
		var tooBig *dijkstra.LeiosClosureTooBigError
		require.ErrorAs(t, err, &tooBig)
		require.Equal(t, total, tooBig.ClosureSize)
	})

	// The EB's transactions are applied before the tick to the CertRB's
	// slot, so they are validated at the announcing RB's slot. The test
	// state has no UTxOs, so a rule fails; its error records the slot it ran
	// at.
	t.Run("transactions validated at the announcing slot", func(t *testing.T) {
		t.Parallel()
		block, state, _ := newCertRbTestBlock(t, certRbTestTxs())
		state.LedgerState = mockledger.NewLedgerStateBuilder().Build()
		err := VerifyBlockState(block, common.VerifyConfig{
			ProtocolParameters:                     certRbTestParams(),
			LedgerState:                            state,
			SkipBlockLimitsValidation:              true,
			SkipStakePoolValidation:                true,
			AllowCertifiedBlockInlinedTransactions: true,
		})
		require.Error(t, err)
		ruleSlot, found := transactionRuleSlot(err)
		require.True(t, found, "no transaction rule error in %v", err)
		require.Equal(t, uint64(certRbTestAnnouncingSlot), ruleSlot)
	})
}

// transactionRuleSlot returns the slot recorded by the first transaction
// rule error in err's chain.
func transactionRuleSlot(err error) (uint64, bool) {
	for err != nil {
		var vErr *common.ValidationError
		if !errors.As(err, &vErr) {
			return 0, false
		}
		if _, ok := vErr.Details["rule_index"]; ok {
			slot, ok := vErr.Details["slot"].(uint64)
			return slot, ok
		}
		err = vErr.Cause
	}
	return 0, false
}

func TestLeiosAnnouncingSlot(t *testing.T) {
	t.Parallel()
	block, state, _ := newCertRbTestBlock(t, certRbTestTxs())

	slot, err := leiosAnnouncingSlot(block.BlockHeader, state)
	require.NoError(t, err)
	require.Equal(t, uint64(certRbTestAnnouncingSlot), slot)

	_, err = leiosAnnouncingSlot(block.BlockHeader, nil)
	require.ErrorContains(t, err, "Leios header state")

	state.header.Predecessor = nil
	_, err = leiosAnnouncingSlot(block.BlockHeader, state)
	require.ErrorContains(t, err, "no announcing block")

	state.header.Predecessor = (*dijkstra.DijkstraBlockHeader)(nil)
	_, err = leiosAnnouncingSlot(block.BlockHeader, state)
	require.ErrorContains(t, err, "no announcing block")
}
