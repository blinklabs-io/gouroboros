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

	_cbor "github.com/fxamacker/cbor/v2"
)

// Wrapper for bytestrings that allows them to be used as keys for a map
// This was originally a full implementation, but now it just extends the upstream
// type
type ByteString struct {
	_cbor.ByteString
}

func NewByteString(data []byte) ByteString {
	bs := ByteString{ByteString: _cbor.ByteString(data)}
	return bs
}

// String returns a hex-encoded representation of the bytestring
func (bs ByteString) String() string {
	return hex.EncodeToString(bs.Bytes())
}
