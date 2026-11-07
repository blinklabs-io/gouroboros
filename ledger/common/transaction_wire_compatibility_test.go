// Copyright 2026 Blink Labs Software

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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

var minimalAlonzoFamilyTxBody = []byte{
	0xa3,
	0x00, 0x80,
	0x01, 0x80,
	0x02, 0x00,
}

func concatenateBytes(parts ...[]byte) []byte {
	var size int
	for _, part := range parts {
		size += len(part)
	}
	ret := make([]byte, 0, size)
	for _, part := range parts {
		ret = append(ret, part...)
	}
	return ret
}

func TestTxSizeUsesCanonicalAlonzoFamilyEnvelope(t *testing.T) {
	metadata := []byte{0xa1, 0x00, 0x00}
	canonicalSize := 1 + len(minimalAlonzoFamilyTxBody) + 1 + len(metadata)
	encodings := []struct {
		name string
		cbor []byte
	}{
		{
			name: "definite four components",
			cbor: concatenateBytes(
				[]byte{0x84},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0, 0xf5},
				metadata,
			),
		},
		{
			name: "definite four components invalid",
			cbor: concatenateBytes(
				[]byte{0x84},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0, 0xf4},
				metadata,
			),
		},
		{
			name: "indefinite four components",
			cbor: concatenateBytes(
				[]byte{0x9f},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0, 0xf5},
				metadata,
				[]byte{0xff},
			),
		},
		{
			name: "wide four component header",
			cbor: concatenateBytes(
				[]byte{0x98, 0x04},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0, 0xf5},
				metadata,
			),
		},
		{
			name: "definite three components",
			cbor: concatenateBytes(
				[]byte{0x83},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0},
				metadata,
			),
		},
		{
			name: "indefinite three components",
			cbor: concatenateBytes(
				[]byte{0x9f},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0},
				metadata,
				[]byte{0xff},
			),
		},
		{
			name: "wide three component header",
			cbor: concatenateBytes(
				[]byte{0x98, 0x03},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0},
				metadata,
			),
		},
	}

	for _, txType := range []struct {
		name string
		id   int
	}{
		{name: "Alonzo", id: alonzo.TxTypeAlonzo},
		{name: "Babbage", id: babbage.TxTypeBabbage},
		{name: "Conway", id: conway.TxTypeConway},
	} {
		t.Run(txType.name, func(t *testing.T) {
			for _, encoding := range encodings {
				t.Run(encoding.name, func(t *testing.T) {
					tx := mockledger.NewTransactionBuilder().WithType(txType.id)
					tx.SetCbor(encoding.cbor)

					size, err := common.TxSizeForFee(tx)
					require.NoError(t, err)
					require.Equal(t, canonicalSize, size)
				})
			}
		})
	}
}

func TestTxSizePreservesMalformedFourComponentEnvelope(t *testing.T) {
	metadata := []byte{0xa1, 0x00, 0x00}
	for _, test := range []struct {
		name     string
		validity []byte
	}{
		{name: "null", validity: []byte{0xf6}},
		{name: "integer", validity: []byte{0x00}},
		{
			name: "larger byte string",
			validity: concatenateBytes(
				[]byte{0x58, 0x20},
				make([]byte, 32),
			),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire := concatenateBytes(
				[]byte{0x84},
				minimalAlonzoFamilyTxBody,
				[]byte{0xa0},
				test.validity,
				metadata,
			)
			tx := mockledger.NewTransactionBuilder().WithType(
				alonzo.TxTypeAlonzo,
			)
			tx.SetCbor(wire)

			size, err := common.TxSizeForFee(tx)
			require.NoError(t, err)
			require.Equal(t, len(wire), size)
		})
	}
}

func TestAlonzoFamilyTransactionDecodeDefaultsOmittedValidity(t *testing.T) {
	type decodeFunc func([]byte) (common.Transaction, error)
	decoders := []struct {
		name   string
		decode decodeFunc
	}{
		{
			name: "Alonzo",
			decode: func(data []byte) (common.Transaction, error) {
				return alonzo.NewAlonzoTransactionFromCbor(data)
			},
		},
		{
			name: "Babbage",
			decode: func(data []byte) (common.Transaction, error) {
				return babbage.NewBabbageTransactionFromCbor(data)
			},
		},
		{
			name: "Conway",
			decode: func(data []byte) (common.Transaction, error) {
				return conway.NewConwayTransactionFromCbor(data)
			},
		},
	}

	threeComponent := concatenateBytes(
		[]byte{0x83},
		minimalAlonzoFamilyTxBody,
		[]byte{0xa0, 0xf6},
	)
	fourComponentInvalid := concatenateBytes(
		[]byte{0x84},
		minimalAlonzoFamilyTxBody,
		[]byte{0xa0, 0xf4, 0xf6},
	)
	threeComponentWithValidity := concatenateBytes(
		[]byte{0x83},
		minimalAlonzoFamilyTxBody,
		[]byte{0xa0, 0xf5},
	)

	for _, decoder := range decoders {
		t.Run(decoder.name, func(t *testing.T) {
			decoded, err := decoder.decode(threeComponent)
			require.NoError(t, err)
			require.True(t, decoded.IsValid())
			require.Equal(t, threeComponent, decoded.Cbor())

			decoded, err = decoder.decode(fourComponentInvalid)
			require.NoError(t, err)
			require.False(t, decoded.IsValid())

			_, err = decoder.decode(threeComponentWithValidity)
			require.Error(t, err)

			_, err = decoder.decode([]byte{0x82, 0xa0, 0xa0})
			require.Error(t, err)

			_, err = decoder.decode([]byte{0x85, 0xa0, 0xa0, 0xf5, 0xf6, 0xf6})
			require.Error(t, err)
		})
	}
}
