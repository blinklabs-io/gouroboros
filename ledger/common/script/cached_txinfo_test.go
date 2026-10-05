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

package script_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func encodeContext(t testing.TB, sc script.ScriptContext) []byte {
	t.Helper()
	b, err := data.Encode(sc.ToPlutusData())
	require.NoError(t, err)
	return b
}

// contextsForEveryRedeemer builds one context per redeemer of txInfo, using
// wrap to choose between the plain and the cached TxInfo.
func contextsForEveryRedeemer(
	txInfo script.TxInfo,
	wrap func(script.TxInfo) script.TxInfo,
) []script.ScriptContext {
	var ret []script.ScriptContext
	switch ti := txInfo.(type) {
	case script.TxInfoV1:
		for _, r := range ti.Redeemers {
			ret = append(ret, script.NewScriptContextV1V2(wrap(ti), r.Key))
		}
	case script.TxInfoV2:
		for _, r := range ti.Redeemers {
			ret = append(ret, script.NewScriptContextV1V2(wrap(ti), r.Key))
		}
	case script.TxInfoV3:
		for _, r := range ti.Redeemers {
			ret = append(
				ret,
				script.NewScriptContextV3(wrap(ti), r.Value, r.Key),
			)
		}
	}
	return ret
}

func TestCachedTxInfoContextBytesUnchanged(t *testing.T) {
	t.Parallel()
	checked := 0
	check := func(t *testing.T, txInfo script.TxInfo) {
		t.Helper()
		plain := contextsForEveryRedeemer(
			txInfo,
			func(ti script.TxInfo) script.TxInfo { return ti },
		)
		require.NotEmpty(t, plain)
		cached := script.NewCachedTxInfo(txInfo)
		got := contextsForEveryRedeemer(
			txInfo,
			func(script.TxInfo) script.TxInfo { return cached },
		)
		require.Len(t, got, len(plain))
		// Encode twice to cover both the first and the reused conversion.
		for pass := 0; pass < 2; pass++ {
			for i := range plain {
				want := encodeContext(t, plain[i])
				have := encodeContext(t, got[i])
				require.True(
					t,
					bytes.Equal(want, have),
					"redeemer %d pass %d: cached context differs",
					i,
					pass,
				)
				checked++
			}
		}
	}
	for _, def := range scriptContextV1TestDefs {
		t.Run("V1/"+def.name, func(t *testing.T) {
			txInfo, err := buildTxInfoV1(
				def.slotState, def.txHex, def.inputsHex, def.outputsHex,
			)
			require.NoError(t, err)
			check(t, txInfo)
		})
	}
	for _, def := range scriptContextV2TestDefs {
		t.Run("V2/"+def.name, func(t *testing.T) {
			txInfo, err := buildTxInfoV2(
				def.slotState, def.txHex, def.inputsHex, def.outputsHex,
			)
			require.NoError(t, err)
			check(t, txInfo)
		})
	}
	for _, def := range scriptContextV3TestDefs {
		t.Run("V3/"+def.name, func(t *testing.T) {
			txInfo, err := buildTxInfoV3(
				def.slotState, def.txHex, def.inputsHex, def.outputsHex,
			)
			require.NoError(t, err)
			check(t, txInfo)
		})
	}
}

func BenchmarkTxInfoContextPerRedeemer(b *testing.B) {
	def := scriptContextV2TestDefs[0]
	txInfo, err := buildTxInfoV2(
		def.slotState, def.txHex, def.inputsHex, def.outputsHex,
	)
	require.NoError(b, err)
	purposes := contextsForEveryRedeemer(
		txInfo,
		func(ti script.TxInfo) script.TxInfo { return ti },
	)
	require.NotEmpty(b, purposes)
	const redeemers = 8
	txInfoV2 := txInfo.(script.TxInfoV2)
	purpose := txInfoV2.Redeemers[0].Key
	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for range redeemers {
				_ = script.NewScriptContextV1V2(txInfo, purpose).ToPlutusData()
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cached := script.NewCachedTxInfo(txInfo)
			for range redeemers {
				_ = script.NewScriptContextV1V2(cached, purpose).ToPlutusData()
			}
		}
	})
}
