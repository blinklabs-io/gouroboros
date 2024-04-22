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

package localstatequery

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
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
		c.Protocol.Start()
		// Start goroutine to cleanup resources on protocol shutdown
		go func() {
			<-c.Protocol.DoneChan()
			close(c.queryResultChan)
			close(c.acquireResultChan)
		}()
	})
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
	c.acquired = true
	c.acquireResultChan <- nil
	c.currentEra = -1
	return nil
}

func (c *Client) handleFailure(msg protocol.Message) error {
	msgFailure := msg.(*MsgFailure)
	switch msgFailure.Failure {
	case AcquireFailurePointTooOld:
		c.acquireResultChan <- AcquireFailurePointTooOldError{}
	case AcquireFailurePointNotOnChain:
		c.acquireResultChan <- AcquireFailurePointNotOnChainError{}
	default:
		return fmt.Errorf("unknown failure type: %d", msgFailure.Failure)
	}
	return nil
}

func (c *Client) handleResult(msg protocol.Message) error {
	msgResult := msg.(*MsgResult)
	c.queryResultChan <- msgResult.Result
	return nil
}

func (c *Client) acquire(point *common.Point) error {
	var msg protocol.Message
	if c.acquired {
		if point != nil {
			msg = NewMsgReAcquire(*point)
		} else {
			msg = NewMsgReAcquireNoPoint()
		}
	} else {
		if point != nil {
			msg = NewMsgAcquire(*point)
		} else {
			msg = NewMsgAcquireNoPoint()
		}
	}
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	err, ok := <-c.acquireResultChan
	if !ok {
		return protocol.ProtocolShuttingDownError
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

func (c *Client) runQuery(query interface{}, result interface{}) error {
	msg := NewMsgQuery(query)
	if !c.acquired {
		if err := c.acquire(nil); err != nil {
			return err
		}
	}
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	resultCbor, ok := <-c.queryResultChan
	if !ok {
		return protocol.ProtocolShuttingDownError
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

// Acquire starts the acquire process for the specified chain point
func (c *Client) Acquire(point *common.Point) error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.acquire(point)
}

// Release releases the previously acquired chain point
func (c *Client) Release() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.release()
}

// GetCurrentEra returns the current era ID
func (c *Client) GetCurrentEra() (int, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.getCurrentEra()
}

// GetSystemStart returns the SystemStart value
func (c *Client) GetSystemStart() (*SystemStartResult, error) {
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

// TODO
/*
query	[2 #6.258([*[0 int]])	int is the stake the user intends to delegate, the array must be sorted
*/
func (c *Client) GetNonMyopicMemberRewards() (*NonMyopicMemberRewardsResult, error) {
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
func (c *Client) GetCurrentProtocolParams() (CurrentProtocolParamsResult, error) {
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
	case ledger.EraIdBabbage:
		result := []ledger.BabbageProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	case ledger.EraIdAlonzo:
		result := []ledger.AlonzoProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	case ledger.EraIdMary:
		result := []ledger.MaryProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	case ledger.EraIdAllegra:
		result := []ledger.AllegraProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	case ledger.EraIdShelley:
		result := []ledger.ShelleyProtocolParameters{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	default:
		result := []any{}
		if err := c.runQuery(query, &result); err != nil {
			return nil, err
		}
		return result[0], nil
	}
}

func (c *Client) GetProposedProtocolParamsUpdates() (*ProposedProtocolParamsUpdatesResult, error) {
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

func (c *Client) GetUTxOByAddress(
	addrs []ledger.Address,
) (*UTxOByAddressResult, error) {
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

func (c *Client) GetUTxOWhole() (*UTxOWholeResult, error) {
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

// TODO
func (c *Client) DebugEpochState() (*DebugEpochStateResult, error) {
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

// TODO
/*
query	[10 #6.258([ *rwdr ])]
*/
func (c *Client) GetFilteredDelegationsAndRewardAccounts(
	creds []interface{},
) (*FilteredDelegationsAndRewardAccountsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyFilteredDelegationAndRewardAccounts,
		// TODO: add params
	)
	var result FilteredDelegationsAndRewardAccountsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetGenesisConfig() (*GenesisConfigResult, error) {
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

// TODO
func (c *Client) DebugNewEpochState() (*DebugNewEpochStateResult, error) {
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

// TODO
func (c *Client) DebugChainDepState() (*DebugChainDepStateResult, error) {
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

func (c *Client) GetUTxOByTxIn(txIns []ledger.TransactionInput) (*UTxOByTxInResult, error) {
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

// TODO
func (c *Client) GetRewardInfoPools() (*RewardInfoPoolsResult, error) {
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

// TODO
func (c *Client) GetPoolState(poolIds []interface{}) (*PoolStateResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolState,
	)
	var result PoolStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TODO
func (c *Client) GetStakeSnapshots(
	poolId interface{},
) (*StakeSnapshotsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyStakeSnapshots,
	)
	var result StakeSnapshotsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TODO
func (c *Client) GetPoolDistr(poolIds []interface{}) (*PoolDistrResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QueryTypeShelleyPoolDistr,
	)
	var result PoolDistrResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
