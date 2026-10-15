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

package cbor_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyDecodeIdFromList is DecodeIdFromList before it stopped decoding the
// whole list. It is the oracle for what the function must keep accepting and
// returning.
func legacyDecodeIdFromList(cborData []byte) (int, error) {
	if len(cborData) < 2 {
		return 0, errors.New("CBOR data too short for list with ID")
	}
	listLen, err := cbor.ListLength(cborData)
	if err != nil {
		return 0, err
	}
	if listLen == 0 {
		return 0, errors.New("cannot return first item from empty list")
	}
	if cborData[0] >= cbor.CborTypeArray &&
		cborData[0] <= (cbor.CborTypeArray+cbor.CborMaxUintSimple) &&
		cborData[1] <= cbor.CborMaxUintSimple {
		return int(cborData[1]), nil
	}
	var tmp cbor.Value
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	val := tmp.Value()
	list, ok := val.([]any)
	if !ok {
		return 0, fmt.Errorf("decoded value was not a list, found: %T", val)
	}
	if len(list) == 0 {
		return 0, errors.New("cannot return first item from empty list")
	}
	switch v := list[0].(type) {
	case uint64:
		if v > uint64(math.MaxInt) {
			return 0, errors.New("decoded numeric value too large: uint64 > int")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("first list item was not numeric, found: %v", v)
	}
}

// idListHead encodes a CBOR head of the given major type. width 0 selects the
// shortest form; 1, 2, 4 and 8 force that many following bytes, which gives
// the non-shortest forms a decoder must still accept.
func idListHead(major byte, value uint64, width int) []byte {
	major <<= 5
	if width == 0 {
		switch {
		case value < 24:
			width = -1
		case value <= math.MaxUint8:
			width = 1
		case value <= math.MaxUint16:
			width = 2
		case value <= math.MaxUint32:
			width = 4
		default:
			width = 8
		}
	}
	if width == -1 {
		return []byte{major | byte(value)}
	}
	head := []byte{major | byte(24+map[int]int{1: 0, 2: 1, 4: 2, 8: 3}[width])}
	for shift := (width - 1) * 8; shift >= 0; shift -= 8 {
		head = append(head, byte(value>>shift))
	}
	return head
}

// idListFits reports whether value can be written with the given width.
func idListFits(value uint64, width int) bool {
	switch width {
	case 1:
		return value <= math.MaxUint8
	case 2:
		return value <= math.MaxUint16
	case 4:
		return value <= math.MaxUint32
	}
	return true
}

// idListCase is a well-formed list [id, filler...] and where its parts are.
type idListCase struct {
	name string
	data []byte
	// count is the declared item count, or -1 for an indefinite list.
	count     int
	headerLen int
	firstLen  int
	// fast is true when the list header and the identifier each take one
	// byte, which is all that is needed to return the identifier.
	fast bool
}

// buildIDList encodes [id, filler x (count-1)] with the list header in the
// given width (-1 for an indefinite list) and the id in idWidth.
func buildIDList(
	headerWidth int,
	count int,
	id uint64,
	idWidth int,
	filler []byte,
) idListCase {
	var data []byte
	headerLen := 1
	if headerWidth == -1 {
		data = []byte{0x9f}
	} else {
		data = idListHead(4, uint64(count), headerWidth)
		headerLen = len(data)
	}
	first := idListHead(0, id, idWidth)
	data = append(data, first...)
	data = append(data, bytes.Repeat(filler, count-1)...)
	declared := count
	if headerWidth == -1 {
		data = append(data, 0xff)
		declared = -1
	}
	return idListCase{
		name: fmt.Sprintf(
			"header=%d count=%d id=%d idWidth=%d filler=%x",
			headerWidth, count, id, idWidth, filler,
		),
		data:      data,
		count:     declared,
		headerLen: headerLen,
		firstLen:  len(first),
		fast: headerWidth == 0 && count >= 1 && count <= 23 &&
			len(first) == 1 && id <= 23,
	}
}

// idListCorpus covers every list header form, every first-item form and a
// range of trailing items.
func idListCorpus() []idListCase {
	fillers := [][]byte{
		{0x00},
		// [1, {1: 2}]
		{0x82, 0x01, 0xa1, 0x01, 0x02},
		// "abc"
		{0x63, 0x61, 0x62, 0x63},
	}
	ids := []uint64{
		0, 1, 5, 23, 24, 25, 255, 256, 65535, 65536,
		math.MaxUint32, 1 << 32, 1 << 62, math.MaxInt64,
	}
	counts := []int{1, 2, 3, 23, 24, 25, 255, 256, 70000}
	largeListIDs := []uint64{0, 24, 65536}
	var corpus []idListCase
	for _, headerWidth := range []int{0, 1, 2, 4, 8, -1} {
		for _, count := range counts {
			if !idListFits(uint64(count), headerWidth) && headerWidth != -1 {
				continue
			}
			for _, id := range ids {
				// The reference decodes every item of the largest lists, so
				// they use a few ids and widths.
				if count > 1000 && !slices.Contains(largeListIDs, id) {
					continue
				}
				for _, idWidth := range []int{0, 1, 2, 4, 8} {
					if count > 1000 && idWidth != 0 && idWidth != 4 {
						continue
					}
					if !idListFits(id, idWidth) {
						continue
					}
					for _, filler := range fillers {
						// Keep the largest lists to one filler.
						if count > 1000 && len(filler) > 1 {
							continue
						}
						corpus = append(corpus, buildIDList(
							headerWidth, count, id, idWidth, filler,
						))
					}
				}
			}
		}
	}
	return corpus
}

// TestDecodeIdFromListMatchesLegacyForWellFormedLists checks that every
// well-formed list gets the identifier, or the error, the full decode gave,
// for each list header form, first-item form and trailing items.
func TestDecodeIdFromListMatchesLegacyForWellFormedLists(t *testing.T) {
	corpus := idListCorpus()
	require.Greater(t, len(corpus), 5000)
	for _, tc := range corpus {
		wantId, wantErr := legacyDecodeIdFromList(tc.data)
		gotId, gotErr := cbor.DecodeIdFromList(tc.data)
		require.Equal(t, wantErr == nil, gotErr == nil,
			"%s: legacy err %v, got err %v", tc.name, wantErr, gotErr)
		if wantErr != nil {
			require.Equal(t, wantErr.Error(), gotErr.Error(), tc.name)
			continue
		}
		require.Equal(t, wantId, gotId, tc.name)
	}
}

// TestDecodeIdFromListFirstItemNotNumeric checks that a first item that is
// not an unsigned integer is refused with the message the full decode gave,
// in each kind of list.
func TestDecodeIdFromListFirstItemNotNumeric(t *testing.T) {
	items := map[string][]byte{
		"negative integer": {0x20},
		"byte string":      {0x42, 0x01, 0x02},
		"text string":      {0x62, 0x68, 0x69},
		"true":             {0xf5},
		"null":             {0xf6},
		"float":            {0xfb, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0},
		"tagged integer":   {0xc1, 0x01},
		"array":            {0x82, 0x01, 0x02},
		"map":              {0xa1, 0x01, 0x02},
		"indefinite array": {0x9f, 0x01, 0xff},
		"huge integer": {
			0x1b,
			0xff,
			0xff,
			0xff,
			0xff,
			0xff,
			0xff,
			0xff,
			0xff,
		},
	}
	heads := map[string][]byte{
		"short":      {0x82},
		"1-byte":     {0x98, 0x02},
		"indefinite": {0x9f},
	}
	for itemName, item := range items {
		for headName, head := range heads {
			data := append(append([]byte{}, head...), item...)
			data = append(data, 0x00)
			if headName == "indefinite" {
				data = append(data, 0xff)
			}
			t.Run(itemName+"/"+headName, func(t *testing.T) {
				_, wantErr := legacyDecodeIdFromList(data)
				_, gotErr := cbor.DecodeIdFromList(data)
				require.Error(t, wantErr)
				require.EqualError(t, gotErr, wantErr.Error())
			})
		}
	}
}

// TestDecodeIdFromListTruncation checks every prefix of lists in each header
// form, the shortest included: the identifier is returned exactly when the
// list header and first item are complete and the declared items could fit,
// whatever was cut from the items after the first. A list whose header and
// identifier each take one byte needs only those two bytes, as it always has,
// and nothing the full decode accepted is refused.
func TestDecodeIdFromListTruncation(t *testing.T) {
	var cases []idListCase
	for _, headerWidth := range []int{0, 1, 2, 4, 8, -1} {
		for _, count := range []int{1, 2, 24, 300} {
			if !idListFits(uint64(count), headerWidth) && headerWidth != -1 {
				continue
			}
			for _, idWidth := range []int{0, 1, 2, 4, 8} {
				// 5 with the shortest header and width is the one-byte
				// shape; 70000 needs the wider forms.
				for _, id := range []uint64{5, 70000} {
					if !idListFits(id, idWidth) {
						continue
					}
					cases = append(cases, buildIDList(
						headerWidth, count, id, idWidth, []byte{0x00},
					))
				}
			}
		}
	}
	for _, tc := range cases {
		for cut := range len(tc.data) + 1 {
			data := tc.data[:cut]
			gotId, gotErr := cbor.DecodeIdFromList(data)
			remaining := max(cut-tc.headerLen, 0)
			var wantOk bool
			switch {
			case cut < 2:
				wantOk = false
			case tc.fast:
				// Two bytes are enough, whatever the list declares.
				wantOk = true
			case cut < tc.headerLen+tc.firstLen:
				wantOk = false
			default:
				wantOk = tc.count == -1 || tc.count <= remaining
			}
			require.Equal(
				t,
				wantOk,
				gotErr == nil,
				"%s cut at %d of %d: err %v",
				tc.name,
				cut,
				len(tc.data),
				gotErr,
			)
			if wantOk {
				wantId, _ := legacyDecodeIdFromList(tc.data)
				require.Equal(t, wantId, gotId, tc.name)
			}
			// Nothing the full decode accepted is refused.
			if wantId, wantErr := legacyDecodeIdFromList(data); wantErr == nil {
				require.NoError(t, gotErr,
					"%s cut at %d: full decode accepted it", tc.name, cut)
				require.Equal(t, wantId, gotId, tc.name)
			}
		}
	}
}

// slowIDList returns a list whose header or first item is not in its one-byte
// form, so the identifier needs more than the two leading bytes, followed by
// trailing after the first item.
func slowIDList(indefinite bool, trailing []byte) []byte {
	// 25 as a 1-byte-form unsigned integer.
	data := []byte{0x82, 0x18, 0x19}
	if indefinite {
		data = []byte{0x9f, 0x18, 0x19}
	}
	data = append(data, trailing...)
	if indefinite {
		data = append(data, 0xff)
	}
	return data
}

// TestDecodeIdFromListIgnoresTrailingItems checks that the identifier comes
// back whatever follows the first item: values nested past the decoder's
// limit, malformed items and a byte string far larger than the list needs.
func TestDecodeIdFromListIgnoresTrailingItems(t *testing.T) {
	deep := append(
		bytes.Repeat([]byte{0x81}, cbor.MaxNestedLevels+1000),
		0x00,
	)
	tests := map[string][]byte{
		"nested past the limit": deep,
		"reserved head":         {0x1c},
		"break outside a list":  {0xff},
		"truncated byte string": {0x5a, 0xff, 0xff, 0xff, 0xff},
		"large byte string": append(
			[]byte{0x5a, 0x00, 0x80, 0x00, 0x00}, make([]byte, 0x800000)...,
		),
	}
	for name, trailing := range tests {
		for _, indefinite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/indefinite=%t", name, indefinite),
				func(t *testing.T) {
					id, err := cbor.DecodeIdFromList(
						slowIDList(indefinite, trailing),
					)
					require.NoError(t, err)
					assert.Equal(t, 25, id)
				})
		}
	}
}

