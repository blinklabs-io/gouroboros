// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package byron

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func realPBFTHeaderFixture(
	t *testing.T,
) (*ledgerbyron.ByronMainBlockHeader, ByronConfig, PBFTIssuer) {
	t.Helper()
	blockBytes, err := hex.DecodeString(testByronMainBlockHex)
	require.NoError(t, err)
	block, err := ledgerbyron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	header := block.BlockHeader
	inner, ok := header.ConsensusData.BlockSig[1].([]any)
	require.True(t, ok)
	certificate, ok := inner[0].([]any)
	require.True(t, ok)
	delegateKey, ok := certificate[2].([]byte)
	require.True(t, ok)
	require.Len(t, delegateKey, 64)
	genesisHash, err := PBFTVerificationKeyHash(header.ConsensusData.PubKey)
	require.NoError(t, err)
	delegateHash, err := PBFTVerificationKeyHash(delegateKey)
	require.NoError(t, err)
	issuer := PBFTIssuer{
		GenesisKeyHash:  genesisHash,
		DelegateKeyHash: delegateHash,
	}
	config := ByronConfig{
		ProtocolMagic:    header.ProtocolMagic,
		SecurityParam:    testByronSecurityParam,
		GenesisKeyHashes: [][]byte{issuer.GenesisKeyHash.Bytes()},
		GenesisDelegations: map[common.Blake2b224]common.Blake2b224{
			issuer.GenesisKeyHash: issuer.DelegateKeyHash,
		},
	}
	return header, config, issuer
}

func TestValidatePBFTHeaderRealMainnet(t *testing.T) {
	header, config, expectedIssuer := realPBFTHeaderFixture(t)
	issuer, err := ValidatePBFTHeader(header, config)
	require.NoError(t, err)
	require.Equal(t, expectedIssuer, issuer)
}

func TestValidatePBFTHeaderCryptoUsesActiveDelegateWithoutRevalidatingCert(
	t *testing.T,
) {
	header, config, issuer := realPBFTHeaderFixture(t)
	inner := header.ConsensusData.BlockSig[1].([]any)
	certificate := inner[0].([]any)
	certificate[0] = header.ConsensusData.SlotId.Epoch + 100
	certificateSignature := certificate[3].([]byte)
	certificateSignature[0] ^= 0xff

	_, err := ValidatePBFTHeaderCrypto(header, config)
	require.NoError(t, err)

	_, err = ValidatePBFTHeader(header, config)
	require.NoError(t, err)
	wrongActiveDelegate := config
	wrongDelegations := make(
		map[common.Blake2b224]common.Blake2b224,
	)
	wrongDelegations[issuer.GenesisKeyHash] = common.Blake2b224Hash(
		[]byte("wrong delegate"),
	)
	wrongActiveDelegate.GenesisDelegations = wrongDelegations
	_, err = ValidatePBFTHeader(header, wrongActiveDelegate)
	require.ErrorContains(t, err, "active delegation does not authorize")
}

func TestPBFTIssuerFromHeaderRealMainnet(t *testing.T) {
	header, _, expectedIssuer := realPBFTHeaderFixture(t)

	issuer, err := PBFTIssuerFromHeader(header)
	require.NoError(t, err)
	require.Equal(t, expectedIssuer.GenesisKeyHash, issuer.GenesisKeyHash)
	require.Equal(t, expectedIssuer.DelegateKeyHash, issuer.DelegateKeyHash)
	require.NotEqual(t, issuer.GenesisKeyHash, issuer.DelegateKeyHash)
}

func TestPBFTIssuerFromHeaderRejectsMalformedIdentity(t *testing.T) {
	header, _, _ := realPBFTHeaderFixture(t)
	_, err := PBFTIssuerFromHeader(nil)
	require.ErrorContains(t, err, "nil")

	header.ConsensusData.PubKey = header.ConsensusData.PubKey[:32]
	_, err = PBFTIssuerFromHeader(header)
	require.ErrorContains(t, err, "genesis issuer key length")
}

