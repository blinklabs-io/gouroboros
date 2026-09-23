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
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

type dijkstraV4TestCredentialKey struct {
	cbor.StructAsArray
	Type uint
	Hash common.Blake2b224
}

func requireDijkstraV4Constr(
	t *testing.T,
	value data.PlutusData,
	tag uint64,
	fieldCount int,
) *data.Constr {
	t.Helper()
	constr, ok := value.(*data.Constr)
	require.True(t, ok, "expected constructor, got %T", value)
	require.Equal(t, new(big.Int).SetUint64(tag), constr.Tag)
	require.Len(t, constr.Fields, fieldCount)
	return constr
}

func requireDijkstraV4Integer(
	t *testing.T,
	value data.PlutusData,
	want int64,
) {
	t.Helper()
	integer, ok := value.(*data.Integer)
	require.True(t, ok, "expected integer, got %T", value)
	require.Equal(t, big.NewInt(want), integer.Inner)
}

func requireDijkstraV4Bytes(
	t *testing.T,
	value data.PlutusData,
	want []byte,
) {
	t.Helper()
	encoded, ok := value.(*data.ByteString)
	require.True(t, ok, "expected byte string, got %T", value)
	require.Equal(t, want, encoded.Inner)
}

func requireDijkstraV4List(
	t *testing.T,
	value data.PlutusData,
	length int,
) *data.List {
	t.Helper()
	list, ok := value.(*data.List)
	require.True(t, ok, "expected list, got %T", value)
	require.Len(t, list.Items, length)
	return list
}

func requireDijkstraV4Map(
	t *testing.T,
	value data.PlutusData,
	length int,
) *data.Map {
	t.Helper()
	dataMap, ok := value.(*data.Map)
	require.True(t, ok, "expected map, got %T", value)
	require.Len(t, dataMap.Pairs, length)
	return dataMap
}

func dijkstraV4TestLedgerState() common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithSlotToTime(func(slot uint64) (time.Time, error) {
			return time.UnixMilli(int64(slot)*1000 + 500), nil
		}).
		Build()
}

func dijkstraV4TestLevel(
	t *testing.T,
	tx *DijkstraTransaction,
) dijkstraScriptLevel {
	t.Helper()
	levels, _, err := dijkstraScriptLevels(tx, dijkstraV4TestLedgerState())
	require.NoError(t, err)
	require.Len(t, levels, 1)
	return levels[0]
}

func TestDijkstraWithdrawalsV4FollowCredentialOrder(t *testing.T) {
	hash := bytes.Repeat([]byte{0x42}, common.AddressHashSize)
	scriptAddress, err := common.NewAddressFromParts(
		common.AddressTypeNoneScript,
		common.AddressNetworkTestnet,
		nil,
		hash,
	)
	require.NoError(t, err)
	keyAddress, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		hash,
	)
	require.NoError(t, err)
	withdrawals := map[*common.Address]*big.Int{
		&scriptAddress: big.NewInt(1),
		&keyAddress:    big.NewInt(2),
	}

	withdrawalsData, err := dijkstraWithdrawalsV4(withdrawals)
	require.NoError(t, err)
	withdrawalMap := requireDijkstraV4Map(t, withdrawalsData, 2)
	first := requireDijkstraV4Constr(t, withdrawalMap.Pairs[0][0], 1, 1)
	requireDijkstraV4Bytes(t, first.Fields[0], hash)
	requireDijkstraV4Integer(t, withdrawalMap.Pairs[0][1], 1)
	second := requireDijkstraV4Constr(t, withdrawalMap.Pairs[1][0], 0, 1)
	requireDijkstraV4Bytes(t, second.Fields[0], hash)
	requireDijkstraV4Integer(t, withdrawalMap.Pairs[1][1], 2)
}

func TestDijkstraRewardPurposeUsesCredentialOrder(t *testing.T) {
	hash := bytes.Repeat([]byte{0x42}, common.AddressHashSize)
	scriptAddress, err := common.NewAddressFromParts(
		common.AddressTypeNoneScript,
		common.AddressNetworkTestnet,
		nil,
		hash,
	)
	require.NoError(t, err)
	keyAddress, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		hash,
	)
	require.NoError(t, err)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{
				&scriptAddress: 1,
				&keyAddress:    2,
			},
		},
		TxIsValid: true,
	}
	level := dijkstraV4TestLevel(t, tx)
	purposes := dijkstraRequiredScriptPurposes(level)
	require.Len(t, purposes, 1)
	require.Equal(t, common.RedeemerKey{
		Tag:   common.RedeemerTagReward,
		Index: 0,
	}, purposes[0].key)
}

