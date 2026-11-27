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
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestValueDecodesSetMembersWithCompositeMapKeys decodes tag 258 sets whose
// members are maps keyed by arrays or maps. RFC 8949 section 3.1 allows any
// data item as a map key, and Conway transactions carry such sets: the
// proposal procedures of cardano-ledger 1210d37f's
// eras/conway/impl/golden/tx.cbor (field 20) hold committee-update maps
// keyed by credentials, which are arrays. The Dijkstra golden from the same
// revision carries 33 tag 258 sets.
func TestValueDecodesSetMembersWithCompositeMapKeys(t *testing.T) {
	cases := map[string]int{
		"d9010281a1820102f5":     1,
		"d9010282a1a10102f5a0":   2,
		"d9010280":               0,
		"d901029f01a1820102f5ff": 2,
	}
	for h, n := range cases {
		data, _ := hex.DecodeString(h)
		var v cbor.Value
		if _, err := cbor.Decode(data, &v); err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		set, ok := v.Value().(cbor.Set)
		if !ok || len(set) != n {
			t.Errorf("%s: decoded as %T %v", h, v.Value(), v.Value())
		}
	}
	for _, h := range []string{"d9010201", "d90102a0"} {
		data, _ := hex.DecodeString(h)
		var v cbor.Value
		if _, err := cbor.Decode(data, &v); err == nil {
			t.Errorf("%s: set of a non-list accepted", h)
		}
	}
	for _, name := range []string{"conway-tx.cbor", "dijkstra-tx.cbor"} {
		tx, err := os.ReadFile(filepath.Join("testdata", "cardano-ledger", name))
		if err != nil {
			t.Fatal(err)
		}
		var v cbor.Value
		if _, err := cbor.DecodeUntrustedExact(tx, &v); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if out, err := v.MarshalCBOR(); err != nil || !bytes.Equal(out, tx) {
			t.Errorf("%s: bytes not kept", name)
		}
		var lv cbor.LazyValue
		if _, err := cbor.Decode(tx, &lv); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if _, err := lv.Decode(); err != nil {
			t.Errorf("%s: LazyValue.Decode: %v", name, err)
		}
	}
}
