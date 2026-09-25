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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestPlutusV3ReferenceInputDisjointnessStartsAtPV11(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"0100000000000000000000000000000000000000000000000000000000000000",
		0,
	)
	tx := &protocolVersionTestTransaction{
		inputs:          []common.TransactionInput{input},
		referenceInputs: []common.TransactionInput{input},
	}
	require.NoError(t, script.ValidatePlutusV3ReferenceInputs(tx, common.ProtocolVersionPlomin))
	require.Error(t, script.ValidatePlutusV3ReferenceInputs(tx, common.ProtocolVersionVanRossem))
	tx.referenceInputs = []common.TransactionInput{
		shelley.NewShelleyTransactionInput(
			"0100000000000000000000000000000000000000000000000000000000000000",
			1,
		),
	}
	require.NoError(t, script.ValidatePlutusV3ReferenceInputs(tx, common.ProtocolVersionVanRossem))
}

type protocolVersionTestTransaction struct {
	common.Transaction
	inputs          []common.TransactionInput
	referenceInputs []common.TransactionInput
}

func (t *protocolVersionTestTransaction) Inputs() []common.TransactionInput {
	return t.inputs
}

func (t *protocolVersionTestTransaction) ReferenceInputs() []common.TransactionInput {
	return t.referenceInputs
}
