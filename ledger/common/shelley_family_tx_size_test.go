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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// A transaction body with one input, no outputs, a zero fee and a zero ttl
// (mandatory in Shelley).
var minimalShelleyFamilyTxBody = concatenateBytes(
	[]byte{0xa4, 0x00, 0x81, 0x82, 0x58, 0x20},
	make([]byte, 32),
	[]byte{0x00, 0x01, 0x80, 0x02, 0x00, 0x03, 0x00},
)

// The reference measures a Shelley, Allegra or Mary transaction as a
// one-byte array header plus its body, witness set and auxiliary data
// (cardano-ledger Shelley/Tx.hs sizeShelleyTxF), whatever framing the
// received outer array used.
func TestTxSizeShelleyFamilyIgnoresOuterArrayFraming(t *testing.T) {
	t.Parallel()
	type txDecoder func([]byte) (common.Transaction, error)
	eras := []struct {
		name   string
		decode txDecoder
		// Auxiliary scripts arrived in Allegra.
		auxiliaryScripts bool
	}{
		{"Shelley", func(b []byte) (common.Transaction, error) {
			return shelley.NewShelleyTransactionFromCbor(b)
		}, false},
		{"Allegra", func(b []byte) (common.Transaction, error) {
			return allegra.NewAllegraTransactionFromCbor(b)
		}, true},
		{"Mary", func(b []byte) (common.Transaction, error) {
			return mary.NewMaryTransactionFromCbor(b)
		}, true},
	}
	framings := []struct {
		name   string
		header []byte
		end    []byte
	}{
		{name: "definite", header: []byte{0x83}},
		{name: "wide definite", header: []byte{0x98, 0x03}},
		{name: "widest definite", header: []byte{0x99, 0x00, 0x03}},
		{name: "indefinite", header: []byte{0x9f}, end: []byte{0xff}},
	}
	auxiliaries := []struct {
		name             string
		data             []byte
		auxiliaryScripts bool
	}{
		{name: "null", data: []byte{0xf6}},
		{name: "metadata", data: []byte{0xa1, 0x00, 0x00}},
		{
			name:             "metadata and scripts",
			data:             []byte{0x82, 0xa1, 0x00, 0x00, 0x80},
			auxiliaryScripts: true,
		},
	}
	witnessSet := []byte{0xa0}

	for _, era := range eras {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			for _, aux := range auxiliaries {
				if aux.auxiliaryScripts && !era.auxiliaryScripts {
					continue
				}
				t.Run(aux.name, func(t *testing.T) {
					t.Parallel()
					want := 1 + len(minimalShelleyFamilyTxBody) +
						len(witnessSet) + len(aux.data)
					for _, framing := range framings {
						t.Run(framing.name, func(t *testing.T) {
							t.Parallel()
							wire := concatenateBytes(
								framing.header,
								minimalShelleyFamilyTxBody,
								witnessSet,
								aux.data,
								framing.end,
							)
							tx, err := era.decode(wire)
							require.NoError(t, err)
							require.Equal(t, wire, tx.Cbor())

							size, err := common.TxSize(tx)
							require.NoError(t, err)
							require.Equal(t, want, size)
							feeSize, err := common.TxSizeForFee(tx)
							require.NoError(t, err)
							require.Equal(t, want, feeSize)
						})
					}
				})
			}
		})
	}
}

// A Shelley-family envelope that is not three components has no reference
// measure, so its received length is kept. Byron transactions are measured
// by their received length too.
func TestTxSizeKeepsReceivedLengthOutsideShelleyFamilyEnvelope(t *testing.T) {
	t.Parallel()
	fourComponents := concatenateBytes(
		[]byte{0x84},
		minimalShelleyFamilyTxBody,
		[]byte{0xa0, 0xf5, 0xf6},
	)
	indefiniteThree := concatenateBytes(
		[]byte{0x9f},
		minimalShelleyFamilyTxBody,
		[]byte{0xa0, 0xf6, 0xff},
	)
	for _, test := range []struct {
		name   string
		txType uint
		wire   []byte
	}{
		{"Shelley four components", shelley.TxTypeShelley, fourComponents},
		{"Mary four components", mary.TxTypeMary, fourComponents},
		{"Byron indefinite", byron.TxTypeByron, indefiniteThree},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tx := mockledger.NewTransactionBuilder().WithType(
				int(test.txType),
			)
			tx.SetCbor(test.wire)
			size, err := common.TxSize(tx)
			require.NoError(t, err)
			require.Equal(t, len(test.wire), size)
		})
	}
}
