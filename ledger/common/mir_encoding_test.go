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

package common_test

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestMIRRewardEncodingPreservesReferenceWire(t *testing.T) {
	key := &common.Credential{
		CredType:   0,
		Credential: common.NewBlake2b224([]byte(strings.Repeat("\xab", 28))),
	}
	script := &common.Credential{
		CredType:   1,
		Credential: common.NewBlake2b224([]byte(strings.Repeat("\xcd", 28))),
	}
	beyond := new(big.Int).Lsh(big.NewInt(1), 64)
	keyWire := "8200581c" + strings.Repeat("ab", 28)
	scriptWire := "8201581c" + strings.Repeat("cd", 28)
	tests := []struct {
		name   string
		reward common.MoveInstantaneousRewardsCertificateReward
		wire   string
	}{
		{
			"zero transfer",
			common.MoveInstantaneousRewardsCertificateReward{Source: 0},
			"820000",
		},
		{
			"treasury transfer",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:   1,
				OtherPot: 77,
			},
			"8201184d",
		},
		{
			"maximum transfer",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:   1,
				OtherPot: ^uint64(0),
			},
			"82011bffffffffffffffff",
		},
		{
			"empty rewards",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:  0,
				Rewards: map[*common.Credential]*big.Int{},
			},
			"8200a0",
		},
		{
			"positive reward",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:  0,
				Rewards: map[*common.Credential]*big.Int{key: big.NewInt(77)},
			},
			"8200a1" + keyWire + "184d",
		},
		{
			"script negative reward",
			common.MoveInstantaneousRewardsCertificateReward{
				Source: 1,
				Rewards: map[*common.Credential]*big.Int{
					script: big.NewInt(-1),
				},
			},
			"8201a1" + scriptWire + "20",
		},
		{
			"large positive reward",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:  0,
				Rewards: map[*common.Credential]*big.Int{key: beyond},
			},
			"8200a1" + keyWire + "c249010000000000000000",
		},
		{
			"large negative reward",
			common.MoveInstantaneousRewardsCertificateReward{
				Source: 0,
				Rewards: map[*common.Credential]*big.Int{
					key: new(
						big.Int,
					).Neg(new(big.Int).Add(beyond, big.NewInt(1))),
				},
			},
			"8200a1" + keyWire + "c349010000000000000000",
		},
		{
			"ordered credentials",
			common.MoveInstantaneousRewardsCertificateReward{
				Source: 1,
				Rewards: map[*common.Credential]*big.Int{
					script: big.NewInt(-1),
					key:    big.NewInt(77),
				},
			},
			"8201a2" + keyWire + "184d" + scriptWire + "20",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected, err := hex.DecodeString(test.wire)
			require.NoError(t, err)
			encoded, err := cbor.Encode(test.reward)
			require.NoError(t, err)
			require.Equal(t, expected, encoded)
			var decoded common.MoveInstantaneousRewardsCertificateReward
			_, err = cbor.Decode(expected, &decoded)
			require.NoError(t, err)
			require.Equal(t, test.reward.Source, decoded.Source)
			require.Equal(t, test.reward.OtherPot, decoded.OtherPot)
			require.Equal(t, test.reward.Rewards == nil, decoded.Rewards == nil)
			require.Len(t, decoded.Rewards, len(test.reward.Rewards))
			for credential, amount := range decoded.Rewards {
				found := false
				for original, delta := range test.reward.Rewards {
					if credential.CredType == original.CredType &&
						credential.Credential == original.Credential {
						require.Zero(t, amount.Cmp(delta))
						found = true
						break
					}
				}
				require.True(t, found)
				credential.SetCbor(nil)
			}
			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err)
			require.Equal(t, expected, reencoded)
			cert := &common.MoveInstantaneousRewardsCertificate{
				CertType: 6,
				Reward:   test.reward,
			}
			outer, err := cbor.Encode(
				&common.CertificateWrapper{Certificate: cert},
			)
			require.NoError(t, err)
			outerExpected := append([]byte{0x82, 0x06}, expected...)
			require.Equal(t, outerExpected, outer)
			var wrapper common.CertificateWrapper
			_, err = cbor.Decode(outerExpected, &wrapper)
			require.NoError(t, err)
			decodedValue := wrapper.Certificate
			decodedCert, ok := decodedValue.(*common.MoveInstantaneousRewardsCertificate)
			require.True(t, ok)
			require.Equal(t, uint(6), wrapper.Type)
			require.Equal(t, uint(6), decodedCert.Type())
			decodedCert.SetCbor(nil)
			for credential := range decodedCert.Reward.Rewards {
				credential.SetCbor(nil)
			}
			outer, err = cbor.Encode(&wrapper)
			require.NoError(t, err)
			require.Equal(t, outerExpected, outer)
		})
	}
}

