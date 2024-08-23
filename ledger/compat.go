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
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// The below are compatability types and functions to keep existing code working
// after a refactor of the ledger package

// Hash types
type Blake2b224 = common.Blake2b224
type Blake2b256 = common.Blake2b256

func NewBlake2b224(data []byte) Blake2b224 {
	return common.NewBlake2b224(data)
}

func NewBlake2b256(data []byte) Blake2b256 {
	return common.NewBlake2b256(data)
}

// Address
type Address = common.Address
type AddrKeyHash = common.AddrKeyHash

func NewAddress(addr string) (Address, error) {
	return common.NewAddress(addr)
}

// Governance types
type VotingProcedure = common.VotingProcedure
type VotingProcedures = common.VotingProcedures
type ProposalProcedure = common.ProposalProcedure

// Certificates
type Certificate = common.Certificate
type CertificateWrapper = common.CertificateWrapper
type PoolRetirementCertificate = common.PoolRetirementCertificate
type PoolRegistrationCertificate = common.PoolRegistrationCertificate
type StakeDelegationCertificate = common.StakeDelegationCertificate

// Other types
type IssuerVkey = common.IssuerVkey

// Pools
type PoolRelay = common.PoolRelay
type PoolId = common.PoolId

func NewPoolIdFromBech32(poolId string) (PoolId, error) {
	return common.NewPoolIdFromBech32(poolId)
}

// Assets
type AssetFingerprint = common.AssetFingerprint

func NewAssetFingerprint(policyId []byte, assetName []byte) AssetFingerprint {
	return common.NewAssetFingerprint(policyId, assetName)
}

// Byron types
type ByronEpochBoundaryBlock = byron.ByronEpochBoundaryBlock
type ByronMainBlock = byron.ByronMainBlock
type ByronEpochBounaryBlockHeader = byron.ByronEpochBoundaryBlockHeader
type ByronMainBlockHeader = byron.ByronMainBlockHeader
type ByronTransaction = byron.ByronTransaction
type ByronTransactionInput = byron.ByronTransactionInput
type ByronTransactionOutput = byron.ByronTransactionOutput

// Byron constants
const (
	EraIdByron           = byron.EraIdByron
	BlockTypeByronEbb    = byron.BlockTypeByronEbb
	BlockTypeByronMain   = byron.BlockTypeByronMain
	BlockHeaderTypeByron = byron.BlockHeaderTypeByron
	TxTypeByron          = byron.TxTypeByron
)

// Byron functions
var (
	NewByronEpochBoundaryBlockFromCbor       = byron.NewByronEpochBoundaryBlockFromCbor
	NewByronMainBlockFromCbor                = byron.NewByronMainBlockFromCbor
	NewByronEpochBoundaryBlockHeaderFromCbor = byron.NewByronEpochBoundaryBlockHeaderFromCbor
	NewByronMainBlockHeaderFromCbor          = byron.NewByronMainBlockHeaderFromCbor
	NewByronTransactionInput                 = byron.NewByronTransactionInput
	NewByronTransactionFromCbor              = byron.NewByronTransactionFromCbor
)

// Shelley types
type ShelleyBlock = shelley.ShelleyBlock
type ShelleyBlockHeader = shelley.ShelleyBlockHeader
type ShelleyTransaction = shelley.ShelleyTransaction
type ShelleyTransactionBody = shelley.ShelleyTransactionBody
type ShelleyTransactionInput = shelley.ShelleyTransactionInput
type ShelleyTransactionOutput = shelley.ShelleyTransactionOutput
type ShelleyTransactionWitnessSet = shelley.ShelleyTransactionWitnessSet
type ShelleyProtocolParameters = shelley.ShelleyProtocolParameters
type ShelleyProtocolParameterUpdate = shelley.ShelleyProtocolParameterUpdate

// Shelley constants
const (
	EraIdShelley           = shelley.EraIdShelley
	BlockTypeShelley       = shelley.BlockTypeShelley
	BlockHeaderTypeShelley = shelley.BlockHeaderTypeShelley
	TxTypeShelley          = shelley.TxTypeShelley
)

// Shelley functions
var (
	NewShelleyBlockFromCbor             = shelley.NewShelleyBlockFromCbor
	NewShelleyBlockHeaderFromCbor       = shelley.NewShelleyBlockHeaderFromCbor
	NewShelleyTransactionInput          = shelley.NewShelleyTransactionInput
	NewShelleyTransactionFromCbor       = shelley.NewShelleyTransactionFromCbor
	NewShelleyTransactionBodyFromCbor   = shelley.NewShelleyTransactionBodyFromCbor
	NewShelleyTransactionOutputFromCbor = shelley.NewShelleyTransactionOutputFromCbor
)

// Allegra types
type AllegraBlock = allegra.AllegraBlock
type AllegraTransaction = allegra.AllegraTransaction
type AllegraTransactionBody = allegra.AllegraTransactionBody
type AllegraProtocolParameters = allegra.AllegraProtocolParameters
type AllegraProtocolParameterUpdate = allegra.AllegraProtocolParameterUpdate

// Allegra constants
const (
	EraIdAllegra           = allegra.EraIdAllegra
	BlockTypeAllegra       = allegra.BlockTypeAllegra
	BlockHeaderTypeAllegra = allegra.BlockHeaderTypeAllegra
	TxTypeAllegra          = allegra.TxTypeAllegra
)

