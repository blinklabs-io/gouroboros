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

package common

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
	"google.golang.org/protobuf/proto"
)

func TestStakeVoteRegistrationUtxorpcPoolIdentity(t *testing.T) {
	stake := Credential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: Blake2b224{0x11},
	}
	pool := PoolKeyHash{0x22}
	for _, drepType := range []int{
		DrepTypeAddrKeyHash, DrepTypeScriptHash,
		DrepTypeAbstain, DrepTypeNoConfidence,
	} {
		t.Run(fmt.Sprint(drepType), func(t *testing.T) {
			drep := Drep{Type: drepType}
			if drepType == DrepTypeAddrKeyHash ||
				drepType == DrepTypeScriptHash {
				drep.Credential = bytes.Repeat([]byte{0x33}, 28)
			}
			cert := StakeVoteRegistrationDelegationCertificate{
				StakeCredential: stake, PoolKeyHash: pool, Drep: drep,
				Amount: 2_000_000,
			}
			converted, err := cert.Utxorpc()
			require.NoError(t, err)
			wire, err := proto.Marshal(converted)
			require.NoError(t, err)
			var decoded utxorpc.Certificate
			require.NoError(t, proto.Unmarshal(wire, &decoded))
			got := decoded.GetStakeVoteRegDelegCert()
			require.NotNil(t, got)
			require.Equal(t, pool.Bytes(), got.GetPoolKeyhash())
			require.Equal(t, stake.Credential.Bytes(),
				got.GetStakeCredential().GetAddrKeyHash())
			require.NotNil(t, got.GetDrep())
			switch drepType {
			case DrepTypeAddrKeyHash:
				require.Equal(t, drep.Credential, got.Drep.GetAddrKeyHash())
			case DrepTypeScriptHash:
				require.Equal(t, drep.Credential, got.Drep.GetScriptHash())
			case DrepTypeAbstain:
				require.IsType(t, &utxorpc.DRep_Abstain{}, got.Drep.Drep)
			case DrepTypeNoConfidence:
				require.IsType(t, &utxorpc.DRep_NoConfidence{}, got.Drep.Drep)
			}
		})
	}
}

func TestRegistrationDelegationUtxorpcDeposits(t *testing.T) {
	stake := Credential{CredType: CredentialTypeAddrKeyHash,
		Credential: Blake2b224{0x11}}
	for _, amount := range []int64{0, 2_000_000, math.MaxInt64} {
		t.Run(fmt.Sprint(amount), func(t *testing.T) {
			certs := []Certificate{
				&RegistrationCertificate{
					StakeCredential: stake, Amount: amount,
				},
				&DeregistrationCertificate{
					StakeCredential: stake, Amount: amount,
				},
				&RegistrationDrepCertificate{
					DrepCredential: stake, Amount: amount,
				},
				&DeregistrationDrepCertificate{
					DrepCredential: stake, Amount: amount,
				},
				&StakeRegistrationDelegationCertificate{
					StakeCredential: stake, PoolKeyHash: PoolKeyHash{0x22},
					Amount: amount,
				},
				&VoteRegistrationDelegationCertificate{
					StakeCredential: stake, Drep: Drep{Type: DrepTypeAbstain},
					Amount: amount,
				},
				&StakeVoteRegistrationDelegationCertificate{
					StakeCredential: stake, PoolKeyHash: PoolKeyHash{0x22},
					Drep: Drep{Type: DrepTypeAbstain}, Amount: amount,
				},
			}
			for _, cert := range certs {
				t.Run(fmt.Sprintf("%T", cert), func(t *testing.T) {
					converted, err := cert.Utxorpc()
					require.NoError(t, err)
					wire, err := proto.Marshal(converted)
					require.NoError(t, err)
					var decoded utxorpc.Certificate
					require.NoError(t, proto.Unmarshal(wire, &decoded))
					var coin *utxorpc.BigInt
					switch value := decoded.Certificate.(type) {
					case *utxorpc.Certificate_RegCert:
						coin = value.RegCert.GetCoin()
					case *utxorpc.Certificate_UnregCert:
						coin = value.UnregCert.GetCoin()
					case *utxorpc.Certificate_RegDrepCert:
						coin = value.RegDrepCert.GetCoin()
					case *utxorpc.Certificate_UnregDrepCert:
						coin = value.UnregDrepCert.GetCoin()
					case *utxorpc.Certificate_StakeRegDelegCert:
						coin = value.StakeRegDelegCert.GetCoin()
					case *utxorpc.Certificate_VoteRegDelegCert:
						coin = value.VoteRegDelegCert.GetCoin()
					case *utxorpc.Certificate_StakeVoteRegDelegCert:
						coin = value.StakeVoteRegDelegCert.GetCoin()
					default:
						t.Fatalf("unexpected certificate %T", value)
					}
					require.NotNil(t, coin, "deposit must be present")
					require.Equal(t, amount, coin.GetInt())
				})
			}
		})
	}
}
