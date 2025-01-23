// Copyright 2023 Blink Labs Software
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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
)

// Query types
const (
	QueryTypeBlock        = 0
	QueryTypeSystemStart  = 1
	QueryTypeChainBlockNo = 2
	QueryTypeChainPoint   = 3

	// Block query sub-types
	QueryTypeShelley  = 0
	QueryTypeHardFork = 2

	// Hard fork query sub-types
	QueryTypeHardForkEraHistory = 0
	QueryTypeHardForkCurrentEra = 1

	// Shelley query sub-types
	QueryTypeShelleyLedgerTip                           = 0
	QueryTypeShelleyEpochNo                             = 1
	QueryTypeShelleyNonMyopicMemberRewards              = 2
	QueryTypeShelleyCurrentProtocolParams               = 3
	QueryTypeShelleyProposedProtocolParamsUpdates       = 4
	QueryTypeShelleyStakeDistribution                   = 5
	QueryTypeShelleyUtxoByAddress                       = 6
	QueryTypeShelleyUtxoWhole                           = 7
	QueryTypeShelleyDebugEpochState                     = 8
	QueryTypeShelleyCbor                                = 9
	QueryTypeShelleyFilteredDelegationAndRewardAccounts = 10
	QueryTypeShelleyGenesisConfig                       = 11
	QueryTypeShelleyDebugNewEpochState                  = 12
	QueryTypeShelleyDebugChainDepState                  = 13
	QueryTypeShelleyRewardProvenance                    = 14
	QueryTypeShelleyUtxoByTxin                          = 15
	QueryTypeShelleyStakePools                          = 16
	QueryTypeShelleyStakePoolParams                     = 17
	QueryTypeShelleyRewardInfoPools                     = 18
	QueryTypeShelleyPoolState                           = 19
	QueryTypeShelleyStakeSnapshots                      = 20
	QueryTypeShelleyPoolDistr                           = 21
)

// simpleQueryBase is a helper type used for various query types to reduce repeat code
type simpleQueryBase struct {
	cbor.StructAsArray
	Type int
}

// QueryWrapper is used for decoding a query from CBOR
type QueryWrapper struct {
	cbor.DecodeStoreCbor
	Query any
}

func (q *QueryWrapper) UnmarshalCBOR(data []byte) error {
	// Store original CBOR
	q.SetCbor(data)
	// Decode query
	tmpQuery, err := decodeQuery(
		data,
		"",
		map[int]any{
			QueryTypeBlock:        &BlockQuery{},
			QueryTypeSystemStart:  &SystemStartQuery{},
			QueryTypeChainBlockNo: &ChainBlockNoQuery{},
			QueryTypeChainPoint:   &ChainPointQuery{},
		},
	)
	if err != nil {
		return err
	}
	q.Query = tmpQuery
	return nil
}

func (q *QueryWrapper) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(q.Query)
}

type BlockQuery struct {
	Query any
}

func (q *BlockQuery) MarshalCBOR() ([]byte, error) {
	tmpData := []any{
		QueryTypeBlock,
		q.Query,
	}
	return cbor.Encode(tmpData)
}

func (q *BlockQuery) UnmarshalCBOR(data []byte) error {
	// Unwrap
	tmpData := struct {
		cbor.StructAsArray
		Type     int
		SubQuery cbor.RawMessage
	}{}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	// Decode query
	tmpQuery, err := decodeQuery(
		tmpData.SubQuery,
		"Block",
		map[int]any{
			QueryTypeShelley:  &ShelleyQuery{},
			QueryTypeHardFork: &HardForkQuery{},
		},
	)
	if err != nil {
		return err
	}
	q.Query = tmpQuery
	return nil
}

type ShelleyQuery struct {
	Era   uint
	Query any
}

func (q *ShelleyQuery) MarshalCBOR() ([]byte, error) {
	tmpData := []any{
		QueryTypeShelley,
		[]any{
			q.Era,
			q.Query,
		},
	}
	return cbor.Encode(tmpData)
}

