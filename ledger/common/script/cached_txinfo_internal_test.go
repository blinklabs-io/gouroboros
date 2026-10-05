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

package script

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

type countingTxInfo struct {
	calls *atomic.Int32
}

func (countingTxInfo) isTxInfo() {}

func (c countingTxInfo) ToPlutusData() data.PlutusData {
	c.calls.Add(1)
	return data.NewConstr(0)
}

func TestCachedTxInfoConvertsOncePerTransaction(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	cached := NewCachedTxInfo(countingTxInfo{calls: &calls})
	purpose := ScriptPurposeMinting{}
	const redeemers = 16
	var wg sync.WaitGroup
	for range redeemers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = NewScriptContextV1V2(cached, purpose).ToPlutusData()
			_ = NewScriptContextV3(cached, Redeemer{}, purpose).ToPlutusData()
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), calls.Load(), "TxInfo conversions")
	require.Same(t, cached, NewCachedTxInfo(cached))
}