func TestMIRRewardEncodingRejectsInvalidTargets(t *testing.T) {
	credential := &common.Credential{CredType: 0}
	duplicate := &common.Credential{CredType: 0}
	tests := []struct {
		name   string
		reward common.MoveInstantaneousRewardsCertificateReward
	}{
		{
			"invalid source",
			common.MoveInstantaneousRewardsCertificateReward{Source: 2},
		},
		{
			"ambiguous target",
			common.MoveInstantaneousRewardsCertificateReward{
				Rewards:  map[*common.Credential]*big.Int{},
				OtherPot: 1,
			},
		},
		{
			"nil credential",
			common.MoveInstantaneousRewardsCertificateReward{
				Rewards: map[*common.Credential]*big.Int{nil: big.NewInt(1)},
			},
		},
		{
			"invalid credential kind",
			common.MoveInstantaneousRewardsCertificateReward{
				Rewards: map[*common.Credential]*big.Int{
					&common.Credential{CredType: 2}: big.NewInt(1),
				},
			},
		},
		{
			"nil delta",
			common.MoveInstantaneousRewardsCertificateReward{
				Rewards: map[*common.Credential]*big.Int{credential: nil},
			},
		},
		{
			"duplicate credentials",
			common.MoveInstantaneousRewardsCertificateReward{
				Rewards: map[*common.Credential]*big.Int{
					credential: big.NewInt(1),
					duplicate:  big.NewInt(2),
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode(test.reward)
			require.Error(t, err)
			require.Nil(t, encoded)
			cert := common.MoveInstantaneousRewardsCertificate{
				CertType: 6,
				Reward:   test.reward,
			}
			encoded, err = cbor.Encode(cert)
			require.Error(t, err)
			require.Nil(t, encoded)
			if test.name == "duplicate credentials" {
				var duplicateError common.DuplicateLogicalMapKeyError
				require.ErrorAs(t, err, &duplicateError)
			}
		})
	}
}

func TestMIRRewardDecodeReplacesTarget(t *testing.T) {
	for _, test := range []struct {
		name, first, second string
		wantMap             bool
		coin                uint64
	}{
		{"coin to map", "8201184d", "8200a0", true, 0},
		{
			"map to coin",
			"8200a18200581c" + strings.Repeat("ab", 28) + "184d",
			"820100",
			false,
			0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reward common.MoveInstantaneousRewardsCertificateReward
			first, err := hex.DecodeString(test.first)
			require.NoError(t, err)
			second, err := hex.DecodeString(test.second)
			require.NoError(t, err)
			_, err = cbor.Decode(first, &reward)
			require.NoError(t, err)
			_, err = cbor.Decode(second, &reward)
			require.NoError(t, err)
			require.Equal(t, test.wantMap, reward.Rewards != nil)
			require.Equal(t, test.coin, reward.OtherPot)
			encoded, err := cbor.Encode(reward)
			require.NoError(t, err)
			require.Equal(t, second, encoded)
		})
	}
}

func TestMIRRewardDecodeRejectsInvalidPotAtomically(t *testing.T) {
	for _, wire := range []string{"820200", "8202a0"} {
		t.Run(wire, func(t *testing.T) {
			original := common.MoveInstantaneousRewardsCertificateReward{
				Source:   1,
				OtherPot: 77,
			}
			reward := original
			encoded, err := hex.DecodeString(wire)
			require.NoError(t, err)
			_, err = cbor.Decode(encoded, &reward)
			require.ErrorContains(t, err, "invalid MIR source pot")
			require.Equal(t, original, reward)
		})
	}
}

