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

package ledger_test

import (
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// TestPoolRelayHostnameBoundThroughPoolRule covers the DnsName decoder of
// libs/cardano-ledger-core/src/Cardano/Ledger/BaseTypes.hs: a dns_name is at
// most 64 bytes before decoder version 9 and at most 128 bytes from it. The
// wire decoder carries no protocol version, so the pool rule enforces the
// bound, as it does for the pool metadata URL.
func TestPoolRelayHostnameBoundThroughPoolRule(t *testing.T) {
	t.Parallel()

	eras := []struct {
		name string
		rule common.UtxoValidationRuleFunc
	}{
		{name: "shelley", rule: shelley.UtxoValidatePoolCertificates},
		{name: "allegra", rule: allegra.UtxoValidatePoolCertificates},
		{name: "mary", rule: mary.UtxoValidatePoolCertificates},
		{name: "alonzo", rule: alonzo.UtxoValidatePoolCertificates},
		{name: "babbage", rule: babbage.UtxoValidatePoolCertificates},
		{name: "conway", rule: conway.UtxoValidatePoolCertificates},
	}
	relayTypes := []struct {
		name  string
		relay func(hostname string) common.PoolRelay
	}{
		{
			name: "single host name",
			relay: func(hostname string) common.PoolRelay {
				port := uint32(3001)
				return common.PoolRelay{
					Type:     common.PoolRelayTypeSingleHostName,
					Port:     &port,
					Hostname: &hostname,
				}
			},
		},
		{
			name: "multi host name",
			relay: func(hostname string) common.PoolRelay {
				return common.PoolRelay{
					Type:     common.PoolRelayTypeMultiHostName,
					Hostname: &hostname,
				}
			},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().Build()

	for _, era := range eras {
		for _, relayType := range relayTypes {
			for _, test := range []struct {
				name          string
				protocolMajor uint
				length        int
				wantErr       bool
			}{
				{"Babbage 64 bytes", common.ProtocolVersionBabbage, 64, false},
				{"Babbage 65 bytes", common.ProtocolVersionBabbage, 65, true},
				{"Babbage 100 bytes", common.ProtocolVersionBabbage, 100, true},
				{"Babbage 128 bytes", common.ProtocolVersionBabbage, 128, true},
				{"Conway 64 bytes", common.ProtocolVersionConway, 64, false},
				{"Conway 100 bytes", common.ProtocolVersionConway, 100, false},
				{"Conway 128 bytes", common.ProtocolVersionConway, 128, false},
			} {
				t.Run(
					era.name+"/"+relayType.name+"/"+test.name,
					func(t *testing.T) {
						t.Parallel()
						cert := &common.PoolRegistrationCertificate{
							Operator:   poolKeyHash(0x01),
							VrfKeyHash: vrfKeyHash(0x02),
							Relays: []common.PoolRelay{
								relayType.relay(
									strings.Repeat("a", test.length),
								),
							},
						}
						err := era.rule(
							poolCertTx(cert),
							0,
							ls,
							conwayPparams(test.protocolMajor, 0),
						)
						if test.wantErr {
							require.ErrorIs(
								t,
								err,
								common.ErrPoolRelayHostnameTooLong,
							)
							return
						}
						require.NoError(t, err)
					},
				)
			}
		}
	}
}
