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

package localstatequery

import (
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// growingMap decodes a CBOR map one entry at a time. Decoding straight into a
// map[K]V sizes the map from the peer-declared count, so a map of two-byte
// entries would reserve a whole slot per entry before the first entry fails
// to decode. Here the map grows only with entries that decode. A repeated key
// is rejected, matching the duplicate-key policy of the default decoder.
type growingMap[K comparable, V any] map[K]V

func (m *growingMap[K, V]) UnmarshalCBOR(data []byte) error {
	// A null or undefined decodes to a nil map, as it does for a plain map.
	if len(data) == 1 && (data[0] == 0xf6 || data[0] == 0xf7) {
		*m = nil
		return nil
	}
	entries := make(map[K]V)
	err := cborpreflight.ValidateMap(
		data,
		math.MaxInt,
		"map",
		func(idx int, rawKey, rawValue []byte) error {
			var key K
			if _, err := cbor.Decode(rawKey, &key); err != nil {
				return fmt.Errorf("decode map key %d: %w", idx, err)
			}
			if _, dup := entries[key]; dup {
				return fmt.Errorf("duplicate map key %d", idx)
			}
			var value V
			if _, err := cbor.Decode(rawValue, &value); err != nil {
				return fmt.Errorf("decode map value %d: %w", idx, err)
			}
			entries[key] = value
			return nil
		},
	)
	if err != nil {
		return err
	}
	*m = entries
	return nil
}