func (q *ShelleyQuery) UnmarshalCBOR(data []byte) error {
	// Unwrap
	tmpData := struct {
		cbor.StructAsArray
		Type  int
		Inner struct {
			cbor.StructAsArray
			Era      uint
			SubQuery cbor.RawMessage
		}
	}{}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	// Decode query
	tmpQuery, err := decodeQuery(
		tmpData.Inner.SubQuery,
		"Block",
		map[int]any{
			QueryTypeShelleyLedgerTip:                           &ShelleyLedgerTipQuery{},
			QueryTypeShelleyEpochNo:                             &ShelleyEpochNoQuery{},
			QueryTypeShelleyNonMyopicMemberRewards:              &ShelleyNonMyopicMemberRewardsQuery{},
			QueryTypeShelleyCurrentProtocolParams:               &ShelleyCurrentProtocolParamsQuery{},
			QueryTypeShelleyProposedProtocolParamsUpdates:       &ShelleyProposedProtocolParamsUpdatesQuery{},
			QueryTypeShelleyStakeDistribution:                   &ShelleyStakeDistributionQuery{},
			QueryTypeShelleyUtxoByAddress:                       &ShelleyUtxoByAddressQuery{},
			QueryTypeShelleyUtxoWhole:                           &ShelleyUtxoWholeQuery{},
			QueryTypeShelleyDebugEpochState:                     &ShelleyDebugEpochStateQuery{},
			QueryTypeShelleyCbor:                                &ShelleyCborQuery{},
			QueryTypeShelleyFilteredDelegationAndRewardAccounts: &ShelleyFilteredDelegationAndRewardAccountsQuery{},
			QueryTypeShelleyGenesisConfig:                       &ShelleyGenesisConfigQuery{},
			QueryTypeShelleyDebugNewEpochState:                  &ShelleyDebugNewEpochStateQuery{},
			QueryTypeShelleyDebugChainDepState:                  &ShelleyDebugChainDepStateQuery{},
			QueryTypeShelleyRewardProvenance:                    &ShelleyRewardProvenanceQuery{},
			QueryTypeShelleyUtxoByTxin:                          &ShelleyUtxoByTxinQuery{},
			QueryTypeShelleyStakePools:                          &ShelleyStakePoolsQuery{},
			QueryTypeShelleyStakePoolParams:                     &ShelleyStakePoolParamsQuery{},
			QueryTypeShelleyRewardInfoPools:                     &ShelleyRewardInfoPoolsQuery{},
			QueryTypeShelleyPoolState:                           &ShelleyPoolStateQuery{},
			QueryTypeShelleyStakeSnapshots:                      &ShelleyStakeSnapshotsQuery{},
			QueryTypeShelleyPoolDistr:                           &ShelleyPoolDistrQuery{},
		},
	)
	if err != nil {
		return err
	}
	q.Era = tmpData.Inner.Era
	q.Query = tmpQuery
	return nil
}

type HardForkQuery struct {
	Query any
}

func (q *HardForkQuery) MarshalCBOR() ([]byte, error) {
	tmpData := []any{
		QueryTypeHardFork,
		q.Query,
	}
	return cbor.Encode(tmpData)
}

func (q *HardForkQuery) UnmarshalCBOR(data []byte) error {
	// Unwrap
	tmpData := struct {
		cbor.StructAsArray
		Type     int
		SubQuery cbor.RawMessage
	}{}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	// Decode query
	tmpQuery, err := decodeQuery(
		tmpData.SubQuery,
		"Hard-fork",
		map[int]any{
			QueryTypeHardForkEraHistory: &HardForkEraHistoryQuery{},
			QueryTypeHardForkCurrentEra: &HardForkCurrentEraQuery{},
		},
	)
	if err != nil {
		return err
	}
	q.Query = tmpQuery
	return nil
}

type ShelleyLedgerTipQuery struct {
	simpleQueryBase
}

type ShelleyEpochNoQuery struct {
	simpleQueryBase
}

type ShelleyNonMyopicMemberRewardsQuery struct {
	simpleQueryBase
}

type ShelleyCurrentProtocolParamsQuery struct {
	simpleQueryBase
}

type ShelleyProposedProtocolParamsUpdatesQuery struct {
	simpleQueryBase
}

type ShelleyStakeDistributionQuery struct {
	simpleQueryBase
}

type ShelleyUtxoByAddressQuery struct {
	cbor.StructAsArray
	Type  int
	Addrs []ledger.Address
}

type ShelleyUtxoWholeQuery struct {
	simpleQueryBase
}

type ShelleyDebugEpochStateQuery struct {
	simpleQueryBase
}

type ShelleyCborQuery struct {
	simpleQueryBase
}

type ShelleyFilteredDelegationAndRewardAccountsQuery struct {
	simpleQueryBase
	// TODO: add params
}

type ShelleyGenesisConfigQuery struct {
	simpleQueryBase
}

type ShelleyDebugNewEpochStateQuery struct {
	simpleQueryBase
}

