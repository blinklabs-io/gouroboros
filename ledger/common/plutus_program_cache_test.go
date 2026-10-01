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
	"errors"
	"math/big"
	"runtime"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

// programCacheTestScript returns the CBOR-wrapped flat encoding of a
// \ctx -> ctx program at the given UPLC version. salt makes the bytes unique
// per test so tests sharing defaultProgramCache never share an entry.
func programCacheTestScript(
	t testing.TB,
	version lang.LanguageVersion,
	salt int64,
) []byte {
	t.Helper()
	var term syn.Term[syn.DeBruijn] = &syn.Constant{
		Con: &syn.Integer{Inner: big.NewInt(salt)},
	}
	// Extra applications grow the program so the size estimate is non-trivial.
	for range 32 {
		term = &syn.Apply[syn.DeBruijn]{
			Function: &syn.Lambda[syn.DeBruijn]{
				Body: &syn.Var[syn.DeBruijn]{Name: syn.DeBruijn(1)},
			},
			Argument: term,
		}
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: version,
		Term:    &syn.Lambda[syn.DeBruijn]{Body: term},
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return wrapper
}

func programCacheTestEvalContext(
	t testing.TB,
	version lang.LanguageVersion,
	protoMajor uint,
) *cek.EvalContext {
	t.Helper()
	params := make([]int64, len(lang.GetParamNamesForVersion(version)))
	for i := range params {
		params[i] = int64(i + 1)
	}
	evalContext, err := cek.NewEvalContext(
		version,
		cek.ProtoVersion{Major: protoMajor},
		params,
	)
	require.NoError(t, err)
	return evalContext
}

func TestDecodePlutusProgramReusesProgramForSameKey(t *testing.T) {
	t.Parallel()

	inner, err := decodePlutusScript(
		programCacheTestScript(t, lang.LanguageVersion{1, 0, 0}, 910001),
		true,
	)
	require.NoError(t, err)
	evalContext := programCacheTestEvalContext(t, lang.LanguageVersionV3, 10)

	first, err := decodePlutusProgram(inner, lang.LanguageVersionV3, evalContext)
	require.NoError(t, err)
	second, err := decodePlutusProgram(inner, lang.LanguageVersionV3, evalContext)
	require.NoError(t, err)
	require.Same(t, first, second,
		"an identical script, language and protocol major must reuse the"+
			" decoded program")

	otherLanguage, err := decodePlutusProgram(
		inner,
		lang.LanguageVersionV2,
		programCacheTestEvalContext(t, lang.LanguageVersionV2, 10),
	)
	require.NoError(t, err)
	require.NotSame(t, first, otherLanguage,
		"a program must never be shared across ledger languages")

	otherMajor, err := decodePlutusProgram(
		inner,
		lang.LanguageVersionV3,
		programCacheTestEvalContext(t, lang.LanguageVersionV3, 11),
	)
	require.NoError(t, err)
	require.NotSame(t, first, otherMajor,
		"a program must never be shared across protocol major versions")
}

// TestDecodePlutusProgramCachedFailureIsKeyedByProtocolMajor proves a cached
// ValidateTermVersionForExecution failure does not leak into a protocol major
// where the same script is legal, and that the failure is repeated unchanged.
func TestDecodePlutusProgramCachedFailureIsKeyedByProtocolMajor(t *testing.T) {
	t.Parallel()

	inner, err := decodePlutusScript(
		programCacheTestScript(t, lang.LanguageVersion{1, 1, 0}, 910002),
		true,
	)
	require.NoError(t, err)
	oldCtx := programCacheTestEvalContext(t, lang.LanguageVersionV2, 10)
	newCtx := programCacheTestEvalContext(t, lang.LanguageVersionV2, 11)

	_, firstErr := decodePlutusProgram(inner, lang.LanguageVersionV2, oldCtx)
	require.Error(t, firstErr)
	_, secondErr := decodePlutusProgram(inner, lang.LanguageVersionV2, oldCtx)
	require.EqualError(t, secondErr, firstErr.Error())

	program, err := decodePlutusProgram(inner, lang.LanguageVersionV2, newCtx)
	require.NoError(t, err)
	require.NotNil(t, program)
}

func TestProgramCacheEvictsByBytes(t *testing.T) {
	t.Parallel()

	evalContext := programCacheTestEvalContext(t, lang.LanguageVersionV3, 10)
	scripts := make([][]byte, 3)
	for i := range scripts {
		wrapped := programCacheTestScript(
			t, lang.LanguageVersion{1, 0, 0}, int64(920000+i),
		)
		inner, err := decodePlutusScript(wrapped, true)
		require.NoError(t, err)
		scripts[i] = inner
	}
	probe, err := decodePlutusProgramUncached(
		scripts[0], lang.LanguageVersionV3, evalContext,
	)
	require.NoError(t, err)
	entrySize := programEntrySize(probe)

	cache := newProgramCache(2*entrySize + entrySize/2)
	decodes := 0
	decode := func(script []byte) *syn.Program[syn.DeBruijn] {
		program, err := cache.decode(
			script,
			lang.LanguageVersionV3,
			10,
			func() (*syn.Program[syn.DeBruijn], error) {
				decodes++
				return decodePlutusProgramUncached(
					script, lang.LanguageVersionV3, evalContext,
				)
			},
		)
		require.NoError(t, err)
		return program
	}

	first := decode(scripts[0])
	decode(scripts[1])
	require.Same(t, first, decode(scripts[0]), "entry within budget is kept")
	decode(scripts[2]) // evicts scripts[1], the least recently used
	require.Equal(t, 3, decodes)
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)

	require.Same(t, first, decode(scripts[0]), "recently used entry survives")
	require.Equal(t, 3, decodes)
	decode(scripts[1])
	require.Equal(t, 4, decodes, "least recently used entry was evicted")
}

