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

package cbor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHexBytesSizeMatchesFormatter pins hexBytesSize to formatHexBytes. The
// hex dump skips a row whose text would not fit by asking for its size
// first, so a size that drifted from the formatter would either truncate
// output that fits or build text that does not.
func TestHexBytesSizeMatchesFormatter(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 2, 3, 4, 5, 9, 10, 40, 99, 100, 101, 100000} {
		data := make([]byte, n)
		for _, maxBytes := range []int{-1, 0, 1, 3, 4, 10, 40, 100, 1 << 20} {
			assert.Equal(
				t,
				len(formatHexBytes(data, maxBytes)),
				hexBytesSize(n, maxBytes),
				"n=%d maxBytes=%d", n, maxBytes,
			)
		}
	}
}

// TestQuotedStringSizeMatchesQuote pins quotedStringSize to strconv.Quote. The
// size is taken before quoting so that an escape-heavy string, whose quoted
// form is several times its length, is not built when it cannot fit.
func TestQuotedStringSizeMatchesQuote(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"",
		"plain ascii",
		`quote " and backslash \`,
		"tab\tnewline\nbell\a\b\f\r\v",
		"nul\x00 and del\x7f and ctrl\x01\x1f",
		"invalid \xff\xfe utf8 \xc3",
		"print é 世界 😀",
		"nonprint \u0085 ​ \U000e0001",
		strings.Repeat("\x00", 64),
		strings.Repeat("\xff", 64),
	} {
		assert.Equal(
			t, len(strconv.Quote(s)), quotedStringSize(s), "%q", s,
		)
	}
}
