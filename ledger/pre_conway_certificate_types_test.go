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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestPreConwayTransactionBodiesRejectConwayCertificates(t *testing.T) {
	testCases := []struct {
		name        string
		certificate any
	}{
		{
			name: "first Conway type",
			certificate: []any{
				uint(common.CertificateTypeRegistration),
				[]any{
					uint(common.CredentialTypeAddrKeyHash),
					make([]byte, common.Blake2b224Size),
				},
				uint64(0),
			},
		},
		{
			name: "last Conway type",
			certificate: []any{
				uint(common.CertificateTypeUpdateDrep),
				[]any{
					uint(common.CredentialTypeAddrKeyHash),
					make([]byte, common.Blake2b224Size),
				},
				nil,
			},
		},
	}
	decoders := []struct {
		name   string
		decode func([]byte) error
	}{
		{
			name: "Shelley",
			decode: func(data []byte) error {
				var body shelley.ShelleyTransactionBody
				return body.UnmarshalCBOR(data)
			},
		},
		{
			name: "Allegra",
			decode: func(data []byte) error {
				var body allegra.AllegraTransactionBody
				return body.UnmarshalCBOR(data)
			},
		},
		{
			name: "Mary",
			decode: func(data []byte) error {
				var body mary.MaryTransactionBody
				return body.UnmarshalCBOR(data)
			},
		},
		{
			name: "Alonzo",
			decode: func(data []byte) error {
				var body alonzo.AlonzoTransactionBody
				return body.UnmarshalCBOR(data)
			},
		},
		{
			name: "Babbage",
			decode: func(data []byte) error {
				var body babbage.BabbageTransactionBody
				return body.UnmarshalCBOR(data)
			},
		},
	}

	for _, decoder := range decoders {
		t.Run(decoder.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					encoded, err := cbor.Encode(map[uint]any{
						0: []any{},
						1: []any{},
						2: uint64(0),
						3: uint64(0),
						4: []any{tc.certificate},
					})
					require.NoError(t, err)
					require.ErrorContains(
						t,
						decoder.decode(encoded),
						"certificate type is not valid before Conway",
					)
				})
			}
		})
	}
}
