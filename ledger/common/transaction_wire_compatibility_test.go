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
	"fmt"
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

func TestTxSizePreservesMalformedThreeComponentBooleanAuxiliaryData(
	t *testing.T,
) {
	txTypes := []struct {
		name string
		id   int
	}{
		{name: "Alonzo", id: alonzo.TxTypeAlonzo},
		{name: "Babbage", id: babbage.TxTypeBabbage},
		{name: "Conway", id: conway.TxTypeConway},
	}
	encodings := []struct {
		name   string
		header []byte
		end    []byte
	}{
		{name: "wide definite array", header: []byte{0x98, 0x03}},
		{
			name:   "indefinite array",
			header: []byte{0x9f},
			end:    []byte{0xff},
		},
	}

	for _, txType := range txTypes {
		t.Run(txType.name, func(t *testing.T) {
			for _, validity := range []byte{0xf4, 0xf5} {
				t.Run(fmt.Sprintf("boolean %x", validity), func(t *testing.T) {
					for _, encoding := range encodings {
						t.Run(encoding.name, func(t *testing.T) {
							wire := concatenateBytes(
								encoding.header,
								minimalAlonzoFamilyTxBody,
								[]byte{0xa0, validity},
								encoding.end,
							)
							tx := mockledger.NewTransactionBuilder().WithType(
								txType.id,
							)
							tx.SetCbor(wire)

							size, err := common.TxSize(tx)
							require.NoError(t, err)
							require.Equal(t, len(wire), size)
						})
					}
				})
			}
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
	fourComponentWithInvalidValidity := concatenateBytes(
		[]byte{0x84},
		minimalAlonzoFamilyTxBody,
		[]byte{0xa0, 0xf6, 0xf6},
	)
	combinedMalformed := []struct {
		name     string
		wire     []byte
		expected string
	}{
		{
			name:     "four components reject body before validity",
			wire:     []byte{0x84, 0x00, 0xa0, 0xf6, 0xf6},
			expected: "failed to decode transaction body",
		},
		{
			name:     "three components reject body before boolean auxiliary data",
			wire:     []byte{0x83, 0x00, 0xa0, 0xf5},
			expected: "failed to decode transaction body",
		},
		{
			name: "four components reject witnesses before validity",
			wire: concatenateBytes(
				[]byte{0x84},
				minimalAlonzoFamilyTxBody,
				[]byte{0x00, 0xf6, 0xf6},
			),
			expected: "failed to decode transaction witness set",
		},
		{
			name: "three components reject witnesses before boolean auxiliary data",
			wire: concatenateBytes(
				[]byte{0x83},
				minimalAlonzoFamilyTxBody,
				[]byte{0x00, 0xf5},
			),
			expected: "failed to decode transaction witness set",
		},
	}

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
			require.ErrorContains(
				t,
				err,
				"invalid transaction: IsValid requires an auxiliary data component",
			)

			_, err = decoder.decode(fourComponentWithInvalidValidity)
			require.ErrorContains(t, err, "failed to decode TxIsValid")

			_, err = decoder.decode([]byte{0x82, 0xa0, 0xa0})
			require.ErrorContains(
				t,
				err,
				"invalid transaction: expected 3 or 4 components, got 2",
			)

			_, err = decoder.decode([]byte{0x85, 0xa0, 0xa0, 0xf5, 0xf6, 0xf6})
			require.ErrorContains(
				t,
				err,
				"invalid transaction: expected 3 or 4 components, got 5",
			)

			for _, test := range combinedMalformed {
				t.Run(test.name, func(t *testing.T) {
					_, err := decoder.decode(test.wire)
					require.ErrorContains(t, err, test.expected)
					require.NotContains(t, err.Error(), "TxIsValid")
					require.NotContains(t, err.Error(), "IsValid requires")
				})
			}
		})
	}
}
