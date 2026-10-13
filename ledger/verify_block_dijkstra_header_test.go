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
	return dijkstraLeiosTestHeaderAfter(
		t, common.Blake2b256{}, slot, major, certified, announcedSize,
	)
}

// dijkstraLeiosTestHeaderAfter builds a header extending prevHash and stores
// its encoding, so that each header hashes to its own identity rather than to
// the hash of empty bytes.
func dijkstraLeiosTestHeaderAfter(
	t *testing.T,
	prevHash common.Blake2b256,
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
	header := &dijkstra.DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				Slot:         slot,
				PrevHash:     prevHash,
				ProtoVersion: babbage.BabbageProtoVersion{Major: major},
			},
		},
		LeiosHeaderExtension: []cbor.RawMessage{certifiedField, announcement},
	}
	encoded, err := header.MarshalCBOR()
	require.NoError(t, err)
	header.SetCbor(encoded)
	return header
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
	certifyingAfter := func(
		prev common.Blake2b256,
		slot uint64,
	) *dijkstra.DijkstraBlock {
		return &dijkstra.DijkstraBlock{
			BlockHeader: dijkstraLeiosTestHeaderAfter(
				t, prev, slot, 12, true, nil,
			),
		}
	}
	certifying := func(slot uint64) *dijkstra.DijkstraBlock {
		return certifyingAfter(announcing.Hash(), slot)
	}
	state := dijkstraLeiosHeaderTestState{
		context: common.DijkstraLeiosHeaderContext{
			Predecessor: announcing,
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
	t.Run("release candidate admits oversized announcement", func(t *testing.T) {
		over := params.MaxEndorserBlockReferencesSize + 1
		require.NoError(t, validateDijkstraLeiosHeader(
			&dijkstra.DijkstraBlock{
				BlockHeader: dijkstraLeiosTestHeader(t, 1, 12, false, &over),
			},
			params,
			nil,
		))
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
	t.Run("release candidate admits certificate before minimum gap", func(t *testing.T) {
		require.NoError(t, validateDijkstraLeiosHeader(
			certifying(109), params, state,
		))
	})
	t.Run("certificate without announcement", func(t *testing.T) {
		silent := dijkstraLeiosTestHeader(t, 100, 12, false, nil)
		block := certifyingAfter(silent.Hash(), 200)
		err := validateDijkstraLeiosHeader(
			block,
			params,
			dijkstraLeiosHeaderTestState{
				context: common.DijkstraLeiosHeaderContext{
					Predecessor: silent,
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
	t.Run("typed nil predecessor is the chain origin", func(t *testing.T) {
		var nilHeader *dijkstra.DijkstraBlockHeader
		var err error
		require.NotPanics(t, func() {
			err = validateDijkstraLeiosHeader(
				certifying(110),
				params,
				dijkstraLeiosHeaderTestState{
					context: common.DijkstraLeiosHeaderContext{
						Predecessor: nilHeader,
					},
				},
			)
		})
		var without *dijkstra.LeiosCertWithoutAnnouncementError
		requireProtocolError(t, err, &without)
	})
	t.Run("predecessor is not the previous header", func(t *testing.T) {
		block := certifyingAfter(common.Blake2b256{0x01}, 110)
		err := validateDijkstraLeiosHeader(block, params, state)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("predecessor is another announcing header", func(t *testing.T) {
		// Same announcement at an earlier slot: it would satisfy the timing
		// rule, so only header identity can reject it.
		other := dijkstraLeiosTestHeader(t, 99, 12, false, &size)
		require.NotEqual(t, announcing.Hash(), other.Hash())
		err := validateDijkstraLeiosHeader(
			certifying(110),
			params,
			dijkstraLeiosHeaderTestState{
				context: common.DijkstraLeiosHeaderContext{
					Predecessor: other,
				},
			},
		)
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
