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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

type dijkstraV4AccountOrderCase struct {
	name string
	// accounts are listed in the order the reference AccountAddress Ord
	// yields: network, then script before key, then hash bytes.
	accounts []dijkstraV4OrderAccount
}

type dijkstraV4OrderAccount struct {
	addrType uint8
	network  uint8
	hash     byte
}

func (a dijkstraV4OrderAccount) key(t *testing.T) cbor.ByteString {
	t.Helper()
	address, err := common.NewAddressFromParts(
		a.addrType,
		a.network,
		nil,
		bytes.Repeat([]byte{a.hash}, common.AddressHashSize),
	)
	require.NoError(t, err)
	raw, err := address.Bytes()
	require.NoError(t, err)
	return cbor.NewByteString(raw)
}

func dijkstraV4AccountOrderCases() []dijkstraV4AccountOrderCase {
	const (
		key     = common.AddressTypeNoneKey
		script  = common.AddressTypeNoneScript
		testnet = common.AddressNetworkTestnet
		mainnet = common.AddressNetworkMainnet
	)
	return []dijkstraV4AccountOrderCase{
		{
			name: "same network equal hash script before key",
			accounts: []dijkstraV4OrderAccount{
				{script, testnet, 0x42},
				{key, testnet, 0x42},
			},
		},
		{
			name: "same network equal hash script before key mainnet",
			accounts: []dijkstraV4OrderAccount{
				{script, mainnet, 0x42},
				{key, mainnet, 0x42},
			},
		},
		{
			name: "network precedes credential type",
			accounts: []dijkstraV4OrderAccount{
				{script, testnet, 0x01},
				{key, testnet, 0x01},
				{script, mainnet, 0x01},
				{key, mainnet, 0x01},
			},
		},
		{
			name: "testnet script precedes mainnet key",
			accounts: []dijkstraV4OrderAccount{
				{script, testnet, 0xff},
				{key, mainnet, 0x00},
			},
		},
		{
			name: "script hash order within a network",
			accounts: []dijkstraV4OrderAccount{
				{script, testnet, 0x01},
				{script, testnet, 0x02},
				{key, testnet, 0x01},
				{key, testnet, 0x02},
			},
		},
	}
}

func TestDijkstraDirectDepositsV4FollowAccountAddressOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range dijkstraV4AccountOrderCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deposits := DijkstraDirectDeposits{}
			// Insert in reverse so map construction order cannot help.
			for i := len(tc.accounts) - 1; i >= 0; i-- {
				deposits[tc.accounts[i].key(t)] = uint64(i + 1)
			}
			body := &DijkstraTransactionBody{TxDirectDeposits: deposits}
			directDeposits, _, _, _, err := dijkstraBodyFieldsV4(body)
			require.NoError(t, err)
			dataMap := requireDijkstraV4Map(t, directDeposits, len(tc.accounts))
			for i, account := range tc.accounts {
				ctor := uint64(0)
				if account.addrType == common.AddressTypeNoneScript {
					ctor = 1
				}
				k := requireDijkstraV4Constr(t, dataMap.Pairs[i][0], ctor, 1)
				requireDijkstraV4Bytes(
					t,
					k.Fields[0],
					bytes.Repeat([]byte{account.hash}, common.AddressHashSize),
				)
				requireDijkstraV4Integer(t, dataMap.Pairs[i][1], int64(i+1))
			}
		})
	}
}

func TestDijkstraBalanceIntervalsV4FollowAccountAddressOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range dijkstraV4AccountOrderCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			intervals := DijkstraAccountBalanceIntervals{}
			for i := len(tc.accounts) - 1; i >= 0; i-- {
				exact := uint64(i + 1)
				intervals[tc.accounts[i].key(t)] = &DijkstraAccountBalanceInterval{
					Exact: &exact,
				}
			}
			body := &DijkstraTransactionBody{
				TxBalanceIntervals: intervals,
			}
			_, balanceIntervals, _, _, err := dijkstraBodyFieldsV4(body)
			require.NoError(t, err)
			dataMap := requireDijkstraV4Map(t, balanceIntervals, len(tc.accounts))
			for i, account := range tc.accounts {
				ctor := uint64(0)
				if account.addrType == common.AddressTypeNoneScript {
					ctor = 1
				}
				k := requireDijkstraV4Constr(t, dataMap.Pairs[i][0], ctor, 1)
				requireDijkstraV4Bytes(
					t,
					k.Fields[0],
					bytes.Repeat([]byte{account.hash}, common.AddressHashSize),
				)
				v := requireDijkstraV4Constr(t, dataMap.Pairs[i][1], 3, 1)
				requireDijkstraV4Integer(t, v.Fields[0], int64(i+1))
			}
		})
	}
}

// TestDijkstraAccountMapsV4AssocMapToList evaluates a V4 script that fails
// unless the script account precedes the equal-hash key account in the
// direct deposits and balance intervals TxInfo maps.
func TestDijkstraAccountMapsV4AssocMapToList(t *testing.T) {
	t.Parallel()
	hash := bytes.Repeat([]byte{0x42}, common.AddressHashSize)
	var credHash common.CredentialHash
	copy(credHash[:], hash)
	scriptCredential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: credHash,
	}
	keyCredential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: credHash,
	}
	scriptAccount := dijkstraV4OrderAccount{
		common.AddressTypeNoneScript, common.AddressNetworkTestnet, 0x42,
	}
	keyAccount := dijkstraV4OrderAccount{
		common.AddressTypeNoneKey, common.AddressNetworkTestnet, 0x42,
	}
	exact := uint64(1)
	interval := func() *DijkstraAccountBalanceInterval {
		return &DijkstraAccountBalanceInterval{Exact: &exact}
	}
	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV4,
		cek.ProtoVersion{Major: MinProtocolVersionDijkstra},
		dijkstraGuardTestPParams().CostModels[3],
	)
	require.NoError(t, err)
	for _, test := range []struct {
		name       string
		fieldIndex int
		body       DijkstraTransactionBody
	}{
		{
			name:       "direct deposits",
			fieldIndex: 8,
			body: DijkstraTransactionBody{
				TxDirectDeposits: DijkstraDirectDeposits{
					keyAccount.key(t):    2,
					scriptAccount.key(t): 1,
				},
			},
		},
		{
			name:       "balance intervals",
			fieldIndex: 9,
			body: DijkstraTransactionBody{
				TxBalanceIntervals: DijkstraAccountBalanceIntervals{
					keyAccount.key(t):    interval(),
					scriptAccount.key(t): interval(),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			observer := dijkstraTxInfoMapOrderScript(
				t,
				test.fieldIndex,
				scriptCredential,
				keyCredential,
			)
			level := dijkstraV4TestLevel(t, &DijkstraTransaction{Body: test.body})
			context, err := dijkstraPlutusV4Context(
				level,
				script.ScriptPurposeGuarding{Guard: scriptCredential},
				common.RedeemerKey{Tag: common.RedeemerTagGuarding},
				common.RedeemerValue{Data: common.Datum{
					Data: data.NewInteger(big.NewInt(9)),
				}},
			)
			require.NoError(t, err)
			_, err = observer.Evaluate(
				context,
				common.ExUnits{Memory: 10_000_000, Steps: 10_000_000},
				evalContext,
			)
			require.NoError(t, err)
		})
	}
}