// TestProgramCacheOversizedEntryKeepsExistingEntries proves an entry larger
// than the whole budget is refused outright rather than admitted and then
// flushing every smaller entry while it evicts itself.
func TestProgramCacheOversizedEntryKeepsExistingEntries(t *testing.T) {
	t.Parallel()

	cache := newProgramCache(3 * programEstimateFloor)
	small := &syn.Program[syn.DeBruijn]{}
	smallDecodes := 0
	decodeSmall := func() (*syn.Program[syn.DeBruijn], error) {
		smallDecodes++
		return small, nil
	}
	_, err := cache.decode([]byte{1}, lang.LanguageVersionV3, 10, decodeSmall)
	require.NoError(t, err)

	hugeTerm := make([]syn.Term[syn.DeBruijn], 0, 4096)
	for range 4096 {
		hugeTerm = append(hugeTerm, &syn.Error{})
	}
	huge := &syn.Program[syn.DeBruijn]{
		Term: &syn.Constr[syn.DeBruijn]{Fields: hugeTerm},
	}
	require.Greater(t, programEntrySize(huge), cache.maxBytes)
	for range 2 {
		got, err := cache.decode(
			[]byte{2}, lang.LanguageVersionV3, 10,
			func() (*syn.Program[syn.DeBruijn], error) { return huge, nil },
		)
		require.NoError(t, err)
		require.Same(t, huge, got)
	}
	require.Len(t, cache.entries, 1, "oversized entry must not be admitted")

	_, err = cache.decode([]byte{1}, lang.LanguageVersionV3, 10, decodeSmall)
	require.NoError(t, err)
	require.Equal(t, 1, smallDecodes, "existing entry must survive")
}

// TestProgramCacheCachesDecodeFailures proves a failed decode is decoded once
// and its error replayed.
func TestProgramCacheCachesDecodeFailures(t *testing.T) {
	t.Parallel()

	cache := newProgramCache(programCacheMaxBytes)
	decodes := 0
	boom := errors.New("decode failed")
	for range 3 {
		program, err := cache.decode(
			[]byte{9}, lang.LanguageVersionV3, 10,
			func() (*syn.Program[syn.DeBruijn], error) {
				decodes++
				return nil, boom
			},
		)
		require.Nil(t, program)
		require.ErrorIs(t, err, boom)
	}
	require.Equal(t, 1, decodes)
}

func programCacheCon(c syn.IConstant) syn.Term[syn.DeBruijn] {
	return &syn.Constant{Con: c}
}

