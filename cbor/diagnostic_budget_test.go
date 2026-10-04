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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticParsingHonorsAllocationBudgets(t *testing.T) {
	t.Parallel()
	data := []byte{0x86, 0, 0, 0, 0, 0, 0}
	for name, limits := range map[string]cbor.DiagnosticParseLimits{
		"nodes":       {MaxNodes: 6},
		"collection":  {MaxCollectionItems: 5},
		"owned bytes": {MaxRetainedBytes: 6},
		"work":        {MaxWorkBytes: 6},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := cbor.Diagnose(
				data,
				cbor.DiagnosticOptions{ParseLimits: limits},
			)
			require.Error(
				t,
				err,
				"parse budget must be enforced before rendering",
			)
			require.Nil(t, result)
		})
	}
	for _, limits := range []cbor.DiagnosticParseLimits{
		{MaxNodes: 7, MaxCollectionItems: 6},
		{},
	} {
		result, err := cbor.Diagnose(data, cbor.DiagnosticOptions{
			ParseLimits: limits, MaxArrayItems: 1,
		})
		require.NoError(t, err)
		require.Len(t, result.Root.Children, 6)
	}
}

func TestDiagnosticRawSpansShareOwnedInput(t *testing.T) {
	t.Parallel()
	input := []byte{0x82, 0x82, 1, 2, 3}
	root, err := cbor.ParseDiagnostic(input)
	require.NoError(t, err)
	child := &root.Children[0]
	require.True(t, &root.RawBytes[child.Offset] == &child.RawBytes[0],
		"nested raw spans must share one buffer instead of copying subtrees")
	require.Equal(t, len(child.RawBytes), cap(child.RawBytes))
	input[2] = 9
	require.Equal(
		t,
		byte(1),
		root.RawBytes[2],
		"caller mutation must not change owned bytes",
	)
}

func TestDiagnosticDeepRawSpansDoNotCopySubtrees(t *testing.T) {
	shallow := append(bytes.Repeat([]byte{0x81}, 512), 0)
	deep := append(bytes.Repeat([]byte{0x81}, 4096), 0)
	shallowAlloc := benchmarkDiagnosticParse(t, shallow, nil)
	deepAlloc := benchmarkDiagnosticParse(t, deep, nil)
	t.Logf(
		"depth %d: %d bytes/op; depth %d: %d bytes/op",
		len(shallow)-1,
		shallowAlloc,
		len(deep)-1,
		deepAlloc,
	)
	require.Less(
		t,
		deepAlloc,
		12*shallowAlloc,
		"eight times the depth must not approach quadratic allocation growth",
	)
}

func benchmarkDiagnosticParse(
	t *testing.T,
	data []byte,
	limits *cbor.DiagnosticParseLimits,
) int64 {
	t.Helper()
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var err error
			if limits == nil {
				_, err = cbor.ParseDiagnostic(data)
			} else {
				_, err = cbor.ParseDiagnosticWithLimits(data, *limits)
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	return result.AllocedBytesPerOp()
}

func TestDiagnosticWrappersApplyParseBudget(t *testing.T) {
	t.Parallel()
	opts := cbor.DiagnosticOptions{
		ParseLimits: cbor.DiagnosticParseLimits{MaxNodes: 1},
	}
	for name, call := range map[string]func() error{
		"transaction": func() error { _, err := cbor.FormatTransactionDiagnostic([]byte{0x82, 0xa0, 0xa0}, opts); return err },
		"block":       func() error { _, err := cbor.FormatBlockDiagnostic([]byte{0x85, 0, 0, 0, 0, 0}, opts); return err },
		"script":      func() error { _, err := cbor.FormatNativeScript([]byte{0x82, 0, 0x40}, opts); return err },
		"data":        func() error { _, err := cbor.FormatPlutusData([]byte{0x81, 0}, opts); return err },
	} {
		t.Run(
			name,
			func(t *testing.T) { require.ErrorContains(t, call(), "budget") },
		)
	}
}

func TestDiagnosticIndefiniteBudgetsAndOwnedStream(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		{0x9f, 0, 0, 0xff},
		{0xbf, 0, 0, 1, 0, 0xff},
		{0x5f, 0x41, 1, 0x41, 2, 0xff},
		{0x7f, 0x61, 'a', 0x61, 'b', 0xff},
	} {
		_, err := cbor.ParseDiagnosticWithLimits(
			data,
			cbor.DiagnosticParseLimits{MaxCollectionItems: 1},
		)
		require.ErrorContains(t, err, "collection budget")
		root, err := cbor.ParseDiagnosticWithLimits(
			data,
			cbor.DiagnosticParseLimits{MaxCollectionItems: 2},
		)
		require.NoError(t, err)
		require.Equal(t, data, root.RawBytes)
	}
	input := []byte{0x82, 1, 2, 3}
	stream, err := cbor.NewStreamDecoder(input)
	require.NoError(t, err)
	root, err := stream.DecodeDiagnostic()
	require.NoError(t, err)
	input[1] = 9
	require.Equal(t, byte(1), root.RawBytes[1])
	last, err := stream.DecodeDiagnostic()
	require.NoError(t, err)
	require.Equal(t, 3, last.Offset)
	require.True(t, stream.EOF())
}

