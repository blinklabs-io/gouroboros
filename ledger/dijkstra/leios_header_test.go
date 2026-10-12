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

package dijkstra

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func testLeiosHeader(
	t *testing.T,
	slot uint64,
	major uint64,
	certified bool,
	announcedSize *uint32,
) *DijkstraBlockHeader {
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
	return &DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				Slot:         slot,
				ProtoVersion: babbage.BabbageProtoVersion{Major: major},
			},
		},
		LeiosHeaderExtension: []cbor.RawMessage{certifiedField, announcement},
	}
}

func testLeiosParams() *DijkstraProtocolParameters {
	pp := &DijkstraProtocolParameters{
		LeiosAnnouncementPeriodLength:  1000,
		LeiosVotePeriodLength:          4000,
		LeiosDiffusionPeriodLength:     2500,
		MaxEndorserBlockReferencesSize: 100000,
	}
	pp.ProtocolVersion.Major = 12
	return pp
}

func TestValidateHeaderProtocolVersion(t *testing.T) {
	t.Parallel()
	pp := testLeiosParams()
	require.NoError(t, ValidateHeaderProtocolVersion(
		testLeiosHeader(t, 1, 12, false, nil), pp,
	))
	// Dijkstra only bounds the issuer's highest supported major from below.
	require.NoError(t, ValidateHeaderProtocolVersion(
		testLeiosHeader(t, 1, 99, false, nil), pp,
	))
	err := ValidateHeaderProtocolVersion(
		testLeiosHeader(t, 1, 11, false, nil), pp,
	)
	var tooLow *HeaderProtVerTooLowError
	require.ErrorAs(t, err, &tooLow)
	require.NotNil(t, tooLow)
	require.Equal(t, uint64(11), tooLow.Supplied)
	require.Equal(t, uint64(12), tooLow.Expected)
}

func TestValidateLeiosAnnouncementSize(t *testing.T) {
	t.Parallel()
	pp := testLeiosParams()
	limit := pp.MaxEndorserBlockReferencesSize
	over := limit + 1
	require.NoError(t, ValidateLeiosAnnouncementSize(
		testLeiosHeader(t, 1, 12, false, nil), pp,
	))
	require.NoError(t, ValidateLeiosAnnouncementSize(
		testLeiosHeader(t, 1, 12, false, &limit), pp,
	))
	err := ValidateLeiosAnnouncementSize(
		testLeiosHeader(t, 1, 12, false, &over), pp,
	)
	var tooBig *LeiosEbTooBigError
	require.ErrorAs(t, err, &tooBig)
	require.NotNil(t, tooBig)
	require.Equal(t, over, tooBig.Announced)
	require.Equal(t, limit, tooBig.Maximum)
}

func TestLeiosMinCertificationSlot(t *testing.T) {
	t.Parallel()
	// (3*1000 + 4000 + 2500) ms over 1 s slots is 9.5, rounded up to 10.
	slot, err := LeiosMinCertificationSlot(time.Second, 1000, 4000, 2500, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(110), slot)
	slot, err = LeiosMinCertificationSlot(time.Second, 1000, 4000, 3000, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(110), slot)
	_, err = LeiosMinCertificationSlot(0, 1000, 4000, 2500, 100)
	require.Error(t, err)
}

