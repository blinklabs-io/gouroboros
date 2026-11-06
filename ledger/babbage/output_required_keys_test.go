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

package babbage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func babbageOutputRequiredKeysFixtures() (addr, value []byte) {
	addr = append([]byte{0x58, 0x1d, 0x61}, bytes.Repeat([]byte{0x11}, 28)...)
	value = []byte{0x1a, 0x00, 0x0f, 0x42, 0x40}
	return addr, value
}

func TestBabbageTransactionOutputMapRequiresAddressAndValue(t *testing.T) {
	t.Parallel()
	addr, value := babbageOutputRequiredKeysFixtures()
	join := func(parts ...[]byte) []byte {
		return bytes.Join(parts, nil)
	}
	withAddr := join([]byte{0x00}, addr)
	withValue := join([]byte{0x01}, value)

	tests := []struct {
		name    string
		cbor    []byte
		wantErr string
	}{
		{
			name: "address and value",
			cbor: join([]byte{0xa2}, withAddr, withValue),
		},
		{
			name: "address, value and inline datum option",
			cbor: join(
				[]byte{0xa3}, withAddr, withValue,
				[]byte{0x02, 0x82, 0x00, 0x58, 0x20},
				bytes.Repeat([]byte{0x22}, 32),
			),
		},
		{
			name:    "missing value",
			cbor:    join([]byte{0xa1}, withAddr),
			wantErr: "value",
		},
		{
			name:    "missing address",
			cbor:    join([]byte{0xa1}, withValue),
			wantErr: "address",
		},
		{
			name:    "empty map",
			cbor:    []byte{0xa0},
			wantErr: "address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := NewBabbageTransactionOutputFromCbor(tt.cbor)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, out)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestBabbageTransactionRejectsOutputMapWithoutValue(t *testing.T) {
	t.Parallel()
	addr, value := babbageOutputRequiredKeysFixtures()
	build := func(output []byte) []byte {
		body := []byte{0xa3, 0x00, 0x80, 0x01, 0x81}
		body = append(body, output...)
		body = append(body, 0x02, 0x00)
		tx := []byte{0x84}
		tx = append(tx, body...)
		return append(tx, 0xa0, 0xf5, 0xf6)
	}
	valid := append([]byte{0xa2, 0x00}, addr...)
	valid = append(valid, 0x01)
	valid = append(valid, value...)
	_, err := NewBabbageTransactionFromCbor(build(valid))
	require.NoError(t, err)

	noValue := append([]byte{0xa1, 0x00}, addr...)
	_, err = NewBabbageTransactionFromCbor(build(noValue))
	require.ErrorContains(t, err, "value")
}

func TestBabbageTransactionOutputMapOtherMalformedForms(t *testing.T) {
	t.Parallel()
	addr, value := babbageOutputRequiredKeysFixtures()
	join := func(parts ...[]byte) []byte {
		return bytes.Join(parts, nil)
	}
	k0 := join([]byte{0x00}, addr)
	k1 := join([]byte{0x01}, value)

	tests := []struct {
		name string
		cbor []byte
	}{
		{"unknown key", join([]byte{0xa3}, k0, k1, []byte{0x04, 0x00})},
		{"duplicate address key", join([]byte{0xa3}, k0, k0, k1)},
		{"duplicate value key", join([]byte{0xa3}, k0, k1, k1)},
		{"null address", join([]byte{0xa2, 0x00, 0xf6}, k1)},
		{"null value", join([]byte{0xa2}, k0, []byte{0x01, 0xf6})},
		{"legacy array of one element", join([]byte{0x81}, addr)},
		{
			"legacy array of four elements",
			join([]byte{0x84}, addr, value, bytes.Repeat([]byte{0x00}, 3)),
		},
		{
			"unsupported datum option tag",
			join(
				[]byte{0xa3}, k0, k1,
				[]byte{0x02, 0x82, 0x02, 0x58, 0x20},
				bytes.Repeat([]byte{0x22}, 32),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewBabbageTransactionOutputFromCbor(tt.cbor)
			require.Error(t, err)
		})
	}
}
