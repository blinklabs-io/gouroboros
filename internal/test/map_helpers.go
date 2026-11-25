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

package test

import "encoding/binary"

// OversizedMapPairs is the pair count of the maps built by MapOfUnknownKeys:
// large enough that sizing a typed map from the declared count costs
// megabytes, small enough that the encoded map is under 400 KiB.
const OversizedMapPairs = 60_000

// MapOfUnknownKeys returns a well-formed definite-length CBOR map of count
// pairs. Every key is a distinct unsigned integer above any protocol
// parameter update field, and every value is an unsigned zero.
func MapOfUnknownKeys(count int) []byte {
	data := make([]byte, 5, 5+6*count)
	data[0] = 0xba
	binary.BigEndian.PutUint32(data[1:], uint32(count)) // #nosec G115
	for i := range count {
		key := 100 + i
		data = append(data, 0x1a)
		data = binary.BigEndian.AppendUint32(data, uint32(key)) // #nosec G115
		data = append(data, 0x00)
	}
	return data
}

// OversizedProposalPairs is the pair count of the maps built by
// ParamUpdateProposals.
const OversizedProposalPairs = 20_000

// ParamUpdateProposals returns the CBOR of a pre-Conway transaction
// protocol parameter update: an array of a definite-length map and an epoch.
// The map has count pairs keyed by distinct 28-byte genesis delegate hashes,
// each paired with an unsigned zero, which is not a parameter update map.
func ParamUpdateProposals(count int) []byte {
	data := make([]byte, 6, 7+31*count)
	data[0] = 0x82
	data[1] = 0xba
	binary.BigEndian.PutUint32(data[2:], uint32(count)) // #nosec G115
	for i := range count {
		data = append(data, 0x58, 0x1c)
		var key [28]byte
		binary.BigEndian.PutUint32(key[24:], uint32(i)) // #nosec G115
		data = append(data, key[:]...)
		data = append(data, 0x00)
	}
	return append(data, 0x00)
}
