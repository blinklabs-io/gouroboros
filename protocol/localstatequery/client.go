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
	ledgercommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
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
func (c *Client) Acquire(point *common.Point) error {
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
func (c *Client) GetChainPoint() (*common.Point, error) {
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
	var result common.Point
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

// TODO (#860)
/*
query	[2 #6.258([*[0 int]])	int is the stake the user intends to delegate, the array must be sorted
*/
func (c *Client) GetNonMyopicMemberRewards() (*NonMyopicMemberRewardsResult, error) {
	c.Protocol.Logger().
		Debug("calling GetNonMyopicMemberRewards()",
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
		QueryTypeShelleyNonMyopicMemberRewards,
	)
	var result NonMyopicMemberRewardsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetCurrentProtocolParams returns the set of protocol params that are currently in effect
func (c *Client) GetCurrentProtocolParams() (ledgercommon.ProtocolParameters, error) {
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
) (*UTxOByAddressResult, error) {
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
	var result UTxOByAddressResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUTxOWhole returns the current UTxO set
func (c *Client) GetUTxOWhole() (*UTxOWholeResult, error) {
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
	var result UTxOWholeResult
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
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolState,
		// TODO: add args (#868)
	)
	var result PoolStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetStakeSnapshots(
	poolId any,
) (*StakeSnapshotsResult, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling GetStakeSnapshots(poolId: %+v)", poolId),
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
		QueryTypeShelleyStakeSnapshots,
		// TODO: add args (#869)
	)
	var result StakeSnapshotsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
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
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolDistr,
		// TODO: add args (#870)
	)
	var result PoolDistrResult
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
