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

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func TestMinCoinTxOutUsesOriginalWireSize(t *testing.T) {
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		make([]byte, common.Blake2b224Size),
		nil,
	)
	require.NoError(t, err)
	addressCBOR, err := cbor.Encode(address)
	require.NoError(t, err)
	indefiniteMap := append([]byte{0xbf, 0x00}, addressCBOR...)
	indefiniteMap = append(indefiniteMap, 0x01, 0x00, 0xff)
	var output babbage.BabbageTransactionOutput
	_, err = cbor.Decode(indefiniteMap, &output)
	require.NoError(t, err)
	require.Equal(t, indefiniteMap, output.Cbor())

	minimum, err := conway.MinCoinTxOut(
		&output,
		&conway.ConwayProtocolParameters{AdaPerUtxoByte: 1},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(160+len(indefiniteMap)), minimum)
}
