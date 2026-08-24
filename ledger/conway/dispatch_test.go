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

package conway

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestConwayGovActionAcceptsListLengthEncodings(t *testing.T) {
	action := &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)}
	canonical, err := (&ConwayGovAction{Action: action}).MarshalCBOR()
	require.NoError(t, err)
	for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
		t.Run(encoding.Name, func(t *testing.T) {
			var decoded ConwayGovAction
			require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
			require.IsType(t, &common.InfoGovAction{}, decoded.Action)
		})
	}
}

func TestConwayWitnessSetAcceptsNonShortestNativeScript(t *testing.T) {
	canonical, err := cbor.Encode(common.NativeScriptInvalidBefore{
		Type: 4,
		Slot: 5,
	})
	require.NoError(t, err)

	// Keep the script's two-element array semantically unchanged while encoding
	// its length with the wider uint16 form. Cardano accepts this form and the
	// witness set must preserve it through decoding.
	nonShortest := append([]byte{0x99, 0x00, 0x02}, canonical[1:]...)
	set := append([]byte{0xd9, 0x01, 0x02, 0x81}, nonShortest...)
	witnessSet, err := cbor.Encode(map[uint]cbor.RawMessage{
		1: cbor.RawMessage(set),
	})
	require.NoError(t, err)

	var decoded ConwayTransactionWitnessSet
	require.NoError(t, decoded.UnmarshalCBOR(witnessSet))
	nativeScripts := decoded.WsNativeScripts.Items()
	require.Len(t, nativeScripts, 1)
	require.Equal(t, nonShortest, nativeScripts[0].Cbor())
	_, ok := nativeScripts[0].Item().(*common.NativeScriptInvalidBefore)
	require.True(t, ok)
}
