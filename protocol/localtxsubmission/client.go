package localtxsubmission

import (
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"sync"
)

// Client implements the LocalTxSubmission client
type Client struct {
	*protocol.Protocol
	config           *Config
	busyMutex        sync.Mutex
	submitResultChan chan error
}

// NewClient returns a new LocalTxSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:           cfg,
		submitResultChan: make(chan error),
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateBusy]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[stateBusy] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                protocolName,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        stateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	// Start goroutine to cleanup resources on protocol shutdown
	go func() {
		<-c.Protocol.DoneChan()
		close(c.submitResultChan)
	}()
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptTx:
		err = c.handleAcceptTx()
	case MessageTypeRejectTx:
		err = c.handleRejectTx(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
	}
	return err
}

// SubmitTx submits a transaction using the specified transaction era ID and TX payload
func (c *Client) SubmitTx(eraId uint16, tx []byte) error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgSubmitTx(eraId, tx)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	err := <-c.submitResultChan
	return err
}

// Stop transations the protocol to the Done state. No more operations will be possible
func (c *Client) Stop() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgDone()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

func (c *Client) handleAcceptTx() error {
	c.submitResultChan <- nil
	return nil
}

func (c *Client) handleRejectTx(msg protocol.Message) error {
	msgRejectTx := msg.(*MsgRejectTx)
	rejectErr, err := ledger.NewTxSubmitErrorFromCbor(msgRejectTx.Reason)
	if err != nil {
		return err
	}
	err = TransactionRejectedError{
		Reason:     rejectErr,
		ReasonCbor: []byte(msgRejectTx.Reason),
	}
	c.submitResultChan <- err
	return nil
}
