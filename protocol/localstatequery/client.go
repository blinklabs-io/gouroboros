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
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the LocalStateQuery client
type Client struct {
	*protocol.Protocol
	config                        *Config
	callbackContext               CallbackContext
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
	busyMutex                     sync.Mutex
	acquired                      bool
	queryResultChan               chan []byte
	acquireResultChan             chan error
	currentEra                    int
	onceStart                     sync.Once
}

// NewClient returns a new LocalStateQuery client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:            cfg,
		queryResultChan:   make(chan []byte),
		acquireResultChan: make(chan error),
		acquired:          false,
		currentEra:        -1,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeouts
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateAcquiring]; ok {
		entry.Timeout = c.config.AcquireTimeout
		stateMap[stateAcquiring] = entry
	}
	if entry, ok := stateMap[stateQuerying]; ok {
		entry.Timeout = c.config.QueryTimeout
		stateMap[stateQuerying] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        stateIdle,
	}
	// Enable version-dependent features
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 10 {
		c.enableGetChainBlockNo = true
		c.enableGetChainPoint = true
	}
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 11 {
		c.enableGetRewardInfoPoolsBlock = true
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
		// Start goroutine to cleanup resources on protocol shutdown
		go func() {
			<-c.DoneChan()
			close(c.queryResultChan)
			close(c.acquireResultChan)
		}()
	})
}

