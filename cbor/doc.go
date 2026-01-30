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

// Package cbor provides CBOR encoding/decoding utilities for Cardano data structures.
//
// # AI Navigation Guide
//
// This package wraps github.com/fxamacker/cbor/v2 with Cardano-specific patterns.
//
// # Key Types
//
// Embeddable types for struct encoding:
//   - StructAsArray: Embed to encode struct fields as CBOR array instead of map
//   - DecodeStoreCbor: Embed to preserve original CBOR bytes for hashing
//
// Utility types:
//   - RawMessage: Deferred decoding (like json.RawMessage)
//   - ByteString: Bytestrings that can be used as map keys
//   - Tag, RawTag: CBOR semantic tags
//   - Value: Constructor/alternative support (being deprecated)
//
// # Critical Pattern: DecodeStoreCbor
//
// When a type needs its original CBOR bytes preserved for hashing:
//
//	type MyType struct {
//	    cbor.DecodeStoreCbor
//	    Field1 string
//	    Field2 int
//	}
//
//	func (m *MyType) UnmarshalCBOR(data []byte) error {
//	    type tMyType MyType  // Type alias to avoid recursion
//	    var tmp tMyType
//	    if _, err := cbor.Decode(data, &tmp); err != nil {
//	        return err
//	    }
//	    *m = MyType(tmp)
//	    m.SetCbor(data)  // CRITICAL: Store original bytes
//	    return nil
//	}
//
// Later, m.Cbor() returns the original bytes for hash computation.
//
// # Encoding Gotchas
//
//  1. Hash computation: Always use Cbor() method, not re-encoded data
//  2. Map key ordering: Cardano uses specific rules (e.g., shortLex for language views)
//  3. Indefinite vs definite length: Some encodings require specific length encoding
//  4. Missing SetCbor call: Forgetting to call SetCbor() causes hash mismatches
package cbor
