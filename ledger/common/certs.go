// Copyright 2025 Blink Labs Software
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
	"errors"
	"fmt"
	"net"

	"github.com/blinklabs-io/gouroboros/cbor"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	CertificateTypeStakeRegistration               = 0
	CertificateTypeStakeDeregistration             = 1
	CertificateTypeStakeDelegation                 = 2
	CertificateTypePoolRegistration                = 3
	CertificateTypePoolRetirement                  = 4
	CertificateTypeGenesisKeyDelegation            = 5
	CertificateTypeMoveInstantaneousRewards        = 6
	CertificateTypeRegistration                    = 7
	CertificateTypeDeregistration                  = 8
	CertificateTypeVoteDelegation                  = 9
	CertificateTypeStakeVoteDelegation             = 10
	CertificateTypeStakeRegistrationDelegation     = 11
	CertificateTypeVoteRegistrationDelegation      = 12
	CertificateTypeStakeVoteRegistrationDelegation = 13
	CertificateTypeAuthCommitteeHot                = 14
	CertificateTypeResignCommitteeCold             = 15
	CertificateTypeRegistrationDrep                = 16
	CertificateTypeDeregistrationDrep              = 17
	CertificateTypeUpdateDrep                      = 18
)

type CertificateWrapper struct {
	Type        uint
	Certificate Certificate
}

func (c *CertificateWrapper) UnmarshalCBOR(data []byte) error {
	// Determine cert type
	certType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var tmpCert Certificate
	switch certType {
	case CertificateTypeStakeRegistration:
		tmpCert = &StakeRegistrationCertificate{}
	case CertificateTypeStakeDeregistration:
		tmpCert = &StakeDeregistrationCertificate{}
	case CertificateTypeStakeDelegation:
		tmpCert = &StakeDelegationCertificate{}
	case CertificateTypePoolRegistration:
		tmpCert = &PoolRegistrationCertificate{}
	case CertificateTypePoolRetirement:
		tmpCert = &PoolRetirementCertificate{}
	case CertificateTypeGenesisKeyDelegation:
		tmpCert = &GenesisKeyDelegationCertificate{}
	case CertificateTypeMoveInstantaneousRewards:
		tmpCert = &MoveInstantaneousRewardsCertificate{}
	case CertificateTypeRegistration:
		tmpCert = &RegistrationCertificate{}
	case CertificateTypeDeregistration:
		tmpCert = &DeregistrationCertificate{}
	case CertificateTypeVoteDelegation:
		tmpCert = &VoteDelegationCertificate{}
	case CertificateTypeStakeVoteDelegation:
		tmpCert = &StakeVoteDelegationCertificate{}
	case CertificateTypeStakeRegistrationDelegation:
		tmpCert = &StakeRegistrationDelegationCertificate{}
	case CertificateTypeVoteRegistrationDelegation:
		tmpCert = &VoteRegistrationDelegationCertificate{}
	case CertificateTypeStakeVoteRegistrationDelegation:
		tmpCert = &StakeVoteRegistrationDelegationCertificate{}
	case CertificateTypeAuthCommitteeHot:
		tmpCert = &AuthCommitteeHotCertificate{}
	case CertificateTypeResignCommitteeCold:
		tmpCert = &ResignCommitteeColdCertificate{}
	case CertificateTypeRegistrationDrep:
		tmpCert = &RegistrationDrepCertificate{}
	case CertificateTypeDeregistrationDrep:
		tmpCert = &DeregistrationDrepCertificate{}
	case CertificateTypeUpdateDrep:
		tmpCert = &UpdateDrepCertificate{}
	default:
		return fmt.Errorf("unknown certificate type: %d", certType)
	}
	// Decode cert
	if _, err := cbor.Decode(data, tmpCert); err != nil {
		return err
	}
	// certType is known within uint range
	c.Type = uint(certType) // #nosec G115
	c.Certificate = tmpCert
	return nil
}

func (c *CertificateWrapper) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(c.Certificate)
}

type Certificate interface {
	isCertificate()
	Cbor() []byte
	Utxorpc() *utxorpc.Certificate
	Type() uint
}

