// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package script

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// boolConstr is the Plutus encoding of a Haskell Bool: Constr 0 [] for False,
// Constr 1 [] for True (mirrors toPlutusData(bool)).
func boolConstr(v bool) data.PlutusData {
	if v {
		return data.NewConstr(1)
	}
	return data.NewConstr(0)
}

// finiteBound builds Constr 0 [ Constr 1 [ Integer value ], <closureBool> ] —
// the Plutus encoding of a Finite `Extended` bound with the given closure.
func finiteBound(value uint64, closed bool) data.PlutusData {
	return data.NewConstr(
		0,
		data.NewConstr(1, data.NewInteger(new(big.Int).SetUint64(value))),
		boolConstr(closed),
	)
}

// infBound builds an infinite `Extended` bound: NegInf -> Constr 0, PosInf ->
// Constr 2; both carry closure True by the Plutus "infinite bounds are always
// exclusive" convention.
func infBound(posInf bool) data.PlutusData {
	tag := uint64(0)
	if posInf {
		tag = 2
	}
	return data.NewConstr(0, data.NewConstr(tag), boolConstr(true))
}

func wholeRange(lower, upper data.PlutusData) data.PlutusData {
	return data.NewConstr(0, lower, upper)
}

// TestTimeRangeToPlutusDataUpperBound pins cardano-ledger's strict upper-bound
// encoding for finite validity-interval upper bounds.
func TestTimeRangeToPlutusDataUpperBound(t *testing.T) {
	tests := []struct {
		name string
		tr   TimeRange
		want data.PlutusData
	}{
		{
			// invalidHereafter set, no invalidBefore, Conway+. This is
			// the case the bug affected. Upper must be EXCLUSIVE.
			name: "conway ttl only (upper present, lower absent)",
			tr: TimeRange{
				upperBound:        1000,
				upperBoundPresent: true,
			},
			want: wholeRange(
				infBound(false),          // NegInf
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		{
			name: "both bounds present",
			tr: TimeRange{
				lowerBound:        500,
				upperBound:        1000,
				lowerBoundPresent: true,
				upperBoundPresent: true,
			},
			want: wholeRange(
				finiteBound(500, true),   // Finite 500, INCLUSIVE
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		{
			name: "ttl only remains exclusive",
			tr: TimeRange{
				upperBound:        1000,
				upperBoundPresent: true,
			},
			want: wholeRange(
				infBound(false),          // NegInf
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		{
			name: "both bounds remain exclusive",
			tr: TimeRange{
				lowerBound:        500,
				upperBound:        1000,
				lowerBoundPresent: true,
				upperBoundPresent: true,
			},
			want: wholeRange(
				finiteBound(500, true),   // Finite 500, INCLUSIVE
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		{
			name: "lower only (upper absent) - conway",
			tr: TimeRange{
				lowerBound:        500,
				lowerBoundPresent: true,
			},
			want: wholeRange(finiteBound(500, true), infBound(true)),
		},
		{
			name: "lower only (upper absent)",
			tr: TimeRange{
				lowerBound:        500,
				lowerBoundPresent: true,
			},
			want: wholeRange(finiteBound(500, true), infBound(true)),
		},
		{
			name: "unbounded",
			tr:   TimeRange{},
			want: wholeRange(infBound(false), infBound(true)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tr.ToPlutusData()
			require.True(
				t,
				tc.want.Equal(got),
				"TimeRange.ToPlutusData mismatch:\n got: %s\nwant: %s",
				got,
				tc.want,
			)
		})
	}
}
