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

package testdata

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// ShelleyVerifyConfig returns complete ledger context for the shared Shelley
// fixture so pipeline tests exercise validation without bypass flags.
func ShelleyVerifyConfig() (common.VerifyConfig, error) {
	var fixture TestBlock
	for _, block := range GetTestBlocks() {
		if block.Name == "Shelley" {
			fixture = block
			break
		}
	}
	if len(fixture.Cbor) == 0 {
		return common.VerifyConfig{}, errors.New("Shelley test block not found")
	}
	block, err := ledger.NewBlockFromCbor(
		fixture.BlockType,
		fixture.Cbor,
		common.VerifyConfig{},
	)
	if err != nil {
		return common.VerifyConfig{}, err
	}
	header, ok := block.Header().(*shelley.ShelleyBlockHeader)
	if !ok {
		return common.VerifyConfig{}, fmt.Errorf(
			"unexpected Shelley header type %T",
			block.Header(),
		)
	}
	vrfKeyHash := common.Blake2b256Hash(header.Body.VrfKey)
	var utxos []common.Utxo
	rewardAccounts := make(map[mockledger.RewardAccountKey]uint64)
	for _, tx := range block.Transactions() {
		var consumed uint64
		for _, output := range tx.Outputs() {
			consumed += output.Amount().Uint64()
		}
		if fee := tx.Fee(); fee != nil {
			consumed += fee.Uint64()
		}
		for address, withdrawal := range tx.Withdrawals() {
			if withdrawal == nil || withdrawal.Uint64() > consumed {
				return common.VerifyConfig{}, errors.New(
					"invalid Shelley fixture withdrawal",
				)
			}
			consumed -= withdrawal.Uint64()
			credential, err := address.RewardAccountCredential()
			if err != nil {
				return common.VerifyConfig{}, err
			}
			rewardAccounts[mockledger.NewRewardAccountKey(credential)] = withdrawal.Uint64()
		}
		witnesses := tx.Witnesses().Vkey()
		if len(witnesses) == 0 {
			return common.VerifyConfig{}, errors.New(
				"Shelley fixture transaction has no key witness",
			)
		}
		paymentHash := common.Blake2b224Hash(witnesses[0].Vkey)
		address, err := common.NewAddressFromParts(
			common.AddressTypeKeyNone,
			common.AddressNetworkMainnet,
			paymentHash[:],
			nil,
		)
		if err != nil {
			return common.VerifyConfig{}, err
		}
		for index, input := range tx.Inputs() {
			amount := uint64(0)
			if index == 0 {
				amount = consumed
			}
			utxos = append(utxos, common.Utxo{
				Id: input,
				Output: shelley.ShelleyTransactionOutput{
					OutputAddress: address,
					OutputAmount:  amount,
				},
			})
		}
	}
	stateBuilder := mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		WithUtxos(utxos).
		WithRewardAccountCredentialBalances(rewardAccounts).
		WithPoolCurrentState(func(poolKeyHash common.PoolKeyHash) (*common.PoolRegistrationCertificate, *uint64, error) {
			return &common.PoolRegistrationCertificate{
				Operator:   poolKeyHash,
				VrfKeyHash: vrfKeyHash,
			}, nil, nil
		})
	for _, tx := range block.Transactions() {
		for _, certificate := range tx.Certificates() {
			delegation, ok := certificate.(*common.StakeDelegationCertificate)
			if !ok {
				continue
			}
			if delegation.StakeCredential == nil ||
				delegation.StakeCredential.CredType != common.CredentialTypeAddrKeyHash {
				return common.VerifyConfig{}, errors.New(
					"unsupported Shelley fixture delegation credential",
				)
			}
			stateBuilder.WithStakeCredentialRegistered(
				delegation.StakeCredential.Credential,
				true,
			)
		}
	}
	return common.VerifyConfig{
		LedgerState: stateBuilder.Build(),
		ProtocolParameters: &shelley.ShelleyProtocolParameters{
			MaxBlockBodySize:   2_097_154,
			MaxTxSize:          2_097_154,
			MaxBlockHeaderSize: 65_536,
		},
	}, nil
}