const (
	DrepTypeAddrKeyHash  = 0
	DrepTypeScriptHash   = 1
	DrepTypeAbstain      = 2
	DrepTypeNoConfidence = 3
)

type Drep struct {
	Type       int
	Credential []byte
}

func (d *Drep) UnmarshalCBOR(data []byte) error {
	drepType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch drepType {
	case DrepTypeAddrKeyHash, DrepTypeScriptHash:
		d.Type = drepType
		tmpData := struct {
			cbor.StructAsArray
			Type       int
			Credential []byte
		}{}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		d.Credential = tmpData.Credential[:]
	case DrepTypeAbstain, DrepTypeNoConfidence:
		d.Type = drepType
	default:
		return fmt.Errorf("unknown drep type: %d", drepType)
	}
	return nil
}

type StakeRegistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType          uint
	StakeRegistration Credential
}

func (c StakeRegistrationCertificate) isCertificate() {}

func (c *StakeRegistrationCertificate) UnmarshalCBOR(cborData []byte) error {
	type tStakeRegistrationCertificate StakeRegistrationCertificate
	var tmp tStakeRegistrationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeRegistrationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeRegistrationCertificate) Utxorpc() *utxorpc.Certificate {
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_StakeRegistration{
			StakeRegistration: c.StakeRegistration.Utxorpc(),
		},
	}
}

func (c *StakeRegistrationCertificate) Type() uint {
	return c.CertType
}

type StakeDeregistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType            uint
	StakeDeregistration Credential
}

func (c StakeDeregistrationCertificate) isCertificate() {}

func (c *StakeDeregistrationCertificate) UnmarshalCBOR(cborData []byte) error {
	type tStakeDeregistrationCertificate StakeDeregistrationCertificate
	var tmp tStakeDeregistrationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeDeregistrationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeDeregistrationCertificate) Utxorpc() *utxorpc.Certificate {
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_StakeDeregistration{
			StakeDeregistration: c.StakeDeregistration.Utxorpc(),
		},
	}
}

func (c *StakeDeregistrationCertificate) Type() uint {
	return c.CertType
}

type StakeDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential *Credential
	PoolKeyHash     PoolKeyHash
}

func (c StakeDelegationCertificate) isCertificate() {}

func (c *StakeDelegationCertificate) UnmarshalCBOR(cborData []byte) error {
	type tStakeDelegationCertificate StakeDelegationCertificate
	var tmp tStakeDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_StakeDelegation{
			StakeDelegation: &utxorpc.StakeDelegationCert{
				StakeCredential: c.StakeCredential.Utxorpc(),
				PoolKeyhash:     c.PoolKeyHash[:],
			},
		},
	}
}

func (c *StakeDelegationCertificate) Type() uint {
	return c.CertType
}

type (
	PoolKeyHash      = Blake2b224
	PoolMetadataHash = Blake2b256
	VrfKeyHash       = Blake2b256
)

type PoolMetadata struct {
	cbor.StructAsArray
	Url  string
	Hash PoolMetadataHash
}

func (p *PoolMetadata) Utxorpc() *utxorpc.PoolMetadata {
	return &utxorpc.PoolMetadata{
		Url:  p.Url,
		Hash: p.Hash[:],
	}
}

const (
	PoolRelayTypeSingleHostAddress = 0
	PoolRelayTypeSingleHostName    = 1
	PoolRelayTypeMultiHostName     = 2
)

type PoolRelay struct {
	Type     int
	Port     *uint32
	Ipv4     *net.IP
	Ipv6     *net.IP
	Hostname *string
}

