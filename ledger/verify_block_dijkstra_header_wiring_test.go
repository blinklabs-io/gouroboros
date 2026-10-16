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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// VerifyBlock applies the Dijkstra header version floor: a signed header whose
// highest supported major version is below the one in force is rejected.
func TestVerifyBlockDijkstraHeaderProtocolVersionFloor(t *testing.T) {
	block := buildDijkstraLimitsTestBlock(t, nil)
	tests := []struct {
		name      string
		major     uint
		wantError bool
	}{
		{name: "at the floor", major: dijkstra.MinProtocolVersionDijkstra},
		{
			name:      "below the floor",
			major:     dijkstra.MinProtocolVersionDijkstra + 1,
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{}
			pp.ProtocolVersion.Major = test.major
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:    true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var tooLow *dijkstra.HeaderProtVerTooLowError
			require.ErrorAs(t, err, &tooLow)
		})
	}
}

// The pipeline's ordered validation calls VerifyBlockState after the parallel
// VerifyBlockLocal pass, so the protocol-parameter header rules must run there
// and not only in the combined VerifyBlock.
func TestVerifyBlockStateDijkstraHeaderProtocolVersionFloor(t *testing.T) {
	block := buildDijkstraLimitsTestBlock(t, nil)
	config := common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
	}
	pp := &dijkstra.DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = dijkstra.MinProtocolVersionDijkstra
	config.ProtocolParameters = pp
	require.NoError(t, ledger.VerifyBlockState(block, config))

	pp = &dijkstra.DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = dijkstra.MinProtocolVersionDijkstra + 1
	config.ProtocolParameters = pp
	var tooLow *dijkstra.HeaderProtVerTooLowError
	require.ErrorAs(t, ledger.VerifyBlockState(block, config), &tooLow)
}

type leiosHeaderWiringState struct {
	common.LedgerState
	context common.DijkstraLeiosHeaderContext
}

func (s leiosHeaderWiringState) DijkstraLeiosHeaderContext(
	common.BlockHeader,
) (common.DijkstraLeiosHeaderContext, error) {
	return s.context, nil
}

func leiosHeaderExtension(
	t *testing.T,
	certified bool,
	announcedSize *uint32,
) []cbor.RawMessage {
	t.Helper()
	certifiedField := cbor.RawMessage{0xf4}
	if certified {
		certifiedField = cbor.RawMessage{0xf5}
	}
	announcement := cbor.RawMessage{0xf6}
	if announcedSize != nil {
		encoded, err := cbor.Encode([]any{
			make([]byte, common.Blake2b256Size),
			*announcedSize,
		})
		require.NoError(t, err)
		announcement = encoded
	}
	return []cbor.RawMessage{certifiedField, announcement}
}

// leiosAnnouncingPredecessor is an unsigned header at slot announcing an
// endorser block. The header state hands it back as the predecessor, so it is
// never verified itself.
func leiosAnnouncingPredecessor(
	t *testing.T,
	slot uint64,
) *dijkstra.DijkstraBlockHeader {
	t.Helper()
	size := uint32(10)
	header := &dijkstra.DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				Slot: slot,
				ProtoVersion: babbage.BabbageProtoVersion{
					Major: dijkstra.MinProtocolVersionDijkstra,
				},
			},
		},
		LeiosHeaderExtension: leiosHeaderExtension(t, false, &size),
	}
	encoded, err := header.MarshalCBOR()
	require.NoError(t, err)
	header.SetCbor(encoded)
	return header
}

func leiosWiringParams() *dijkstra.DijkstraProtocolParameters {
	pp := &dijkstra.DijkstraProtocolParameters{
		LeiosAnnouncementPeriodLength:  1000,
		LeiosVotePeriodLength:          4000,
		LeiosDiffusionPeriodLength:     2500,
		MaxEndorserBlockReferencesSize: 1000,
	}
	pp.ProtocolVersion.Major = dijkstra.MinProtocolVersionDijkstra
	return pp
}

// leiosWiringGapSlots is the certification gap for leiosWiringParams over
// one-second slots: (3*1000 + 4000 + 2500) ms is 9.5 slots, rounded up.
const leiosWiringGapSlots = 10

func leiosWiringCertificate() *dijkstra.DijkstraLeiosCertificate {
	return &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
}

func verifyLeiosWiringBlock(
	t *testing.T,
	block ledger.Block,
	pp common.ProtocolParameters,
	state common.LedgerState,
) (bool, []error) {
	t.Helper()
	config := common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
		ProtocolParameters:        pp,
		LedgerState:               state,
	}
	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		config,
	)
	// VerifyBlockState is the entry point the ordered pipeline uses, so each
	// rule must hold there as well as in the combined VerifyBlock.
	return valid, []error{err, ledger.VerifyBlockState(block, config)}
}

// VerifyBlock and VerifyBlockState reject a header announcing an endorser
// block larger than maxEndorserBlockReferencesSize, as leiosHeaderChecks does
// with LeiosEbTooBig in ouroboros-consensus 5cc95f4c. The bound is inclusive.
func TestVerifyBlockDijkstraLeiosAnnouncedSize(t *testing.T) {
	pp := leiosWiringParams()
	announcing := func(size uint32) ledger.Block {
		header := signedDijkstraTestHeader(
			t,
			func(h *dijkstra.DijkstraBlockHeader) {
				h.LeiosHeaderExtension = leiosHeaderExtension(t, false, &size)
			},
		)
		return decodeDijkstraWiringBlock(t, header, dijkstra.DijkstraBlockBody{})
	}

	valid, errs := verifyLeiosWiringBlock(
		t, announcing(pp.MaxEndorserBlockReferencesSize), pp, nil,
	)
	require.True(t, valid)
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	valid, errs = verifyLeiosWiringBlock(
		t, announcing(pp.MaxEndorserBlockReferencesSize+1), pp, nil,
	)
	require.False(t, valid)
	for _, got := range errs {
		var tooBig *dijkstra.LeiosEbTooBigError
		require.ErrorAs(t, got, &tooBig)
		require.NotNil(t, tooBig)
		require.Equal(t, pp.MaxEndorserBlockReferencesSize+1, tooBig.Announced)
		require.Equal(t, pp.MaxEndorserBlockReferencesSize, tooBig.Maximum)
	}
}

