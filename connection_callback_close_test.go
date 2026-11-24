// Copyright 2026 Blink Labs Software
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

package ouroboros_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/perasvotes"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const callbackCloseTimeout = 5 * time.Second

const callbackCloseMagic = 764824073

// callbackCloser closes one side of a connection pair from inside a
// mini-protocol callback, with the context that callback was given.
type callbackCloser struct {
	conn   atomic.Pointer[ouroboros.Connection]
	closed chan error
}

func newCallbackCloser() *callbackCloser {
	return &callbackCloser{closed: make(chan error, 1)}
}

// close is called by the callback under test. It closes the connection with
// the callback's context and records what Close returned.
func (cc *callbackCloser) close(ctx context.Context) {
	conn := cc.conn.Load()
	if conn == nil {
		return
	}
	err := conn.Close(ctx)
	select {
	case cc.closed <- err:
	default:
	}
}

// connectPair connects a client and a server connection over net.Pipe and
// returns them once both handshakes are done.
func connectPair(
	t *testing.T,
	common []ouroboros.ConnectionOptionFunc,
	clientOpts []ouroboros.ConnectionOptionFunc,
	serverOpts []ouroboros.ConnectionOptionFunc,
) (*ouroboros.Connection, *ouroboros.Connection) {
	t.Helper()
	clientSide, serverSide := net.Pipe()
	type result struct {
		conn *ouroboros.Connection
		err  error
	}
	serverResult := make(chan result, 1)
	go func() {
		opts := append(
			[]ouroboros.ConnectionOptionFunc{
				ouroboros.WithConnection(serverSide),
				ouroboros.WithServer(true),
				ouroboros.WithNetworkMagic(callbackCloseMagic),
			},
			common...,
		)
		conn, err := ouroboros.NewConnection(append(opts, serverOpts...)...)
		serverResult <- result{conn, err}
	}()
	opts := append(
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithConnection(clientSide),
			ouroboros.WithNetworkMagic(callbackCloseMagic),
		},
		common...,
	)
	client, err := ouroboros.NewConnection(append(opts, clientOpts...)...)
	require.NoError(t, err)
	var server result
	select {
	case server = <-serverResult:
	case <-time.After(callbackCloseTimeout):
		t.Fatal("server handshake did not finish")
	}
	require.NoError(t, server.err)
	t.Cleanup(func() {
		_ = client.Close(context.Background())
		_ = server.conn.Close(context.Background())
	})
	return client, server.conn
}

// requireClosedFromCallback checks that the callback's Close returned
// without error and that the connection is fully joined: another Close
// returns, the error channel is closed, and, once the peer is closed too,
// no goroutine is left.
func requireClosedFromCallback(
	t *testing.T,
	cc *callbackCloser,
	peer *ouroboros.Connection,
) {
	t.Helper()
	select {
	case err := <-cc.closed:
		require.NoError(t, err, "Close from the callback")
	case <-time.After(callbackCloseTimeout):
		t.Fatal("Close from the callback did not return")
	}
	conn := cc.conn.Load()
	ctx, cancel := context.WithTimeout(
		context.Background(), callbackCloseTimeout,
	)
	defer cancel()
	// This Close is not the callback's, so it waits for the callback's
	// protocol instance too.
	require.NoError(t, conn.Close(ctx), "connection was not joined")
	_, open := <-conn.ErrorChan()
	require.False(t, open, "error channel still open after Close")
	require.NoError(t, peer.Close(ctx))
}

func nodeToNode() []ouroboros.ConnectionOptionFunc {
	return []ouroboros.ConnectionOptionFunc{
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(false),
	}
}

func nodeToClient() []ouroboros.ConnectionOptionFunc {
	return []ouroboros.ConnectionOptionFunc{ouroboros.WithNodeToNode(false)}
}

func TestBlockFetchCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg, err := blockfetch.NewConfig()
	require.NoError(t, err)
	cfg.RequestRangeFunc = func(
		ctx blockfetch.CallbackContext, _, _ pcommon.Point,
	) error {
		cc.close(ctx.Context())
		return nil
	}
	client, server := connectPair(t, nodeToNode(), nil,
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithBlockFetchConfig(cfg)})
	cc.conn.Store(server)
	go func() {
		_ = client.BlockFetch().Client.GetBlockRange(
			pcommon.NewPointOrigin(), pcommon.NewPointOrigin(),
		)
	}()
	requireClosedFromCallback(t, cc, client)
}

func TestChainSyncCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := chainsync.NewConfig()
	cfg.FindIntersectFunc = func(
		ctx chainsync.CallbackContext, _ []pcommon.Point,
	) (pcommon.Point, chainsync.Tip, error) {
		cc.close(ctx.Context())
		return pcommon.NewPointOrigin(), chainsync.Tip{}, nil
	}
	client, server := connectPair(t, nodeToNode(), nil,
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithChainSyncConfig(cfg)})
	cc.conn.Store(server)
	go func() { _, _ = client.ChainSync().Client.GetCurrentTip() }()
	requireClosedFromCallback(t, cc, client)
}

func TestTxSubmissionCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	server := txsubmission.NewConfig()
	server.InitFunc = func(ctx txsubmission.CallbackContext) error {
		cc.close(ctx.Context())
		return nil
	}
	client, srv := connectPair(t, nodeToNode(), nil,
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithTxSubmissionConfig(server)})
	cc.conn.Store(srv)
	// MsgInit runs InitFunc on the server.
	client.TxSubmission().Client.Init()
	requireClosedFromCallback(t, cc, client)
}

func TestPeerSharingCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := peersharing.NewConfig()
	cfg.ShareRequestFunc = func(
		ctx peersharing.CallbackContext, _ int,
	) ([]peersharing.PeerAddress, error) {
		cc.close(ctx.Context())
		return nil, nil
	}
	common := append(nodeToNode(), ouroboros.WithPeerSharing(true))
	client, server := connectPair(t, common, nil,
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithPeerSharingConfig(cfg)})
	cc.conn.Store(server)
	go func() { _, _ = client.PeerSharing().Client.GetPeers(1) }()
	requireClosedFromCallback(t, cc, client)
}

func TestLeiosNotifyCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := leiosnotify.NewConfig()
	cfg.RequestNextFunc = func(
		ctx leiosnotify.CallbackContext,
	) (protocol.Message, error) {
		cc.close(ctx.Context())
		return nil, protocol.ErrProtocolShuttingDown
	}
	clientCfg := leiosnotify.NewConfig()
	clientCfg.NotificationFunc = func(
		leiosnotify.CallbackContext, protocol.Message,
	) error {
		return nil
	}
	client, server := connectPair(t, nodeToNode(),
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLeiosNotifyConfig(clientCfg),
		},
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithLeiosNotifyConfig(cfg)})
	cc.conn.Store(server)
	go func() { _ = client.LeiosNotify().Client.Sync() }()
	requireClosedFromCallback(t, cc, client)
}

func TestLeiosFetchCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := leiosfetch.NewConfig()
	cfg.BlockRequestFunc = func(
		ctx leiosfetch.CallbackContext, _ pcommon.Point,
	) (protocol.Message, error) {
		cc.close(ctx.Context())
		return nil, protocol.ErrProtocolShuttingDown
	}
	client, server := connectPair(t, nodeToNode(),
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLeiosFetchConfig(leiosfetch.NewConfig()),
		},
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithLeiosFetchConfig(cfg)})
	cc.conn.Store(server)
	go func() {
		_, _ = client.LeiosFetch().Client.BlockRequest(
			context.Background(), pcommon.NewPointOrigin(),
		)
	}()
	requireClosedFromCallback(t, cc, client)
}

func TestLeiosVotesCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := leiosvotes.NewConfig()
	cfg.RequestNextFunc = func(
		ctx leiosvotes.CallbackContext, _ uint64,
	) ([]leiosvotes.Vote, error) {
		cc.close(ctx.Context())
		return nil, protocol.ErrProtocolShuttingDown
	}
	client, server := connectPair(t, nodeToNode(),
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLeiosVotesConfig(leiosvotes.NewConfig()),
		},
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithLeiosVotesConfig(cfg)})
	cc.conn.Store(server)
	go func() { _, _ = client.LeiosVotes().Client.RequestNext(1) }()
	requireClosedFromCallback(t, cc, client)
}

func TestPerasVotesCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := perasvotes.NewConfig()
	cfg.ObjectIDsFunc = func(
		perasvotes.CallbackContext, uint16, uint16,
	) ([]perasvotes.VoteID, error) {
		return nil, nil
	}
	cfg.AwaitObjectIDsFunc = func(
		_ context.Context, ctx perasvotes.CallbackContext, _ uint16,
	) ([]perasvotes.VoteID, error) {
		cc.close(ctx.Context())
		return nil, protocol.ErrProtocolShuttingDown
	}
	clientCfg := perasvotes.NewConfig()
	clientCfg.VoteFunc = func(
		perasvotes.CallbackContext, perasvotes.VoteID, perasvotes.VoteObject,
	) error {
		return nil
	}
	client, server := connectPair(t, nodeToNode(),
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithPerasVotesConfig(clientCfg)},
		[]ouroboros.ConnectionOptionFunc{ouroboros.WithPerasVotesConfig(cfg)})
	require.NotNil(t, server.PerasVotes(), "Peras votes not negotiated")
	cc.conn.Store(server)
	go func() { _ = client.PerasVotes().Client.Sync(context.Background()) }()
	requireClosedFromCallback(t, cc, client)
}

func TestLocalStateQueryCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := localstatequery.NewConfig()
	cfg.AcquireFunc = func(
		ctx localstatequery.CallbackContext,
		_ localstatequery.AcquireTarget,
		_ bool,
	) error {
		cc.close(ctx.Context())
		return nil
	}
	client, server := connectPair(t, nodeToClient(), nil,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalStateQueryConfig(cfg),
		})
	cc.conn.Store(server)
	go func() { _ = client.LocalStateQuery().Client.AcquireVolatileTip() }()
	requireClosedFromCallback(t, cc, client)
}

func TestLocalTxMonitorCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := localtxmonitor.NewConfig()
	cfg.GetMempoolFunc = func(
		ctx localtxmonitor.CallbackContext,
	) (uint64, uint32, []localtxmonitor.TxAndEraId, error) {
		cc.close(ctx.Context())
		return 0, 0, nil, nil
	}
	client, server := connectPair(t, nodeToClient(), nil,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalTxMonitorConfig(cfg),
		})
	cc.conn.Store(server)
	go func() { _ = client.LocalTxMonitor().Client.Acquire() }()
	requireClosedFromCallback(t, cc, client)
}

func TestLocalTxSubmissionCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := localtxsubmission.NewConfig()
	cfg.SubmitTxFunc = func(
		ctx localtxsubmission.CallbackContext,
		_ localtxsubmission.MsgSubmitTxTransaction,
	) error {
		cc.close(ctx.Context())
		return nil
	}
	client, server := connectPair(t, nodeToClient(), nil,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalTxSubmissionConfig(cfg),
		})
	cc.conn.Store(server)
	go func() {
		_ = client.LocalTxSubmission().Client.SubmitTx(6, []byte{0x80})
	}()
	requireClosedFromCallback(t, cc, client)
}

func TestLocalMessageSubmissionCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := localmessagesubmission.NewConfig()
	cfg.Authenticator = pcommon.NewNoOpAuthenticator(nil)
	cfg.TTLValidator = pcommon.NewNoOpTTLValidator(nil)
	cfg.SubmitMessageFunc = func(
		ctx localmessagesubmission.CallbackContext,
		_ *pcommon.DmqMessage,
	) pcommon.RejectReason {
		cc.close(ctx.Context())
		return nil
	}
	common := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithDMQ(true), ouroboros.WithNodeToNode(false),
	}
	// Both sides check a message before it goes further.
	client, server := connectPair(t, common,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalMessageSubmissionConfig(cfg),
		},
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalMessageSubmissionConfig(cfg),
		})
	cc.conn.Store(server)
	go func() {
		_ = client.LocalMessageSubmission().Client.SubmitMessage(
			&pcommon.DmqMessage{},
		)
	}()
	requireClosedFromCallback(t, cc, client)
}

func TestLocalMessageNotificationCallbackClosesConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	cc := newCallbackCloser()
	cfg := localmessagenotification.NewConfig()
	cfg.Authenticator = pcommon.NewNoOpAuthenticator(nil)
	cfg.ReplyMessagesFunc = func(
		ctx localmessagenotification.CallbackContext,
		_ []pcommon.DmqMessage,
		_ bool,
	) {
		cc.close(ctx.Context())
	}
	common := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithDMQ(true), ouroboros.WithNodeToNode(false),
	}
	client, server := connectPair(t, common,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalMessageNotificationConfig(cfg),
		},
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithLocalMessageNotificationConfig(
				localmessagenotification.NewConfig(),
			),
		})
	cc.conn.Store(client)
	go func() {
		_ = client.LocalMessageNotification().Client.RequestMessagesNonBlocking()
	}()
	requireClosedFromCallback(t, cc, server)
}