func (p *PoolRelay) UnmarshalCBOR(data []byte) error {
	tmpId, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	p.Type = tmpId
	switch tmpId {
	case PoolRelayTypeSingleHostAddress:
		var tmpData struct {
			cbor.StructAsArray
			Type uint
			Port *uint32
			Ipv4 *net.IP
			Ipv6 *net.IP
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		p.Port = tmpData.Port
		p.Ipv4 = tmpData.Ipv4
		p.Ipv6 = tmpData.Ipv6
	case PoolRelayTypeSingleHostName:
		var tmpData struct {
			cbor.StructAsArray
			Type     uint
			Port     *uint32
			Hostname *string
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		p.Port = tmpData.Port
		p.Hostname = tmpData.Hostname
	case PoolRelayTypeMultiHostName:
		var tmpData struct {
			cbor.StructAsArray
			Type     uint
			Hostname *string
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		p.Hostname = tmpData.Hostname
	default:
		return fmt.Errorf("invalid relay type: %d", tmpId)
	}
	return nil
}

func (p *PoolRelay) Utxorpc() *utxorpc.Relay {
	ret := &utxorpc.Relay{}
	if p.Port != nil {
		ret.Port = *p.Port
	}
	if p.Ipv4 != nil {
		ret.IpV4 = []byte(*p.Ipv4)
	}
	if p.Ipv6 != nil {
		ret.IpV6 = []byte(*p.Ipv6)
	}
	if p.Port != nil {
		ret.Port = *p.Port
	}
	return ret
}

type PoolRegistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType      uint
	Operator      PoolKeyHash
	VrfKeyHash    VrfKeyHash
	Pledge        uint64
	Cost          uint64
	Margin        cbor.Rat
	RewardAccount AddrKeyHash
	PoolOwners    []AddrKeyHash
	Relays        []PoolRelay
	PoolMetadata  *PoolMetadata
}

func (c PoolRegistrationCertificate) isCertificate() {}

func (c *PoolRegistrationCertificate) UnmarshalCBOR(cborData []byte) error {
	type tPoolRegistrationCertificate PoolRegistrationCertificate
	var tmp tPoolRegistrationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = PoolRegistrationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *PoolRegistrationCertificate) Utxorpc() *utxorpc.Certificate {
	tmpPoolOwners := make([][]byte, len(c.PoolOwners))
	for i, owner := range c.PoolOwners {
		tmpPoolOwners[i] = owner[:]
	}
	tmpRelays := make([]*utxorpc.Relay, len(c.Relays))
	for i, relay := range c.Relays {
		tmpRelays[i] = relay.Utxorpc()
	}
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_PoolRegistration{
			PoolRegistration: &utxorpc.PoolRegistrationCert{
				Operator:   c.Operator[:],
				VrfKeyhash: c.VrfKeyHash[:],
				Pledge:     c.Pledge,
				Cost:       c.Cost,
				// #nosec G115
				Margin: &utxorpc.RationalNumber{
					Numerator:   int32(c.Margin.Num().Int64()),
					Denominator: uint32(c.Margin.Denom().Uint64()),
				},
				RewardAccount: c.RewardAccount[:],
				PoolOwners:    tmpPoolOwners,
				Relays:        tmpRelays,
				PoolMetadata:  c.PoolMetadata.Utxorpc(),
			},
		},
	}
}

func (c *PoolRegistrationCertificate) Type() uint {
	return c.CertType
}

type PoolRetirementCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType    uint
	PoolKeyHash PoolKeyHash
	Epoch       uint64
}

func (c PoolRetirementCertificate) isCertificate() {}

func (c *PoolRetirementCertificate) UnmarshalCBOR(cborData []byte) error {
	type tPoolRetirementCertificate PoolRetirementCertificate
	var tmp tPoolRetirementCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = PoolRetirementCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *PoolRetirementCertificate) Utxorpc() *utxorpc.Certificate {
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_PoolRetirement{
			PoolRetirement: &utxorpc.PoolRetirementCert{
				PoolKeyhash: c.PoolKeyHash[:],
				Epoch:       c.Epoch,
			},
		},
	}
}

func (c *PoolRetirementCertificate) Type() uint {
	return c.CertType
}

type GenesisKeyDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType            uint
	GenesisHash         []byte
	GenesisDelegateHash []byte
	VrfKeyHash          VrfKeyHash
}

func (c GenesisKeyDelegationCertificate) isCertificate() {}

func (c *GenesisKeyDelegationCertificate) UnmarshalCBOR(cborData []byte) error {
	type tGenesisKeyDelegationCertificate GenesisKeyDelegationCertificate
	var tmp tGenesisKeyDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = GenesisKeyDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *GenesisKeyDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_GenesisKeyDelegation{
			GenesisKeyDelegation: &utxorpc.GenesisKeyDelegationCert{
				GenesisHash:         c.GenesisHash[:],
				GenesisDelegateHash: c.GenesisDelegateHash[:],
				VrfKeyhash:          c.VrfKeyHash[:],
			},
		},
	}
}

