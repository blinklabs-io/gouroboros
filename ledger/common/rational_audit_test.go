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

package common

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scaledRatCBOR(t testing.TB, n, d int64, seed int64) []byte {
	t.Helper()
	g := scaledBig(big.NewInt(1), 10*1024*8, seed)
	return ratCBOR(
		t,
		new(big.Int).Mul(g, big.NewInt(n)),
		new(big.Int).Mul(g, big.NewInt(d)),
	)
}

func hugeRatCBOR(t testing.TB, seed int64) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	return ratCBOR(t, randomBig(rng, 128*1024), randomBig(rng, 128*1024))
}

func updateWithField(t testing.TB, key uint64, raw []byte) []byte {
	t.Helper()
	out, err := cbor.Encode(map[uint64]cbor.RawMessage{key: raw})
	require.NoError(t, err)
	return out
}

func TestProtocolParameterUpdateDomainsBoundRawRationals(t *testing.T) {
	t.Parallel()
	eras := []struct {
		name string
		era  ProtocolParameterUpdateEra
	}{
		{"shelley", ProtocolParameterUpdateEraShelley},
		{"mary", ProtocolParameterUpdateEraMary},
		{"alonzo", ProtocolParameterUpdateEraAlonzo},
		{"babbage", ProtocolParameterUpdateEraBabbage},
	}
	for _, era := range eras {
		for _, key := range []uint64{9, 10, 11, 12} {
			if key == 12 && era.era > ProtocolParameterUpdateEraAlonzo {
				continue
			}
			t.Run(
				era.name+"/"+string(rune('0'+key/10))+string(rune('0'+key%10)),
				func(t *testing.T) {
					t.Parallel()
					_, err := ValidateProtocolParameterUpdateDomains(
						updateWithField(t, key, hugeRatCBOR(t, int64(key))),
						era.era,
					)
					var domainErr ProtocolParameterUpdateDomainError
					require.ErrorAs(t, err, &domainErr)
					assert.Regexp(
						t,
						`numerator \d+ bits, denominator \d+ bits`,
						domainErr.Reason,
					)
					assert.Less(t, len(domainErr.Reason), 200)

					_, err = ValidateProtocolParameterUpdateDomains(
						updateWithField(t, key, scaledRatCBOR(t, 1, 2, 5)),
						era.era,
					)
					require.NoError(t, err)

					if key == 9 {
						return
					}
					_, err = ValidateProtocolParameterUpdateDomains(
						updateWithField(t, key, scaledRatCBOR(t, 3, 2, 6)),
						era.era,
					)
					require.ErrorAs(t, err, &domainErr)
					assert.Contains(t, domainErr.Reason, "between zero and one")
				},
			)
		}
	}
}

func TestProtocolParameterUpdateDomainsBoundRawExecutionPrices(t *testing.T) {
	t.Parallel()
	for _, era := range []ProtocolParameterUpdateEra{
		ProtocolParameterUpdateEraAlonzo,
		ProtocolParameterUpdateEraBabbage,
	} {
		prices := func(mem, step []byte) []byte {
			raw, err := cbor.Encode(
				[]cbor.RawMessage{mem, step},
			)
			require.NoError(t, err)
			return updateWithField(t, 19, raw)
		}
		_, err := ValidateProtocolParameterUpdateDomains(
			prices(
				scaledRatCBOR(t, 577, 10000, 7),
				scaledRatCBOR(t, 721, 10000000, 8),
			),
			era,
		)
		require.NoError(t, err)

		_, err = ValidateProtocolParameterUpdateDomains(
			prices(scaledRatCBOR(t, 577, 10000, 7), hugeRatCBOR(t, 9)),
			era,
		)
		var domainErr ProtocolParameterUpdateDomainError
		require.ErrorAs(t, err, &domainErr)
		assert.Regexp(
			t,
			`numerator \d+ bits, denominator \d+ bits`,
			domainErr.Reason,
		)
		assert.Less(t, len(domainErr.Reason), 200)
	}
}

func TestUpdateCommitteeGovActionBoundsRawQuorum(t *testing.T) {
	t.Parallel()
	action := func(quorum []byte) []byte {
		raw, err := cbor.Encode([]any{
			uint(GovActionTypeUpdateCommittee),
			nil,
			[]any{},
			map[uint]uint{},
			cbor.RawMessage(quorum),
		})
		require.NoError(t, err)
		return raw
	}
	var decoded UpdateCommitteeGovAction
	require.NoError(
		t,
		decoded.UnmarshalCBOR(action(scaledRatCBOR(t, 1, 2, 10))),
	)
	assert.Zero(t, decoded.Quorum.Cmp(big.NewRat(1, 2)))

	err := decoded.UnmarshalCBOR(action(scaledRatCBOR(t, 3, 2, 11)))
	require.Error(t, err)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "outside [0,1]")

	err = decoded.UnmarshalCBOR(action(hugeRatCBOR(t, 12)))
	require.Error(t, err)
	require.NotNil(t, err)
	assert.Regexp(t, `numerator \d+ bits, denominator \d+ bits`, err.Error())
	assert.Less(t, len(err.Error()), 300)
}
