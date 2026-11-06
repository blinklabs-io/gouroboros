// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package shelley_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// byronAttrsFromWire builds attributes the way a decoded block does: a CBOR
// map of Word8 keys to raw byte-string values, so the HD payload value is the
// CBOR encoding of the payload byte string.
func byronAttrsFromWire(
	t *testing.T,
	hdPayloadLen int,
	network *uint32,
	unknown map[uint8][]byte,
) common.ByronAddressAttributes {
	t.Helper()
	m := map[uint8][]byte{}
	for k, v := range unknown {
		m[k] = v
	}
	if hdPayloadLen > 0 {
		enc, err := cbor.Encode(bytes.Repeat([]byte{0xAB}, hdPayloadLen))
		require.NoError(t, err)
		m[1] = enc
	}
	if network != nil {
		enc, err := cbor.Encode(*network)
		require.NoError(t, err)
		m[2] = enc
	}
	data, err := cbor.Encode(m)
	require.NoError(t, err)
	var attrs common.ByronAddressAttributes
	_, err = cbor.Decode(data, &attrs)
	require.NoError(t, err)
	return attrs
}

func byronAttrSizeErr(
	t *testing.T,
	attrs common.ByronAddressAttributes,
) error {
	t.Helper()
	addr, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		bytes.Repeat([]byte{0x01}, 28),
		attrs,
	)
	require.NoError(t, err)
	tx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			TxOutputs: []shelley.ShelleyTransactionOutput{
				{OutputAddress: addr},
			},
		},
	}
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		&shelley.ShelleyProtocolParameters{},
	)
}

func TestByronOutputAttrSizeMatchesReference(t *testing.T) {
	t.Parallel()
	magic := uint32(1097911063)
	tests := []struct {
		name   string
		attrs  func(t *testing.T) common.ByronAddressAttributes
		tooBig bool
	}{
		{
			name: "60 byte HD payload is accepted",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(t, 60, nil, nil)
			},
		},
		{
			name: "64 byte HD payload is accepted",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(t, 64, nil, nil)
			},
		},
		{
			name: "65 byte HD payload is rejected",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(t, 65, nil, nil)
			},
			tooBig: true,
		},
		{
			name: "network magic is not counted",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(t, 64, &magic, nil)
			},
		},
		{
			name: "unknown attribute counted by value length at limit",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(
					t, 0, nil,
					map[uint8][]byte{7: bytes.Repeat([]byte{1}, 64)},
				)
			},
		},
		{
			name: "unknown attribute over limit is rejected",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(
					t, 0, nil,
					map[uint8][]byte{7: bytes.Repeat([]byte{1}, 65)},
				)
			},
			tooBig: true,
		},
		{
			name: "HD payload and unknown values are summed",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(
					t, 40, nil,
					map[uint8][]byte{
						7: bytes.Repeat([]byte{1}, 12),
						9: bytes.Repeat([]byte{2}, 12),
					},
				)
			},
		},
		{
			name: "HD payload and unknown values over limit are rejected",
			attrs: func(t *testing.T) common.ByronAddressAttributes {
				return byronAttrsFromWire(
					t, 40, nil,
					map[uint8][]byte{
						7: bytes.Repeat([]byte{1}, 13),
						9: bytes.Repeat([]byte{2}, 12),
					},
				)
			},
			tooBig: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := byronAttrSizeErr(t, tc.attrs(t))
			if tc.tooBig {
				require.Error(t, err)
				assert.IsType(
					t,
					shelley.OutputBootAddrAttrsTooBigError{},
					err,
				)
				return
			}
			assert.NoError(t, err)
		})
	}
}