// Acquire starts the acquire process for the specified chain point
func (c *Client) Acquire(point *pcommon.Point) error {
	// Use volatile tip if no point provided
	if point == nil {
		return c.AcquireVolatileTip()
	}
	c.Protocol.Logger().
		Debug(
			fmt.Sprintf(
				"calling Acquire(point: {Slot: %d, Hash: %x})",
				point.Slot,
				point.Hash,
			),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	acquireTarget := AcquireSpecificPoint{
		Point: *point,
	}
	return c.acquire(acquireTarget)
}

func (c *Client) AcquireVolatileTip() error {
	c.Protocol.Logger().
		Debug(
			"calling AcquireVolatileTip",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	acquireTarget := AcquireVolatileTip{}
	return c.acquire(acquireTarget)
}

func (c *Client) AcquireImmutableTip() error {
	c.Protocol.Logger().
		Debug(
			"calling AcquireImmutableTip",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	acquireTarget := AcquireImmutableTip{}
	return c.acquire(acquireTarget)
}

// Release releases the previously acquired chain point
func (c *Client) Release() error {
	c.Protocol.Logger().
		Debug("calling Release()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.release()
}

// GetCurrentEra returns the current era ID
func (c *Client) GetCurrentEra() (int, error) {
	c.Protocol.Logger().
		Debug("calling GetCurrentEra()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.getCurrentEra()
}

// GetSystemStart returns the SystemStart value
func (c *Client) GetSystemStart() (*SystemStartResult, error) {
	c.Protocol.Logger().
		Debug("calling GetSystemStart()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QueryTypeSystemStart,
	)
	var result SystemStartResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetChainBlockNo returns the latest block number
func (c *Client) GetChainBlockNo() (int64, error) {
	c.Protocol.Logger().
		Debug("calling GetChainBlockNo()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QueryTypeChainBlockNo,
	)
	result := []int64{}
	if err := c.runQuery(query, &result); err != nil {
		return 0, err
	}
	return result[1], nil
}

// GetChainPoint returns the current chain tip
func (c *Client) GetChainPoint() (*pcommon.Point, error) {
	c.Protocol.Logger().
		Debug("calling GetChainPoint()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QueryTypeChainPoint,
	)
	var result pcommon.Point
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetEraHistory returns the era history
func (c *Client) GetEraHistory() ([]EraHistoryResult, error) {
	c.Protocol.Logger().
		Debug("calling GetEraHistory()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildHardForkQuery(QueryTypeHardForkEraHistory)
	var result []EraHistoryResult
	if err := c.runQuery(query, &result); err != nil {
		return []EraHistoryResult{}, err
	}
	return result, nil
}

// GetEpochNo returns the current epoch number
func (c *Client) GetEpochNo() (int, error) {
	c.Protocol.Logger().
		Debug("calling GetEpochNo()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return 0, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyEpochNo,
	)
	result := []int{}
	if err := c.runQuery(query, &result); err != nil {
		return 0, err
	}
	return result[0], nil
}

// GetNonMyopicMemberRewards returns the non-myopic member rewards for the given stakes
// stakes should be an array of [tag, stake_amount] pairs where tag is 0 for KeyHash credentials
// The array must be sorted by stake amount
func (c *Client) GetNonMyopicMemberRewards(stakes []any) (*NonMyopicMemberRewardsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetNonMyopicMemberRewards(stakes: %+v)", stakes),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	// Wrap the stakes in a CBOR set (tag 258)
	stakesSet := cbor.NewSetType(stakes, true)
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyNonMyopicMemberRewards,
		stakesSet,
	)
	// The result is wrapped in a single-element array
	var wrappedResult []NonMyopicMemberRewardsResult
	if err := c.runQuery(query, &wrappedResult); err != nil {
		return nil, err
	}
	if len(wrappedResult) == 0 {
		return nil, errors.New("empty result from non-myopic member rewards query")
	}
	return &wrappedResult[0], nil
}

// GetCurrentProtocolParams returns the set of protocol params that are currently in effect
func (c *Client) GetCurrentProtocolParams() (lcommon.ProtocolParameters, error) {
	c.Protocol.Logger().
		Debug("calling GetCurrentProtocolParams()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyCurrentProtocolParams,
	)
	switch currentEra {
	case ledger.EraIdConway:
		result := []ledger.ConwayProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	case ledger.EraIdBabbage:
		result := []ledger.BabbageProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	case ledger.EraIdAlonzo:
		result := []ledger.AlonzoProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	case ledger.EraIdMary:
		result := []ledger.MaryProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	case ledger.EraIdAllegra:
		result := []ledger.AllegraProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	case ledger.EraIdShelley:
		result := []ledger.ShelleyProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return &result[0], nil
	default:
		return nil, fmt.Errorf("unknown era ID: %d", currentEra)
	}
}

// GetProposedProtocolParamsUpdates returns the set of proposed protocol params updates
func (c *Client) GetProposedProtocolParamsUpdates() (*ProposedProtocolParamsUpdatesResult, error) {
	c.Protocol.Logger().
		Debug("calling GetProposedProtocolParamsUpdates()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyProposedProtocolParamsUpdates,
	)
	var result ProposedProtocolParamsUpdatesResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetStakeDistribution returns the stake distribution
func (c *Client) GetStakeDistribution() (*StakeDistributionResult, error) {
	c.Protocol.Logger().
		Debug("calling GetStakeDistribution()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyStakeDistribution,
	)
	var result StakeDistributionResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUTxOByAddress returns the UTxOs for a given list of ledger.Address structs
func (c *Client) GetUTxOByAddress(
	addrs []ledger.Address,
) (*UTxOsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetUTxOByAddress(addrs: %+v)", addrs),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyUtxoByAddress,
		addrs,
	)
	var result UTxOsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUTxOWhole returns the current UTxO set
func (c *Client) GetUTxOWhole() (*UTxOsResult, error) {
	c.Protocol.Logger().
		Debug("calling GetUTxOWhole()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyUtxoWhole,
	)
	var result UTxOsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DebugEpochState() (*DebugEpochStateResult, error) {
	c.Protocol.Logger().
		Debug("calling DebugEpochState()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyDebugEpochState,
	)
	var result DebugEpochStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TODO (#858)
/*
query	[10 #6.258([ *rwdr ])]
*/
func (c *Client) GetFilteredDelegationsAndRewardAccounts(
	creds []any,
) (*FilteredDelegationsAndRewardAccountsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetFilteredDelegationsAndRewardAccounts(creds: %+v)", creds),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyFilteredDelegationAndRewardAccounts,
		// TODO: add params (#858)
	)
	var result FilteredDelegationsAndRewardAccountsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetGenesisConfig() (*GenesisConfigResult, error) {
	c.Protocol.Logger().
		Debug("calling GetGenesisConfig()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyGenesisConfig,
	)
	result := []GenesisConfigResult{}
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result[0], nil
}

func (c *Client) DebugNewEpochState() (*DebugNewEpochStateResult, error) {
	c.Protocol.Logger().
		Debug("calling DebugNewEpochState()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyDebugNewEpochState,
	)
	var result DebugNewEpochStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DebugChainDepState() (*DebugChainDepStateResult, error) {
	c.Protocol.Logger().
		Debug("calling DebugChainDepState()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyDebugChainDepState,
	)
	var result DebugChainDepStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetRewardProvenance() (*RewardProvenanceResult, error) {
	c.Protocol.Logger().
		Debug("calling GetRewardProvenance()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyRewardProvenance,
	)
	var result RewardProvenanceResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetUTxOByTxIn(
	txIns []ledger.TransactionInput,
) (*UTxOByTxInResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetUTxOByTxIn(txIns: %+v)", txIns),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyUtxoByTxin,
		txIns,
	)
	var result UTxOByTxInResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetStakePools() (*StakePoolsResult, error) {
	c.Protocol.Logger().
		Debug("calling GetStakePools()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyStakePools,
	)
	var result StakePoolsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetStakePoolParams(
	poolIds []ledger.PoolId,
) (*StakePoolParamsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetStakePoolParams(poolIds: %+v)", poolIds),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyStakePoolParams,
		cbor.Tag{
			Number:  cbor.CborTagSet,
			Content: poolIds,
		},
	)
	var result StakePoolParamsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetRewardInfoPools() (*RewardInfoPoolsResult, error) {
	c.Protocol.Logger().
		Debug("calling GetRewardInfoPools()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyRewardInfoPools,
	)
	var result RewardInfoPoolsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetPoolState(poolIds []any) (*PoolStateResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetPoolState(poolIds: %+v)", poolIds),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	// GetPoolState requires a pool set parameter similar to GetPoolDistr
	// If no pool IDs specified, use an empty set to query all pools
	if poolIds == nil {
		poolIds = []any{}
	}
	params := []any{
		poolIds,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolState,
		params...,
	)
	result := []PoolStateResult{}
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("empty result from pool state query")
	}
	return &result[0], nil
}

func (c *Client) GetStakeSnapshots(
	poolIds []any,
) (*StakeSnapshotsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetStakeSnapshots(poolIds: %+v)", poolIds),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	// Wrap the poolIds in a CBOR set (tag 258)
	poolIdSet := cbor.NewSetType(poolIds, true)
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyStakeSnapshots,
		poolIdSet,
	)
	// The result is wrapped in a single-element array
	var wrappedResult []StakeSnapshotsResult
	if err := c.runQuery(query, &wrappedResult); err != nil {
		return nil, err
	}
	if len(wrappedResult) == 0 {
		return nil, errors.New("empty result from stake snapshots query")
	}
	return &wrappedResult[0], nil
}

func (c *Client) GetPoolDistr(poolIds []any) (*PoolDistrResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetPoolDistr(poolIds: %+v)", poolIds),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	// GetPoolDistr always requires a pool set parameter according to the Haskell implementation
	// The query expects (len=2, tag=21) format: [21, Set(poolIds)]
	// If no pool IDs specified, use an empty set to query all pools
	if poolIds == nil {
		poolIds = []any{}
	}
	params := []any{
		poolIds,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolDistr,
		params...,
	)
	var result PoolDistrResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetConstitution returns the current constitution (Conway era)
// The constitution contains an anchor URL/hash and an optional guardrails script hash
func (c *Client) GetConstitution() (*ConstitutionResult, error) {
	c.Protocol.Logger().
		Debug("calling GetConstitution()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetConstitution requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyConstitution,
	)
	var result ConstitutionResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetGovState returns the full governance state (Conway era)
// This includes proposals, committee, constitution, and protocol parameters
func (c *Client) GetGovState() (*GovStateResult, error) {
	c.Protocol.Logger().
		Debug("calling GetGovState()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetGovState requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyGovState,
	)
	var result GovStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDRepState returns the state of specified DReps (Conway era)
// If credentials is nil or empty, returns state for all DReps
func (c *Client) GetDRepState(
	credentials []lcommon.Credential,
) (*DRepStateResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetDRepState(credentials: %d)", len(credentials)),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetDRepState requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	// Create a CBOR set from the credentials
	if credentials == nil {
		credentials = []lcommon.Credential{}
	}
	credSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: credentials,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyDRepState,
		credSet,
	)
	var result DRepStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDRepStakeDistr returns the stake distribution for specified DReps (Conway era)
// If dreps is nil or empty, returns distribution for all DReps
func (c *Client) GetDRepStakeDistr(
	dreps []lcommon.Drep,
) (*DRepStakeDistrResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetDRepStakeDistr(dreps: %d)", len(dreps)),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetDRepStakeDistr requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	// Create a CBOR set from the dreps
	if dreps == nil {
		dreps = []lcommon.Drep{}
	}
	drepSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: dreps,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyDRepStakeDistr,
		drepSet,
	)
	var result DRepStakeDistrResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetCommitteeMembersState returns the state of committee members (Conway era)
// The filter parameters allow querying by cold credentials, hot credentials, or member status
// Pass nil/empty to query without that filter
func (c *Client) GetCommitteeMembersState(
	coldCreds []lcommon.Credential,
	hotCreds []lcommon.Credential,
	statuses []int,
) (*CommitteeMembersStateResult, error) {
	c.Protocol.Logger().
		Debug("calling GetCommitteeMembersState()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetCommitteeMembersState requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	// Create CBOR sets for each filter
	if coldCreds == nil {
		coldCreds = []lcommon.Credential{}
	}
	if hotCreds == nil {
		hotCreds = []lcommon.Credential{}
	}
	if statuses == nil {
		statuses = []int{}
	}
	coldSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: coldCreds,
	}
	hotSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: hotCreds,
	}
	statusSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: statuses,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyCommitteeMembersState,
		coldSet,
		hotSet,
		statusSet,
	)
	var result CommitteeMembersStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetFilteredVoteDelegatees returns the DRep delegations for specified stake credentials (Conway era)
// If credentials is nil or empty, returns delegations for all credentials
func (c *Client) GetFilteredVoteDelegatees(
	credentials []lcommon.Credential,
) (*FilteredVoteDelegateesResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetFilteredVoteDelegatees(credentials: %d)", len(credentials)),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetFilteredVoteDelegatees requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	// Create a CBOR set from the credentials
	if credentials == nil {
		credentials = []lcommon.Credential{}
	}
	credSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: credentials,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyFilteredVoteDelegatees,
		credSet,
	)
	var result FilteredVoteDelegateesResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSPOStakeDistr returns the SPO stake distribution for governance voting (Conway era)
// If poolIds is nil or empty, returns distribution for all pools
func (c *Client) GetSPOStakeDistr(
	poolIds []ledger.PoolId,
) (*SPOStakeDistrResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetSPOStakeDistr(poolIds: %d)", len(poolIds)),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	if currentEra < ledger.EraIdConway {
		return nil, fmt.Errorf(
			"GetSPOStakeDistr requires Conway era or later (current era: %d)",
			currentEra,
		)
	}
	// Create a CBOR set from the pool IDs
	if poolIds == nil {
		poolIds = []ledger.PoolId{}
	}
	poolSet := cbor.Tag{
		Number:  cbor.CborTagSet,
		Content: poolIds,
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleySPOStakeDistr,
		poolSet,
	)
	var result SPOStakeDistrResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquired:
		err = c.handleAcquired()
	case MessageTypeFailure:
		err = c.handleFailure(msg)
	case MessageTypeResult:
		err = c.handleResult(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleAcquired() error {
	c.Protocol.Logger().
		Debug("acquired",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	c.acquired = true
	c.acquireResultChan <- nil
	c.currentEra = -1
	return nil
}

func (c *Client) handleFailure(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("failed",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	msgFailure := msg.(*MsgFailure)
	switch msgFailure.Failure {
	case AcquireFailurePointTooOld:
		c.acquireResultChan <- ErrAcquireFailurePointTooOld
	case AcquireFailurePointNotOnChain:
		c.acquireResultChan <- ErrAcquireFailurePointNotOnChain
	default:
		return fmt.Errorf("unknown failure type: %d", msgFailure.Failure)
	}
	return nil
}

func (c *Client) handleResult(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("results",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	msgResult := msg.(*MsgResult)
	c.queryResultChan <- msgResult.Result
	return nil
}

func (c *Client) acquire(acquireTarget AcquireTarget) error {
	var msg protocol.Message
	if c.acquired {
		switch t := acquireTarget.(type) {
		case AcquireSpecificPoint:
			msg = NewMsgReAcquire(t.Point)
		case AcquireVolatileTip:
			msg = NewMsgReAcquireVolatileTip()
		case AcquireImmutableTip:
			msg = NewMsgReAcquireImmutableTip()
		default:
			return errors.New("invalid acquire point provided")
		}
	} else {
		switch t := acquireTarget.(type) {
		case AcquireSpecificPoint:
			msg = NewMsgAcquire(t.Point)
		case AcquireVolatileTip:
			msg = NewMsgAcquireVolatileTip()
		case AcquireImmutableTip:
			msg = NewMsgAcquireImmutableTip()
		default:
			return errors.New("invalid acquire point provided")
		}
	}
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	err, ok := <-c.acquireResultChan
	if !ok {
		return protocol.ErrProtocolShuttingDown
	}
	return err
}

func (c *Client) release() error {
	msg := NewMsgRelease()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	c.acquired = false
	c.currentEra = -1
	return nil
}

func (c *Client) runQuery(query any, result any) error {
	msg := NewMsgQuery(query)
	if !c.acquired {
		if err := c.acquire(AcquireVolatileTip{}); err != nil {
			return err
		}
	}
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	resultCbor, ok := <-c.queryResultChan
	if !ok {
		return protocol.ErrProtocolShuttingDown
	}
	if _, err := cbor.Decode(resultCbor, result); err != nil {
		return err
	}
	return nil
}

// Helper function for getting the current era
// The current era is needed for many other queries
func (c *Client) getCurrentEra() (int, error) {
	// Return cached era, if available
	if c.currentEra > -1 {
		return c.currentEra, nil
	}
	query := buildHardForkQuery(QueryTypeHardForkCurrentEra)
	var result int
	if err := c.runQuery(query, &result); err != nil {
		return -1, err
	}
	return result, nil
}
