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

package cbor_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// generalConstructorItems is large enough that holding one slice entry per
// item would cost megabytes, while the input stays around 100 KB.
const generalConstructorItems = 100_000

// generalConstructor builds a tag 102 constructor whose content array holds
// generalConstructorItems one-byte items instead of the required two.
func generalConstructor(indefinite bool) []byte {
	data := []byte{0xd8, 0x66}
	if indefinite {
		data = append(data, 0x9f)
	} else {
		data = append(data, 0x9a, 0x00, 0x01, 0x86, 0xa0)
	}
	data = append(data, bytes.Repeat([]byte{0x00}, generalConstructorItems)...)
	if indefinite {
		data = append(data, 0xff)
	}
	return data
}

func TestGeneralConstructorRejectsWrongShapeBeforeAllocating(t *testing.T) {
	decoders := []struct {
		name   string
		decode func([]byte) error
	}{
		{"constructor decoder", func(data []byte) error {
			var cd cbor.ConstructorDecoder
			_, err := cbor.Decode(data, &cd)
			return err
		}},
		{"value", func(data []byte) error {
			var v cbor.Value
			_, err := cbor.Decode(data, &v)
			return err
		}},
	}
	for _, dec := range decoders {
		for _, indefinite := range []bool{false, true} {
			name := dec.name + "/definite"
			if indefinite {
				name = dec.name + "/indefinite"
			}
			t.Run(name, func(t *testing.T) {
				data := generalConstructor(indefinite)
				require.ErrorContains(
					t,
					dec.decode(data),
					"expected 2 elements for alternative 128+",
				)
				result := testing.Benchmark(func(b *testing.B) {
					for range b.N {
						if dec.decode(data) == nil {
							b.Fatal("malformed constructor decoded")
						}
					}
				})
				// A slice entry per item costs 24 bytes. Value keeps a few
				// copies of the input bytes, about 4 bytes per item here,
				// as it does for a valid constructor of the same size.
				require.Less(
					t,
					result.AllocedBytesPerOp(),
					int64(generalConstructorItems*8),
				)
			})
		}
	}
}
