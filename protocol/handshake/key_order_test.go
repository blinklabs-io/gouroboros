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

package handshake

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Recognised versions in a version table must be in ascending order: the
// reference codec fails to decode a MsgProposeVersions, MsgReplyVersions or
// MsgQueryReply whose recognised keys are not. It skips a version it does
// not recognise, wherever that version appears.
func TestVersionMapKeysMustAscend(t *testing.T) {
	for kindName, kind := range map[string]uint{
		"propose":     MessageTypeProposeVersions,
		"query reply": MessageTypeQueryReply,
	} {
		for _, tc := range []struct {
			name      string
			table     []byte
			errorText string
		}{
			{"ascending", []byte{0xa2, 0x0e, 0xf4, 0x0f, 0xf4}, ""},
			{"descending", []byte{0xa2, 0x0f, 0xf4, 0x0e, 0xf4}, "out of order"},
			{
				"descending indefinite",
				[]byte{0xbf, 0x0f, 0xf4, 0x0e, 0xf4, 0xff},
				"out of order",
			},
			{
				"ascending node-to-client",
				[]byte{0xa2, 0x19, 0x80, 0x0f, 0xf4, 0x19, 0x80, 0x10, 0xf4},
				"",
			},
			{
				"descending node-to-client",
				[]byte{0xa2, 0x19, 0x80, 0x10, 0xf4, 0x19, 0x80, 0x0f, 0xf4},
				"out of order",
			},
			{
				"descending across encoded widths",
				[]byte{0xa2, 0x19, 0x80, 0x10, 0xf4, 0x0e, 0xf4},
				"out of order",
			},
			{
				"unrecognised key first",
				[]byte{0xa2, 0x18, 0x63, 0xf4, 0x0e, 0xf4},
				"",
			},
			{
				"unrecognised key between ascending",
				[]byte{0xa3, 0x0e, 0xf4, 0x18, 0x63, 0xf4, 0x0f, 0xf4},
				"",
			},
			{
				"unrecognised key between descending",
				[]byte{0xa3, 0x0f, 0xf4, 0x18, 0x63, 0xf4, 0x0e, 0xf4},
				"out of order",
			},
			{"duplicate", []byte{0xa2, 0x0e, 0xf4, 0x0e, 0xf4}, "duplicate"},
		} {
			t.Run(kindName+"/"+tc.name, func(t *testing.T) {
				wire := append([]byte{0x82, byte(kind)}, tc.table...)
				msg, err := NewMsgFromCbor(kind, wire)
				if tc.errorText == "" {
					require.NoError(t, err)
					require.NotNil(t, msg)
					return
				}
				require.ErrorContains(t, err, tc.errorText)
				require.Nil(t, msg)
			})
		}
	}
}