// TestDecodeIdFromListWorkIsIndependentOfTrailingItems checks the work done
// does not grow with the items after the first: the number of allocations is
// the same, and small, for no trailing items and for a very large or deeply
// nested one.
func TestDecodeIdFromListWorkIsIndependentOfTrailingItems(t *testing.T) {
	trailings := map[string][]byte{
		"none": nil,
		"1 MiB byte string": append(
			[]byte{0x5a, 0x00, 0x10, 0x00, 0x00}, make([]byte, 1<<20)...,
		),
		"8 MiB byte string": append(
			[]byte{0x5a, 0x00, 0x80, 0x00, 0x00}, make([]byte, 8<<20)...,
		),
		"nested 100000 deep": append(
			bytes.Repeat([]byte{0x81}, 100_000), 0x00,
		),
		"1000000 items declared and present": append(
			[]byte{0x9a, 0x00, 0x0f, 0x42, 0x40}, make([]byte, 1_000_000)...,
		),
	}
	for _, indefinite := range []bool{false, true} {
		var baseline float64
		for _, name := range []string{
			"none", "1 MiB byte string", "8 MiB byte string",
			"nested 100000 deep", "1000000 items declared and present",
		} {
			data := slowIDList(indefinite, trailings[name])
			var id int
			var err error
			allocs := testing.AllocsPerRun(20, func() {
				id, err = cbor.DecodeIdFromList(data)
			})
			require.NoError(t, err, "%s (indefinite=%t)", name, indefinite)
			require.Equal(t, 25, id, "%s (indefinite=%t)", name, indefinite)
			if name == "none" {
				baseline = allocs
				require.LessOrEqual(t, allocs, 20.0)
			}
			require.Equal(t, baseline, allocs,
				"%s (indefinite=%t)", name, indefinite)
		}
	}
}

