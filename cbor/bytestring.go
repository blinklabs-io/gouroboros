// Copyright 2023 Blink Labs, LLC.
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

package cbor

import (
	"encoding/hex"
)

// Wrapper for bytestrings that allows them to be used as keys for a map
type ByteString struct {
	// We use a string because []byte isn't comparable, which means it can't be used as a map key
	data string
}

func NewByteString(data []byte) ByteString {
	bs := ByteString{
		data: string(data),
	}
	return bs
}

func (bs *ByteString) UnmarshalCBOR(data []byte) error {
	tmpValue := []byte{}
	if _, err := Decode(data, &tmpValue); err != nil {
		return err
	}
	bs.data = string(tmpValue)
	return nil
}

func (bs ByteString) Bytes() []byte {
	return []byte(bs.data)
}

func (bs ByteString) String() string {
	return hex.EncodeToString([]byte(bs.data))
}
