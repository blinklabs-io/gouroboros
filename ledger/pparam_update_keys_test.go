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

// pparamUpdateFieldValue returns a well-formed value for the protocol
// parameter update field with the given key, whatever era defines it.
func pparamUpdateFieldValue(t *testing.T, key uint64) []byte {
	t.Helper()
	half := cbor.Tag{Number: 30, Content: []uint64{1, 2}}
	var value any
	switch key {
	case 9, 10, 11, 12, 33:
		value = half
	case 25:
		value = []any{half, half, half, half, half}
	case 26:
		value = []any{
			half, half, half, half, half, half, half, half, half, half,
		}
	case 13:
		value = []uint64{0}
	case 14:
		value = []uint64{2, 0}
	case 18:
		value = map[uint64][]int64{}
	case 19:
		value = []any{half, half}
	case 20, 21:
		value = []uint64{1, 1}
	default:
		value = uint64(1)
	}
	ret, err := cbor.Encode(value)
	require.NoError(t, err)
	return ret
}

func pparamUpdateKeyRange(lo, hi uint64) map[uint64]bool {
	ret := make(map[uint64]bool)
	for key := lo; key <= hi; key++ {
		ret[key] = true
	}
	return ret
}

// TestProtocolParameterUpdateKeysPerEra checks which keys each era's
// update decoder accepts. The reference decodes a PParamsUpdate with
// SparseKeyed and routes a key outside the era's eraPParams list to
// invalidField, which fails the decode (DecCBOR (PParamsUpdate era) in
// libs/cardano-ledger-core/src/Cardano/Ledger/Core/PParams.hs). The lists are
// shelleyPParams (keys 0-16) for Shelley, Allegra and Mary; Alonzo drops
// minUTxOValue (15) and adds 17-24; Babbage further drops d (12) and
// extraEntropy (13) (eraPParams in each era's PParams.hs).
func TestProtocolParameterUpdateKeysPerEra(t *testing.T) {
	shelleyKeys := pparamUpdateKeyRange(0, 16)
	alonzoKeys := pparamUpdateKeyRange(0, 24)
	delete(alonzoKeys, 15)
	babbageKeys := pparamUpdateKeyRange(0, 24)
	for _, key := range []uint64{12, 13, 15} {
		delete(babbageKeys, key)
	}
	// Conway drops the protocol version (14) and adds the governance
	// parameters 25-33 (eraPParams in eras/conway/impl/src/Cardano/Ledger/
	// Conway/PParams.hs).
	conwayKeys := pparamUpdateKeyRange(0, 33)
	for _, key := range []uint64{12, 13, 14, 15} {
		delete(conwayKeys, key)
	}
	cases := []struct {
		era    string
		keys   map[uint64]bool
		decode func([]byte) error
	}{
		{"shelley", shelleyKeys, func(b []byte) error {
			var u shelley.ShelleyProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			return err
		}},
		{"mary", shelleyKeys, func(b []byte) error {
			var u mary.MaryProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			return err
		}},
		{"alonzo", alonzoKeys, func(b []byte) error {
			var u alonzo.AlonzoProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			return err
		}},
		{"babbage", babbageKeys, func(b []byte) error {
			var u babbage.BabbageProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			return err
		}},
		{"conway", conwayKeys, func(b []byte) error {
			var u conway.ConwayProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.era, func(t *testing.T) {
			for key := uint64(0); key <= 40; key++ {
				update, err := cbor.Encode(map[uint64]cbor.RawMessage{
					key: pparamUpdateFieldValue(t, key),
				})
				require.NoError(t, err)
				err = tc.decode(update)
				if tc.keys[key] {
					assert.NoError(t, err, "key %d", key)
				} else {
					assert.Error(t, err, "key %d", key)
				}
			}
		})
	}
}

// TestProtocolParameterUpdateApplyPerEra applies a one-field update
// for every key the era accepts and checks the update changed the current
// parameters. applyPPUpdates replaces each field the update sets
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/PParams.hs), so a key that
// decodes but leaves the parameters unchanged is dropped on adoption.
func TestProtocolParameterUpdateApplyPerEra(t *testing.T) {
	encode := func(t *testing.T, key uint64) []byte {
		value := pparamUpdateFieldValue(t, key)
		switch key {
		case 13:
			nonce, err := cbor.Encode([]any{uint64(1), make([]byte, 32)})
			require.NoError(t, err)
			nonce[len(nonce)-1] = 1
			value = nonce
		case 14:
			v, err := cbor.Encode([]uint64{3, 1})
			require.NoError(t, err)
			value = v
		case 18:
			v, err := cbor.Encode(map[uint64][]int64{0: make([]int64, 166)})
			require.NoError(t, err)
			value = v
		}
		ret, err := cbor.Encode(map[uint64]cbor.RawMessage{key: value})
		require.NoError(t, err)
		return ret
	}
	cases := []struct {
		era   string
		keys  []uint64
		apply func(*testing.T, []byte) (any, any)
	}{
		{"shelley", keysOf(pparamUpdateKeyRange(0, 16)),
			func(t *testing.T, b []byte) (any, any) {
				var u shelley.ShelleyProtocolParameterUpdate
				_, err := cbor.Decode(b, &u)
				require.NoError(t, err)
				p := shelley.ShelleyProtocolParameters{}
				p.Update(&u)
				return p, shelley.ShelleyProtocolParameters{}
			}},
		{"mary", keysOf(pparamUpdateKeyRange(0, 16)),
			func(t *testing.T, b []byte) (any, any) {
				var u mary.MaryProtocolParameterUpdate
				_, err := cbor.Decode(b, &u)
				require.NoError(t, err)
				p := mary.MaryProtocolParameters{}
				p.Update(&u)
				return p, mary.MaryProtocolParameters{}
			}},
		{"alonzo", []uint64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 18,
			19, 20, 21, 22, 23, 24,
		}, func(t *testing.T, b []byte) (any, any) {
			var u alonzo.AlonzoProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			require.NoError(t, err)
			p := alonzo.AlonzoProtocolParameters{}
			p.Update(&u)
			return p, alonzo.AlonzoProtocolParameters{}
		}},
		{"babbage", []uint64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 14, 16, 17, 18, 19, 20,
			21, 22, 23, 24,
		}, func(t *testing.T, b []byte) (any, any) {
			var u babbage.BabbageProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			require.NoError(t, err)
			p := babbage.BabbageProtocolParameters{}
			p.Update(&u)
			return p, babbage.BabbageProtocolParameters{}
		}},
		{"conway", []uint64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 17, 18, 19, 20, 21,
			22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33,
		}, func(t *testing.T, b []byte) (any, any) {
			var u conway.ConwayProtocolParameterUpdate
			_, err := cbor.Decode(b, &u)
			require.NoError(t, err)
			p := conway.ConwayProtocolParameters{}
			require.NoError(t, p.ApplyUpdate(&u))
			return p, conway.ConwayProtocolParameters{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.era, func(t *testing.T) {
			for _, key := range tc.keys {
				got, zero := tc.apply(t, encode(t, key))
				assert.NotEqual(t, zero, got, "key %d not applied", key)
			}
		})
	}
}

func keysOf(m map[uint64]bool) []uint64 {
	ret := make([]uint64, 0, len(m))
	for key := range m {
		ret = append(ret, key)
	}
	return ret
}