// programCacheRetentionShapes returns programs chosen to stress the size
// estimate: every term kind in a few bytes, deep application chains, and
// constants whose retained memory is large relative to their flat encoding.
func programCacheRetentionShapes() map[string]syn.Term[syn.DeBruijn] {
	const n = 20000
	var ints, units, bools, empties, lists, pairs []syn.IConstant
	var dataItems []data.PlutusData
	for i := range n {
		ints = append(ints, &syn.Integer{Inner: big.NewInt(int64(i % 60))})
		units = append(units, &syn.Unit{})
		bools = append(bools, &syn.Bool{Inner: true})
		empties = append(empties, &syn.ByteString{Inner: []byte{}})
		lists = append(lists, &syn.ProtoList{LTyp: &syn.TUnit{}})
		pairs = append(pairs, &syn.ProtoPair{
			FstType: &syn.TUnit{},
			SndType: &syn.TUnit{},
			First:   &syn.Unit{},
			Second:  &syn.Unit{},
		})
		dataItems = append(dataItems, data.NewInteger(big.NewInt(int64(i%20))))
	}
	allKinds := func() syn.Term[syn.DeBruijn] {
		var term syn.Term[syn.DeBruijn] = &syn.Error{}
		term = &syn.Case[syn.DeBruijn]{
			Constr: &syn.Constr[syn.DeBruijn]{
				Fields: []syn.Term[syn.DeBruijn]{term},
			},
			Branches: []syn.Term[syn.DeBruijn]{&syn.Builtin{}},
		}
		term = &syn.Force[syn.DeBruijn]{
			Term: &syn.Delay[syn.DeBruijn]{Term: term},
		}
		term = &syn.Apply[syn.DeBruijn]{
			Function: &syn.Lambda[syn.DeBruijn]{
				Body: &syn.Var[syn.DeBruijn]{Name: 1},
			},
			Argument: term,
		}
		return &syn.Lambda[syn.DeBruijn]{
			Body: &syn.Apply[syn.DeBruijn]{
				Function: term,
				Argument: programCacheCon(
					&syn.Integer{Inner: big.NewInt(1)},
				),
			},
		}
	}
	applyChain := func() syn.Term[syn.DeBruijn] {
		term := programCacheCon(&syn.Integer{Inner: big.NewInt(1)})
		for range 5000 {
			term = &syn.Apply[syn.DeBruijn]{
				Function: &syn.Lambda[syn.DeBruijn]{
					Body: &syn.Var[syn.DeBruijn]{Name: 1},
				},
				Argument: term,
			}
		}
		return &syn.Lambda[syn.DeBruijn]{Body: term}
	}
	// A constant's type is decoded into heap nodes that the decoded value
	// keeps alive, so a deeply nested type costs memory without any value.
	deepListType := func() syn.Typ {
		var typ syn.Typ = &syn.TUnit{}
		for range 12000 {
			typ = &syn.TList{Typ: typ}
		}
		return typ
	}
	deepPairType := func() syn.Typ {
		var typ syn.Typ = &syn.TUnit{}
		for range 6000 {
			typ = &syn.TPair{First: typ, Second: &syn.TUnit{}}
		}
		return typ
	}
	return map[string]syn.Term[syn.DeBruijn]{
		"all node kinds": allKinds(),
		"deep list type": programCacheCon(
			&syn.ProtoList{LTyp: deepListType()}),
		"deep pair type": programCacheCon(
			&syn.ProtoList{LTyp: deepPairType()}),
		"apply chain": applyChain(),
		"integer list": programCacheCon(
			&syn.ProtoList{LTyp: &syn.TInteger{}, List: ints}),
		"unit list": programCacheCon(
			&syn.ProtoList{LTyp: &syn.TUnit{}, List: units}),
		"bool list": programCacheCon(
			&syn.ProtoList{LTyp: &syn.TBool{}, List: bools}),
		"empty bytestring list": programCacheCon(
			&syn.ProtoList{LTyp: &syn.TByteString{}, List: empties}),
		"nested empty lists": programCacheCon(
			&syn.ProtoList{
				LTyp: &syn.TList{Typ: &syn.TUnit{}}, List: lists,
			}),
		"unit pair list": programCacheCon(
			&syn.ProtoList{
				LTyp: &syn.TPair{
					First: &syn.TUnit{}, Second: &syn.TUnit{},
				},
				List: pairs,
			}),
		"data list": programCacheCon(
			&syn.Data{Inner: data.NewList(dataItems...)}),
	}
}

