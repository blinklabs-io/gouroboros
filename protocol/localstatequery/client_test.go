// Copyright 2024 Blink Labs Software
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

package localstatequery_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"go.uber.org/goleak"
)

var conversationHandshakeAcquire = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeAcquireVolatileTip,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgAcquired(),
		},
	},
}

var conversationCurrentEra = append(
	conversationHandshakeAcquire,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeQuery,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgResult([]byte{0x5}),
		},
	},
)

type testInnerFunc func(*testing.T, *ouroboros.Connection)

func runTest(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
		close(asyncErrChan)
	}()
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	// Async error handler
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		// We can't call t.Fatalf() from a different Goroutine, so we panic instead
		panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
	}()
	// Run test inner function
	innerFunc(t, oConn)
	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close Ouroboros connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Ouroboros object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestGetCurrentEra(t *testing.T) {
	runTest(
		t,
		conversationCurrentEra,
		func(t *testing.T, oConn *ouroboros.Connection) {
			// Run query
			currentEra, err := oConn.LocalStateQuery().Client.GetCurrentEra()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if currentEra != 5 {
				t.Fatalf(
					"did not receive expected result: got %d, expected %d",
					currentEra,
					5,
				)
			}
		},
	)
}

func TestGetChainPoint(t *testing.T) {
	expectedPoint := ocommon.NewPoint(123, []byte{0xa, 0xb, 0xc})
	cborData, err := cbor.Encode(expectedPoint)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			chainPoint, err := oConn.LocalStateQuery().Client.GetChainPoint()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(chainPoint, &expectedPoint) {
				t.Fatalf(
					"did not receive expected result\n  got:    %#v\n  wanted: %#v",
					chainPoint,
					expectedPoint,
				)
			}
		},
	)
}

func TestGetEpochNo(t *testing.T) {
	expectedEpochNo := 123456
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(
					// [123456]
					test.DecodeHexString("811a0001e240"),
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			epochNo, err := oConn.LocalStateQuery().Client.GetEpochNo()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if epochNo != expectedEpochNo {
				t.Fatalf(
					"did not receive expected result, got %d, wanted %d",
					epochNo,
					expectedEpochNo,
				)
			}
		},
	)
}

func TestGetUTxOByAddress(t *testing.T) {
	testAddress, err := ledger.NewAddress(
		"addr_test1vrk294czhxhglflvxla7vxj2cjz7wyrdpxl3fj0vych5wws77xuc7",
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedUtxoId := localstatequery.UtxoId{
		Hash: ledger.NewBlake2b256([]byte{0x1, 0x2}),
		Idx:  7,
	}
	expectedResult := localstatequery.UTxOByAddressResult{
		Results: map[localstatequery.UtxoId]ledger.BabbageTransactionOutput{
			expectedUtxoId: {
				OutputAddress: testAddress,
				OutputAmount: ledger.MaryTransactionOutputValue{
					Amount: 234567,
				},
			},
		},
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			utxos, err := oConn.LocalStateQuery().Client.GetUTxOByAddress(
				[]ledger.Address{testAddress},
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			// Set stored CBOR to nil to make comparison easier
			for k, v := range utxos.Results {
				v.SetCbor(nil)
				utxos.Results[k] = v
			}
			if !reflect.DeepEqual(utxos, &expectedResult) {
				t.Fatalf(
					"did not receive expected result\n got:    %#v\n  wanted: %#v",
					utxos,
					expectedResult,
				)
			}
		},
	)
}

func TestGenesisConfigJSON(t *testing.T) {
	genesisConfig := localstatequery.GenesisConfigResult{
		Start: localstatequery.SystemStartResult{
			Year:        *big.NewInt(2024),
			Day:         35,
			Picoseconds: *big.NewInt(1234567890123456),
		},
		NetworkMagic:      764824073,
		NetworkId:         1,
		ActiveSlotsCoeff:  []any{0.1, 0.2},
		SecurityParam:     2160,
		EpochLength:       432000,
		SlotsPerKESPeriod: 129600,
		MaxKESEvolutions:  62,
		SlotLength:        1,
		UpdateQuorum:      5,
		MaxLovelaceSupply: 45000000000000000,
		ProtocolParams: localstatequery.GenesisConfigResultProtocolParameters{
			MinFeeA:               44,
			MinFeeB:               155381,
			MaxBlockBodySize:      65536,
			MaxTxSize:             16384,
			MaxBlockHeaderSize:    1100,
			KeyDeposit:            2000000,
			PoolDeposit:           500000000,
			EMax:                  18,
			NOpt:                  500,
			A0:                    []int{0},
			Rho:                   []int{1},
			Tau:                   []int{2},
			DecentralizationParam: []int{3},
			ExtraEntropy:          nil,
			ProtocolVersionMajor:  2,
			ProtocolVersionMinor:  0,
			MinUTxOValue:          1000000,
			MinPoolCost:           340000000,
		},
		GenDelegs: []byte("mocked_cbor_data"),
		Unknown1:  nil,
		Unknown2:  nil,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(genesisConfig)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Unmarshal back to struct
	var result localstatequery.GenesisConfigResult
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	//Compare everything after unmarshalling

	if !reflect.DeepEqual(genesisConfig, result) {
		t.Errorf(
			"Mismatch after JSON marshalling/unmarshalling. Expected:\n%+v\nGot:\n%+v",
			genesisConfig,
			result,
		)
	} else {
		t.Logf("Successfully validated the GenesisConfigResult after JSON marshalling and unmarshalling.")
	}
}
