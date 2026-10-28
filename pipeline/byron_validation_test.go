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

package pipeline

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestValidatingPipelineAcceptsByronEpochBoundaryBlock(t *testing.T) {
	fixture, err := os.ReadFile(
		"../protocol/chainsync/testdata/" +
			"byron_ebb_testnet_" +
			"8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)
	require.NoError(t, err)
	rawBlock, err := hex.DecodeString(strings.TrimSpace(string(fixture)))
	require.NoError(t, err)

	var eta0Calls atomic.Uint64
	var chainContextCalls atomic.Uint64
	var applyCalls atomic.Uint64
	p := NewBlockPipeline(
		WithDecodeWorkers(1),
		WithValidateWorkers(1),
		WithBlockTypeResolver(func(context.Context, []byte) (uint, error) {
			return ledger.BlockTypeByronEbb, nil
		}),
		WithEta0Provider(func(uint64) (string, error) {
			eta0Calls.Add(1)
			return shelleyBlockEta0, nil
		}),
		WithSlotsPerKesPeriod(129600),
		WithVerifyConfig(common.VerifyConfig{}),
		WithVerifyConfigProvider(func(
			context.Context,
			*BlockItem,
		) (common.VerifyConfig, error) {
			return common.VerifyConfig{}, nil
		}),
		WithChainContextValidator(func(context.Context, *BlockItem) error {
			chainContextCalls.Add(1)
			return nil
		}),
		WithApplyFunc(func(*BlockItem) error {
			applyCalls.Add(1)
			return nil
		}),
	)
	require.NoError(t, p.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, p.Stop()) })

	require.NoError(t, p.Submit(
		t.Context(),
		ledger.BlockTypeByronEbb,
		rawBlock,
		createTestTip(0, 0),
	))
	require.NoError(t, p.Fence(t.Context()))
	result := <-p.Results()
	t.Cleanup(result.Release)
	require.True(t, result.IsValid())
	require.True(t, result.IsApplied())
	require.Zero(t, eta0Calls.Load())
	require.Equal(t, uint64(1), chainContextCalls.Load())
	require.Equal(t, uint64(1), applyCalls.Load())
}
