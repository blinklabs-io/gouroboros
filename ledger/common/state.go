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

package common

import (
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// UtxoState defines the interface for querying the UTxO state
type UtxoState interface {
	UtxoById(TransactionInput) (Utxo, error)
}

// CertState defines the interface for querying the certificate state
type CertState interface {
	StakeRegistration([]byte) ([]StakeRegistrationCertificate, error)
	PoolRegistration([]byte) ([]PoolRegistrationCertificate, error)
}

// LedgerState defines the interface for querying the ledger
type LedgerState interface {
	UtxoState
	CertState
	NetworkId() uint
}

// TipState defines the interface for querying the current tip
type TipState interface {
	Tip() (pcommon.Tip, error)
}