// Allegra functions
var (
	NewAllegraBlockFromCbor           = allegra.NewAllegraBlockFromCbor
	NewAllegraTransactionFromCbor     = allegra.NewAllegraTransactionFromCbor
	NewAllegraTransactionBodyFromCbor = allegra.NewAllegraTransactionBodyFromCbor
)

// Mary types
type MaryBlock = mary.MaryBlock
type MaryBlockHeader = mary.MaryBlockHeader
type MaryTransaction = mary.MaryTransaction
type MaryTransactionBody = mary.MaryTransactionBody
type MaryTransactionOutput = mary.MaryTransactionOutput
type MaryTransactionOutputValue = mary.MaryTransactionOutputValue
type MaryTransactionWitnessSet = mary.MaryTransactionWitnessSet
type MaryProtocolParameters = mary.MaryProtocolParameters
type MaryProtocolParameterUpdate = mary.MaryProtocolParameterUpdate

// Mary constants
const (
	EraIdMary           = mary.EraIdMary
	BlockTypeMary       = mary.BlockTypeMary
	BlockHeaderTypeMary = mary.BlockHeaderTypeMary
	TxTypeMary          = mary.TxTypeMary
)

// Mary functions
var (
	NewMaryBlockFromCbor             = mary.NewMaryBlockFromCbor
	NewMaryTransactionFromCbor       = mary.NewMaryTransactionFromCbor
	NewMaryTransactionBodyFromCbor   = mary.NewMaryTransactionBodyFromCbor
	NewMaryTransactionOutputFromCbor = mary.NewMaryTransactionOutputFromCbor
)

// Alonzo types
type AlonzoBlock = alonzo.AlonzoBlock
type AlonzoBlockHeader = alonzo.AlonzoBlockHeader
type AlonzoTransaction = alonzo.AlonzoTransaction
type AlonzoTransactionBody = alonzo.AlonzoTransactionBody
type AlonzoTransactionOutput = alonzo.AlonzoTransactionOutput
type AlonzoTransactionWitnessSet = alonzo.AlonzoTransactionWitnessSet
type AlonzoProtocolParameters = alonzo.AlonzoProtocolParameters
type AlonzoProtocolParameterUpdate = alonzo.AlonzoProtocolParameterUpdate

// Alonzo constants
const (
	EraIdAlonzo           = alonzo.EraIdAlonzo
	BlockTypeAlonzo       = alonzo.BlockTypeAlonzo
	BlockHeaderTypeAlonzo = alonzo.BlockHeaderTypeAlonzo
	TxTypeAlonzo          = alonzo.TxTypeAlonzo
)

// Alonzo functions
var (
	NewAlonzoBlockFromCbor             = alonzo.NewAlonzoBlockFromCbor
	NewAlonzoTransactionFromCbor       = alonzo.NewAlonzoTransactionFromCbor
	NewAlonzoTransactionBodyFromCbor   = alonzo.NewAlonzoTransactionBodyFromCbor
	NewAlonzoTransactionOutputFromCbor = alonzo.NewAlonzoTransactionOutputFromCbor
)

// Babbage types
type BabbageBlock = babbage.BabbageBlock
type BabbageBlockHeader = babbage.BabbageBlockHeader
type BabbageTransaction = babbage.BabbageTransaction
type BabbageTransactionBody = babbage.BabbageTransactionBody
type BabbageTransactionOutput = babbage.BabbageTransactionOutput
type BabbageTransactionWitnessSet = babbage.BabbageTransactionWitnessSet
type BabbageProtocolParameters = babbage.BabbageProtocolParameters
type BabbageProtocolParameterUpdate = babbage.BabbageProtocolParameterUpdate

// Babbage constants
const (
	EraIdBabbage           = babbage.EraIdBabbage
	BlockTypeBabbage       = babbage.BlockTypeBabbage
	BlockHeaderTypeBabbage = babbage.BlockHeaderTypeBabbage
	TxTypeBabbage          = babbage.TxTypeBabbage
)

// Babbage functions
var (
	NewBabbageBlockFromCbor             = babbage.NewBabbageBlockFromCbor
	NewBabbageBlockHeaderFromCbor       = babbage.NewBabbageBlockHeaderFromCbor
	NewBabbageTransactionFromCbor       = babbage.NewBabbageTransactionFromCbor
	NewBabbageTransactionBodyFromCbor   = babbage.NewBabbageTransactionBodyFromCbor
	NewBabbageTransactionOutputFromCbor = babbage.NewBabbageTransactionOutputFromCbor
)

// Conway types
type ConwayBlock = conway.ConwayBlock
type ConwayBlockHeader = conway.ConwayBlockHeader
type ConwayTransaction = conway.ConwayTransaction
type ConwayTransactionBody = conway.ConwayTransactionBody
type ConwayTransactionWitnessSet = conway.ConwayTransactionWitnessSet
type ConwayProtocolParameters = conway.ConwayProtocolParameters
type ConwayProtocolParameterUpdate = conway.ConwayProtocolParameterUpdate

// Conway constants
const (
	EraIdConway           = conway.EraIdConway
	BlockTypeConway       = conway.BlockTypeConway
	BlockHeaderTypeConway = conway.BlockHeaderTypeConway
	TxTypeConway          = conway.TxTypeConway
)

// Conway functions
var (
	NewConwayBlockFromCbor           = conway.NewConwayBlockFromCbor
	NewConwayBlockHeaderFromCbor     = conway.NewConwayBlockHeaderFromCbor
	NewConwayTransactionFromCbor     = conway.NewConwayTransactionFromCbor
	NewConwayTransactionBodyFromCbor = conway.NewConwayTransactionBodyFromCbor
)
