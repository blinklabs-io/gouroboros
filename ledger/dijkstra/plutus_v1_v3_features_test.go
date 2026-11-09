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

package dijkstra

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/stretchr/testify/require"
)

// The Plutus V1 to V3 contexts cannot represent the Dijkstra-only body fields,
// so cardano-ledger fails the translation when any is present.

type plutusV1V3Builder struct {
	name  string
	build func(tx common.Transaction) error
}

func plutusV1V3Builders() []plutusV1V3Builder {
	return []plutusV1V3Builder{
		{"V1", func(tx common.Transaction) error {
			_, err := script.NewTxInfoV1FromTransaction(
				nil, tx, nil, true, common.ProtocolVersionDijkstra,
			)
			return err
		}},
		{"V2", func(tx common.Transaction) error {
			_, err := script.NewTxInfoV2FromTransaction(
				nil, tx, nil, true, common.ProtocolVersionDijkstra,
			)
			return err
		}},
		{"V3", func(tx common.Transaction) error {
			_, err := script.NewTxInfoV3FromTransaction(
				nil, tx, nil, common.ProtocolVersionDijkstra,
			)
			return err
		}},
	}
}

func TestDijkstraPlutusV1ToV3RejectUnsupportedBodyFields(t *testing.T) {
	t.Parallel()
	account := cbor.NewByteString(bytes.Repeat([]byte{0x01}, 29))
	keyGuard := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.NewBlake2b224(bytes.Repeat([]byte{0x02}, 28)),
	}
	scriptGuard := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.NewBlake2b224(bytes.Repeat([]byte{0x03}, 28)),
	}
	cases := []struct {
		name    string
		body    DijkstraTransactionBody
		feature string
	}{
		{
			name: "direct deposits",
			body: DijkstraTransactionBody{
				TxDirectDeposits: DijkstraDirectDeposits{account: 50},
			},
			feature: "direct deposits",
		},
		{
			name: "account balance intervals",
			body: DijkstraTransactionBody{
				TxBalanceIntervals: DijkstraAccountBalanceIntervals{
					account: &DijkstraAccountBalanceInterval{},
				},
			},
			feature: "account balance intervals",
		},
		{
			name: "required top-level guards",
			body: DijkstraTransactionBody{
				TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
					&keyGuard: nil,
				},
			},
			feature: "required top-level guards",
		},
		{
			name: "script-hash guards",
			body: DijkstraTransactionBody{
				TxGuards: &DijkstraGuards{
					Credentials: []common.Credential{scriptGuard},
				},
			},
			feature: "script-hash guards",
		},
	}
	for _, tc := range cases {
		for _, builder := range plutusV1V3Builders() {
			t.Run(builder.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				tx := &DijkstraTransaction{Body: tc.body, TxIsValid: true}
				err := builder.build(tx)
				require.ErrorContains(t, err, tc.feature)
			})
		}
	}

	t.Run("allowed bodies translate", func(t *testing.T) {
		t.Parallel()
		for name, body := range map[string]DijkstraTransactionBody{
			"empty": {},
			"key-hash guard only": {
				TxGuards: &DijkstraGuards{
					Credentials: []common.Credential{keyGuard},
				},
			},
		} {
			for _, builder := range plutusV1V3Builders() {
				tx := &DijkstraTransaction{Body: body, TxIsValid: true}
				require.NoErrorf(
					t,
					builder.build(tx),
					"%s/%s",
					builder.name,
					name,
				)
			}
		}
	})
}

// Script evaluation wraps the transaction to hide guarding redeemers, so the
// check has to survive the wrapper; the V4 base must not apply it, because the
// V4 context carries these fields.
func TestDijkstraPlutusV1ToV3CheckThroughWrappers(t *testing.T) {
	t.Parallel()
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxDirectDeposits: DijkstraDirectDeposits{
				cbor.NewByteString(bytes.Repeat([]byte{0x01}, 29)): 50,
			},
		},
		TxIsValid: true,
	}
	wrapped := transactionWithoutGuardingRedeemers{Transaction: tx}
	for _, builder := range plutusV1V3Builders() {
		require.ErrorContainsf(
			t,
			builder.build(wrapped),
			"direct deposits",
			builder.name,
		)
	}

	base := plutusV4BaseTransaction{wrapped}
	_, err := script.NewTxInfoV3FromTransaction(
		nil, base, nil, common.ProtocolVersionDijkstra,
	)
	require.NoError(t, err)
}
