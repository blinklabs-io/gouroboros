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

package shelley_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// {0: [[h'00*32', 0]], 1: [], 2: 0, 3: 0} -- a Shelley transaction body
	// carrying an explicit zero TTL under mandatory key 3.
	testShelleyBodyZeroTtlHex = "a40081825820000000000000000000000000000000000000000000000000000000000000000000018002000300"
	// The same body with key 3 omitted entirely.
	testShelleyBodyNoTtlHex = "a3008182582000000000000000000000000000000000000000000000000000000000000000000001800200"
)

func decodeShelleyBody(
	t *testing.T,
	bodyHex string,
) (*shelley.ShelleyTransactionBody, error) {
	t.Helper()
	raw, err := hex.DecodeString(bodyHex)
	require.NoError(t, err)
	var body shelley.ShelleyTransactionBody
	if _, err := cbor.Decode(raw, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// TestShelleyTransactionBodyRejectsMissingTtl pins key 3 as mandatory.
// cardano-ledger decodes the Shelley body with
// SparseKeyed "TxBody" basicShelleyTxBodyRaw boxBody
// [(0,"inputs"),(1,"outputs"),(2,"fee"),(3,"ttl")], whose final argument is
// the required-field list, so a body without key 3 fails to decode there.
func TestShelleyTransactionBodyRejectsMissingTtl(t *testing.T) {
	t.Parallel()
	_, err := decodeShelleyBody(t, testShelleyBodyNoTtlHex)
	require.Error(t, err)
}

// TestShelleyTransactionBodyKeepsPresentZeroTtl confirms an explicitly
// encoded zero TTL is retained as present rather than collapsing into the
// "absent" case a zero value would otherwise mean.
func TestShelleyTransactionBodyKeepsPresentZeroTtl(t *testing.T) {
	t.Parallel()
	body, err := decodeShelleyBody(t, testShelleyBodyZeroTtlHex)
	require.NoError(t, err)
	upperBound, present := common.TransactionValidityIntervalUpperBound(body)
	assert.Equal(t, uint64(0), upperBound)
	assert.True(t, present)
}

// TestShelleyTransactionBodyEncodesZeroTtl confirms mandatory key 3 survives
// a re-encode. cardano-ledger's txSparse emits Key 3 (To ttl) unconditionally,
// with no Omit wrapper, unlike keys 4 through 7.
func TestShelleyTransactionBodyEncodesZeroTtl(t *testing.T) {
	t.Parallel()
	body := shelley.ShelleyTransactionBody{TxFee: 1, Ttl: 0}
	encoded, err := cbor.Encode(&body)
	require.NoError(t, err)
	var fields map[int]cbor.RawMessage
	_, err = cbor.Decode(encoded, &fields)
	require.NoError(t, err)
	assert.Contains(t, fields, 3)
}

// TestUtxoValidateTimeToLiveRejectsPresentZeroTtl drives the production rule
// with a decoded body. cardano-ledger's validateTimeToLive is
// failureUnless (ttl >= slot) with no exemption for zero, so a present zero
// TTL expires at every slot above zero.
func TestUtxoValidateTimeToLiveRejectsPresentZeroTtl(t *testing.T) {
	t.Parallel()
	body, err := decodeShelleyBody(t, testShelleyBodyZeroTtlHex)
	require.NoError(t, err)
	tx := &shelley.ShelleyTransaction{Body: *body}

	// ttl >= slot holds only at slot 0.
	require.NoError(t, shelley.UtxoValidateTimeToLive(tx, 0, nil, nil))

	err = shelley.UtxoValidateTimeToLive(tx, 1, nil, nil)
	require.Error(t, err)
	assert.IsType(t, shelley.ExpiredUtxoError{}, err)
}
