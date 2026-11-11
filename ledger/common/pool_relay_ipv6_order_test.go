package common_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reference encoder: ipv6ToBytes in Cardano/Ledger/Binary/Encoding/Encoder.hs
// writes each 32-bit word little-endian; ipv4ToBytes is network order.
const (
	relayWireHex = "8400190bb944" + "2d3212c9" + "50" +
		"f0190120a6240170ff040054969931fe"
	relayIpv6Std = "2001:19f0:7001:24a6:5400:4ff:fe31:9996"
)

func TestPoolRelayIpv6DecodesToStandardOrder(t *testing.T) {
	t.Parallel()
	raw, err := hex.DecodeString(relayWireHex)
	require.NoError(t, err)
	var relay common.PoolRelay
	_, err = cbor.Decode(raw, &relay)
	require.NoError(t, err)
	require.NotNil(t, relay.Ipv6)
	assert.Equal(t, relayIpv6Std, relay.Ipv6.String())
	require.NotNil(t, relay.Ipv4)
	assert.Equal(t, "45.50.18.201", relay.Ipv4.String())
}

func TestPoolRelayIpv6ReencodesToWireOrder(t *testing.T) {
	t.Parallel()
	raw, err := hex.DecodeString(relayWireHex)
	require.NoError(t, err)
	var relay common.PoolRelay
	_, err = cbor.Decode(raw, &relay)
	require.NoError(t, err)
	out, err := relay.MarshalCBOR()
	require.NoError(t, err)
	assert.Equal(t, raw, out)
}

func TestPoolRelayIpv6FromJSONEncodesToWireOrder(t *testing.T) {
	t.Parallel()
	var relay common.PoolRelay
	require.NoError(t, relay.UnmarshalJSON([]byte(
		`{"single host address":{"port":3001,"IPv4":"45.50.18.201","IPv6":"`+
			relayIpv6Std+`"}}`)))
	out, err := relay.MarshalCBOR()
	require.NoError(t, err)
	assert.Equal(t, relayWireHex, hex.EncodeToString(out))
}

// UTxO RPC carries the wire bytes, as cardano-rpc (ipv6ToBytes) and pallas do.
func TestPoolRelayIpv6UtxorpcWireOrder(t *testing.T) {
	t.Parallel()
	raw, err := hex.DecodeString(relayWireHex)
	require.NoError(t, err)
	var relay common.PoolRelay
	_, err = cbor.Decode(raw, &relay)
	require.NoError(t, err)
	r, err := relay.Utxorpc()
	require.NoError(t, err)
	assert.Equal(t, raw[len(raw)-16:], r.IpV6)
	assert.Equal(t, relayIpv6Std, relay.Ipv6.String())
}