// TestDecodeIdFromListErrorWorkIsIndependentOfTrailingItems checks that a
// first item that is not an unsigned integer, which is decoded to describe
// it, allocates a small fixed amount whatever follows it: a very large byte
// string or a deeply nested value. The bound is a tiny fraction of the
// trailing item, where decoding it once would cost at least its size. It
// reads the process-wide allocation counter, so it must not run in parallel.
func TestDecodeIdFromListErrorWorkIsIndependentOfTrailingItems(t *testing.T) {
	firsts := map[string][]byte{
		"true":           {0xf5},
		"short array":    {0x82, 0x01, 0x02},
		"tagged integer": {0xc1, 0x01},
	}
	trailings := map[string][]byte{
		"8 MiB byte string": append(
			[]byte{0x5a, 0x00, 0x80, 0x00, 0x00}, make([]byte, 8<<20)...,
		),
		"nested 100000 deep": append(
			bytes.Repeat([]byte{0x81}, 100_000), 0x00,
		),
	}
	const (
		calls = 20
		// Per call, with slack for allocations by the runtime itself.
		maxBytesPerCall = 16 << 10
		slack           = 1 << 20
	)
	for firstName, first := range firsts {
		for trailingName, trailing := range trailings {
			for _, indefinite := range []bool{false, true} {
				head := []byte{0x82}
				if indefinite {
					head = []byte{0x9f}
				}
				data := append(append([]byte{}, head...), first...)
				data = append(data, trailing...)
				if indefinite {
					data = append(data, 0xff)
				}
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				for range calls {
					_, err := cbor.DecodeIdFromList(data)
					require.Error(t, err)
				}
				runtime.ReadMemStats(&after)
				require.LessOrEqual(
					t,
					after.TotalAlloc-before.TotalAlloc,
					uint64(calls*maxBytesPerCall+slack),
					"%s then %s (indefinite=%t)",
					firstName, trailingName, indefinite,
				)
			}
		}
	}
}

