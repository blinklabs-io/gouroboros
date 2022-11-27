package localstatequery

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"sync"
)

type Client struct {
	*protocol.Protocol
	config                        *Config
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
	busyMutex                     sync.Mutex
	acquired                      bool
	queryResultChan               chan []byte
	acquireResultChan             chan error
	currentEra                    int
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	c := &Client{
		config:            cfg,
		queryResultChan:   make(chan []byte),
		acquireResultChan: make(chan error),
		acquired:          false,
		currentEra:        -1,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        STATE_IDLE,
	}
	// Enable version-dependent features
	if protoOptions.Version >= 10 {
		c.enableGetChainBlockNo = true
		c.enableGetChainPoint = true
	}
	if protoOptions.Version >= 11 {
		c.enableGetRewardInfoPoolsBlock = true
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_ACQUIRED:
		err = c.handleAcquired()
	case MESSAGE_TYPE_FAILURE:
		err = c.handleFailure(msg)
	case MESSAGE_TYPE_RESULT:
		err = c.handleResult(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
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
	case ACQUIRE_FAILURE_POINT_TOO_OLD:
		c.acquireResultChan <- AcquireFailurePointTooOldError{}
	case ACQUIRE_FAILURE_POINT_NOT_ON_CHAIN:
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
	err := <-c.acquireResultChan
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
	resultCbor := <-c.queryResultChan
	if _, err := utils.CborDecode(resultCbor, result); err != nil {
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
	query := buildHardForkQuery(QUERY_TYPE_HARD_FORK_CURRENT_ERA)
	var result int
	if err := c.runQuery(query, &result); err != nil {
		return -1, err
	}
	return result, nil
}

func (c *Client) Acquire(point *common.Point) error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.acquire(point)
}

func (c *Client) Release() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.release()
}

func (c *Client) GetCurrentEra() (int, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.getCurrentEra()
}

func (c *Client) GetSystemStart() (*SystemStartResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QUERY_TYPE_SYSTEM_START,
	)
	var result SystemStartResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetChainBlockNo() (int64, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QUERY_TYPE_CHAIN_BLOCK_NO,
	)
	var result []int64
	if err := c.runQuery(query, &result); err != nil {
		return 0, err
	}
	return result[1], nil
}

func (c *Client) GetChainPoint() (*common.Point, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildQuery(
		QUERY_TYPE_CHAIN_POINT,
	)
	var result common.Point
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetEraHistory() (*EraHistoryResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	query := buildHardForkQuery(QUERY_TYPE_HARD_FORK_ERA_HISTORY)
	var result EraHistoryResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetEpochNo() (int, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return 0, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_EPOCH_NO,
	)
	var result []int
	if err := c.runQuery(query, &result); err != nil {
		return 0, err
	}
	return result[0], nil
}

func (c *Client) GetNonMyopicMemberRewards() (*NonMyopicMemberRewardsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_NON_MYOPIC_MEMBER_REWARDS,
	)
	var result NonMyopicMemberRewardsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetCurrentProtocolParams() (*CurrentProtocolParamsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_CURRENT_PROTOCOL_PARAMS,
	)
	var result CurrentProtocolParamsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

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
		QUERY_TYPE_SHELLEY_PROPOSED_PROTOCOL_PARAMS_UPDATES,
	)
	var result ProposedProtocolParamsUpdatesResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetStakeDistribution() (*StakeDistributionResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_STAKE_DISTRIBUTION,
	)
	var result StakeDistributionResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetUTxOByAddress(addrs []interface{}) (*UTxOByAddressResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_UTXO_BY_ADDRESS,
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
		QUERY_TYPE_SHELLEY_UTXO_WHOLE,
	)
	var result UTxOWholeResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) DebugEpochState() (*DebugEpochStateResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_DEBUG_EPOCH_STATE,
	)
	var result DebugEpochStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetFilteredDelegationsAndRewardAccounts(creds []interface{}) (*FilteredDelegationsAndRewardAccountsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_FILTERED_DELEGATION_AND_REWARD_ACCOUNTS,
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
		QUERY_TYPE_SHELLEY_GENESIS_CONFIG,
	)
	var result GenesisConfigResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) DebugNewEpochState() (*DebugNewEpochStateResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_DEBUG_NEW_EPOCH_STATE,
	)
	var result DebugNewEpochStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) DebugChainDepState() (*DebugChainDepStateResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_DEBUG_CHAIN_DEP_STATE,
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
		QUERY_TYPE_SHELLEY_REWARD_PROVENANCE,
	)
	var result RewardProvenanceResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetUTxOByTxIn(txins []interface{}) (*UTxOByTxInResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_UTXO_BY_TXIN,
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
		QUERY_TYPE_SHELLEY_STAKE_POOLS,
	)
	var result StakePoolsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetStakePoolParams(poolIds []interface{}) (*StakePoolParamsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_STAKE_POOL_PARAMS,
	)
	var result StakePoolParamsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetRewardInfoPools() (*RewardInfoPoolsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_REWARD_INFO_POOLS,
	)
	var result RewardInfoPoolsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetPoolState(poolIds []interface{}) (*PoolStateResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_POOL_STATE,
	)
	var result PoolStateResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetStakeSnapshots(poolId interface{}) (*StakeSnapshotsResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_STAKE_SNAPSHOTS,
	)
	var result StakeSnapshotsResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}

func (c *Client) GetPoolDistr(poolIds []interface{}) (*PoolDistrResult, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	currentEra, err := c.getCurrentEra()
	if err != nil {
		return nil, err
	}
	query := buildShelleyQuery(
		currentEra,
		QUERY_TYPE_SHELLEY_POOL_DISTR,
	)
	var result PoolDistrResult
	if err := c.runQuery(query, &result); err != nil {
		return nil, err
	}
	return &result, nil

}