func TestDijkstraTxInfoV4ReleasedSchema(t *testing.T) {
	var policy common.Blake2b224
	copy(policy[:], bytes.Repeat([]byte{0x31}, len(policy)))
	mint := common.NewMultiAsset(map[common.Blake2b224]map[cbor.ByteString]*big.Int{
		policy: {
			cbor.NewByteString([]byte("asset")): big.NewInt(23),
		},
	})
	guard := common.Credential{
		CredType: common.CredentialTypeScriptHash,
	}
	copy(guard.Credential[:], bytes.Repeat([]byte{0x42}, len(guard.Credential)))
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxFee:                   777,
			TxValidityIntervalStart: 2,
			TxMint:                  &mint,
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guard},
			},
			TxCurrentTreasuryValue: 33,
			TxDonation:             44,
		},
		TxIsValid: true,
	}
	tx.Body.SetValidityIntervalUpperBound(7)

	txInfoData, err := dijkstraTxInfoV4(dijkstraV4TestLevel(t, tx))
	require.NoError(t, err)
	txInfo := requireDijkstraV4Constr(t, txInfoData, 0, 19)

	requireDijkstraV4Bytes(t, txInfo.Fields[0], tx.Id().Bytes())
	requireDijkstraV4Constr(t, txInfo.Fields[1], 1, 0)
	requireDijkstraV4List(t, txInfo.Fields[2], 0)
	requireDijkstraV4List(t, txInfo.Fields[3], 0)
	requireDijkstraV4List(t, txInfo.Fields[4], 0)
	require.True(t, mint.ToPlutusData().Equal(txInfo.Fields[5]))
	requireDijkstraV4List(t, txInfo.Fields[6], 0)
	requireDijkstraV4Map(t, txInfo.Fields[7], 0)
	requireDijkstraV4Map(t, txInfo.Fields[8], 0)
	requireDijkstraV4Map(t, txInfo.Fields[9], 0)

	validRange := requireDijkstraV4Constr(t, txInfo.Fields[10], 0, 2)
	lower := requireDijkstraV4Constr(t, validRange.Fields[0], 0, 1)
	requireDijkstraV4Integer(t, lower.Fields[0], 2500)
	upper := requireDijkstraV4Constr(t, validRange.Fields[1], 0, 1)
	requireDijkstraV4Integer(t, upper.Fields[0], 7500)

	guards := requireDijkstraV4List(t, txInfo.Fields[11], 1)
	require.True(t, guard.ToPlutusData().Equal(guards.Items[0]))
	requireDijkstraV4Map(t, txInfo.Fields[12], 0)
	requireDijkstraV4Map(t, txInfo.Fields[13], 0)
	requireDijkstraV4Map(t, txInfo.Fields[14], 0)
	requireDijkstraV4Map(t, txInfo.Fields[15], 0)
	requireDijkstraV4List(t, txInfo.Fields[16], 0)
	treasury := requireDijkstraV4Constr(t, txInfo.Fields[17], 0, 1)
	requireDijkstraV4Integer(t, treasury.Fields[0], 33)
	requireDijkstraV4Integer(t, txInfo.Fields[18], 44)
}

func TestDijkstraTxInfoV4POSIXTimeRange(t *testing.T) {
	testCases := []struct {
		name          string
		start         uint64
		upper         uint64
		explicitLower bool
		hasUpper      bool
		wantLower     *int64
		wantUpper     *int64
	}{
		{name: "unbounded"},
		{name: "lower", start: 3, wantLower: new(int64)},
		{
			name:          "explicit zero lower",
			explicitLower: true,
			wantLower:     new(int64),
		},
		{name: "upper", upper: 9, hasUpper: true, wantUpper: new(int64)},
		{
			name:      "explicit zero upper",
			hasUpper:  true,
			wantUpper: new(int64),
		},
		{
			name:      "bounded",
			start:     4,
			upper:     11,
			hasUpper:  true,
			wantLower: new(int64),
			wantUpper: new(int64),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tx := &DijkstraTransaction{TxIsValid: true}
			tx.Body.TxValidityIntervalStart = testCase.start
			if testCase.explicitLower {
				bodyCbor, err := cbor.Encode(map[uint]uint64{8: 0})
				require.NoError(t, err)
				tx.Body.SetCbor(bodyCbor)
			}
			if testCase.hasUpper {
				tx.Body.SetValidityIntervalUpperBound(testCase.upper)
			}
			if testCase.wantLower != nil {
				*testCase.wantLower = int64(testCase.start)*1000 + 500
			}
			if testCase.wantUpper != nil {
				*testCase.wantUpper = int64(testCase.upper)*1000 + 500
			}

			txInfoData, err := dijkstraTxInfoV4(dijkstraV4TestLevel(t, tx))
			require.NoError(t, err)
			txInfo := requireDijkstraV4Constr(t, txInfoData, 0, 19)
			validRange := requireDijkstraV4Constr(t, txInfo.Fields[10], 0, 2)
			for idx, want := range []*int64{
				testCase.wantLower,
				testCase.wantUpper,
			} {
				if want == nil {
					requireDijkstraV4Constr(t, validRange.Fields[idx], 1, 0)
					continue
				}
				bound := requireDijkstraV4Constr(
					t,
					validRange.Fields[idx],
					0,
					1,
				)
				requireDijkstraV4Integer(t, bound.Fields[0], *want)
			}
		})
	}
}

