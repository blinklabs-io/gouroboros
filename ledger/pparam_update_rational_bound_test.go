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

package ledger_test

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hugeRationalCBOR returns a tag-30 rational whose independent random
// components are 128 KiB each.
func hugeRationalCBOR(t *testing.T) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	components := make([]*big.Int, 2)
	for i := range components {
		buf := make([]byte, 128*1024)
		_, _ = rng.Read(buf)
		components[i] = new(big.Int).SetBytes(buf)
	}
	content, err := cbor.Encode(components)
	require.NoError(t, err)
	raw, err := cbor.Encode(
		cbor.RawTag{Number: cbor.CborTagRational, Content: content},
	)
	require.NoError(t, err)
	return raw
}

func TestProtocolParameterUpdateDecodeBoundsRawRationals(t *testing.T) {
	t.Parallel()
	huge := hugeRationalCBOR(t)
	for _, test := range []struct {
		name   string
		key    uint64
		target interface{ UnmarshalCBOR([]byte) error }
	}{
		{"shelley rho", 10, &shelley.ShelleyProtocolParameterUpdate{}},
		{"mary tau", 11, &mary.MaryProtocolParameterUpdate{}},
		{"alonzo a0", 9, &alonzo.AlonzoProtocolParameterUpdate{}},
		{"babbage rho", 10, &babbage.BabbageProtocolParameterUpdate{}},
		{"conway tau", 11, &conway.ConwayProtocolParameterUpdate{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			update, err := cbor.Encode(
				map[uint64]cbor.RawMessage{test.key: huge},
			)
			require.NoError(t, err)
			err = test.target.UnmarshalCBOR(update)
			require.Error(t, err)
			require.NotNil(t, err)
			assert.Regexp(
				t,
				`numerator \d+ bits, denominator \d+ bits`,
				err.Error(),
			)
			assert.Less(t, len(err.Error()), 300)
		})
	}
}
