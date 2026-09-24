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

package common_test

import (
	"math"
	"math/big"
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

type protocolParameterUpdateDecoder interface {
	UnmarshalCBOR([]byte) error
}

func TestClassicProtocolParameterUpdateDomains(t *testing.T) {
	newDecoders := map[string]func() protocolParameterUpdateDecoder{
		"Shelley": func() protocolParameterUpdateDecoder {
			return &shelley.ShelleyProtocolParameterUpdate{}
		},
		"Allegra": func() protocolParameterUpdateDecoder {
			return &allegra.AllegraProtocolParameterUpdate{}
		},
		"Mary": func() protocolParameterUpdateDecoder {
			return &mary.MaryProtocolParameterUpdate{}
		},
		"Alonzo": func() protocolParameterUpdateDecoder {
			return &alonzo.AlonzoProtocolParameterUpdate{}
		},
		"Babbage": func() protocolParameterUpdateDecoder {
			return &babbage.BabbageProtocolParameterUpdate{}
		},
	}
	unitOutside := &cbor.Rat{Rat: big.NewRat(5, 4)}
	negative := &cbor.Rat{Rat: big.NewRat(-1, 4)}
	tooWideRatio := cbor.RawMessage{
		0xd8, 0x1e, 0x82, 0xc2, 0x49,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01,
	}
	tests := []struct {
		name       string
		field      uint64
		value      any
		wantError  bool
		postAlonzo bool
	}{
		{name: "Word32 body size width", field: 2, value: uint64(math.MaxUint32) + 1, wantError: true},
		{name: "Word32 transaction size width", field: 3, value: uint64(math.MaxUint32) + 1, wantError: true},
		{name: "Word16 header size width", field: 4, value: uint64(math.MaxUint16) + 1, wantError: true},
		{name: "MaxEpoch must be unsigned", field: 7, value: int64(-1), wantError: true},
		{name: "Word16 desired pool count width", field: 8, value: uint64(math.MaxUint16) + 1, wantError: true},
		{name: "negative A0", field: 9, value: negative, wantError: true},
		{name: "unit interval upper bound", field: 10, value: unitOutside, wantError: true},
		{name: "unit interval endpoint", field: 10, value: &cbor.Rat{Rat: big.NewRat(1, 1)}},
		{name: "bounded rational numerator", field: 9, value: tooWideRatio, wantError: true},
		{name: "protocol major Word16 width", field: 14, value: common.ProtocolParametersProtocolVersion{Major: uint(math.MaxUint16) + 1}, wantError: true},
		{name: "Word32 value size width", field: 22, value: uint64(math.MaxUint32) + 1, wantError: true, postAlonzo: true},
		{name: "Word16 collateral percentage width", field: 23, value: uint64(math.MaxUint16) + 1, wantError: true, postAlonzo: true},
		{name: "Word16 collateral input count width", field: 24, value: uint64(math.MaxUint16) + 1, wantError: true, postAlonzo: true},
		{name: "negative execution price", field: 19, value: []any{negative, &cbor.Rat{Rat: big.NewRat(1, 2)}}, wantError: true, postAlonzo: true},
		{name: "negative execution budget", field: 20, value: []any{int64(-1), int64(0)}, wantError: true, postAlonzo: true},
	}
	for era, newDecoder := range newDecoders {
		for _, test := range tests {
			if test.postAlonzo && era != "Alonzo" && era != "Babbage" {
				continue
			}
			t.Run(era+"/"+test.name, func(t *testing.T) {
				encoded, err := cbor.Encode(map[uint64]any{test.field: test.value})
				require.NoError(t, err)
				decoder := newDecoder()
				err = decoder.UnmarshalCBOR(encoded)
				if test.wantError {
					require.Error(t, err)
					var domainErr common.ProtocolParameterUpdateDomainError
					require.ErrorAs(t, err, &domainErr)
					return
				}
				require.NoError(t, err)
				reencoded, err := cbor.Encode(decoder)
				require.NoError(t, err)
				require.NotEmpty(t, reencoded)
			})
		}
	}
}
