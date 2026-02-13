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

package localstatequery

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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

	// Conway governance queries (v8+)
	QueryTypeShelleyConstitution           = 23
	QueryTypeShelleyGovState               = 24
	QueryTypeShelleyDRepState              = 25
	QueryTypeShelleyDRepStakeDistr         = 26
	QueryTypeShelleyCommitteeMembersState  = 27
	QueryTypeShelleyFilteredVoteDelegatees = 28
	QueryTypeShelleySPOStakeDistr          = 30
	QueryTypeShelleyGetProposals           = 31
)

// simpleQueryBase is a helper type used for various query types
// to reduce repeat code
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
			// Conway governance queries
			QueryTypeShelleyConstitution:           &ShelleyConstitutionQuery{},
			QueryTypeShelleyGovState:               &ShelleyGovStateQuery{},
			QueryTypeShelleyDRepState:              &ShelleyDRepStateQuery{},
			QueryTypeShelleyDRepStakeDistr:         &ShelleyDRepStakeDistrQuery{},
			QueryTypeShelleyCommitteeMembersState:  &ShelleyCommitteeMembersStateQuery{},
			QueryTypeShelleyFilteredVoteDelegatees: &ShelleyFilteredVoteDelegateesQuery{},
			QueryTypeShelleySPOStakeDistr:          &ShelleySPOStakeDistrQuery{},
			QueryTypeShelleyGetProposals:           &ShelleyGetProposalsQuery{},
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
	Addrs []lcommon.Address
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
	// TODO: add params (#858)
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
	cbor.StructAsArray
	Type    int
	PoolIds cbor.SetType[ledger.PoolId]
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

func buildQuery(queryType int, params ...any) []any {
	ret := []any{queryType}
	if len(params) > 0 {
		ret = append(ret, params...)
	}
	return ret
}

func buildHardForkQuery(queryType int, params ...any) []any {
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
	params ...any,
) []any {
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
	cbor.StructAsArray
	Year        big.Int
	Day         int
	Picoseconds big.Int
}

func (s SystemStartResult) String() string {
	return fmt.Sprintf(
		"SystemStart %s %d %s",
		s.Year.String(),
		s.Day,
		s.Picoseconds.String(),
	)
}

func (s SystemStartResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Year        string `json:"year"`
		Day         int    `json:"day"`
		Picoseconds string `json:"picoseconds"`
	}{
		Year:        s.Year.String(),
		Day:         s.Day,
		Picoseconds: s.Picoseconds.String(),
	})
}

