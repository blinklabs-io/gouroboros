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

// TestTimeRangeToPlutusDataUpperBoundEraDependent pins cardano-ledger's
// ERA-DEPENDENT encoding of a finite validity-interval upper bound
// (invalidHereafter):
//
//   - Conway and later (strictUpperBound == true): the upper bound is EXCLUSIVE
//     in every case (Conway.transValidityInterval / cardano-ledger#3043).
//   - Alonzo/Babbage (strictUpperBound == false): an upper-only interval uses
//     PV1.to, a CLOSED/INCLUSIVE upper bound; a two-sided interval already uses
//     strictUpperBound (EXCLUSIVE).
//
// The bug that motivated this test: gouroboros previously emitted
// `!lowerBoundPresent` for every era, giving Conway-era TTL-only transactions
// an INCLUSIVE upper bound and mis-computing script execution units.
func TestTimeRangeToPlutusDataUpperBoundEraDependent(t *testing.T) {
	tests := []struct {
		name string
		tr   TimeRange
		want data.PlutusData
	}{
		// --- Conway and later: upper bound always EXCLUSIVE ---
		{
			// invalidHereafter set, no invalidBefore, Conway+. This is
			// the case the bug affected. Upper must be EXCLUSIVE.
			name: "conway ttl only (upper present, lower absent)",
			tr: TimeRange{
				upperBound:        1000,
				upperBoundPresent: true,
				strictUpperBound:  true,
			},
			want: wholeRange(
				infBound(false),          // NegInf
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		{
			name: "conway both bounds present",
			tr: TimeRange{
				lowerBound:        500,
				upperBound:        1000,
				lowerBoundPresent: true,
				upperBoundPresent: true,
				strictUpperBound:  true,
			},
			want: wholeRange(
				finiteBound(500, true),   // Finite 500, INCLUSIVE
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		// --- Alonzo/Babbage: upper-only is INCLUSIVE, two-sided EXCLUSIVE ---
		{
			// invalidHereafter set, no invalidBefore, pre-Conway. Upper
			// must be INCLUSIVE (PV1.to). A version/language-only gate
			// that keyed off "V1/V2" would get this right but would then
			// wrongly apply it to Conway-era V1/V2 as well — hence the
			// gate is on the ERA, not the Plutus version.
			name: "preconway ttl only (upper present, lower absent)",
			tr: TimeRange{
				upperBound:        1000,
				upperBoundPresent: true,
				strictUpperBound:  false,
			},
			want: wholeRange(
				infBound(false),         // NegInf
				finiteBound(1000, true), // Finite 1000, INCLUSIVE
			),
		},
		{
			// Two-sided interval is EXCLUSIVE-upper even pre-Conway
			// (transVITime uses strictUpperBound when both bounds exist).
			name: "preconway both bounds present",
			tr: TimeRange{
				lowerBound:        500,
				upperBound:        1000,
				lowerBoundPresent: true,
				upperBoundPresent: true,
				strictUpperBound:  false,
			},
			want: wholeRange(
				finiteBound(500, true),   // Finite 500, INCLUSIVE
				finiteBound(1000, false), // Finite 1000, EXCLUSIVE
			),
		},
		// --- era-invariant shapes ---
		{
			name: "lower only (upper absent) - conway",
			tr: TimeRange{
				lowerBound:        500,
				lowerBoundPresent: true,
				strictUpperBound:  true,
			},
			want: wholeRange(finiteBound(500, true), infBound(true)),
		},
		{
			name: "lower only (upper absent) - preconway",
			tr: TimeRange{
				lowerBound:        500,
				lowerBoundPresent: true,
				strictUpperBound:  false,
			},
			want: wholeRange(finiteBound(500, true), infBound(true)),
		},
		{
			name: "unbounded - conway",
			tr:   TimeRange{strictUpperBound: true},
			want: wholeRange(infBound(false), infBound(true)),
		},
		{
			name: "unbounded - preconway",
			tr:   TimeRange{strictUpperBound: false},
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