func TestDijkstraBodyFieldsV4AccountBalanceIntervals(t *testing.T) {
	var hash1, hash2 common.Blake2b224
	hash1[0] = 1
	hash2[0] = 2
	cred1 := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash1,
	}
	cred2 := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash2,
	}
	lower := uint64(10)
	upper := uint64(20)
	exact := uint64(30)
	body := &DijkstraTransactionBody{
		TxBalanceIntervals: DijkstraAccountBalanceIntervals{
			// Inserted out of sorted order to confirm the output is sorted
			// by credential rather than by map iteration order.
			&cred2: {Exact: &exact},
			&cred1: {LowerBound: &lower, UpperBound: &upper},
		},
	}
	_, balanceIntervals, _, _, err := dijkstraBodyFieldsV4(body)
	require.NoError(t, err)
	dataMap := requireDijkstraV4Map(t, balanceIntervals, 2)
	firstKey := requireDijkstraV4Constr(t, dataMap.Pairs[0][0], 0, 1)
	requireDijkstraV4Bytes(t, firstKey.Fields[0], cred1.Credential.Bytes())
	both := requireDijkstraV4Constr(t, dataMap.Pairs[0][1], 2, 2)
	requireDijkstraV4Integer(t, both.Fields[0], 10)
	requireDijkstraV4Integer(t, both.Fields[1], 20)
	secondKey := requireDijkstraV4Constr(t, dataMap.Pairs[1][0], 0, 1)
	requireDijkstraV4Bytes(t, secondKey.Fields[0], cred2.Credential.Bytes())
	exactField := requireDijkstraV4Constr(t, dataMap.Pairs[1][1], 3, 1)
	requireDijkstraV4Integer(t, exactField.Fields[0], 30)
}

func TestDijkstraBodyFieldsV4AccountBalanceIntervalBoundShapes(t *testing.T) {
	guard := testGuardCredential()
	lower := uint64(7)
	upper := uint64(9)
	tests := []struct {
		name     string
		interval *DijkstraAccountBalanceInterval
		wantTag  uint64
	}{
		{
			name:     "lower only",
			interval: &DijkstraAccountBalanceInterval{LowerBound: &lower},
			wantTag:  0,
		},
		{
			name:     "upper only",
			interval: &DijkstraAccountBalanceInterval{UpperBound: &upper},
			wantTag:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &DijkstraSubTransactionBody{
				TxAccountBalanceIntervals: DijkstraAccountBalanceIntervals{
					&guard: test.interval,
				},
			}
			_, balanceIntervals, _, _, err := dijkstraBodyFieldsV4(body)
			require.NoError(t, err)
			dataMap := requireDijkstraV4Map(t, balanceIntervals, 1)
			requireDijkstraV4Constr(t, dataMap.Pairs[0][1], test.wantTag, 1)
		})
	}
}