// TestDecodeIdFromListOneByteFormsIgnoreDeclaredCount documents that a list
// whose header and identifier each take one byte returns the identifier from
// those two bytes whatever item count the header declares. Callers and tests
// pass such bare identifiers, for example Nonce.UnmarshalCBOR with
// 0x82 0x02, so this is kept as it was; every other encoding is checked.
func TestDecodeIdFromListOneByteFormsIgnoreDeclaredCount(t *testing.T) {
	tests := map[string]struct {
		data []byte
		id   int
	}{
		"two items, one present":  {[]byte{0x82, 0x02}, 2},
		"23 items, one present":   {[]byte{0x97, 0x00}, 0},
		"one item":                {[]byte{0x81, 0x17}, 23},
		"two items, both present": {[]byte{0x82, 0x05, 0x00}, 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			id, err := cbor.DecodeIdFromList(tc.data)
			require.NoError(t, err)
			assert.Equal(t, tc.id, id)
			legacyId, legacyErr := legacyDecodeIdFromList(tc.data)
			require.NoError(t, legacyErr)
			assert.Equal(t, legacyId, id)
		})
	}
	// The same truncation is refused once the header or identifier takes
	// more than one byte.
	refused := map[string][]byte{
		"1-byte list header":      {0x98, 0x02, 0x00},
		"1-byte identifier":       {0x83, 0x18, 0x05},
		"2-byte identifier":       {0x83, 0x19, 0x01},
		"empty list, any second":  {0x80, 0x00},
		"indefinite, empty":       {0x9f, 0xff},
		"indefinite, no item yet": {0x9f},
	}
	for name, data := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			_, err := cbor.DecodeIdFromList(data)
			assert.Error(t, err)
		})
	}
}

