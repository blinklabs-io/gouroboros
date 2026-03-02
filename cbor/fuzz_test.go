// Copyright 2024 Cardano Foundation
// Copyright 2025 Blink Labs Software
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

//go:build go1.18

package cbor

import "testing"

func FuzzDecode(f *testing.F) {
	// Seed corpus with valid CBOR samples
	f.Add([]byte{0xa0})                         // empty map
	f.Add([]byte{0x80})                         // empty array
	f.Add([]byte{0xbf, 0xff})                   // indefinite map
	f.Add([]byte{0x9f, 0xff})                   // indefinite array
	f.Add([]byte{0x00})                         // integer 0
	f.Add([]byte{0x18, 0x64})                   // integer 100
	f.Add([]byte{0x19, 0x27, 0x10})             // integer 10000
	f.Add([]byte{0x1a, 0x00, 0x01, 0x86, 0xa0}) // integer 100000
	f.Add(
		[]byte{0x3a, 0x00, 0x01, 0x86, 0x9f},
	) // negative integer -100000
	f.Add([]byte{0x40})                               // empty bytestring
	f.Add([]byte{0x44, 0x01, 0x02, 0x03, 0x04})       // bytestring
	f.Add([]byte{0x60})                               // empty text string
	f.Add([]byte{0x65, 0x68, 0x65, 0x6c, 0x6c, 0x6f}) // "hello"
	f.Add([]byte{0xf4})                               // false
	f.Add([]byte{0xf5})                               // true
	f.Add([]byte{0xf6})                               // null
	f.Add([]byte{0xf7})                               // undefined

	f.Fuzz(func(t *testing.T, data []byte) {
		var result any
		_, _ = Decode(data, &result)
		// Should not panic - that's the test
	})
}

func FuzzDecodeStruct(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		type TestStruct struct {
			Field1 uint64
			Field2 []byte
			Field3 string
		}
		var result TestStruct
		_, _ = Decode(data, &result)
	})
}