type ShelleyDebugChainDepStateQuery struct {
	simpleQueryBase
}

type ShelleyRewardProvenanceQuery struct {
	simpleQueryBase
}

type ShelleyUtxoByTxinQuery struct {
	cbor.StructAsArray
	Type  int
	TxIns []ledger.ShelleyTransactionInput
}

type ShelleyStakePoolsQuery struct {
	simpleQueryBase
}

type ShelleyStakePoolParamsQuery struct {
	simpleQueryBase
	// TODO: add params
}

type ShelleyRewardInfoPoolsQuery struct {
	simpleQueryBase
}

type ShelleyPoolStateQuery struct {
	simpleQueryBase
}

type ShelleyStakeSnapshotsQuery struct {
	simpleQueryBase
}

type ShelleyPoolDistrQuery struct {
	simpleQueryBase
}

func decodeQuery(
	data []byte,
	typeDesc string,
	queryTypes map[int]any,
) (any, error) {
	// Determine query type
	queryType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return nil, err
	}
	var tmpQuery any
	for typeId, queryObj := range queryTypes {
		if queryType == typeId {
			tmpQuery = queryObj
			break
		}
	}
	if tmpQuery == nil {
		errMsg := "unknown query type"
		if typeDesc != "" {
			errMsg = fmt.Sprintf("unknown %s query type", typeDesc)
		}
		return nil, fmt.Errorf("%s: %d", errMsg, queryType)
	}
	// Decode query
	if _, err := cbor.Decode(data, tmpQuery); err != nil {
		return nil, err
	}
	return tmpQuery, nil
}

func buildQuery(queryType int, params ...interface{}) []interface{} {
	ret := []interface{}{queryType}
	if len(params) > 0 {
		ret = append(ret, params...)
	}
	return ret
}

func buildHardForkQuery(queryType int, params ...interface{}) []interface{} {
	ret := buildQuery(
		QueryTypeBlock,
		buildQuery(
			QueryTypeHardFork,
			buildQuery(
				queryType,
				params...,
			),
		),
	)
	return ret
}

func buildShelleyQuery(
	era int,
	queryType int,
	params ...interface{},
) []interface{} {
	ret := buildQuery(
		QueryTypeBlock,
		buildQuery(
			QueryTypeShelley,
			buildQuery(
				era,
				buildQuery(
					queryType,
					params...,
				),
			),
		),
	)
	return ret
}

type SystemStartQuery struct {
	simpleQueryBase
}

type SystemStartResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Year        int
	Day         int
	Picoseconds uint64
}

type ChainBlockNoQuery struct {
	simpleQueryBase
}

type ChainPointQuery struct {
	simpleQueryBase
}

type HardForkCurrentEraQuery struct {
	simpleQueryBase
}

type HardForkEraHistoryQuery struct {
	simpleQueryBase
}

type EraHistoryResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_      struct{} `cbor:",toarray"`
	Begin  eraHistoryResultBeginEnd
	End    eraHistoryResultBeginEnd
	Params eraHistoryResultParams
}

type eraHistoryResultBeginEnd struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Timespan interface{}
	SlotNo   int
	EpochNo  int
}

type eraHistoryResultParams struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                 struct{} `cbor:",toarray"`
	EpochLength       int
	SlotLength        int
	SlotsPerKESPeriod struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_      struct{} `cbor:",toarray"`
		Dummy1 int
		Value  int
		Dummy2 []int
	}
	Unknown int
}

// TODO
/*
result	[{ *[0 int] => non_myopic_rewards }]	for each stake display reward
non_myopic_rewards	{ *poolid => int }	int is the amount of lovelaces each pool would reward
*/
type NonMyopicMemberRewardsResult interface{}

type CurrentProtocolParamsResult interface {
	ledger.AllegraProtocolParameters |
		ledger.AlonzoProtocolParameters |
		ledger.BabbageProtocolParameters |
		ledger.ConwayProtocolParameters |
		ledger.MaryProtocolParameters |
		ledger.ShelleyProtocolParameters |
		any
}

// TODO
type ProposedProtocolParamsUpdatesResult interface{}

type StakeDistributionResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]struct {
		cbor.StructAsArray
		StakeFraction *cbor.Rat
		VrfHash       ledger.Blake2b256
	}
}

type UTxOByAddressResult struct {
	cbor.StructAsArray
	Results map[UtxoId]ledger.BabbageTransactionOutput
}

