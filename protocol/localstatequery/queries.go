package localstatequery

import (
	"github.com/fxamacker/cbor/v2"
)

const (
	QUERY_TYPE_BLOCK          = 0
	QUERY_TYPE_SYSTEM_START   = 1
	QUERY_TYPE_CHAIN_BLOCK_NO = 2
	QUERY_TYPE_CHAIN_POINT    = 3

	// Block query sub-types
	QUERY_TYPE_SHELLEY   = 0
	QUERY_TYPE_HARD_FORK = 2

	// Hard fork query sub-types
	QUERY_TYPE_HARD_FORK_ERA_HISTORY = 0
	QUERY_TYPE_HARD_FORK_CURRENT_ERA = 1

	// Shelley query sub-types
	QUERY_TYPE_SHELLEY_LEDGER_TIP                              = 0
	QUERY_TYPE_SHELLEY_EPOCH_NO                                = 1
	QUERY_TYPE_SHELLEY_NON_MYOPIC_MEMBER_REWARDS               = 2
	QUERY_TYPE_SHELLEY_CURRENT_PROTOCOL_PARAMS                 = 3
	QUERY_TYPE_SHELLEY_PROPOSED_PROTOCOL_PARAMS_UPDATES        = 4
	QUERY_TYPE_SHELLEY_STAKE_DISTRIBUTION                      = 5
	QUERY_TYPE_SHELLEY_UTXO_BY_ADDRESS                         = 6
	QUERY_TYPE_SHELLEY_UTXO_WHOLE                              = 7
	QUERY_TYPE_SHELLEY_DEBUG_EPOCH_STATE                       = 8
	QUERY_TYPE_SHELLEY_CBOR                                    = 9
	QUERY_TYPE_SHELLEY_FILTERED_DELEGATION_AND_REWARD_ACCOUNTS = 10
	QUERY_TYPE_SHELLEY_GENESIS_CONFIG                          = 11
	QUERY_TYPE_SHELLEY_DEBUG_NEW_EPOCH_STATE                   = 12
	QUERY_TYPE_SHELLEY_DEBUG_CHAIN_DEP_STATE                   = 13
	QUERY_TYPE_SHELLEY_REWARD_PROVENANCE                       = 14
	QUERY_TYPE_SHELLEY_UTXO_BY_TXIN                            = 15
	QUERY_TYPE_SHELLEY_STAKE_POOLS                             = 16
	QUERY_TYPE_SHELLEY_STAKE_POOL_PARAMS                       = 17
	QUERY_TYPE_SHELLEY_REWARD_INFO_POOLS                       = 18
	QUERY_TYPE_SHELLEY_POOL_STATE                              = 19
	QUERY_TYPE_SHELLEY_STAKE_SNAPSHOTS                         = 20
	QUERY_TYPE_SHELLEY_POOL_DISTR                              = 21
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
		QUERY_TYPE_BLOCK,
		buildQuery(
			QUERY_TYPE_HARD_FORK,
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
		QUERY_TYPE_BLOCK,
		buildQuery(
			QUERY_TYPE_SHELLEY,
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
