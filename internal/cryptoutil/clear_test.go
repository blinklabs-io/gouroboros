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

package cryptoutil

import (
	"bytes"
	"testing"

	"filippo.io/edwards25519"
	"github.com/stretchr/testify/require"
)

func TestWipe(t *testing.T) {
	secret := bytes.Repeat([]byte{0xa5}, 64)

	Wipe(secret)

	require.Equal(t, make([]byte, len(secret)), secret)
}

func TestWipeScalar(t *testing.T) {
	encoded := make([]byte, 32)
	encoded[0] = 7
	scalar, err := edwards25519.NewScalar().SetCanonicalBytes(encoded)
	require.NoError(t, err)
	require.NotEqual(t, make([]byte, 32), scalar.Bytes())

	WipeScalar(scalar)

	require.Equal(t, make([]byte, 32), scalar.Bytes())
}
