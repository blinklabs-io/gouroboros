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

package ledger

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const genericErrorOverLimitCount = 131073

// genericErrorArrayOverLimit declares one array entry more than the
// untrusted-data limit, using one-byte entries.
func genericErrorArrayOverLimit() []byte {
	data := []byte{0x9a, 0, 0x02, 0x00, 0x01}
	return append(data, make([]byte, genericErrorOverLimitCount)...)
}

// genericErrorMapOverLimit declares one map pair more than the
// untrusted-data limit. Keys are distinct so duplicate-key rejection cannot
// mask the count check.
func genericErrorMapOverLimit() []byte {
	data := []byte{0xba, 0, 0x02, 0x00, 0x01}
	for i := range genericErrorOverLimitCount {
		data = append(
			data,
			0x1a,
			byte(i>>24),
			byte(i>>16),
			byte(i>>8),
			byte(i),
			0x00,
		)
	}
	return data
}

func TestGenericErrorRejectsOversizedCollections(t *testing.T) {
	t.Parallel()
	cases := map[string][]byte{
		"array": genericErrorArrayOverLimit(),
		"map":   genericErrorMapOverLimit(),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := NewGenericErrorFromCbor(data)
			require.Error(t, err, "peer-declared count must be bounded")
			require.Nil(t, got)
			// A GenericError nested in another failure is decoded by
			// its own UnmarshalCBOR, not by NewGenericErrorFromCbor.
			var nested GenericError
			require.Error(
				t,
				nested.UnmarshalCBOR(data),
				"nested peer-declared count must be bounded",
			)
		})
	}
}

func TestGenericErrorDecodesSmallCollections(t *testing.T) {
	t.Parallel()
	data := []byte{0x83, 0x01, 0x02, 0x03}
	got, err := NewGenericErrorFromCbor(data)
	require.NoError(t, err)
	genErr, ok := got.(*GenericError)
	require.True(t, ok)
	require.Equal(t, data, genErr.Cbor)
	require.Equal(t, []any{uint64(1), uint64(2), uint64(3)}, genErr.Value)
}

// genericErrorNested returns an outer array of inner arrays. Each inner array
// is within the per-collection decoder limit; only the total exceeds the
// rejection item budget.
func genericErrorNested(inner, entries int) []byte {
	data := []byte{0x98, byte(inner)}
	for range inner {
		data = append(
			data,
			0x9a,
			0,
			byte(entries>>16),
			byte(entries>>8),
			byte(entries),
		)
		data = append(data, make([]byte, entries)...)
	}
	return data
}

// shelleyRejectionManyFailures returns a Conway ShelleyTxValidationError
// whose ApplyTxError lists count unrecognized one-field failures. Each is
// preserved as an UnknownApplyTxFailureError, so without a total bound every
// failure is decoded and retained.
func shelleyRejectionManyFailures(count int) []byte {
	data := []byte{
		0x81, 0x82, 0x06,
		0x9a, 0, byte(count >> 16), byte(count >> 8), byte(count),
	}
	for range count {
		data = append(data, 0x81, 0x00)
	}
	return data
}

func TestRejectionRejectsNestedCollectionsOverBudget(t *testing.T) {
	t.Parallel()
	nested := genericErrorNested(3, 100000)
	shelley := shelleyRejectionManyFailures(70000)
	cases := map[string]struct {
		fn   NewErrorFromCborFunc
		data []byte
	}{
		"tx submit generic": {NewTxSubmitErrorFromCbor, nested},
		"tx submit shelley": {NewTxSubmitErrorFromCbor, shelley},
		"generic":           {NewGenericErrorFromCbor, nested},
		"shelley":           {NewShelleyTxValidationErrorFromCbor, shelley},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.fn(tc.data)
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
	var generic GenericError
	require.Error(t, generic.UnmarshalCBOR(nested))
	var apply ApplyTxError
	require.Error(t, apply.UnmarshalCBOR(shelley[3:]))
}

func TestRejectionBudgetAcceptsUnderBound(t *testing.T) {
	t.Parallel()
	// 2 inner arrays of 60000 plus 3 container items: under the budget.
	data := genericErrorNested(2, 60000)
	got, err := NewTxSubmitErrorFromCbor(data)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestRejectionBudgetStructure(t *testing.T) {
	t.Parallel()
	valid := [][]byte{
		{0x00},
		{0x9f, 0x01, 0x02, 0xff},             // indefinite array
		{0xbf, 0x01, 0x02, 0xff},             // indefinite map
		{0xc1, 0x1a, 0x00, 0x00, 0x00, 0x01}, // tag
		{0x63, 'a', 'b', 'c'},                // text string
		{0x5f, 0x41, 0x01, 0xff},             // indefinite bytes
		{0xa1, 0x01, 0x82, 0x01, 0xf6},       // map with nested array
	}
	for _, d := range valid {
		require.NoError(t, checkRejectionBudget(d), "%x", d)
	}
	invalid := map[string][]byte{
		"empty":           {},
		"truncated array": {0x82, 0x01},
		"truncated head":  {0x1a, 0x00},
		"stray break":     {0x82, 0xff, 0x01},
		"long string":     {0x45, 0x01},
		"huge array": append(
			[]byte{0x9b},
			bytes.Repeat([]byte{0xff}, 8)...),
		"huge map": append(
			[]byte{0xbb},
			bytes.Repeat([]byte{0xff}, 8)...),
		"reserved info": {0x1c},
	}
	for name, d := range invalid {
		require.Error(t, checkRejectionBudget(d), name)
	}
	deep := make([]byte, 0, 20000)
	for range 20000 {
		deep = append(deep, 0x81)
	}
	require.Error(t, checkRejectionBudget(append(deep, 0x00)))
}

func TestRejectionOverBudgetAllocatesLittle(t *testing.T) {
	// Not t.Parallel: reads process-wide allocation counters.
	data := genericErrorNested(30, 131072)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := NewTxSubmitErrorFromCbor(data)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Less(
		t,
		after.TotalAlloc-before.TotalAlloc,
		uint64(8<<20),
		"rejected payload must not be materialized",
	)
}
