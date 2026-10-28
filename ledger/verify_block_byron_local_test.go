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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

func TestVerifyBlockLocalValidatesByronBodyProof(t *testing.T) {
	block, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeByronMain,
		testdata.MustDecodeHex(testdata.ByronBlockHex),
	)
	require.NoError(t, err)

	valid, vrfOutput, blockNumber, slot, err := ledger.VerifyBlockLocal(
		block,
		"",
		0,
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.Empty(t, vrfOutput)
	require.Equal(t, block.BlockNumber(), blockNumber)
	require.Equal(t, block.SlotNumber(), slot)

	byronBlock := block.(*byron.ByronMainBlock)
	byronBlock.Body.TxPayload = nil
	valid, _, _, _, err = ledger.VerifyBlockLocal(byronBlock, "", 0)
	require.ErrorIs(t, err, byron.ErrBodyProofMismatch)
	require.False(t, valid)
}
