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

package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// From node-to-node version 13, and in DMQ node-to-node, peer sharing is 0
// (disabled) or 1 (enabled); the reference decoders fail on any other value
// ("peerSharing is out of bound"), so the server refuses with DecodeError
// and the client fails.
func TestVersionDataPeerSharingRange(t *testing.T) {
	for _, test := range []struct {
		name   string
		decode NewVersionDataFromCborFunc
		data   []byte
		ok     bool
	}{
		{"v13+ disabled", NewVersionDataNtN13andUpFromCbor, []byte{0x84, 0x01, 0xf4, 0x00, 0xf4}, true},
		{"v13+ enabled", NewVersionDataNtN13andUpFromCbor, []byte{0x84, 0x01, 0xf4, 0x01, 0xf4}, true},
		{"v13+ two", NewVersionDataNtN13andUpFromCbor, []byte{0x84, 0x01, 0xf4, 0x02, 0xf4}, false},
		{"v13+ large", NewVersionDataNtN13andUpFromCbor, []byte{0x84, 0x01, 0xf4, 0x19, 0x01, 0x2c, 0xf4}, false},
		{"v16 enabled", NewVersionDataNtN16andUpFromCbor, []byte{0x85, 0x01, 0xf4, 0x01, 0xf4, 0xf4}, true},
		{"v16 two", NewVersionDataNtN16andUpFromCbor, []byte{0x85, 0x01, 0xf4, 0x02, 0xf4, 0xf4}, false},
		// Versions 11 and 12 had three modes, 0 to 2.
		{"v11 public", NewVersionDataNtN11to12FromCbor, []byte{0x84, 0x01, 0xf4, 0x02, 0xf4}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.decode(test.data)
			if test.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "peer sharing")
		})
	}
}
