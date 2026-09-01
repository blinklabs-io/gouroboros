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

package ledger

import (
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// The below are compatibility types and functions to keep existing code working
// after a refactor of the ledger package

// Hash types
type (
	Blake2b224 = common.Blake2b224
	Blake2b256 = common.Blake2b256
)

func NewBlake2b224(data []byte) Blake2b224 {
	return common.NewBlake2b224(data)
}

func NewBlake2b256(data []byte) Blake2b256 {
	return common.NewBlake2b256(data)
}

// Address
type (
	Address     = common.Address
	AddrKeyHash = common.AddrKeyHash
)

const (
	AddressTypeScriptNone = common.AddressTypeScriptNone
	AddressTypeKeyNone    = common.AddressTypeKeyNone
	AddressTypeKeyKey     = common.AddressTypeKeyKey
)

var (
	NewAddress          = common.NewAddress
	NewAddressFromParts = common.NewAddressFromParts
)

// Governance types
type (
	VotingProcedure   = common.VotingProcedure
	VotingProcedures  = common.VotingProcedures
	ProposalProcedure = common.ProposalProcedure
)

// Certificates
type (
	Certificate                 = common.Certificate
	CertificateWrapper          = common.CertificateWrapper
	PoolRetirementCertificate   = common.PoolRetirementCertificate
	PoolRegistrationCertificate = common.PoolRegistrationCertificate
	StakeDelegationCertificate  = common.StakeDelegationCertificate
)

// Other types
type IssuerVkey = common.IssuerVkey

// Pools
type (
	PoolRelay = common.PoolRelay
	PoolId    = common.PoolId
)

func NewPoolIdFromBech32(poolId string) (PoolId, error) {
	return common.NewPoolIdFromBech32(poolId)
}

// Assets
type (
	MultiAssetTypeMint   = common.MultiAssetTypeMint
	MultiAssetTypeOutput = common.MultiAssetTypeOutput
	AssetFingerprint     = common.AssetFingerprint
)

func NewAssetFingerprint(policyId []byte, assetName []byte) AssetFingerprint {
	return common.NewAssetFingerprint(policyId, assetName)
}