type UtxoId struct {
	cbor.StructAsArray
	Hash      ledger.Blake2b256
	Idx       int
	DatumHash ledger.Blake2b256
}

func (u *UtxoId) UnmarshalCBOR(data []byte) error {
	listLen, err := cbor.ListLength(data)
	if err != nil {
		return err
	}
	switch listLen {
	case 2:
		var tmpData struct {
			cbor.StructAsArray
			Hash ledger.Blake2b256
			Idx  int
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		u.Hash = tmpData.Hash
		u.Idx = tmpData.Idx
	case 3:
		return cbor.DecodeGeneric(data, u)
	default:
		return fmt.Errorf("invalid list length: %d", listLen)
	}
	return nil
}

func (u *UtxoId) MarshalCBOR() ([]byte, error) {
	var tmpData []any
	if u.DatumHash == ledger.NewBlake2b256(nil) {
		tmpData = []any{
			u.Hash,
			u.Idx,
		}
	} else {
		tmpData = []any{
			u.Hash,
			u.Idx,
			u.DatumHash,
		}
	}
	return cbor.Encode(tmpData)
}

// TODO
/*
result	[{* utxo => value }]
*/
type UTxOWholeResult interface{}

// TODO
type DebugEpochStateResult interface{}

// TODO
/*
rwdr	[flag bytestring]	bytestring is the keyhash of the staking vkey
flag	0/1	0=keyhash 1=scripthash
result	[[ delegation rewards] ]
delegation	{ * rwdr => poolid }	poolid is a bytestring
rewards	{ * rwdr => int }
It seems to be a requirement to sort the reward addresses on the query. Scripthash addresses come first, then within a group the bytestring being a network order integer sort ascending.
*/
type FilteredDelegationsAndRewardAccountsResult interface{}

type GenesisConfigResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                 struct{} `cbor:",toarray"`
	Start             SystemStartResult
	NetworkMagic      int
	NetworkId         uint8
	ActiveSlotsCoeff  []interface{}
	SecurityParam     int
	EpochLength       int
	SlotsPerKESPeriod int
	MaxKESEvolutions  int
	SlotLength        int
	UpdateQuorum      int
	MaxLovelaceSupply int64
	ProtocolParams    struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_                     struct{} `cbor:",toarray"`
		MinFeeA               int
		MinFeeB               int
		MaxBlockBodySize      int
		MaxTxSize             int
		MaxBlockHeaderSize    int
		KeyDeposit            int
		PoolDeposit           int
		EMax                  int
		NOpt                  int
		A0                    []int
		Rho                   []int
		Tau                   []int
		DecentralizationParam []int
		ExtraEntropy          interface{}
		ProtocolVersionMajor  int
		ProtocolVersionMinor  int
		MinUTxOValue          int
		MinPoolCost           int
	}
	// This value contains maps with bytestring keys, which we can't parse yet
	GenDelegs cbor.RawMessage
	Unknown1  interface{}
	Unknown2  interface{}
}

// TODO
type DebugNewEpochStateResult interface{}

// TODO
type DebugChainDepStateResult interface{}

// TODO
/*
result	[ *Element ]	Expanded in order on the next rows.
Element	CDDL	Comment
epochLength
poolMints	{ *poolid => block-count }
maxLovelaceSupply
NA
NA
NA
?circulatingsupply?
total-blocks
?decentralization?	[num den]
?available block entries
success-rate	[num den]
NA
NA		??treasuryCut
activeStakeGo
nil
nil
*/
type RewardProvenanceResult interface{}

type UTxOByTxInResult struct {
	cbor.StructAsArray
	Results map[UtxoId]ledger.BabbageTransactionOutput
}

type StakePoolsResult struct {
	cbor.StructAsArray
	Results []ledger.PoolId
}

type StakePoolParamsResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]struct {
		cbor.StructAsArray
		Operator      ledger.Blake2b224
		VrfKeyHash    ledger.Blake2b256
		Pledge        uint
		FixedCost     uint
		Margin        *cbor.Rat
		RewardAccount ledger.Address
		PoolOwners    []ledger.Blake2b224
		Relays        []ledger.PoolRelay
		PoolMetadata  *struct {
			cbor.StructAsArray
			Url          string
			MetadataHash ledger.Blake2b256
		}
	}
}

// TODO
type RewardInfoPoolsResult interface{}

// TODO
type PoolStateResult interface{}

// TODO
type StakeSnapshotsResult interface{}

// TODO
type PoolDistrResult interface{}