// VerifyBlock and VerifyBlockState reject a header certifying an endorser block
// announced by its predecessor fewer than minCertificationSlot slots earlier,
// as leiosHeaderChecks does with LeiosCertTooYoung in ouroboros-consensus
// 5cc95f4c. At the earliest allowed slot the header and body rules pass and the
// block fails later, on its certificate.
func TestVerifyBlockDijkstraLeiosCertificationGap(t *testing.T) {
	pp := leiosWiringParams()
	header := signedDijkstraTestHeader(t, nil)
	slot := header.SlotNumber()
	require.Greater(t, slot, uint64(leiosWiringGapSlots))
	certifying := func(announcingSlot uint64) (ledger.Block, common.LedgerState) {
		predecessor := leiosAnnouncingPredecessor(t, announcingSlot)
		header := signedDijkstraTestHeader(
			t,
			func(h *dijkstra.DijkstraBlockHeader) {
				h.Body.PrevHash = predecessor.Hash()
				h.LeiosHeaderExtension = leiosHeaderExtension(t, true, nil)
			},
		)
		block := decodeDijkstraWiringBlock(
			t,
			header,
			dijkstra.DijkstraBlockBody{
				LeiosCertificate: leiosWiringCertificate(),
			},
		)
		return block, leiosHeaderWiringState{
			context: common.DijkstraLeiosHeaderContext{
				Predecessor: predecessor,
				SlotLength:  time.Second,
			},
		}
	}

	block, state := certifying(slot - leiosWiringGapSlots)
	valid, errs := verifyLeiosWiringBlock(t, block, pp, state)
	require.False(t, valid, "the unverifiable certificate still fails")
	for _, got := range errs {
		// The parameters seat no committee, so certificate verification is
		// the first rule left to fail.
		require.ErrorContains(t, got, "non-zero committee size")
	}

	block, state = certifying(slot - leiosWiringGapSlots + 1)
	valid, errs = verifyLeiosWiringBlock(t, block, pp, state)
	require.False(t, valid)
	for _, got := range errs {
		var tooYoung *dijkstra.LeiosCertTooYoungError
		require.ErrorAs(t, got, &tooYoung)
		require.NotNil(t, tooYoung)
		require.Equal(t, slot-leiosWiringGapSlots+1, tooYoung.AnnouncingSlot)
		require.Equal(t, slot, tooYoung.Slot)
		require.Equal(t, slot+1, tooYoung.EarliestAllowed)
	}
}

// VerifyBlock and VerifyBlockState reject a certified block that also carries
// transactions, as blockMatchesHeader does in ouroboros-consensus 5cc95f4c.
// The decoder already refuses such a body, so the block is built directly.
func TestVerifyBlockDijkstraLeiosCertificateWithTransactions(t *testing.T) {
	pp := leiosWiringParams()
	header := signedDijkstraTestHeader(t, nil)
	predecessor := leiosAnnouncingPredecessor(
		t, header.SlotNumber()-leiosWiringGapSlots,
	)
	header = signedDijkstraTestHeader(
		t,
		func(h *dijkstra.DijkstraBlockHeader) {
			h.Body.PrevHash = predecessor.Hash()
			h.LeiosHeaderExtension = leiosHeaderExtension(t, true, nil)
		},
	)
	state := leiosHeaderWiringState{
		context: common.DijkstraLeiosHeaderContext{
			Predecessor: predecessor,
			SlotLength:  time.Second,
		},
	}
	body := dijkstra.DijkstraBlockBody{
		LeiosCertificate: leiosWiringCertificate(),
		Transactions:     []dijkstra.DijkstraTransaction{{TxIsValid: true}},
	}
	encodedBody, err := cbor.Encode(body)
	require.NoError(t, err)
	var decoded dijkstra.DijkstraBlockBody
	_, err = cbor.Decode(encodedBody, &decoded)
	var decodeErr *dijkstra.LeiosCertifiedBlockTransactionsError
	require.ErrorAs(t, err, &decodeErr)

	block := &dijkstra.DijkstraBlock{BlockHeader: header, BlockBody: body}
	valid, errs := verifyLeiosWiringBlock(t, block, pp, state)
	require.False(t, valid)
	for _, got := range errs {
		var both *dijkstra.LeiosCertifiedBlockTransactionsError
		require.ErrorAs(t, got, &both)
		require.NotNil(t, both)
		require.Equal(t, 1, both.TransactionCount)
	}
}

func decodeDijkstraWiringBlock(
	t *testing.T,
	header *dijkstra.DijkstraBlockHeader,
	body dijkstra.DijkstraBlockBody,
) ledger.Block {
	t.Helper()
	blockCbor, err := cbor.Encode(&dijkstra.DijkstraBlock{
		BlockHeader: header,
		BlockBody:   body,
	})
	require.NoError(t, err)
	block, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeDijkstra,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return block
}
