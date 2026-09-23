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

package common

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func govActionIdCbor(t *testing.T, txId []byte, idx uint64) []byte {
	t.Helper()
	encoded, err := cbor.Encode([]any{txId, idx})
	if err != nil {
		t.Fatalf("encoding gov action id fixture: %v", err)
	}
	return encoded
}

// The Conway and Dijkstra CDDL both type the index as
// `gov_action_index : uint .size 2`, and cardano-ledger holds it in
// `newtype GovActionIx = GovActionIx Word16` whose derived decoder fails
// above 65535. GovActionIdx is a uint32 here, so the bound has to be checked
// rather than carried by the type.
func TestGovActionIdUnmarshalCBORRejectsIndexAboveUint16(t *testing.T) {
	t.Parallel()
	txId := bytes.Repeat([]byte{0x11}, Blake2b256Size)
	testCases := []struct {
		name string
		idx  uint64
	}{
		{"one above the domain", math.MaxUint16 + 1},
		{"max uint32", math.MaxUint32},
		{"above uint32", math.MaxUint32 + 1},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var id GovActionId
			err := id.UnmarshalCBOR(govActionIdCbor(t, txId, testCase.idx))
			if err == nil {
				t.Fatalf(
					"decoded gov action index %d without error;"+
						" the CDDL types it as uint .size 2",
					testCase.idx,
				)
			}
		})
	}
}

// The whole 0..65535 domain must keep decoding. 256..65535 in particular is
// wire-legal even though CIP-0129's bech32 form cannot carry it.
func TestGovActionIdUnmarshalCBORAcceptsUint16Domain(t *testing.T) {
	t.Parallel()
	txId := bytes.Repeat([]byte{0x22}, Blake2b256Size)
	for _, idx := range []uint64{0, 1, 255, 256, 65534, math.MaxUint16} {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			t.Parallel()
			var id GovActionId
			if err := id.UnmarshalCBOR(
				govActionIdCbor(t, txId, idx),
			); err != nil {
				t.Fatalf("decoding gov action index %d: %v", idx, err)
			}
			if uint64(id.GovActionIdx) != idx {
				t.Errorf(
					"gov action index: got %d, want %d",
					id.GovActionIdx,
					idx,
				)
			}
			if !bytes.Equal(id.TransactionId[:], txId) {
				t.Errorf(
					"transaction id: got %x, want %x",
					id.TransactionId[:],
					txId,
				)
			}
		})
	}
}
