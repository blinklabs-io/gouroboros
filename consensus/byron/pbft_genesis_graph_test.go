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

package byron

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func signedGenesisDelegation(
	t *testing.T,
	protocolMagic uint32,
	epoch uint64,
	issuerKey []byte,
	issuerPrivateKey ed25519.PrivateKey,
	delegateKey []byte,
) ledgerbyron.ByronGenesisHeavyDelegation {
	t.Helper()
	epochCBOR, err := ledgerbyron.EncodeDelegationEpoch(epoch)
	require.NoError(t, err)
	inner := make([]byte, 0, 2+len(delegateKey)+len(epochCBOR))
	inner = append(inner, '0', '0')
	inner = append(inner, delegateKey...)
	inner = append(inner, epochCBOR...)
	innerCBOR, err := cbor.Encode(inner)
	require.NoError(t, err)
	protocolMagicCBOR, err := cbor.Encode(protocolMagic)
	require.NoError(t, err)
	signed := []byte{ledgerbyron.SignTagCertificate}
	signed = append(signed, protocolMagicCBOR...)
	signed = append(signed, innerCBOR...)
	return ledgerbyron.ByronGenesisHeavyDelegation{
		Cert:       hex.EncodeToString(ed25519.Sign(issuerPrivateKey, signed)),
		DelegatePk: base64.StdEncoding.EncodeToString(delegateKey),
		IssuerPk:   base64.StdEncoding.EncodeToString(issuerKey),
		Omega:      int(epoch),
	}
}

func genesisWithDelegationGraph(
	t *testing.T,
	delegates map[common.Blake2b224][]byte,
) ledgerbyron.ByronGenesis {
	t.Helper()
	const protocolMagic = uint32(42)
	issuers := make([][]byte, len(delegates))
	privateKeys := make([]ed25519.PrivateKey, len(delegates))
	genesisHashes := make([]common.Blake2b224, len(delegates))
	for i := 0; i < len(delegates); i++ {
		issuerKey, privateKey := deterministicPBFTVerificationKey(byte(i + 1))
		issuerHash, err := PBFTVerificationKeyHash(issuerKey)
		require.NoError(t, err)
		issuers[i] = issuerKey
		privateKeys[i] = privateKey
		genesisHashes[i] = issuerHash
	}
	heavyDelegation := make(
		map[string]ledgerbyron.ByronGenesisHeavyDelegation,
		len(delegates),
	)
	for i, issuerHash := range genesisHashes {
		delegateKey, ok := delegates[issuerHash]
		require.True(t, ok, "missing delegate for genesis issuer %s", issuerHash)
		heavyDelegation[issuerHash.String()] = signedGenesisDelegation(
			t,
			protocolMagic,
			0,
			issuers[i],
			privateKeys[i],
			delegateKey,
		)
	}
	return ledgerbyron.ByronGenesis{
		BlockVersionData: ledgerbyron.ByronGenesisBlockVersionData{
			SlotDuration: 20000,
		},
		ProtocolConsts: ledgerbyron.ByronGenesisProtocolConsts{
			K:             1,
			ProtocolMagic: int(protocolMagic),
		},
		HeavyDelegation: heavyDelegation,
	}
}

func TestNewByronConfigFromGenesisRejectsIssuerDelegateGraph(t *testing.T) {
	keyA, _ := deterministicPBFTVerificationKey(1)
	keyB, _ := deterministicPBFTVerificationKey(2)
	keyC, _ := deterministicPBFTVerificationKey(3)
	keyD, _ := deterministicPBFTVerificationKey(4)
	issuerA, err := PBFTVerificationKeyHash(keyA)
	require.NoError(t, err)
	issuerB, err := PBFTVerificationKeyHash(keyB)
	require.NoError(t, err)

	tests := []struct {
		name      string
		delegates map[common.Blake2b224][]byte
	}{
		{
			name: "self reference",
			delegates: map[common.Blake2b224][]byte{
				issuerA: keyA,
				issuerB: keyD,
			},
		},
		{
			name: "transitive chain",
			delegates: map[common.Blake2b224][]byte{
				issuerA: keyB,
				issuerB: keyC,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			genesis := genesisWithDelegationGraph(t, test.delegates)
			config, err := NewByronConfigFromGenesis(&genesis)
			require.ErrorContains(t, err, "genesis heavy-certificate issuer")
			require.Empty(t, config.GenesisDelegations)
		})
	}
}

func TestNewByronConfigFromGenesisAcceptsIndependentDelegations(t *testing.T) {
	keyA, _ := deterministicPBFTVerificationKey(1)
	keyB, _ := deterministicPBFTVerificationKey(2)
	keyC, _ := deterministicPBFTVerificationKey(3)
	keyD, _ := deterministicPBFTVerificationKey(4)
	issuerA, err := PBFTVerificationKeyHash(keyA)
	require.NoError(t, err)
	issuerB, err := PBFTVerificationKeyHash(keyB)
	require.NoError(t, err)
	genesis := genesisWithDelegationGraph(t, map[common.Blake2b224][]byte{
		issuerA: keyC,
		issuerB: keyD,
	})
	config, err := NewByronConfigFromGenesis(&genesis)
	require.NoError(t, err)
	state, err := NewPBFTDelegationState(config)
	require.NoError(t, err)
	require.Len(t, state.ActiveDelegations(), 2)
}
