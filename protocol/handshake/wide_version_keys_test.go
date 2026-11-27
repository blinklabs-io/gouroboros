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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// A version table may hold keys gOuroboros cannot represent as a uint16: an
// integer above 65535, a negative integer, a text string, or a tagged
// number. The reference version codecs do not recognise them, and
// decodeVersions skips any key it does not recognise (Codec.hs), so the
// rest of the table still negotiates.
func TestVersionMapSkipsUnrepresentableKeys(t *testing.T) {
	v14 := []byte{0x0e, 0x84, 0x1a, 0x2d, 0x96, 0x4a, 0x09, 0xf4, 0x00, 0xf4}
	v15 := []byte{0x0f, 0x84, 0x1a, 0x2d, 0x96, 0x4a, 0x09, 0xf4, 0x00, 0xf4}
	cat := func(parts ...[]byte) []byte {
		var out []byte
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for kindName, kind := range map[string]uint{
		"propose":     MessageTypeProposeVersions,
		"query reply": MessageTypeQueryReply,
	} {
		for _, test := range []struct {
			name  string
			table []byte
			want  []uint16
			err   string
		}{
			{"above uint16 after", cat([]byte{0xa2}, v14, []byte{0x1a, 0x00, 0x01, 0x11, 0x70, 0xf4}), []uint16{14}, ""},
			{"above uint16 first", cat([]byte{0xa2, 0x1a, 0x00, 0x01, 0x11, 0x70, 0xf4}, v14), []uint16{14}, ""},
			{"negative", cat([]byte{0xa3, 0x20, 0xf4}, v14, v15), []uint16{14, 15}, ""},
			{"text", cat([]byte{0xa2, 0x61, 0x61, 0xf4}, v14), []uint16{14}, ""},
			{"tagged", cat([]byte{0xa2, 0xc1, 0x0e, 0xf4}, v15), []uint16{15}, ""},
			{"indefinite", cat([]byte{0xbf}, v14, []byte{0x1b, 0, 0, 0, 1, 0, 0, 0, 0, 0xf4, 0xff}), []uint16{14}, ""},
			// The recognised keys still have to ascend around a skipped one.
			{"descending around", cat([]byte{0xa3}, v15, []byte{0x1a, 0x00, 0x01, 0x11, 0x70, 0xf4}, v14), nil, "out of order"},
			// A skipped key still needs a complete value.
			{"truncated value", []byte{0xa1, 0x1a, 0x00, 0x01, 0x11, 0x70}, nil, "decode"},
		} {
			t.Run(kindName+"/"+test.name, func(t *testing.T) {
				wire := append([]byte{0x82, byte(kind)}, test.table...)
				msg, err := NewMsgFromCbor(kind, wire)
				if test.err != "" {
					require.ErrorContains(t, err, test.err)
					return
				}
				require.NoError(t, err)
				var got map[uint16]cbor.RawMessage
				switch m := msg.(type) {
				case *MsgProposeVersions:
					got = m.VersionMap
				case *MsgQueryReply:
					got = m.VersionMap
				}
				require.Len(t, got, len(test.want))
				for _, v := range test.want {
					require.Contains(t, got, v)
				}
				require.Equal(t, wire, msg.Cbor())
			})
		}
	}
}
