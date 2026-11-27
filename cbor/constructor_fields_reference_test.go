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

package cbor_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestConstructorFieldsMustBeAList follows plutus bdc12f105a98,
// plutus-core/plutus-core/src/PlutusCore/Data.hs decodeConstr (286-307):
// the content of tags 121-127 and 1280-1400, and the second element under
// tag 102, are read with decodeListOf, so anything but a list is refused,
// both by ConstructorDecoder and by Value, which decodes constructors
// through it.
func TestConstructorFieldsMustBeAList(t *testing.T) {
	for _, h := range []string{"d87980", "d8799f01ff", "d866820380"} {
		data, _ := hex.DecodeString(h)
		var cd cbor.ConstructorDecoder
		if _, err := cbor.Decode(data, &cd); err != nil {
			t.Errorf("%s: %v", h, err)
		}
	}
	for _, h := range []string{
		"d87901", "d87a4100", "d905786161", "d87aa0", "d866820301",
		"d866820341ff",
	} {
		data, _ := hex.DecodeString(h)
		var cd cbor.ConstructorDecoder
		if _, err := cbor.Decode(data, &cd); err == nil {
			t.Errorf("%s: accepted as alternative %d", h, cd.Tag())
		}
		var v cbor.Value
		if _, err := cbor.Decode(data, &v); err == nil {
			t.Errorf("%s: Value accepted it", h)
		}
	}
}
