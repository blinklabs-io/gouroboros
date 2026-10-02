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

package ledgertest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// WidenToDijkstraHeader appends leios_certified=false and a null
// leios_announcement to a 10-field header body so that a Babbage/Conway-shaped
// fixture decodes as a current 12-field Dijkstra header. The signature is left
// untouched, so the result is for decode tests only.
func WidenToDijkstraHeader(headerCbor []byte) ([]byte, error) {
	var top []cbor.RawMessage
	if _, err := cbor.Decode(headerCbor, &top); err != nil {
		return nil, err
	}
	if len(top) != 2 {
		return nil, fmt.Errorf("header has %d elements, want 2", len(top))
	}
	body := make([]cbor.RawMessage, 0, 12)
	if _, err := cbor.Decode(top[0], &body); err != nil {
		return nil, err
	}
	if len(body) != 10 {
		return nil, fmt.Errorf("header body has %d fields, want 10", len(body))
	}
	body = append(body, cbor.RawMessage{0xf4}, cbor.RawMessage{0xf6})
	var err error
	if top[0], err = cbor.Encode(body); err != nil {
		return nil, err
	}
	return cbor.Encode(top)
}

// WidenToDijkstraBlockHeader applies WidenToDijkstraHeader to the header of a
// [header, block_body, ...] block.
func WidenToDijkstraBlockHeader(blockCbor []byte) ([]byte, error) {
	var block []cbor.RawMessage
	if _, err := cbor.Decode(blockCbor, &block); err != nil {
		return nil, err
	}
	if len(block) == 0 {
		return nil, errors.New("empty block")
	}
	header, err := WidenToDijkstraHeader(block[0])
	if err != nil {
		return nil, err
	}
	block[0] = header
	return cbor.Encode(block)
}

// MustWidenToDijkstraHeader is WidenToDijkstraHeader failing the test on error.
func MustWidenToDijkstraHeader(t testing.TB, headerCbor []byte) []byte {
	t.Helper()
	out, err := WidenToDijkstraHeader(headerCbor)
	require.NoError(t, err)
	return out
}
