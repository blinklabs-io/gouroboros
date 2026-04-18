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

package common

import (
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type mockTransactionBody struct {
	TransactionBodyBase
	id              Blake2b256
	inputs          []TransactionInput
	outputs         []TransactionOutput
	referenceInputs []TransactionInput
	certificates    []Certificate
	fee             *big.Int
}

func (m *mockTransactionBody) Id() Blake2b256 {
	return m.id
}

func (m *mockTransactionBody) Inputs() []TransactionInput {
	return m.inputs
}

func (m *mockTransactionBody) Outputs() []TransactionOutput {
	return m.outputs
}

func (m *mockTransactionBody) ReferenceInputs() []TransactionInput {
	return m.referenceInputs
}

func (m *mockTransactionBody) Certificates() []Certificate {
	return m.certificates
}

func (m *mockTransactionBody) Fee() *big.Int {
	return m.fee
}

func (m *mockTransactionBody) ProtocolParameterUpdates() (
	uint64,
	map[Blake2b224]ProtocolParameterUpdate,
) {
	return 0, nil
}

type mockRpcInput struct {
	id  Blake2b256
	idx uint32
	rpc *utxorpc.TxInput
	err error
}

func (m *mockRpcInput) Id() Blake2b256 {
	return m.id
}

func (m *mockRpcInput) Index() uint32 {
	return m.idx
}

func (m *mockRpcInput) String() string {
	return m.id.String()
}

func (m *mockRpcInput) Utxorpc() (*utxorpc.TxInput, error) {
	return m.rpc, m.err
}

func (m *mockRpcInput) ToPlutusData() data.PlutusData {
	return nil
}

type mockRpcOutput struct {
	rpc *utxorpc.TxOutput
	err error
}

func (m *mockRpcOutput) Address() Address {
	return Address{}
}

func (m *mockRpcOutput) Amount() *big.Int {
	return big.NewInt(0)
}

func (m *mockRpcOutput) Assets() *MultiAsset[MultiAssetTypeOutput] {
	return nil
}

func (m *mockRpcOutput) Datum() *Datum {
	return nil
}

func (m *mockRpcOutput) DatumHash() *Blake2b256 {
	return nil
}

func (m *mockRpcOutput) Cbor() []byte {
	return nil
}

func (m *mockRpcOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	return m.rpc, m.err
}

func (m *mockRpcOutput) ScriptRef() Script {
	return nil
}

func (m *mockRpcOutput) ToPlutusData() data.PlutusData {
	return nil
}

func (m *mockRpcOutput) String() string {
	return ""
}

type mockRpcCertificate struct {
	rpc *utxorpc.Certificate
	err error
}

func (m *mockRpcCertificate) isCertificate() {}

func (m *mockRpcCertificate) Cbor() []byte {
	return nil
}

func (m *mockRpcCertificate) Utxorpc() (*utxorpc.Certificate, error) {
	return m.rpc, m.err
}

func (m *mockRpcCertificate) Type() uint {
	return 0
}

func TestTransactionBodyToUtxorpc(t *testing.T) {
	t.Run("converts_reference_inputs_and_certificates", func(t *testing.T) {
		id := Blake2b256{0xaa}
		input := &mockRpcInput{
			id:  id,
			idx: 1,
			rpc: &utxorpc.TxInput{TxHash: id.Bytes(), OutputIndex: 1},
		}
		output := &mockRpcOutput{
			rpc: &utxorpc.TxOutput{Coin: ToUtxorpcBigInt(42)},
		}
		refInput := &mockRpcInput{
			id:  Blake2b256{0xbb},
			idx: 2,
			rpc: &utxorpc.TxInput{TxHash: []byte{0xbb}, OutputIndex: 2},
		}
		cert := &mockRpcCertificate{
			rpc: &utxorpc.Certificate{},
		}
		body := &mockTransactionBody{
			id:              id,
			inputs:          []TransactionInput{input},
			outputs:         []TransactionOutput{output},
			referenceInputs: []TransactionInput{refInput},
			certificates:    []Certificate{cert},
			fee:             big.NewInt(99),
		}

		got, err := TransactionBodyToUtxorpc(body)
		if !assert.NoError(t, err) || !assert.NotNil(t, got) {
			return
		}
		if assert.Len(t, got.Inputs, 1) {
			assert.Equal(t, uint32(1), got.Inputs[0].OutputIndex)
		}
		if assert.Len(t, got.Outputs, 1) {
			assert.Equal(t, int64(42), got.Outputs[0].Coin.GetInt())
		}
		if assert.Len(t, got.ReferenceInputs, 1) {
			assert.Equal(t, uint32(2), got.ReferenceInputs[0].OutputIndex)
		}
		assert.Len(t, got.Certificates, 1)
		if assert.NotNil(t, got.Fee) {
			assert.Equal(t, int64(99), got.Fee.GetInt())
		}
		assert.Equal(t, id.Bytes(), got.Hash)
	})

	t.Run("propagates_input_conversion_errors", func(t *testing.T) {
		expectedErr := errors.New("input conversion failed")
		body := &mockTransactionBody{
			inputs: []TransactionInput{
				&mockRpcInput{err: expectedErr},
			},
			fee: big.NewInt(0),
		}

		_, err := TransactionBodyToUtxorpc(body)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("propagates_output_conversion_errors", func(t *testing.T) {
		expectedErr := errors.New("output conversion failed")
		body := &mockTransactionBody{
			outputs: []TransactionOutput{
				&mockRpcOutput{err: expectedErr},
			},
			fee: big.NewInt(0),
		}

		_, err := TransactionBodyToUtxorpc(body)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("propagates_reference_input_conversion_errors", func(t *testing.T) {
		expectedErr := errors.New("reference input conversion failed")
		body := &mockTransactionBody{
			referenceInputs: []TransactionInput{
				&mockRpcInput{err: expectedErr},
			},
			fee: big.NewInt(0),
		}

		_, err := TransactionBodyToUtxorpc(body)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("propagates_certificate_conversion_errors", func(t *testing.T) {
		expectedErr := errors.New("certificate conversion failed")
		body := &mockTransactionBody{
			certificates: []Certificate{
				&mockRpcCertificate{err: expectedErr},
			},
			fee: big.NewInt(0),
		}

		_, err := TransactionBodyToUtxorpc(body)
		assert.ErrorIs(t, err, expectedErr)
	})
}