// TestDecodeIdFromListRejectsMalformedLists checks the inputs that cannot
// give an identifier because the list header or first item is missing,
// truncated or invalid.
func TestDecodeIdFromListRejectsMalformedLists(t *testing.T) {
	tests := map[string]struct {
		data []byte
		want string
	}{
		"nil":      {nil, "too short"},
		"one byte": {[]byte{0x9f}, "too short"},
		"not a list": {
			[]byte{0xa1, 0x01},
			"not a valid list",
		},
		"integer": {
			[]byte{0x18, 0x19},
			"not a valid list",
		},
		"empty list, any second byte": {[]byte{0x80, 0x18}, "empty list"},
		"empty list, 1-byte form": {
			[]byte{0x98, 0x00, 0x01},
			"empty list",
		},
		"empty indefinite list": {[]byte{0x9f, 0xff}, "empty list"},
		"indefinite list without item": {
			[]byte{0x9f, 0xff, 0x01},
			"empty list",
		},
		"truncated 1-byte length": {[]byte{0x98}, "too short"},
		"truncated 2-byte length": {
			[]byte{0x99, 0x01},
			"not a valid list",
		},
		"reserved length": {
			[]byte{0x9c, 0x01},
			"not a valid list",
		},
		"count above the bytes left": {
			[]byte{0x98, 0xff, 0x18, 0x19}, "declares 255 items in 2 bytes",
		},
		"count of 2^40": {
			[]byte{0x9b, 0, 0, 1, 0, 0, 0, 0, 0, 0x18, 0x19},
			"not a valid list",
		},
		"first item cut short":     {[]byte{0x82, 0x19, 0x01}, ""},
		"first item reserved head": {[]byte{0x82, 0x1c, 0x00}, ""},
		"unsigned above MaxInt": {
			[]byte{0x82, 0x1b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			"too large",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := cbor.DecodeIdFromList(tc.data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
