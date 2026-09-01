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

// ReassembleTransactionCbor builds the on-wire Alonzo-compatible transaction
// form `[body, witness_set, is_valid, auxiliary_data_or_null]` from already
// encoded component bytes.
//
// This must be used when raw component bytes are available because re-encoding
// decoded structs can normalize CBOR details such as tagged set forms, which
// changes the measured transaction size for fee validation.
func ReassembleTransactionCbor(
	body []byte,
	witness []byte,
	isValid bool,
	auxiliary []byte,
) []byte {
	size := 1 + len(body) + len(witness) + 1
	if len(auxiliary) > 0 {
		size += len(auxiliary)
	} else {
		size++
	}

	ret := make([]byte, 0, size)
	ret = append(ret, 0x84)
	ret = append(ret, body...)
	ret = append(ret, witness...)
	if isValid {
		ret = append(ret, 0xf5)
	} else {
		ret = append(ret, 0xf4)
	}
	if len(auxiliary) > 0 {
		ret = append(ret, auxiliary...)
	} else {
		ret = append(ret, 0xf6)
	}
	return ret
}