func TestDijkstraBodyFieldsV4RequiredTopLevelGuards(t *testing.T) {
	guard := testGuardCredential()
	tests := []struct {
		name  string
		datum *common.Datum
	}{
		{name: "no datum"},
		{
			name:  "with datum",
			datum: &common.Datum{Data: data.NewInteger(big.NewInt(7))},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &DijkstraSubTransactionBody{
				TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
					&guard: test.datum,
				},
			}
			_, _, _, requiredGuards, err := dijkstraBodyFieldsV4(body)
			require.NoError(t, err)
			dataMap := requireDijkstraV4Map(t, requiredGuards, 1)
			if test.datum == nil {
				requireDijkstraV4Constr(t, dataMap.Pairs[0][1], 1, 0)
				return
			}
			present := requireDijkstraV4Constr(t, dataMap.Pairs[0][1], 0, 1)
			requireDijkstraV4Integer(t, present.Fields[0], 7)
		})
	}
	t.Run("transaction body", func(t *testing.T) {
		body := &DijkstraTransactionBody{
			TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
				&guard: {Data: data.NewInteger(big.NewInt(9))},
			},
		}
		_, _, _, requiredGuards, err := dijkstraBodyFieldsV4(body)
		require.NoError(t, err)
		dataMap := requireDijkstraV4Map(t, requiredGuards, 1)
		present := requireDijkstraV4Constr(t, dataMap.Pairs[0][1], 0, 1)
		requireDijkstraV4Integer(t, present.Fields[0], 9)
	})
}

// TestDijkstraBodyFieldsV4RejectsMalformedDirectlyConstructedMaps proves
// dijkstraBodyFieldsV4 validates a directly constructed
// DijkstraAccountBalanceIntervals/DijkstraRequiredTopLevelGuards rather than
// panicking or silently emitting invalid Plutus data for it. Decoding a
// transaction from CBOR can never produce these states (UnmarshalCBOR
// already rejects them), but nothing stops a caller from building one of
// these exported map types by hand.
func TestDijkstraBodyFieldsV4RejectsMalformedDirectlyConstructedMaps(t *testing.T) {
	guard := testGuardCredential()
	unsupported := guard
	unsupported.CredType = 2
	lower := uint64(1)

	t.Run("balance intervals: unsupported credential type", func(t *testing.T) {
		body := &DijkstraTransactionBody{
			TxBalanceIntervals: DijkstraAccountBalanceIntervals{
				&unsupported: {LowerBound: &lower},
			},
		}
		_, _, _, _, err := dijkstraBodyFieldsV4(body)
		require.ErrorContains(t, err, "unsupported credential type")
	})

	t.Run("balance intervals: nil interval", func(t *testing.T) {
		body := &DijkstraTransactionBody{
			TxBalanceIntervals: DijkstraAccountBalanceIntervals{&guard: nil},
		}
		_, _, _, _, err := dijkstraBodyFieldsV4(body)
		require.ErrorContains(t, err, "nil interval")
	})

	t.Run("balance intervals: all-nil bounds", func(t *testing.T) {
		body := &DijkstraTransactionBody{
			TxBalanceIntervals: DijkstraAccountBalanceIntervals{
				&guard: {},
			},
		}
		_, _, _, _, err := dijkstraBodyFieldsV4(body)
		require.ErrorContains(t, err, "requires a lower or upper bound")
	})

	t.Run("required top-level guards: unsupported credential type", func(t *testing.T) {
		body := &DijkstraSubTransactionBody{
			TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
				&unsupported: nil,
			},
		}
		_, _, _, _, err := dijkstraBodyFieldsV4(body)
		require.ErrorContains(t, err, "unsupported credential type")
	})

	t.Run("required top-level guards: datum missing Plutus data", func(t *testing.T) {
		body := &DijkstraSubTransactionBody{
			TxRequiredTopLevelGuards: DijkstraRequiredTopLevelGuards{
				&guard: {},
			},
		}
		_, _, _, _, err := dijkstraBodyFieldsV4(body)
		require.ErrorContains(t, err, "missing Plutus data")
	})
}

