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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

type dijkstraLeiosHeaderTestState struct {
	common.LedgerState
	context common.DijkstraLeiosHeaderContext
	err     error
}

func (s dijkstraLeiosHeaderTestState) DijkstraLeiosHeaderContext(
	common.BlockHeader,
) (common.DijkstraLeiosHeaderContext, error) {
	return s.context, s.err
}

func dijkstraLeiosTestHeader(
	t *testing.T,
	slot uint64,
	major uint64,
	certified bool,
	announcedSize *uint32,
) *dijkstra.DijkstraBlockHeader {
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
	return &dijkstra.DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				Slot:         slot,
				ProtoVersion: babbage.BabbageProtoVersion{Major: major},
			},
		},
		LeiosHeaderExtension: []cbor.RawMessage{certifiedField, announcement},
	}
}

func TestValidateDijkstraLeiosHeader(t *testing.T) {
	t.Parallel()
	params := &dijkstra.DijkstraProtocolParameters{
		LeiosAnnouncementPeriodLength:  1000,
		LeiosVotePeriodLength:          4000,
		LeiosDiffusionPeriodLength:     2500,
		MaxEndorserBlockReferencesSize: 100000,
	}
	params.ProtocolVersion.Major = 12
	size := uint32(10)
	announcing := dijkstraLeiosTestHeader(t, 100, 12, false, &size)
	certifying := func(slot uint64) *dijkstra.DijkstraBlock {
		header := dijkstraLeiosTestHeader(t, slot, 12, true, nil)
		header.Body.PrevHash = announcing.Hash()
		return &dijkstra.DijkstraBlock{BlockHeader: header}
	}
	state := dijkstraLeiosHeaderTestState{
		context: common.DijkstraLeiosHeaderContext{
			Predecessor: announcing,
			SlotLength:  time.Second,
		},
	}
	requireProtocolError := func(t *testing.T, err error, target any) {
		t.Helper()
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
		require.ErrorAs(t, err, target)
	}

	t.Run("plain block", func(t *testing.T) {
		require.NoError(t, validateDijkstraLeiosHeader(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 12, false, &size),
			},
			params,
			nil,
		))
	})
	t.Run("no protocol parameters", func(t *testing.T) {
		require.NoError(t, validateDijkstraLeiosHeader(
			certifying(110), nil, nil,
		))
	})
	t.Run("header protocol version too low", func(t *testing.T) {
		err := validateDijkstraLeiosHeader(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 11, false, nil),
			},
			params,
			nil,
		)
		var tooLow *dijkstra.HeaderProtVerTooLowError
		requireProtocolError(t, err, &tooLow)
	})
	t.Run("announced endorser block too big", func(t *testing.T) {
		over := params.MaxEndorserBlockReferencesSize + 1
		err := validateDijkstraLeiosHeader(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 12, false, &over),
			},
			params,
			nil,
		)
		var tooBig *dijkstra.LeiosEbTooBigError
		requireProtocolError(t, err, &tooBig)
	})
	t.Run("announced endorser block at the limit", func(t *testing.T) {
		limit := params.MaxEndorserBlockReferencesSize
		require.NoError(t, validateDijkstraLeiosHeader(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 12, false, &limit),
			},
			params,
			nil,
		))
	})
	t.Run("certificate at the earliest slot", func(t *testing.T) {
		require.NoError(t, validateDijkstraLeiosHeader(
			certifying(110), params, state,
		))
	})
	t.Run("certificate too young", func(t *testing.T) {
		err := validateDijkstraLeiosHeader(certifying(109), params, state)
		var tooYoung *dijkstra.LeiosCertTooYoungError
		requireProtocolError(t, err, &tooYoung)
	})
	t.Run("certificate without announcement", func(t *testing.T) {
		silent := dijkstraLeiosTestHeader(t, 100, 12, false, nil)
		block := certifying(200)
		block.BlockHeader.Body.PrevHash = silent.Hash()
		err := validateDijkstraLeiosHeader(
			block,
			params,
			dijkstraLeiosHeaderTestState{
				context: common.DijkstraLeiosHeaderContext{
					Predecessor: silent,
					SlotLength:  time.Second,
				},
			},
		)
		var without *dijkstra.LeiosCertWithoutAnnouncementError
		requireProtocolError(t, err, &without)
	})
	t.Run("certified header requires header state", func(t *testing.T) {
		err := validateDijkstraLeiosHeader(certifying(110), params, nil)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("header state failure", func(t *testing.T) {
		cause := errors.New("no predecessor")
		err := validateDijkstraLeiosHeader(
			certifying(110),
			params,
			dijkstraLeiosHeaderTestState{err: cause},
		)
		require.ErrorIs(t, err, cause)
	})
	t.Run("predecessor is not the previous header", func(t *testing.T) {
		block := certifying(110)
		block.BlockHeader.Body.PrevHash = common.Blake2b256{0x01}
		err := validateDijkstraLeiosHeader(block, params, state)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("non-Dijkstra protocol parameters", func(t *testing.T) {
		err := validateDijkstraLeiosHeader(
			certifying(110),
			&babbage.BabbageProtocolParameters{},
			state,
		)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
}

func TestValidateDijkstraBlockCertificatesBodyRules(t *testing.T) {
	t.Parallel()
	cert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	t.Run("header without extension carries a certificate", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(
			&dijkstra.DijkstraBlock{
				BlockHeader: &dijkstra.DijkstraBlockHeader{},
				BlockBody: dijkstra.DijkstraBlockBody{
					LeiosCertificate: cert,
				},
			},
			nil,
			nil,
		)
		var mismatch *dijkstra.LeiosCertificateFlagMismatchError
		require.ErrorAs(t, err, &mismatch)
	})
	t.Run("certificate and transactions", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 12, true, nil),
				BlockBody: dijkstra.DijkstraBlockBody{
					LeiosCertificate: cert,
					Transactions:     []dijkstra.DijkstraTransaction{{}},
				},
			},
			nil,
			nil,
		)
		var both *dijkstra.LeiosCertifiedBlockTransactionsError
		require.ErrorAs(t, err, &both)
	})
	t.Run("Peras certificate", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(
			&dijkstra.DijkstraBlock{
				BlockBody: dijkstra.DijkstraBlockBody{
					PerasCertificate: []byte{},
				},
			},
			nil,
			nil,
		)
		var peras *dijkstra.PerasCertValidationFailedError
		require.ErrorAs(t, err, &peras)
	})
}
