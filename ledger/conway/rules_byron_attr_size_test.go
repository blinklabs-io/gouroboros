// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package conway_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisteredByronOutputAttrSizeRule(t *testing.T) {
	t.Parallel()
	var rule common.UtxoValidationRuleFunc
	for _, d := range conway.UtxoValidationRuleDescriptors() {
		if d.Id == common.UtxoValidationRuleOutputBootAddrAttrsTooBig {
			rule = d.Validator
		}
	}
	require.NotNil(t, rule)
	run := func(t *testing.T, hdLen int) error {
		t.Helper()
		enc, err := cbor.Encode(bytes.Repeat([]byte{0xAB}, hdLen))
		require.NoError(t, err)
		data, err := cbor.Encode(map[uint8][]byte{1: enc})
		require.NoError(t, err)
		var attrs common.ByronAddressAttributes
		_, err = cbor.Decode(data, &attrs)
		require.NoError(t, err)
		addr, err := common.NewByronAddressFromParts(
			common.ByronAddressTypePubkey,
			bytes.Repeat([]byte{0x01}, 28),
			attrs,
		)
		require.NoError(t, err)
		tx := &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxOutputs: []babbage.BabbageTransactionOutput{
					{OutputAddress: addr},
				},
			},
		}
		return rule(
			tx,
			0,
			mockledger.NewLedgerStateBuilder().Build(),
			&conway.ConwayProtocolParameters{},
		)
	}
	assert.NoError(t, run(t, 60))
	assert.NoError(t, run(t, 64))
	err := run(t, 65)
	require.Error(t, err)
	assert.IsType(t, shelley.OutputBootAddrAttrsTooBigError{}, err)
}
