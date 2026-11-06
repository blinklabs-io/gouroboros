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

package conway

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConwayTransactionRejectsOutputMapMissingRequiredKeys(t *testing.T) {
	t.Parallel()
	addr := append([]byte{0x58, 0x1d, 0x61}, bytes.Repeat([]byte{0x11}, 28)...)
	value := []byte{0x1a, 0x00, 0x0f, 0x42, 0x40}
	build := func(output []byte) []byte {
		body := []byte{0xa3, 0x00, 0x80, 0x01, 0x81}
		body = append(body, output...)
		body = append(body, 0x02, 0x00)
		tx := []byte{0x84}
		tx = append(tx, body...)
		return append(tx, 0xa0, 0xf5, 0xf6)
	}
	valid := append([]byte{0xa2, 0x00}, addr...)
	valid = append(valid, 0x01)
	valid = append(valid, value...)
	_, err := NewConwayTransactionFromCbor(build(valid))
	require.NoError(t, err)

	noValue := append([]byte{0xa1, 0x00}, addr...)
	_, err = NewConwayTransactionFromCbor(build(noValue))
	require.ErrorContains(t, err, "value")

	noAddress := append([]byte{0xa1, 0x01}, value...)
	_, err = NewConwayTransactionFromCbor(build(noAddress))
	require.ErrorContains(t, err, "address")
}
