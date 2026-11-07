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

package transactionwire

import "testing"

func TestDecodeAlonzoFamilyEnvelopeReferencesInput(t *testing.T) {
	wire := []byte{0x83, 0xa0, 0xa0, 0xf6}
	envelope, err := DecodeAlonzoFamilyEnvelope(wire)
	if err != nil {
		t.Fatal(err)
	}

	wire[1] = 0x80
	if envelope.Body[0] != wire[1] {
		t.Fatal("transaction body was copied instead of referencing the input")
	}
}