func TestPBFTIssuerFromHeaderIsParseOnly(t *testing.T) {
	header, _, expectedIssuer := realPBFTHeaderFixture(t)
	inner := header.ConsensusData.BlockSig[1].([]any)
	certificate := inner[0].([]any)
	certificate[0] = header.ConsensusData.SlotId.Epoch + 1
	signature := inner[1].([]byte)
	signature[0] ^= 0xff

	issuer, err := PBFTIssuerFromHeader(header)
	require.NoError(t, err)
	require.Equal(t, expectedIssuer, issuer)
}

func TestValidateProxySignatureRejectsTypeConstraintMismatch(t *testing.T) {
	header, config, _ := realPBFTHeaderFixture(t)
	blockSig := append([]any(nil), header.ConsensusData.BlockSig...)
	blockSig[0] = uint64(byronSigTypeLight)
	input := &ValidateHeaderInput{
		Slot:          header.SlotNumber(),
		BlockNumber:   header.BlockNumber(),
		ProtocolMagic: header.ProtocolMagic,
		IssuerPubKey:  header.ConsensusData.PubKey[:32],
		BlockSig:      blockSig,
		HeaderCbor:    header.Cbor(),
	}

	err := NewHeaderValidator(config).validateBlockSignature(input)
	require.ErrorContains(t, err, "2-element epoch range")
}

