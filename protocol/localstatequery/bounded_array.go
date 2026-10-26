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
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// boundArray rejects data unless it is one array of at most maxCount items.
// Decoding into a slice sizes it from the peer-controlled array length, so a
// reply whose shape is fixed must be bounded here, before the slice exists,
// rather than by a length check on the decoded result.
func boundArray(data []byte, maxCount int, label string) error {
	return cborpreflight.ValidateArray(data, maxCount, label, nil)
}

// decodeBoundedArray decodes data as an array of at most maxCount raw items.
func decodeBoundedArray(
	data []byte,
	maxCount int,
	label string,
) ([]cbor.RawMessage, error) {
	if err := boundArray(data, maxCount, label); err != nil {
		return nil, err
	}
	var items []cbor.RawMessage
	if _, err := cbor.Decode(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}
