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
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type encodableTransactionBody interface {
	common.TransactionBody
	MarshalCBOR() ([]byte, error)
	SetCbor([]byte)
}

// nonPreferredTransactionBodyHex is a transaction body whose inputs use an
// indefinite-length array and whose fee (5) uses a four-byte integer, so its
// wire bytes differ from what the encoder would produce for the same values.
const nonPreferredTransactionBodyHex = "a4" +
	"00" + "9f" + "825820" +
	"0000000000000000000000000000000000000000000000000000000000000000" +
	"00" + "ff" +
	"01" + "80" +
	"02" + "1a00000005" +
	"03" + "1864"

// A transaction id is the hash of the body's CBOR: the preserved wire bytes
// for a decoded body, and the body's current encoding for one with no stored
// CBOR, whether built in code or cleared with SetCbor(nil) after a mutation.
func TestTransactionBodyIdHashesBodyEncoding(t *testing.T) {
	wire, err := hex.DecodeString(nonPreferredTransactionBodyHex)
	require.NoError(t, err)
	emptyHash := common.Blake2b256Hash(nil)
	tests := []struct {
		name        string
		txType      uint
		constructed func(fee uint64) encodableTransactionBody
	}{
		{"shelley", ledger.TxTypeShelley, func(fee uint64) encodableTransactionBody {
			return &shelley.ShelleyTransactionBody{TxFee: fee}
		}},
		{"allegra", ledger.TxTypeAllegra, func(fee uint64) encodableTransactionBody {
			return &allegra.AllegraTransactionBody{TxFee: fee}
		}},
		{"mary", ledger.TxTypeMary, func(fee uint64) encodableTransactionBody {
			return &mary.MaryTransactionBody{TxFee: fee}
		}},
		{"alonzo", ledger.TxTypeAlonzo, func(fee uint64) encodableTransactionBody {
			return &alonzo.AlonzoTransactionBody{TxFee: fee}
		}},
		{"babbage", ledger.TxTypeBabbage, func(fee uint64) encodableTransactionBody {
			return &babbage.BabbageTransactionBody{TxFee: fee}
		}},
		{"conway", ledger.TxTypeConway, func(fee uint64) encodableTransactionBody {
			return &conway.ConwayTransactionBody{TxFee: fee}
		}},
		{"dijkstra", ledger.TxTypeDijkstra, func(fee uint64) encodableTransactionBody {
			return &dijkstra.DijkstraTransactionBody{TxFee: fee}
		}},
	}
	for _, test := range tests {
		t.Run(test.name+"/constructed", func(t *testing.T) {
			first := test.constructed(5)
			second := test.constructed(6)
			encoded, err := first.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, common.Blake2b256Hash(encoded), first.Id())
			require.NotEqual(t, emptyHash, first.Id())
			require.NotEqual(t, first.Id(), second.Id())
		})
		t.Run(test.name+"/decoded", func(t *testing.T) {
			decoded, err := ledger.NewTransactionBodyFromCbor(test.txType, wire)
			require.NoError(t, err)
			body, ok := decoded.(encodableTransactionBody)
			require.True(t, ok)
			require.Equal(t, common.Blake2b256Hash(wire), body.Id())

			body.SetCbor(nil)
			reencoded, err := body.MarshalCBOR()
			require.NoError(t, err)
			require.False(
				t,
				bytes.Equal(wire, reencoded),
				"the fixture must not already be in the encoder's form",
			)
			require.Equal(t, common.Blake2b256Hash(reencoded), body.Id())
		})
	}
}