func TestDiagnosticDecodedMemoryEnvelope(t *testing.T) {
	for name, data := range map[string][]byte{
		"scalars": append([]byte{0x99, 0x10, 0}, make([]byte, 4096)...),
		"maps":    append([]byte{0xb9, 0x08, 0}, make([]byte, 4096)...),
		"tags":    append(bytes.Repeat([]byte{0xc6}, 4096), 0),
		"bytes":   append([]byte{0x59, 0xc3, 0x50}, make([]byte, 50000)...),
		"chunks":  append(append([]byte{0x5f, 0x59, 0xc3, 0x50}, make([]byte, 50000)...), 0xff),
	} {
		t.Run(name, func(t *testing.T) {
			limits := cbor.DiagnosticParseLimits{
				MaxRetainedBytes: 64 * 1024,
				MaxWorkBytes:     64 * 1024,
			}
			bounded := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					_, err := cbor.ParseDiagnosticWithLimits(data, limits)
					if err == nil {
						b.Fatal("decoded allocation budget not enforced")
					}
				}
			})
			permissive := benchmarkDiagnosticParse(t, data, nil)
			t.Logf(
				"%d encoded bytes: bounded=%d bytes/op; permissive=%d bytes/op",
				len(data),
				bounded.AllocedBytesPerOp(),
				permissive,
			)
			require.Less(
				t,
				bounded.AllocedBytesPerOp(),
				permissive,
				"budget rejection must avoid the admitted parse's allocations",
			)
		})
	}
}

func TestDiagnosticCorpusWithinDefaultBudgets(t *testing.T) {
	t.Parallel()
	for _, block := range testdata.GetTestBlocks() {
		t.Run(block.Name, func(t *testing.T) {
			result, err := cbor.Diagnose(block.Cbor, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			t.Logf(
				"encoded=%d nodes=%d depth=%d",
				len(block.Cbor),
				result.Statistics.ElementCount,
				result.Statistics.MaxDepth,
			)
		})
	}
}

func TestDiagnosticNestedCollectionsShareBudget(t *testing.T) {
	t.Parallel()
	data := []byte{0x82, 0x82, 0, 0, 0x82, 0, 0}
	_, err := cbor.Diagnose(
		data,
		cbor.DiagnosticOptions{
			ParseLimits: cbor.DiagnosticParseLimits{MaxCollectionItems: 5},
		},
	)
	require.ErrorContains(t, err, "collection budget")
	_, err = cbor.Diagnose(
		data,
		cbor.DiagnosticOptions{
			ParseLimits: cbor.DiagnosticParseLimits{MaxCollectionItems: 6},
		},
	)
	require.NoError(t, err)
}

func TestDiagnosticTag24SharesOuterAndInnerBudget(t *testing.T) {
	t.Parallel()
	inner := []byte{0x85, 0, 0, 0, 0, 0}
	wire := append([]byte{0xd8, 0x18, 0x46}, inner...)
	for name, limits := range map[string]cbor.DiagnosticParseLimits{
		"nodes": {MaxNodes: 6},
		"work":  {MaxWorkBytes: 40},
	} {
		t.Run(name, func(t *testing.T) {
			opts := cbor.DiagnosticOptions{ParseLimits: limits}
			_, err := cbor.DiagnoseBlock(wire, opts)
			require.ErrorContains(t, err, "budget")
			_, err = cbor.FormatBlockDiagnostic(wire, opts)
			require.ErrorContains(t, err, "budget")
		})
	}
	_, err := cbor.DiagnoseBlock(
		wire,
		cbor.DiagnosticOptions{
			ParseLimits: cbor.DiagnosticParseLimits{MaxNodes: 8},
		},
	)
	require.NoError(t, err)
	t.Run("retained bytes", func(t *testing.T) {
		for capBytes := 1024; capBytes <= 64*1024; capBytes += 1024 {
			limits := cbor.DiagnosticParseLimits{MaxRetainedBytes: capBytes}
			_, outerErr := cbor.ParseDiagnosticWithLimits(wire, limits)
			_, innerErr := cbor.ParseDiagnosticWithLimits(inner, limits)
			if outerErr != nil || innerErr != nil {
				continue
			}
			_, err := cbor.DiagnoseBlock(
				wire,
				cbor.DiagnosticOptions{ParseLimits: limits},
			)
			require.ErrorContains(
				t,
				err,
				"budget",
				"both individual trees fit %d bytes; their combined operation must be charged",
				capBytes,
			)
			_, err = cbor.FormatBlockDiagnostic(
				wire,
				cbor.DiagnosticOptions{ParseLimits: limits},
			)
			require.ErrorContains(t, err, "budget")
			return
		}
		t.Fatal("no budget admits individual control trees")
	})
}
