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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// encodeLangViewParamsDirect is the language-view parameter encoding without
// the memo.
func encodeLangViewParamsDirect(
	t *testing.T,
	version uint,
	costModel []int64,
) []byte {
	t.Helper()
	if version == 0 {
		indef := make(cbor.IndefLengthList, len(costModel))
		for i, v := range costModel {
			indef[i] = any(v)
		}
		inner, err := cbor.Encode(indef)
		if err != nil {
			t.Fatal(err)
		}
		out, err := cbor.Encode(inner)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out, err := cbor.Encode(costModel)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestLangViewParamsMemoIsKeyedByContent checks that a memoized encoding is
// returned only for the same version and the same cost model values: cost
// models of equal length that differ in one parameter, or the same values
// under another language, must not share an encoding.
func TestLangViewParamsMemoIsKeyedByContent(t *testing.T) {
	base := make([]int64, 175)
	for i := range base {
		base[i] = int64(i*7919 + 1)
	}
	changed := append([]int64(nil), base...)
	changed[100]++

	for _, version := range []uint{0, 1, 2, 3} {
		for _, model := range [][]int64{base, changed, base} {
			for range 2 {
				got, err := langViewParams(version, model)
				if err != nil {
					t.Fatal(err)
				}
				if want := encodeLangViewParamsDirect(t, version, model); !bytes.Equal(
					got,
					want,
				) {
					t.Fatalf(
						"version %d: memoized encoding differs from a fresh one",
						version,
					)
				}
			}
		}
	}

	// The memo copies the cost model. Were it to keep the caller's slice, a
	// caller reusing that slice for other values would leave an entry
	// claiming the new values encode as the old ones.
	reused := make([]int64, len(base))
	for i := range reused {
		reused[i] = base[i] + 1_000_000
	}
	if _, err := langViewParams(2, reused); err != nil {
		t.Fatal(err)
	}
	for i := range reused {
		reused[i] += 1_000_000
	}
	got, err := langViewParams(2, reused)
	if err != nil {
		t.Fatal(err)
	}
	if want := encodeLangViewParamsDirect(t, 2, reused); !bytes.Equal(
		got,
		want,
	) {
		t.Fatal(
			"a reused cost model slice was matched to its earlier values' encoding",
		)
	}
}

func TestLangViewParamsMemoStaysBounded(t *testing.T) {
	for i := range 3 * langViewParamsMemoMax {
		if _, err := langViewParams(1, []int64{int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	langViewParamsMemo.Lock()
	n := len(langViewParamsMemo.entries)
	langViewParamsMemo.Unlock()
	if n > langViewParamsMemoMax {
		t.Fatalf("memo holds %d entries, bound is %d", n, langViewParamsMemoMax)
	}
}

func TestLangViewParamsDistinguishesNilFromEmpty(t *testing.T) {
	for _, order := range [][2][]int64{{nil, {}}, {{}, nil}} {
		for _, model := range order {
			got, err := langViewParams(2, model)
			if err != nil {
				t.Fatal(err)
			}
			want, err := cbor.Encode(model)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf(
					"model nil=%v: got %x, want %x",
					model == nil,
					got,
					want,
				)
			}
		}
	}
}
