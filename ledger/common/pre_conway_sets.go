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

import (
	"bytes"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// setTagHeads are the encodings of a tag 258 head: the minimal two-byte
// argument and the wider four- and eight-byte ones.
var setTagHeads = [][]byte{
	{0xd9, 0x01, 0x02},
	{0xda, 0x00, 0x00, 0x01, 0x02},
	{0xdb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02},
}

// ValidatePreConwaySetTags rejects a tag 258 on any of the given fields of
// a CBOR map. Before protocol version 9 cardano-ledger decodes set-shaped
// transaction fields with decodeSet or decodeList, neither of which accepts
// a tag, so a Shelley to Babbage body or witness set carrying one does not
// decode there. The map is only re-read when its bytes contain a tag 258
// head.
func ValidatePreConwaySetTags(cborData []byte, keys ...uint64) error {
	found := false
	for _, head := range setTagHeads {
		if bytes.Contains(cborData, head) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	fields, _, err := cbor.DecodeRawMap[uint64](cborData)
	if err != nil {
		return err
	}
	for _, key := range keys {
		value := fields[key]
		if len(value) > 0 &&
			value[0]&cbor.CborTypeMask == cbor.CborTypeTag {
			return fmt.Errorf(
				"field %d carries a CBOR tag, which is not allowed before Conway",
				key,
			)
		}
	}
	return nil
}
