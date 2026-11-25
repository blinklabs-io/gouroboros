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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// The reference decodes a Byron transaction's inputs and outputs as
// NonEmpty lists, so neither list may be empty.
func TestVerifyBlockStateRejectsByronTransactionWithoutInputs(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{7},
		OutputIndex: 0,
	}
	_, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		0,
	)
	body := byron.ByronTransactionBody{
		TxOutputs: []byron.ByronTransactionOutput{
			{OutputAddress: address, OutputAmount: 0},
		},
		Attributes: cbor.RawMessage{0xa0},
	}
	body.SetCbor([]byte{0x80})
	tx := byron.ByronTransaction{Body: body}
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "no inputs")
}

func TestVerifyBlockStateRejectsByronTransactionWithoutOutputs(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{8},
		OutputIndex: 0,
	}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		0,
	)
	tx.Body.TxOutputs = nil
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "no outputs")
}
