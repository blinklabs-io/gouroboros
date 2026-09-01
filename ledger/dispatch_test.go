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
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/stretchr/testify/require"
)

func TestErrorDispatchersAcceptListLengthEncodings(t *testing.T) {
	t.Run("ApplyTxError", func(t *testing.T) {
		failure, err := cbor.Encode([]any{42})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(failure) {
			t.Run(encoding.Name, func(t *testing.T) {
				data, err := cbor.Encode([]cbor.RawMessage{encoding.Data})
				require.NoError(t, err)
				decoded := ApplyTxError{era: EraIdConway}
				require.NoError(t, decoded.UnmarshalCBOR(data))
				require.Len(t, decoded.Failures, 1)
				unknown, ok := decoded.Failures[0].(*UnknownApplyTxFailureError)
				require.True(t, ok)
				require.Equal(t, 42, unknown.FailureType)
			})
		}
	})

	t.Run("UtxowFailure", func(t *testing.T) {
		canonical, err := cbor.Encode([]any{ConwayUtxowInvalidMetadata})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				decoded := UtxowFailure{era: EraIdConway}
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.IsType(t, &InvalidMetadata{}, decoded.Err)
			})
		}
	})

	t.Run("UtxoFailure", func(t *testing.T) {
		inner, err := cbor.Encode([]any{42})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(inner) {
			t.Run(encoding.Name, func(t *testing.T) {
				data, err := cbor.Encode([]any{
					uint8(EraIdConway),
					cbor.RawMessage(encoding.Data),
				})
				require.NoError(t, err)
				var decoded UtxoFailure
				require.NoError(t, decoded.UnmarshalCBOR(data))
				unknown, ok := decoded.Err.(*UnknownUtxoFailureError)
				require.True(t, ok)
				require.Equal(t, 42, unknown.FailureType)
			})
		}
	})

	directCases := []struct {
		name      string
		canonical []byte
		decode    func(*testing.T, []byte) error
	}{
		{
			name:      "ShelleyUtxowFailure",
			canonical: mustEncodeDispatchFixture(t, []any{ShelleyUtxowInvalidMetadata}),
			decode: func(t *testing.T, data []byte) error {
				var decoded ShelleyUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					require.IsType(t, &InvalidMetadata{}, decoded.Err)
				}
				return err
			},
		},
		{
			name:      "AlonzoUtxowFailure",
			canonical: mustEncodeDispatchFixture(t, []any{42, nil}),
			decode: func(t *testing.T, data []byte) error {
				var decoded AlonzoUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					unknown, ok := decoded.Err.(*UnknownUtxowFailureError)
					require.True(t, ok)
					require.Equal(t, 42, unknown.FailureType)
				}
				return err
			},
		},
		{
			name:      "BabbageUtxoFailure",
			canonical: mustEncodeDispatchFixture(t, []any{42, nil}),
			decode: func(t *testing.T, data []byte) error {
				var decoded BabbageUtxoFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					unknown, ok := decoded.Err.(*UnknownUtxoFailureError)
					require.True(t, ok)
					require.Equal(t, 42, unknown.FailureType)
				}
				return err
			},
		},
		{
			name:      "ConwayUtxowFailure",
			canonical: mustEncodeDispatchFixture(t, []any{ConwayUtxowInvalidMetadata}),
			decode: func(t *testing.T, data []byte) error {
				var decoded ConwayUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					require.IsType(t, &InvalidMetadata{}, decoded.Err)
				}
				return err
			},
		},
	}
	for _, tc := range directCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, encoding := range test.CanonicalAndNonShortestList(tc.canonical) {
				t.Run(encoding.Name, func(t *testing.T) {
					require.NoError(t, tc.decode(t, encoding.Data))
				})
			}
		})
	}
}

func mustEncodeDispatchFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := cbor.Encode(value)
	require.NoError(t, err)
	return data
}
