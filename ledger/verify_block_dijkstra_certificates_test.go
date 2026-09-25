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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

func TestValidateDijkstraBlockCertificates(t *testing.T) {
	t.Parallel()

	validCert := &dijkstra.DijkstraLeiosCertificate{
		Signers:             []byte{0x80, 0x80},
		AggregatedSignature: make([]byte, common.LeiosBlsSignatureSize),
	}
	block := &dijkstra.DijkstraBlock{
		BlockHeader: &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf5}},
		},
		BlockBody: dijkstra.DijkstraBlockBody{LeiosCertificate: validCert},
	}
	params := &dijkstra.DijkstraProtocolParameters{LeiosCommitteeSize: 9}

	t.Run("valid certificate", func(t *testing.T) {
		require.NoError(t, validateDijkstraBlockCertificates(block, params))
	})
	t.Run("missing protocol parameters", func(t *testing.T) {
		err := validateDijkstraBlockCertificates(block, nil)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})
	t.Run("invalid committee bitfield", func(t *testing.T) {
		invalid := *validCert
		invalid.Signers = []byte{0x80}
		invalidBlock := &dijkstra.DijkstraBlock{
			BlockBody: dijkstra.DijkstraBlockBody{LeiosCertificate: &invalid},
		}
		err := validateDijkstraBlockCertificates(invalidBlock, params)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("block without certificate", func(t *testing.T) {
		require.NoError(t, validateDijkstraBlockCertificates(
			&dijkstra.DijkstraBlock{},
			nil,
		))
	})
	t.Run("zero committee size", func(t *testing.T) {
		zeroCommittee := &dijkstra.DijkstraProtocolParameters{}
		err := validateDijkstraBlockCertificates(block, zeroCommittee)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeConfiguration, validationErr.Type)
	})
	t.Run("certified header without certificate", func(t *testing.T) {
		missing := &dijkstra.DijkstraBlock{
			BlockHeader: block.BlockHeader,
		}
		err := validateDijkstraBlockCertificates(missing, params)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("uncertified header with certificate", func(t *testing.T) {
		uncertified := *block
		uncertified.BlockHeader = &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0xf4}},
		}
		err := validateDijkstraBlockCertificates(&uncertified, params)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
	t.Run("malformed certified flag", func(t *testing.T) {
		malformed := *block
		malformed.BlockHeader = &dijkstra.DijkstraBlockHeader{
			LeiosHeaderExtension: []cbor.RawMessage{{0x01}},
		}
		err := validateDijkstraBlockCertificates(&malformed, params)
		require.Error(t, err)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(t, common.ValidationErrorTypeProtocol, validationErr.Type)
	})
}
