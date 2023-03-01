package localstatequery

import (
	"github.com/cloudstruct/go-ouroboros-network/cbor"
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

func buildShelleyQuery(era int, queryType int, params ...interface{}) []interface{} {
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

type SystemStartResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Year        int
	Day         int
	Picoseconds uint64
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
		Dummy1 interface{}
		Value  int
		Dummy2 interface{}
	}
}

// TODO
type NonMyopicMemberRewardsResult interface{}

type CurrentProtocolParamsResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_                  struct{} `cbor:",toarray"`
	MinFeeA            int
	MinFeeB            int
	MaxBlockBodySize   int
	MaxTxSize          int
	MaxBlockHeaderSize int
	KeyDeposit         int
	PoolDeposit        int
	EMax               int
	NOpt               int
	A0                 []int
	Rho                []int
	Tau                []int
	// This field no longer exists in Babbage, but we're keeping this here for reference
	// unless we need to support querying a node still on an older era
	//DecentralizationParam  []int
	ProtocolVersionMajor   int
	ProtocolVersionMinor   int
	MinPoolCost            int
	Unknown                interface{}
	CostModels             interface{}
	ExecutionUnitPrices    interface{} // [priceMemory priceSteps]	both elements are fractions
	MaxTxExecutionUnits    []uint
	MaxBlockExecutionUnits []uint
	MaxValueSize           int
	CollateralPercentage   int
}

// TODO
type ProposedProtocolParamsUpdatesResult interface{}
type StakeDistributionResult interface{}
type UTxOByAddressResult interface{}
type UTxOWholeResult interface{}
type DebugEpochStateResult interface{}
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
type DebugChainDepStateResult interface{}
type RewardProvenanceResult interface{}
type UTxOByTxInResult interface{}
type StakePoolsResult interface{}
type StakePoolParamsResult interface{}
type RewardInfoPoolsResult interface{}
type PoolStateResult interface{}
type StakeSnapshotsResult interface{}
type PoolDistrResult interface{}