func TestValidateLeiosCertificateTiming(t *testing.T) {
	t.Parallel()
	pp := testLeiosParams()
	size := uint32(10)
	announcing := testLeiosHeader(t, 100, 12, false, &size)

	t.Run("uncertified header needs no predecessor", func(t *testing.T) {
		require.NoError(t, ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 101, 12, false, nil), nil, pp, time.Second,
		))
	})
	t.Run("certificate at the earliest slot", func(t *testing.T) {
		require.NoError(t, ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 110, 12, true, nil),
			announcing, pp, time.Second,
		))
	})
	t.Run("predecessor wrapped by the chain state", func(t *testing.T) {
		wrapped := struct{ *DijkstraBlockHeader }{announcing}
		require.NoError(t, ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 110, 12, true, nil),
			wrapped, pp, time.Second,
		))
		err := ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 109, 12, true, nil),
			wrapped, pp, time.Second,
		)
		var tooYoung *LeiosCertTooYoungError
		require.ErrorAs(t, err, &tooYoung)
	})
	t.Run("certificate too young", func(t *testing.T) {
		err := ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 109, 12, true, nil),
			announcing, pp, time.Second,
		)
		var tooYoung *LeiosCertTooYoungError
		require.ErrorAs(t, err, &tooYoung)
		require.Equal(t, uint64(100), tooYoung.AnnouncingSlot)
		require.Equal(t, uint64(109), tooYoung.Slot)
		require.Equal(t, uint64(110), tooYoung.EarliestAllowed)
	})
	t.Run("predecessor announced nothing", func(t *testing.T) {
		err := ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 200, 12, true, nil),
			testLeiosHeader(t, 100, 12, false, nil),
			pp, time.Second,
		)
		var without *LeiosCertWithoutAnnouncementError
		require.ErrorAs(t, err, &without)
	})
	t.Run("no predecessor", func(t *testing.T) {
		err := ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 200, 12, true, nil), nil, pp, time.Second,
		)
		var without *LeiosCertWithoutAnnouncementError
		require.ErrorAs(t, err, &without)
	})
	t.Run("pre-Dijkstra predecessor", func(t *testing.T) {
		err := ValidateLeiosCertificateTiming(
			testLeiosHeader(t, 200, 12, true, nil),
			&babbage.BabbageBlockHeader{},
			pp, time.Second,
		)
		var without *LeiosCertWithoutAnnouncementError
		require.ErrorAs(t, err, &without)
	})
}

func TestValidateLeiosBlockBody(t *testing.T) {
	t.Parallel()
	cert := &DijkstraLeiosCertificate{
		Signers:             []byte{0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	certified := testLeiosHeader(t, 1, 12, true, nil)
	uncertified := testLeiosHeader(t, 1, 12, false, nil)

	t.Run("certified block with certificate", func(t *testing.T) {
		require.NoError(t, ValidateLeiosBlockBody(&DijkstraBlock{
			BlockHeader: certified,
			BlockBody:   DijkstraBlockBody{LeiosCertificate: cert},
		}))
	})
	t.Run("plain block", func(t *testing.T) {
		require.NoError(t, ValidateLeiosBlockBody(&DijkstraBlock{
			BlockHeader: uncertified,
			BlockBody: DijkstraBlockBody{
				Transactions: []DijkstraTransaction{{}},
			},
		}))
	})
	t.Run("header without extension carries a certificate", func(t *testing.T) {
		err := ValidateLeiosBlockBody(&DijkstraBlock{
			BlockHeader: &DijkstraBlockHeader{},
			BlockBody:   DijkstraBlockBody{LeiosCertificate: cert},
		})
		var mismatch *LeiosCertificateFlagMismatchError
		require.ErrorAs(t, err, &mismatch)
		require.False(t, mismatch.HeaderCertified)
		require.True(t, mismatch.BodyHasCertificate)
	})
	t.Run("certified header without certificate", func(t *testing.T) {
		err := ValidateLeiosBlockBody(&DijkstraBlock{BlockHeader: certified})
		var mismatch *LeiosCertificateFlagMismatchError
		require.ErrorAs(t, err, &mismatch)
	})
	t.Run("certificate and transactions", func(t *testing.T) {
		err := ValidateLeiosBlockBody(&DijkstraBlock{
			BlockHeader: certified,
			BlockBody: DijkstraBlockBody{
				LeiosCertificate: cert,
				Transactions:     []DijkstraTransaction{{}},
			},
		})
		var both *LeiosCertifiedBlockTransactionsError
		require.ErrorAs(t, err, &both)
		require.Equal(t, 1, both.TransactionCount)
	})
	t.Run("any Peras certificate", func(t *testing.T) {
		for _, perasCert := range [][]byte{{}, {0x01}} {
			err := ValidateLeiosBlockBody(&DijkstraBlock{
				BlockHeader: uncertified,
				BlockBody:   DijkstraBlockBody{PerasCertificate: perasCert},
			})
			var peras *PerasCertValidationFailedError
			require.ErrorAs(t, err, &peras)
		}
	})
}