func TestValidatePBFTHeaderRejectsInvalidVectors(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *ledgerbyron.ByronMainBlockHeader, *ByronConfig)
		wantError string
	}{
		{
			name: "protocol magic",
			mutate: func(_ *testing.T, header *ledgerbyron.ByronMainBlockHeader, _ *ByronConfig) {
				header.ProtocolMagic++
			},
			wantError: "protocol magic",
		},
		{
			name: "block signature",
			mutate: func(t *testing.T, header *ledgerbyron.ByronMainBlockHeader, _ *ByronConfig) {
				inner, ok := header.ConsensusData.BlockSig[1].([]any)
				require.True(t, ok)
				signature, ok := inner[1].([]byte)
				require.True(t, ok)
				signature[0] ^= 0xff
			},
			wantError: "block signature",
		},
		{
			name: "unknown genesis issuer",
			mutate: func(_ *testing.T, _ *ledgerbyron.ByronMainBlockHeader, config *ByronConfig) {
				unknown := common.Blake2b224Hash([]byte("unknown genesis issuer"))
				config.GenesisKeyHashes = [][]byte{unknown.Bytes()}
			},
			wantError: "genesis issuer",
		},
		{
			name: "replaced or revoked delegate",
			mutate: func(t *testing.T, header *ledgerbyron.ByronMainBlockHeader, config *ByronConfig) {
				genesisHash, err := PBFTVerificationKeyHash(
					header.ConsensusData.PubKey,
				)
				require.NoError(t, err)
				config.GenesisDelegations[genesisHash] = common.Blake2b224Hash(
					[]byte("different delegate"),
				)
			},
			wantError: "active delegation",
		},
		{
			name: "unsupported lightweight delegation",
			mutate: func(_ *testing.T, header *ledgerbyron.ByronMainBlockHeader, _ *ByronConfig) {
				header.ConsensusData.BlockSig[0] = uint64(byronSigTypeLight)
			},
			wantError: "unsupported Byron PBFT signature type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header, config, _ := realPBFTHeaderFixture(t)
			test.mutate(t, header, &config)
			_, err := ValidatePBFTHeader(header, config)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidatePBFTCertificateEpoch(t *testing.T) {
	tests := []struct {
		name            string
		activationEpoch uint64
		headerEpoch     uint64
		wantError       string
	}{
		{
			name:            "activated in earlier epoch",
			activationEpoch: 0,
			headerEpoch:     10,
		},
		{
			name:            "activated in header epoch",
			activationEpoch: 10,
			headerEpoch:     10,
		},
		{
			name:            "activation is in future",
			activationEpoch: 11,
			headerEpoch:     10,
			wantError:       "not active",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePBFTCertificateEpoch(
				test.activationEpoch,
				test.headerEpoch,
			)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestPBFTStateTransitionVectors(t *testing.T) {
	issuerA := common.Blake2b224Hash([]byte("issuer-a"))
	issuerB := common.Blake2b224Hash([]byte("issuer-b"))
	issuerC := common.Blake2b224Hash([]byte("issuer-c"))
	state, err := NewPBFTState(nil, 10)
	require.NoError(t, err)

	for _, issuer := range []common.Blake2b224{issuerA, issuerB, issuerA} {
		state, err = state.Transition(issuer)
		require.NoError(t, err)
	}
	require.Equal(
		t,
		[]common.Blake2b224{issuerA, issuerB, issuerA},
		state.SignatureHistory(),
	)

	_, err = state.Transition(issuerA)
	require.ErrorContains(t, err, "signature threshold")
	require.Equal(
		t,
		[]common.Blake2b224{issuerA, issuerB, issuerA},
		state.SignatureHistory(),
		"a rejected transition must not mutate prior state",
	)

	for range 8 {
		state, err = state.Observe(issuerC)
		require.NoError(t, err)
	}
	require.Len(t, state.SignatureHistory(), 10)
	require.Equal(t, issuerB, state.SignatureHistory()[0])
}

func TestPBFTStateRejectsInvalidState(t *testing.T) {
	issuer := common.Blake2b224Hash([]byte("issuer"))
	_, err := NewPBFTState([]common.Blake2b224{issuer}, 0)
	require.ErrorContains(t, err, "security parameter")
	_, err = NewPBFTState(
		[]common.Blake2b224{issuer, issuer, issuer},
		2,
	)
	require.ErrorContains(t, err, "issuer history")
	_, err = (PBFTState{}).Observe(issuer)
	require.ErrorContains(t, err, "security parameter")
}

func TestPBFTMaxSignatures(t *testing.T) {
	tests := []struct {
		securityParam uint64
		want          uint64
	}{
		{securityParam: 1, want: 0},
		{securityParam: 4, want: 0},
		{securityParam: 5, want: 1},
		{securityParam: 100, want: 22},
		{securityParam: 2160, want: 475},
	}
	for _, test := range tests {
		require.Equal(
			t,
			test.want,
			pbftMaxSignatures(
				test.securityParam,
				DefaultPBFTSignatureThresholdNumerator,
				DefaultPBFTSignatureThresholdDenominator,
			),
		)
	}
}

func TestPBFTMaxSignaturesUsesConfiguredThresholdRatios(t *testing.T) {
	tests := []struct {
		name        string
		numerator   uint64
		denominator uint64
		want        uint64
	}{
		{name: "0.10", numerator: 1, denominator: 10, want: 1},
		{name: "0.22", numerator: 22, denominator: 100, want: 2},
		{name: "0.50", numerator: 1, denominator: 2, want: 5},
		{name: "1.1", numerator: 11, denominator: 10, want: 11},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(
				t,
				test.want,
				pbftMaxSignatures(10, test.numerator, test.denominator),
			)
		})
	}
}

func TestPBFTStateUsesConfiguredSignatureThreshold(t *testing.T) {
	issuer := common.Blake2b224Hash([]byte("issuer"))
	config := ByronConfig{
		SecurityParam:                     10,
		PBFTSignatureThresholdNumerator:   1,
		PBFTSignatureThresholdDenominator: 2,
	}
	state, err := NewPBFTStateFromConfig(nil, config)
	require.NoError(t, err)
	state, err = state.Transition(issuer)
	require.NoError(t, err)
	state, err = state.Transition(issuer)
	require.NoError(t, err)
	state, err = state.Transition(issuer)
	require.NoError(t, err)
	state, err = state.Transition(issuer)
	require.NoError(t, err)
	state, err = state.Transition(issuer)
	require.NoError(t, err)
	_, err = state.Transition(issuer)
	require.ErrorContains(t, err, "signature threshold")

	_, err = NewPBFTStateWithThreshold(nil, 10, 1, 0)
	require.ErrorContains(t, err, "signature threshold")
}

func TestNewByronConfigFromGenesisBuildsPBFTDelegationView(t *testing.T) {
	genesis, err := ledgerbyron.NewByronGenesisFromReader(
		strings.NewReader(testByronGenesisJSON),
	)
	require.NoError(t, err)
	config, err := NewByronConfigFromGenesis(&genesis)
	require.NoError(t, err)
	require.Len(t, config.GenesisKeyHashes, len(genesis.BootStakeholders))
	require.Len(t, config.GenesisDelegations, len(genesis.BootStakeholders))

	for genesisHashHex, delegation := range genesis.HeavyDelegation {
		genesisHashBytes, err := hex.DecodeString(genesisHashHex)
		require.NoError(t, err)
		genesisHash := common.NewBlake2b224(genesisHashBytes)
		delegateKey, err := base64.StdEncoding.DecodeString(
			delegation.DelegatePk,
		)
		require.NoError(t, err)
		delegateHash, err := PBFTVerificationKeyHash(delegateKey)
		require.NoError(t, err)
		if _, isIssuer := config.GenesisDelegations[genesisHash]; isIssuer {
			require.Equal(t, delegateHash, config.GenesisDelegations[genesisHash])
		}
	}
	state, err := NewPBFTDelegationState(config)
	require.NoError(t, err)
	require.Len(t, state.ActiveDelegations(), len(genesis.BootStakeholders))
	unassignedBootStakeholder := common.Blake2b224{}
	require.Equal(
		t,
		state.ActiveDelegations()[unassignedBootStakeholder],
		unassignedBootStakeholder,
	)
}

func TestGenesisDerivedConfigValidatesRealPBFTHeader(t *testing.T) {
	genesis, err := ledgerbyron.NewByronGenesisFromReader(
		strings.NewReader(testByronGenesisJSON),
	)
	require.NoError(t, err)
	config, err := NewByronConfigFromGenesis(&genesis)
	require.NoError(t, err)

	blockBytes, err := hex.DecodeString(testByronMainBlockHex)
	require.NoError(t, err)
	block, err := ledgerbyron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	issuer, err := ValidatePBFTHeader(block.BlockHeader, config)
	require.NoError(t, err)
	require.Contains(t, config.GenesisKeyHashes, issuer.GenesisKeyHash.Bytes())
	require.Equal(
		t,
		issuer.DelegateKeyHash,
		config.GenesisDelegations[issuer.GenesisKeyHash],
	)
}

func TestNewByronConfigFromGenesisRejectsInvalidDelegationCertificate(t *testing.T) {
	genesis, err := ledgerbyron.NewByronGenesisFromReader(
		strings.NewReader(testByronGenesisJSON),
	)
	require.NoError(t, err)
	const genesisHash = "af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1"
	delegation := genesis.HeavyDelegation[genesisHash]
	delegation.Cert = "00" + delegation.Cert[2:]
	genesis.HeavyDelegation[genesisHash] = delegation

	_, err = NewByronConfigFromGenesis(&genesis)
	require.ErrorContains(t, err, "validate delegation certificate")
}

func TestNewByronConfigFromGenesisRejectsNonBootStakeholderIssuer(
	t *testing.T,
) {
	genesis, err := ledgerbyron.NewByronGenesisFromReader(
		strings.NewReader(testByronGenesisJSON),
	)
	require.NoError(t, err)
	delete(
		genesis.BootStakeholders,
		"1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b",
	)
	_, err = NewByronConfigFromGenesis(&genesis)
	require.ErrorContains(t, err, "not a boot stakeholder")
}

func TestNewByronConfigFromGenesisRejectsIssuerAsDelegate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]ledgerbyron.ByronGenesisHeavyDelegation)
	}{
		{
			name: "self reference",
			mutate: func(
				delegations map[string]ledgerbyron.ByronGenesisHeavyDelegation,
			) {
				for issuer, delegation := range delegations {
					delegation.DelegatePk = delegation.IssuerPk
					delegations[issuer] = delegation
					return
				}
			},
		},
		{
			name: "transitive chain",
			mutate: func(
				delegations map[string]ledgerbyron.ByronGenesisHeavyDelegation,
			) {
				issuers := make([]string, 0, len(delegations))
				for issuer := range delegations {
					issuers = append(issuers, issuer)
				}
				require.GreaterOrEqual(t, len(issuers), 2)
				first := delegations[issuers[0]]
				second := delegations[issuers[1]]
				first.DelegatePk = second.IssuerPk
				delegations[issuers[0]] = first
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			genesis, err := ledgerbyron.NewByronGenesisFromReader(
				strings.NewReader(testByronGenesisJSON),
			)
			require.NoError(t, err)
			test.mutate(genesis.HeavyDelegation)
			_, err = NewByronConfigFromGenesis(&genesis)
			require.ErrorContains(t, err, "heavy-certificate graph")
		})
	}
}