// TestProgramCacheChargeCoversRetainedHeap decodes copies of programs whose
// retained memory is far from proportional to script length and requires the
// charge each would receive to be at least the heap it keeps alive.
func TestProgramCacheChargeCoversRetainedHeap(t *testing.T) {
	// Not t.Parallel: runtime.MemStats.HeapAlloc is process-wide, so
	// concurrent tests would pollute the measured delta.
	evalContext := programCacheTestEvalContext(t, lang.LanguageVersionV3, 10)
	const copies = 20
	for name, term := range programCacheRetentionShapes() {
		t.Run(name, func(t *testing.T) {
			flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
				Version: lang.LanguageVersion{1, 1, 0},
				Term:    term,
			})
			require.NoError(t, err)
			wrapped, err := cbor.Encode(flat)
			require.NoError(t, err)
			inner, err := decodePlutusScript(wrapped, true)
			require.NoError(t, err)

			keep := make([]*syn.Program[syn.DeBruijn], 0, copies)
			runtime.GC()
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range copies {
				program, err := decodePlutusProgramUncached(
					inner, lang.LanguageVersionV3, evalContext,
				)
				require.NoError(t, err)
				keep = append(keep, program)
			}
			runtime.GC()
			runtime.GC()
			runtime.ReadMemStats(&after)
			retained := (int64(after.HeapAlloc) - int64(before.HeapAlloc)) /
				copies
			charge := programEntrySize(keep[0])
			runtime.KeepAlive(keep)
			require.GreaterOrEqual(t, charge, retained,
				"charged %d bytes for a %d byte script that keeps %d bytes",
				charge, len(inner), retained)
		})
	}
}

// TestEvaluateReusingCachedProgramIsUnchanged evaluates one script many times
// from many goroutines against the shared cached program. Results must equal
// the first evaluation, and the program's encoding must be unchanged
// afterwards. Run under -race to catch writes through the shared term graph.
func TestEvaluateReusingCachedProgramIsUnchanged(t *testing.T) {
	t.Parallel()

	script := PlutusV3Script(
		programCacheTestScript(t, lang.LanguageVersion{1, 0, 0}, 910003),
	)
	inner, err := decodePlutusScript([]byte(script), true)
	require.NoError(t, err)
	evalContext := programCacheTestEvalContext(t, lang.LanguageVersionV3, 10)
	budget := machinePoolTestBudget()
	scriptContext := machinePoolTestScriptContext()

	want, err := script.Evaluate(scriptContext, budget, evalContext)
	require.NoError(t, err)
	program, err := decodePlutusProgram(inner, lang.LanguageVersionV3, evalContext)
	require.NoError(t, err)
	encodedBefore, err := syn.Encode(program)
	require.NoError(t, err)

	const workers = 8
	const rounds = 50
	errs := make(chan error, workers*rounds)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				got, err := script.Evaluate(scriptContext, budget, evalContext)
				if err != nil {
					errs <- err
					continue
				}
				if got != want {
					errs <- errors.New("evaluation result changed on reuse")
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	after, err := decodePlutusProgram(inner, lang.LanguageVersionV3, evalContext)
	require.NoError(t, err)
	require.Same(t, program, after)
	encodedAfter, err := syn.Encode(after)
	require.NoError(t, err)
	require.Equal(t, encodedBefore, encodedAfter)
}

// BenchmarkPlutusProgramDecode isolates the CBOR unwrap and UPLC decode from
// CEK execution: "uncached" always decodes, "cached" goes through
// decodePlutusProgram, and "evaluate" is the full Evaluate path.
func BenchmarkPlutusProgramDecode(b *testing.B) {
	wrapped := programCacheTestScript(b, lang.LanguageVersion{1, 0, 0}, 910004)
	evalContext := programCacheTestEvalContext(b, lang.LanguageVersionV3, 10)

	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			inner, err := decodePlutusScript(wrapped, true)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := decodePlutusProgramUncached(
				inner,
				lang.LanguageVersionV3,
				evalContext,
			); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			inner, err := decodePlutusScript(wrapped, true)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := decodePlutusProgram(
				inner,
				lang.LanguageVersionV3,
				evalContext,
			); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("evaluate", func(b *testing.B) {
		script := PlutusV3Script(wrapped)
		budget := machinePoolTestBudget()
		scriptContext := machinePoolTestScriptContext()
		b.ReportAllocs()
		for b.Loop() {
			if _, err := script.Evaluate(
				scriptContext,
				budget,
				evalContext,
			); err != nil {
				b.Fatal(err)
			}
		}
	})
}