func (c *GenesisKeyDelegationCertificate) Type() uint {
	return c.CertType
}

type MirSource int32

const (
	MirSourceUnspecified MirSource = 0
	MirSourceReserves    MirSource = 1
	MirSourceTreasury    MirSource = 2
)

type MoveInstantaneousRewardsCertificateReward struct {
	Source   uint
	Rewards  map[*Credential]uint64
	OtherPot uint64
}

func (r *MoveInstantaneousRewardsCertificateReward) UnmarshalCBOR(
	data []byte,
) error {
	// Try to parse as map
	tmpMapData := struct {
		cbor.StructAsArray
		Source  uint
		Rewards map[*Credential]uint64
	}{}
	if _, err := cbor.Decode(data, &tmpMapData); err == nil {
		r.Rewards = tmpMapData.Rewards
		r.Source = tmpMapData.Source
		return nil
	}
	// Try to parse as coin
	tmpCoinData := struct {
		cbor.StructAsArray
		Source uint
		Coin   uint64
	}{}
	if _, err := cbor.Decode(data, &tmpCoinData); err == nil {
		r.OtherPot = tmpCoinData.Coin
		r.Source = tmpMapData.Source
		return nil
	}
	return errors.New("failed to decode as known types")
}

type MoveInstantaneousRewardsCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType uint
	Reward   MoveInstantaneousRewardsCertificateReward
}

func (c MoveInstantaneousRewardsCertificate) isCertificate() {}

func (c *MoveInstantaneousRewardsCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tMoveInstantaneousRewardsCertificate MoveInstantaneousRewardsCertificate
	var tmp tMoveInstantaneousRewardsCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = MoveInstantaneousRewardsCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *MoveInstantaneousRewardsCertificate) Utxorpc() *utxorpc.Certificate {
	tmpMirTargets := []*utxorpc.MirTarget{}
	for stakeCred, deltaCoin := range c.Reward.Rewards {
		tmpMirTargets = append(
			tmpMirTargets,
			&utxorpc.MirTarget{
				StakeCredential: stakeCred.Utxorpc(),
				// potential integer overflow
				// #nosec G115
				DeltaCoin: int64(deltaCoin),
			},
		)
	}
	return &utxorpc.Certificate{
		Certificate: &utxorpc.Certificate_MirCert{
			MirCert: &utxorpc.MirCert{
				// potential integer overflow
				// #nosec G115
				From:     utxorpc.MirSource(c.Reward.Source),
				To:       tmpMirTargets,
				OtherPot: c.Reward.OtherPot,
			},
		},
	}
}

func (c *MoveInstantaneousRewardsCertificate) Type() uint {
	return c.CertType
}

type RegistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	Amount          int64
}

func (c RegistrationCertificate) isCertificate() {}

func (c *RegistrationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tRegistrationCertificate RegistrationCertificate
	var tmp tRegistrationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = RegistrationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *RegistrationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *RegistrationCertificate) Type() uint {
	return c.CertType
}

type DeregistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	Amount          int64
}

func (c DeregistrationCertificate) isCertificate() {}

func (c *DeregistrationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tDeregistrationCertificate DeregistrationCertificate
	var tmp tDeregistrationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = DeregistrationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *DeregistrationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *DeregistrationCertificate) Type() uint {
	return c.CertType
}

type VoteDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	Drep            Drep
}

func (c VoteDelegationCertificate) isCertificate() {}

func (c *VoteDelegationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tVoteDelegationCertificate VoteDelegationCertificate
	var tmp tVoteDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = VoteDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *VoteDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *VoteDelegationCertificate) Type() uint {
	return c.CertType
}

type StakeVoteDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	PoolKeyHash     []byte
	Drep            Drep
}

func (c StakeVoteDelegationCertificate) isCertificate() {}