func TestDijkstraPlutusV4GuardingUsesCurrentReferenceShape(t *testing.T) {
	guard := common.Credential{
		CredType: common.CredentialTypeScriptHash,
	}
	copy(guard.Credential[:], bytes.Repeat([]byte{0x51}, len(guard.Credential)))
	otherGuard := common.Credential{
		CredType: common.CredentialTypeScriptHash,
	}
	copy(
		otherGuard.Credential[:],
		bytes.Repeat([]byte{0x52}, len(otherGuard.Credential)),
	)
	requiredGuards := DijkstraRequiredTopLevelGuards{
		&guard:      &common.Datum{Data: data.NewInteger(big.NewInt(123))},
		&otherGuard: &common.Datum{Data: data.NewInteger(big.NewInt(456))},
	}
	var policy common.Blake2b224
	copy(policy[:], bytes.Repeat([]byte{0x53}, len(policy)))
	subMint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			policy: {cbor.NewByteString([]byte("shared")): big.NewInt(5)},
		},
	)
	topMint := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			policy: {cbor.NewByteString([]byte("shared")): big.NewInt(-3)},
		},
	)
	subBody := DijkstraSubTransactionBody{
		TxValidityIntervalStart: 3,
		TxMint:                  &subMint,
		TxGuards: &DijkstraGuards{
			Credentials: []common.Credential{guard},
		},
		TxRequiredTopLevelGuards: requiredGuards,
		TxDonation:               5,
	}
	subBody.SetValidityIntervalUpperBound(8)
	subBody2 := subBody
	subBody2.TxDonation = 6
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxValidityIntervalStart: 2,
			TxMint:                  &topMint,
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guard},
			},
			TxDonation: 7,
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{
					{Body: subBody},
					{Body: subBody2},
				},
				false,
			),
		},
		TxIsValid: true,
	}
	tx.Body.SetValidityIntervalUpperBound(10)
	levels, _, err := dijkstraScriptLevels(tx, dijkstraV4TestLedgerState())
	require.NoError(t, err)
	require.Len(t, levels, 3)
	purpose := script.ScriptPurposeGuarding{Guard: guard}
	key := common.RedeemerKey{Tag: common.RedeemerTagGuarding, Index: 0}
	redeemer := common.RedeemerValue{
		Data: common.Datum{Data: data.NewInteger(big.NewInt(91))},
	}

	topContextData, err := dijkstraPlutusV4Context(
		levels[1],
		purpose,
		key,
		redeemer,
	)
	require.NoError(t, err)
	topContext := requireDijkstraV4Constr(t, topContextData, 0, 4)
	requireDijkstraV4Constr(t, topContext.Fields[0], 0, 19)
	requireDijkstraV4Integer(t, topContext.Fields[1], 91)
	requireDijkstraV4Bytes(t, topContext.Fields[3], purpose.ScriptHash().Bytes())
	topScriptInfo := requireDijkstraV4Constr(t, topContext.Fields[2], 6, 2)
	requireDijkstraV4Integer(t, topScriptInfo.Fields[0], 0)
	requireDijkstraV4Constr(t, topScriptInfo.Fields[1], 1, 0)
	topTxInfo := requireDijkstraV4Constr(t, topContext.Fields[0], 0, 19)
	requireDijkstraV4Constr(t, topTxInfo.Fields[1], 1, 0)

	for _, level := range levels[:2] {
		subContextData, err := dijkstraPlutusV4Context(
			level,
			purpose,
			key,
			redeemer,
		)
		require.NoError(t, err)
		subContext := requireDijkstraV4Constr(t, subContextData, 0, 4)
		subTxInfo := requireDijkstraV4Constr(t, subContext.Fields[0], 0, 19)
		requireDijkstraV4Constr(t, subTxInfo.Fields[1], 1, 0)
		subScriptInfo := requireDijkstraV4Constr(t, subContext.Fields[2], 6, 2)
		requireDijkstraV4Constr(t, subScriptInfo.Fields[1], 1, 0)
	}
}

func TestDijkstraAddressV4BasePaymentCredentials(t *testing.T) {
	paymentHash := bytes.Repeat([]byte{0x61}, common.AddressHashSize)
	stakingHash := bytes.Repeat([]byte{0x72}, common.AddressHashSize)
	testCases := []struct {
		name        string
		addressType uint8
		paymentTag  uint64
		stakingTag  uint64
	}{
		{name: "key-key", addressType: common.AddressTypeKeyKey},
		{
			name:        "script-key",
			addressType: common.AddressTypeScriptKey,
			paymentTag:  1,
		},
		{
			name:        "key-script",
			addressType: common.AddressTypeKeyScript,
			stakingTag:  1,
		},
		{
			name:        "script-script",
			addressType: common.AddressTypeScriptScript,
			paymentTag:  1,
			stakingTag:  1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			address, err := common.NewAddressFromParts(
				testCase.addressType,
				common.AddressNetworkMainnet,
				paymentHash,
				stakingHash,
			)
			require.NoError(t, err)
			encoded, err := dijkstraAddressV4(address)
			require.NoError(t, err)
			addressData := requireDijkstraV4Constr(t, encoded, 0, 2)
			payment := requireDijkstraV4Constr(
				t,
				addressData.Fields[0],
				testCase.paymentTag,
				1,
			)
			requireDijkstraV4Bytes(t, payment.Fields[0], paymentHash)
			stakeOption := requireDijkstraV4Constr(t, addressData.Fields[1], 0, 1)
			stake := requireDijkstraV4Constr(
				t,
				stakeOption.Fields[0],
				testCase.stakingTag,
				1,
			)
			requireDijkstraV4Bytes(t, stake.Fields[0], stakingHash)
		})
	}
}
