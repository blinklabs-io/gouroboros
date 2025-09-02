// Copyright 2025 Blink Labs Software
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

package test_ledger

import (
	"errors"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type MockLedgerState struct {
	MockNetworkId         uint
	MockUtxos             []common.Utxo
	MockStakeRegistration []common.StakeRegistrationCertificate
	MockPoolRegistration  []common.PoolRegistrationCertificate
}

func (ls MockLedgerState) NetworkId() uint {
	return ls.MockNetworkId
}

func (ls MockLedgerState) UtxoById(
	id common.TransactionInput,
) (common.Utxo, error) {
	for _, tmpUtxo := range ls.MockUtxos {
		if id.Index() != tmpUtxo.Id.Index() {
			continue
		}
		if string(id.Id().Bytes()) != string(tmpUtxo.Id.Id().Bytes()) {
			continue
		}
		return tmpUtxo, nil
	}
	return common.Utxo{}, errors.New("not found")
}

func (ls MockLedgerState) StakeRegistration(
	stakingKey []byte,
) ([]common.StakeRegistrationCertificate, error) {
	ret := []common.StakeRegistrationCertificate{}
	for _, cert := range ls.MockStakeRegistration {
		if string(
			common.Blake2b224(cert.StakeCredential.Credential).Bytes(),
		) == string(
			stakingKey,
		) {
			ret = append(ret, cert)
		}
	}
	return ret, nil
}

func (ls MockLedgerState) PoolRegistration(
	poolKeyHash []byte,
) ([]common.PoolRegistrationCertificate, error) {
	ret := []common.PoolRegistrationCertificate{}
	for _, cert := range ls.MockPoolRegistration {
		if string(
			common.Blake2b224(cert.Operator).Bytes(),
		) == string(
			poolKeyHash,
		) {
			ret = append(ret, cert)
		}
	}
	return ret, nil
}

func (ls MockLedgerState) SlotToTime(slot uint64) (time.Time, error) {
	// TODO
	return time.Now(), nil
}

func (ls MockLedgerState) TimeToSlot(t time.Time) (uint64, error) {
	// TODO
	return 0, nil
}