func (c *StakeVoteDelegationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tStakeVoteDelegationCertificate StakeVoteDelegationCertificate
	var tmp tStakeVoteDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeVoteDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeVoteDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *StakeVoteDelegationCertificate) Type() uint {
	return c.CertType
}

type StakeRegistrationDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	PoolKeyHash     []byte
	Amount          int64
}

func (c StakeRegistrationDelegationCertificate) isCertificate() {}

func (c *StakeRegistrationDelegationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tStakeRegistrationDelegationCertificate StakeRegistrationDelegationCertificate
	var tmp tStakeRegistrationDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeRegistrationDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeRegistrationDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *StakeRegistrationDelegationCertificate) Type() uint {
	return c.CertType
}

type VoteRegistrationDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	Drep            Drep
	Amount          int64
}

func (c VoteRegistrationDelegationCertificate) isCertificate() {}

func (c *VoteRegistrationDelegationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tVoteRegistrationDelegationCertificate VoteRegistrationDelegationCertificate
	var tmp tVoteRegistrationDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = VoteRegistrationDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *VoteRegistrationDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *VoteRegistrationDelegationCertificate) Type() uint {
	return c.CertType
}

type StakeVoteRegistrationDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType        uint
	StakeCredential Credential
	PoolKeyHash     PoolKeyHash
	Drep            Drep
	Amount          int64
}

func (c StakeVoteRegistrationDelegationCertificate) isCertificate() {}

func (c *StakeVoteRegistrationDelegationCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tStakeVoteRegistrationDelegationCertificate StakeVoteRegistrationDelegationCertificate
	var tmp tStakeVoteRegistrationDelegationCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = StakeVoteRegistrationDelegationCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *StakeVoteRegistrationDelegationCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *StakeVoteRegistrationDelegationCertificate) Type() uint {
	return c.CertType
}

type AuthCommitteeHotCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType       uint
	ColdCredential Credential
	HostCredential Credential
}

func (c AuthCommitteeHotCertificate) isCertificate() {}

func (c *AuthCommitteeHotCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tAuthCommitteeHotCertificate AuthCommitteeHotCertificate
	var tmp tAuthCommitteeHotCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = AuthCommitteeHotCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *AuthCommitteeHotCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *AuthCommitteeHotCertificate) Type() uint {
	return c.CertType
}

type ResignCommitteeColdCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType       uint
	ColdCredential Credential
	Anchor         *GovAnchor
}

func (c ResignCommitteeColdCertificate) isCertificate() {}

func (c *ResignCommitteeColdCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tResignCommitteeColdCertificate ResignCommitteeColdCertificate
	var tmp tResignCommitteeColdCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = ResignCommitteeColdCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *ResignCommitteeColdCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *ResignCommitteeColdCertificate) Type() uint {
	return c.CertType
}

type RegistrationDrepCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType       uint
	DrepCredential Credential
	Amount         int64
	Anchor         *GovAnchor
}

func (c RegistrationDrepCertificate) isCertificate() {}

func (c *RegistrationDrepCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tRegistrationDrepCertificate RegistrationDrepCertificate
	var tmp tRegistrationDrepCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = RegistrationDrepCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *RegistrationDrepCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *RegistrationDrepCertificate) Type() uint {
	return c.CertType
}

type DeregistrationDrepCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType       uint
	DrepCredential Credential
	Amount         int64
}

func (c DeregistrationDrepCertificate) isCertificate() {}

func (c *DeregistrationDrepCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tDeregistrationDrepCertificate DeregistrationDrepCertificate
	var tmp tDeregistrationDrepCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = DeregistrationDrepCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *DeregistrationDrepCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *DeregistrationDrepCertificate) Type() uint {
	return c.CertType
}

type UpdateDrepCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CertType       uint
	DrepCredential Credential
	Anchor         *GovAnchor
}

func (c UpdateDrepCertificate) isCertificate() {}

func (c *UpdateDrepCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tUpdateDrepCertificate UpdateDrepCertificate
	var tmp tUpdateDrepCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*c = UpdateDrepCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

func (c *UpdateDrepCertificate) Utxorpc() *utxorpc.Certificate {
	// TODO (#850)
	return nil
}

func (c *UpdateDrepCertificate) Type() uint {
	return c.CertType
}
