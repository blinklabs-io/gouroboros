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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestValidateConwayProtocolParameterUpdateOptionalFields(t *testing.T) {
	fee := uint(1)
	update := ConwayProtocolParameterUpdate{MinFeeA: &fee}

	require.NoError(t, validateProtocolParameterUpdate(&update))
}

func TestValidateConwayProtocolParameterUpdateRejectsInvalidDomains(
	t *testing.T,
) {
	tests := []struct {
		name   string
		update ConwayProtocolParameterUpdate
	}{
		{
			name: "negative a0",
			update: ConwayProtocolParameterUpdate{
				A0: &cbor.Rat{Rat: big.NewRat(-1, 2)},
			},
		},
		{
			name: "a0 above one",
			update: ConwayProtocolParameterUpdate{
				A0: &cbor.Rat{Rat: big.NewRat(2, 1)},
			},
		},
		{
			name: "negative rho",
			update: ConwayProtocolParameterUpdate{
				Rho: &cbor.Rat{Rat: big.NewRat(-1, 2)},
			},
		},
		{
			name: "negative execution memory",
			update: ConwayProtocolParameterUpdate{
				MaxTxExUnits: &common.ExUnits{Memory: -1},
			},
		},
		{
			name: "null execution price",
			update: ConwayProtocolParameterUpdate{
				ExecutionCosts: &common.ExUnitPrice{
					MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
					StepPrice: nil,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var updateErr ConwayProtocolParameterUpdateError
			err := validateProtocolParameterUpdate(&test.update)
			require.ErrorAs(t, err, &updateErr)
		})
	}
}

func TestConwayProtocolParameterUpdateDecodeRejectsNullExecutionPrice(
	t *testing.T,
) {
	raw, err := cbor.Encode(map[uint]any{
		19: []any{nil, []any{uint64(1), uint64(2)}},
	})
	require.NoError(t, err)

	var update ConwayProtocolParameterUpdate
	_, err = cbor.Decode(raw, &update)
	var updateErr ConwayProtocolParameterUpdateError
	require.ErrorAs(t, err, &updateErr)
}

func TestConwayProtocolParameterUpdateAcceptsDomainBoundaries(t *testing.T) {
	update := ConwayProtocolParameterUpdate{
		A0:  &cbor.Rat{Rat: big.NewRat(0, 1)},
		Rho: &cbor.Rat{Rat: big.NewRat(0, 1)},
		Tau: &cbor.Rat{Rat: big.NewRat(1, 1)},
		ExecutionCosts: &common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(0, 1)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(1, 2)},
		},
		MaxTxExUnits:    &common.ExUnits{},
		MaxBlockExUnits: &common.ExUnits{},
	}
	data, err := cbor.Encode(update)
	require.NoError(t, err)

	var decoded ConwayProtocolParameterUpdate
	_, err = cbor.Decode(data, &decoded)
	require.NoError(t, err)
	require.NoError(t, validateProtocolParameterUpdate(&decoded))
}

func TestConwayTransactionBodyDecodeRejectsRemovedUpdateField(t *testing.T) {
	raw, err := cbor.Encode(map[uint]any{6: nil})
	require.NoError(t, err)

	_, err = NewConwayTransactionBodyFromCbor(raw)
	var bodyErr ConwayTransactionBodyFieldError
	require.ErrorAs(t, err, &bodyErr)
	require.Equal(t, 6, bodyErr.FieldKey)
}
