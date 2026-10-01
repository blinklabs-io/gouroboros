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
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/cek"
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

	entrySize := programCacheEntryOverhead +
		int64(100)*programArenaBytesPerScriptByte
	cache := newProgramCache(2 * entrySize)
	decodes := 0
	decode := func(script []byte) *syn.Program[syn.DeBruijn] {
		program, err := cache.decode(
			script,
			lang.LanguageVersionV3,
			10,
			func() (*syn.Program[syn.DeBruijn], error) {
				decodes++
				return &syn.Program[syn.DeBruijn]{}, nil
			},
		)
		require.NoError(t, err)
		return program
	}
	scripts := make([][]byte, 3)
	for i := range scripts {
		scripts[i] = make([]byte, 100)
		scripts[i][0] = byte(i + 1)
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

	oversized := newProgramCache(entrySize - 1)
	for range 2 {
		_, err := oversized.decode(
			scripts[0],
			lang.LanguageVersionV3,
			10,
			func() (*syn.Program[syn.DeBruijn], error) {
				decodes++
				return &syn.Program[syn.DeBruijn]{}, nil
			},
		)
		require.NoError(t, err)
	}
	require.Equal(t, 6, decodes, "an entry larger than the budget is not kept")
	require.Zero(t, oversized.bytes)
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