func TestMIRCertificateEncodingServesShelleyTransaction(t *testing.T) {
	credential := &common.Credential{
		CredType:   0,
		Credential: common.NewBlake2b224([]byte(strings.Repeat("\xab", 28))),
	}
	for _, test := range []struct {
		name   string
		reward common.MoveInstantaneousRewardsCertificateReward
		wire   string
	}{
		{
			"coin",
			common.MoveInstantaneousRewardsCertificateReward{
				Source:   1,
				OtherPot: 77,
			},
			"82068201184d",
		},
		{
			"rewards",
			common.MoveInstantaneousRewardsCertificateReward{
				Source: 0,
				Rewards: map[*common.Credential]*big.Int{
					credential: big.NewInt(77),
				},
			},
			"82068200a18200581c" + strings.Repeat("ab", 28) + "184d",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cert := &common.MoveInstantaneousRewardsCertificate{
				CertType: 6,
				Reward:   test.reward,
			}
			body := shelley.ShelleyTransactionBody{
				TxInputs: shelley.NewShelleyTransactionInputSet(
					[]shelley.ShelleyTransactionInput{},
				),
				TxOutputs: []shelley.ShelleyTransactionOutput{}, TxFee: 2, Ttl: 100,
				TxCertificates: []common.CertificateWrapper{
					{Certificate: cert},
				},
			}
			tx := shelley.ShelleyTransaction{Body: body}
			encoded, err := cbor.Encode(&tx)
			require.NoError(t, err)
			var envelope []cbor.RawMessage
			_, err = cbor.Decode(encoded, &envelope)
			require.NoError(t, err)
			require.Len(t, envelope, 3)
			var fields map[uint]cbor.RawMessage
			_, err = cbor.Decode(envelope[0], &fields)
			require.NoError(t, err)
			expected, err := hex.DecodeString("81" + test.wire)
			require.NoError(t, err)
			require.Equal(t, expected, []byte(fields[4]))
			decoded, err := shelley.NewShelleyTransactionFromCbor(encoded)
			require.NoError(t, err)
			require.Len(t, decoded.Certificates(), 1)
			decodedValue := decoded.Certificates()[0]
			decodedCert, ok := decodedValue.(*common.MoveInstantaneousRewardsCertificate)
			require.True(t, ok)
			require.Equal(t, test.reward.Source, decodedCert.Reward.Source)
			require.Equal(t, test.reward.OtherPot, decodedCert.Reward.OtherPot)
			require.Equal(
				t,
				test.reward.Rewards == nil,
				decodedCert.Reward.Rewards == nil,
			)
			require.Len(t, decodedCert.Reward.Rewards, len(test.reward.Rewards))
			for key, delta := range decodedCert.Reward.Rewards {
				require.Equal(t, credential.CredType, key.CredType)
				require.Equal(t, credential.Credential, key.Credential)
				require.Zero(t, delta.Cmp(big.NewInt(77)))
				key.SetCbor(nil)
			}
			decoded.SetCbor(nil)
			decoded.Body.SetCbor(nil)
			decodedCert.SetCbor(nil)
			reencoded, err := cbor.Encode(decoded)
			require.NoError(t, err)
			require.Equal(t, encoded, reencoded)
		})
	}
}

func TestMIRRewardDecodeRejectsCoercedScalarsAtomically(t *testing.T) {
	for _, wire := range []string{
		"82f600", "82f700", "82f6a0", "82f7a0",
		"8200c24101", "82c24100a0",
		"81a0", "83000000", "d864820000",
	} {
		t.Run(wire, func(t *testing.T) {
			original := common.MoveInstantaneousRewardsCertificateReward{
				Source:   1,
				OtherPot: 77,
			}
			reward := original
			encoded, err := hex.DecodeString(wire)
			require.NoError(t, err)
			_, err = cbor.Decode(encoded, &reward)
			require.Error(t, err)
			require.Equal(t, original, reward)
		})
	}
}