func (s *SystemStartResult) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Year        string `json:"year"`
		Day         int    `json:"day"`
		Picoseconds string `json:"picoseconds"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.Year.SetString(tmp.Year, 10)
	s.Day = tmp.Day
	s.Picoseconds.SetString(tmp.Picoseconds, 10)
	return nil
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
	cbor.StructAsArray
	Begin  eraHistoryResultBeginEnd
	End    eraHistoryResultBeginEnd
	Params eraHistoryResultParams
}

type eraHistoryResultBeginEnd struct {
	cbor.StructAsArray
	Timespan any
	SlotNo   int
	EpochNo  int
}

type eraHistoryResultParams struct {
	cbor.StructAsArray
	EpochLength       int
	SlotLength        int
	SlotsPerKESPeriod struct {
		cbor.StructAsArray
		Dummy1 int
		Value  int
		Dummy2 []int
	}
	Unknown int
}

// StakeCredential represents a stake credential as [tag, bytes]
// where tag indicates the credential type (0 for KeyHash, 1 for ScriptHash)
type StakeCredential struct {
	cbor.StructAsArray
	Tag   uint64
	Bytes ledger.Blake2b224
}

// NonMyopicMemberRewardsResult represents the non-myopic member rewards result
// The result is a map where each key is a stake credential
// and each value is a map of pool IDs to their reward amounts in lovelaces
type NonMyopicMemberRewardsResult map[StakeCredential]map[ledger.Blake2b224]uint64

type CurrentProtocolParamsResult interface {
	ledger.AlonzoProtocolParameters |
		ledger.BabbageProtocolParameters |
		ledger.ConwayProtocolParameters |
		ledger.ShelleyProtocolParameters |
		any
}

type ProposedProtocolParamsUpdatesResult map[lcommon.GenesisHash]lcommon.ProtocolParameterUpdate

type StakeDistributionResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]struct {
		cbor.StructAsArray
		StakeFraction *cbor.Rat
		VrfHash       ledger.Blake2b256
	}
}

type UTxOsResult struct {
	cbor.StructAsArray
	Results map[UtxoId]ledger.BabbageTransactionOutput
}

type (
	UTxOByAddressResult = UTxOsResult
	UTxOWholeResult     = UTxOsResult
)

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
		type tUtxoId UtxoId
		var tmp tUtxoId
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		*u = UtxoId(tmp)
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

// TODO (#863)
type DebugEpochStateResult any

// TODO (#858)
// rwdr: [flag bytestring] bytestring is the keyhash of the staking vkey
// flag: 0/1 (0=keyhash 1=scripthash)
// result: [[ delegation rewards] ]
// delegation: { * rwdr => poolid } poolid is a bytestring
// rewards: { * rwdr => int }
// Note: It seems to be a requirement to sort the reward addresses on the
// query. Scripthash addresses come first, then within a group the bytestring
// being a network order integer sort ascending.
type FilteredDelegationsAndRewardAccountsResult any

type GenesisConfigResult struct {
	cbor.StructAsArray
	Start             SystemStartResult
	NetworkMagic      int
	NetworkId         uint8
	ActiveSlotsCoeff  []any
	SecurityParam     int
	EpochLength       int
	SlotsPerKESPeriod int
	MaxKESEvolutions  int
	SlotLength        int
	UpdateQuorum      int
	MaxLovelaceSupply int64
	ProtocolParams    GenesisConfigResultProtocolParameters
	// This value contains maps with bytestring keys, which we can't parse yet
	GenDelegs cbor.RawMessage
	Unknown1  any
	Unknown2  any
}

type GenesisConfigResultProtocolParameters struct {
	cbor.StructAsArray
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
	ExtraEntropy          any
	ProtocolVersionMajor  int
	ProtocolVersionMinor  int
	MinUTxOValue          int
	MinPoolCost           int
}

// TODO (#864)
type DebugNewEpochStateResult any

// TODO (#865)
type DebugChainDepStateResult any

// TODO (#866)
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
type RewardProvenanceResult any

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

// TODO (#867)
type RewardInfoPoolsResult any

// PoolStateParams represents the pool registration parameters
// without the cert type
type PoolStateParams struct {
	cbor.StructAsArray
	Operator      ledger.Blake2b224
	VrfKeyHash    ledger.Blake2b256
	Pledge        uint64
	Cost          uint64
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

// PoolStateResult represents the pool state result
// The result is a 4-element array: [pstate, fstate, retiring, deposits]
// where pstate maps pool IDs to their registration parameters
type PoolStateResult struct {
	cbor.StructAsArray
	PState map[ledger.Blake2b224]*PoolStateParams
	FState map[ledger.Blake2b224]*PoolStateParams // Future pool parameters
	// Retiring contains pools scheduled to retire (epoch number)
	Retiring map[ledger.Blake2b224]uint64
	Deposits map[ledger.Blake2b224]uint64 // Pool deposits
}

// PoolStakeSnapshot represents the stake distribution for a pool
// across different snapshots
type PoolStakeSnapshot struct {
	cbor.StructAsArray
	StakeMark uint64 // Stake snapshot from mark
	StakeSet  uint64 // Stake snapshot from set
	StakeGo   uint64 // Stake snapshot from go
}

// StakeSnapshotsResult represents the stake snapshots result.
// The result is a 4-element array:
// [snapshots, total_stake_mark, total_stake_set, total_stake_go]
// where snapshots maps pool IDs to their stake distribution
// across mark/set/go snapshots
type StakeSnapshotsResult struct {
	cbor.StructAsArray
	PoolSnapshots  map[ledger.Blake2b224]*PoolStakeSnapshot
	TotalStakeMark uint64
	TotalStakeSet  uint64
	TotalStakeGo   uint64
}

// PoolDistrResult represents the pool distribution result.
// It contains a map of pool IDs to their stake distribution
// (fraction and VRF hash)
type PoolDistrResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]struct {
		cbor.StructAsArray
		StakeFraction *cbor.Rat
		VrfHash       ledger.Blake2b256
	}
}

// Conway governance query types

type ShelleyConstitutionQuery struct {
	simpleQueryBase
}

type ShelleyGovStateQuery struct {
	simpleQueryBase
}

type ShelleyDRepStateQuery struct {
	cbor.StructAsArray
	Type        int
	Credentials cbor.SetType[lcommon.Credential]
}

type ShelleyDRepStakeDistrQuery struct {
	cbor.StructAsArray
	Type  int
	DReps cbor.SetType[lcommon.Drep]
}

type ShelleyCommitteeMembersStateQuery struct {
	cbor.StructAsArray
	Type         int
	ColdCreds    cbor.SetType[lcommon.Credential]
	HotCreds     cbor.SetType[lcommon.Credential]
	MemberStatus cbor.SetType[int]
}

type ShelleyFilteredVoteDelegateesQuery struct {
	cbor.StructAsArray
	Type        int
	Credentials cbor.SetType[lcommon.Credential]
}

type ShelleySPOStakeDistrQuery struct {
	cbor.StructAsArray
	Type    int
	PoolIds cbor.SetType[ledger.PoolId]
}

type ShelleyGetProposalsQuery struct {
	simpleQueryBase
}

// Conway governance result types

// ConstitutionResult represents the constitution query result.
// The constitution contains an anchor (URL and hash) and an optional
// guardrails script hash
type ConstitutionResult struct {
	cbor.StructAsArray
	Anchor     lcommon.GovAnchor
	ScriptHash []byte // Optional guardrails script hash (nil if no guardrails)
}

// GovStateResult represents the full governance state.
// This includes proposals, committee, constitution, protocol parameters,
// and DRep pulsing state.
// The raw messages can be further decoded based on the era
type GovStateResult struct {
	cbor.StructAsArray
	Proposals        cbor.RawMessage // Complex nested structure
	Committee        cbor.RawMessage // StrictMaybe Committee
	Constitution     ConstitutionResult
	CurrentPParams   cbor.RawMessage // Era-specific protocol parameters
	PrevPParams      cbor.RawMessage // Previous era protocol parameters
	FuturePParams    cbor.RawMessage // Scheduled parameter changes
	DRepPulsingState cbor.RawMessage // DRep pulsing state
}

// DRepStateEntry represents the state of a single DRep
type DRepStateEntry struct {
	cbor.StructAsArray
	Expiry  uint64             // Epoch when DRep expires
	Anchor  *lcommon.GovAnchor // Optional metadata anchor
	Deposit uint64             // Deposit amount
}

// DRepStateResult represents the DRep state query result.
// The result is a map of stake credentials to DRep state entries.
type DRepStateResult map[StakeCredential]DRepStateEntry

// DRepStakeDistrResult represents the DRep stake distribution
// The result is returned as raw CBOR that maps DReps to stake amounts
type DRepStakeDistrResult cbor.RawMessage

// CommitteeMemberState represents the state of a committee member
type CommitteeMemberState struct {
	cbor.StructAsArray
	// Status: 0=Unrecognized, 1=Active, 2=Expired, 3=Resigned
	Status        int
	HotCredential *lcommon.Credential // Hot credential if authorized
	Expiry        uint64              // Term expiry epoch
	NextEpoch     *uint64             // Next epoch for cold key rotation
}

// CommitteeMembersStateResult represents the committee members state
// query result. Contains the committee members and voting threshold
// as raw CBOR
type CommitteeMembersStateResult struct {
	cbor.StructAsArray
	Members   cbor.RawMessage // Map of cold credentials to member state
	Threshold *cbor.Rat       // Voting threshold
}

// FilteredVoteDelegateesResult represents the vote delegatees
// for stake credentials.
// The result maps stake credentials to their DRep delegations
type FilteredVoteDelegateesResult map[StakeCredential]lcommon.Drep

// SPOStakeDistrResult represents the SPO stake distribution for governance
// Maps pool IDs to their governance voting power
type SPOStakeDistrResult struct {
	cbor.StructAsArray
	Results map[ledger.PoolId]uint64
}

// GovActionState represents the state of a governance action (proposal).
// Each entry includes the governance action ID, votes from committees/DReps/SPOs,
// the proposal procedure, and the epoch range during which it is active.
type GovActionState struct {
	cbor.StructAsArray
	Id                lcommon.GovActionId
	CommitteeVotes    map[StakeCredential]lcommon.Vote
	DRepVotes         map[StakeCredential]lcommon.Vote
	SPOVotes          map[ledger.Blake2b224]lcommon.Vote
	ProposalProcedure cbor.RawMessage // Conway proposal procedures are complex, keep as RawMessage
	ProposedIn        uint64
	ExpiresAfter      uint64
}

// ProposalsResult represents the result of a GetProposals query.
// It contains a list of governance action states for all active proposals.
type ProposalsResult []GovActionState
