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
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type pbftScheduledDelegation struct {
	activationSlot uint64
	delegator      common.Blake2b224
	delegate       common.Blake2b224
}

type pbftDelegationKeyEpoch struct {
	epoch     uint64
	delegator common.Blake2b224
}

// PBFTDelegationState is the Byron heavyweight-delegation ledger view used by
// PBFT header validation. Values are immutable: every update returns a new
// state so callers can publish it only after their surrounding transaction
// commits.
type PBFTDelegationState struct {
	activeDelegations    map[common.Blake2b224]common.Blake2b224
	delegationSlots      map[common.Blake2b224]uint64
	scheduledDelegations []pbftScheduledDelegation
	keyEpochDelegations  map[pbftDelegationKeyEpoch]struct{}
	allowedDelegators    map[common.Blake2b224]struct{}
	protocolMagic        uint32
	securityParam        uint64
}

// NewPBFTDelegationState constructs the initial Byron delegation state. Every
// configured genesis key initially delegates to itself, then the genesis
// heavyweight-delegation view overrides those entries at slot zero.
func NewPBFTDelegationState(
	config ByronConfig,
) (PBFTDelegationState, error) {
	if config.SecurityParam == 0 {
		return PBFTDelegationState{}, errors.New(
			"byron PBFT delegation security parameter must be greater than zero",
		)
	}
	if len(config.GenesisKeyHashes) == 0 {
		return PBFTDelegationState{}, errors.New(
			"byron PBFT delegation genesis issuer set is empty",
		)
	}
	state := PBFTDelegationState{
		activeDelegations: make(
			map[common.Blake2b224]common.Blake2b224,
			len(config.GenesisKeyHashes),
		),
		delegationSlots: make(
			map[common.Blake2b224]uint64,
			len(config.GenesisKeyHashes),
		),
		keyEpochDelegations: make(
			map[pbftDelegationKeyEpoch]struct{},
			len(config.GenesisKeyHashes),
		),
		allowedDelegators: make(
			map[common.Blake2b224]struct{},
			len(config.GenesisKeyHashes),
		),
		protocolMagic: config.ProtocolMagic,
		securityParam: config.SecurityParam,
	}
	for _, hashBytes := range config.GenesisKeyHashes {
		if len(hashBytes) != common.Blake2b224Size {
			return PBFTDelegationState{}, fmt.Errorf(
				"invalid Byron PBFT genesis issuer hash length: got %d, expected %d",
				len(hashBytes),
				common.Blake2b224Size,
			)
		}
		hash := common.NewBlake2b224(hashBytes)
		if _, exists := state.allowedDelegators[hash]; exists {
			return PBFTDelegationState{}, fmt.Errorf(
				"duplicate Byron PBFT genesis issuer %s",
				hash.String(),
			)
		}
		state.allowedDelegators[hash] = struct{}{}
		state.activeDelegations[hash] = hash
		state.delegationSlots[hash] = 0
	}
	for delegator, delegate := range config.GenesisDelegations {
		if _, ok := state.allowedDelegators[delegator]; !ok {
			return PBFTDelegationState{}, fmt.Errorf(
				"Byron PBFT genesis delegation has unknown delegator %s",
				delegator.String(),
			)
		}
		if state.delegateIsActiveForOther(delegator, delegate) {
			return PBFTDelegationState{}, fmt.Errorf(
				"Byron PBFT genesis delegate %s is already active",
				delegate.String(),
			)
		}
		state.activeDelegations[delegator] = delegate
		state.keyEpochDelegations[pbftDelegationKeyEpoch{
			epoch:     0,
			delegator: delegator,
		}] = struct{}{}
	}
	return state, nil
}

// ActiveDelegations returns an independent copy of the current mapping from
// genesis verification-key hashes to active block-signing delegate hashes.
func (s PBFTDelegationState) ActiveDelegations() map[common.Blake2b224]common.Blake2b224 {
	ret := make(
		map[common.Blake2b224]common.Blake2b224,
		len(s.activeDelegations),
	)
	for delegator, delegate := range s.activeDelegations {
		ret[delegator] = delegate
	}
	return ret
}

// Tick activates all scheduled delegations due at currentSlot and prunes
// schedules and epoch duplicate records that can no longer affect the ledger.
func (s PBFTDelegationState) Tick(
	currentEpoch uint64,
	currentSlot uint64,
) PBFTDelegationState {
	next := s.clone()
	for _, delegation := range next.scheduledDelegations {
		if delegation.activationSlot <= currentSlot {
			next.activate(delegation)
		}
	}
	kept := next.scheduledDelegations[:0]
	for _, delegation := range next.scheduledDelegations {
		if delegation.activationSlot > currentSlot {
			kept = append(kept, delegation)
		}
	}
	next.scheduledDelegations = kept
	for keyEpoch := range next.keyEpochDelegations {
		if keyEpoch.epoch < currentEpoch {
			delete(next.keyEpochDelegations, keyEpoch)
		}
	}
	return next
}

// ApplyPayload schedules the certificates in a Byron main-block delegation
// payload and performs the delegation tick for that block. The update is
// atomic: malformed or invalid certificates leave the input state unchanged.
func (s PBFTDelegationState) ApplyPayload(
	currentEpoch uint64,
	currentSlot uint64,
	payload []any,
) (PBFTDelegationState, error) {
	next := s.clone()
	for i, rawCertificate := range payload {
		if err := next.scheduleCertificate(
			currentEpoch,
			currentSlot,
			rawCertificate,
		); err != nil {
			return PBFTDelegationState{}, fmt.Errorf(
				"schedule Byron PBFT delegation certificate %d: %w",
				i,
				err,
			)
		}
	}
	return next.Tick(currentEpoch, currentSlot), nil
}

func (s PBFTDelegationState) clone() PBFTDelegationState {
	next := PBFTDelegationState{
		activeDelegations: make(
			map[common.Blake2b224]common.Blake2b224,
			len(s.activeDelegations),
		),
		delegationSlots: make(
			map[common.Blake2b224]uint64,
			len(s.delegationSlots),
		),
		scheduledDelegations: append(
			[]pbftScheduledDelegation(nil),
			s.scheduledDelegations...,
		),
		keyEpochDelegations: make(
			map[pbftDelegationKeyEpoch]struct{},
			len(s.keyEpochDelegations),
		),
		allowedDelegators: make(
			map[common.Blake2b224]struct{},
			len(s.allowedDelegators),
		),
		protocolMagic: s.protocolMagic,
		securityParam: s.securityParam,
	}
	for delegator, delegate := range s.activeDelegations {
		next.activeDelegations[delegator] = delegate
	}
	for delegator, slot := range s.delegationSlots {
		next.delegationSlots[delegator] = slot
	}
	for keyEpoch := range s.keyEpochDelegations {
		next.keyEpochDelegations[keyEpoch] = struct{}{}
	}
	for delegator := range s.allowedDelegators {
		next.allowedDelegators[delegator] = struct{}{}
	}
	return next
}

func (s *PBFTDelegationState) scheduleCertificate(
	currentEpoch uint64,
	currentSlot uint64,
	rawCertificate any,
) error {
	certificate, ok := rawCertificate.([]any)
	if !ok || len(certificate) != 4 {
		return fmt.Errorf(
			"invalid certificate shape: got %T with %d elements",
			rawCertificate,
			len(certificate),
		)
	}
	delegationEpoch, err := extractUint64(certificate[0])
	if err != nil {
		return fmt.Errorf("decode delegation epoch: %w", err)
	}
	if delegationEpoch < currentEpoch ||
		delegationEpoch-currentEpoch > 1 {
		return fmt.Errorf(
			"delegation epoch %d is not current epoch %d or its successor",
			delegationEpoch,
			currentEpoch,
		)
	}
	issuerKey, ok := certificate[1].([]byte)
	if !ok || len(issuerKey) != 64 {
		return fmt.Errorf(
			"invalid issuer verification key: got %T with length %d",
			certificate[1],
			len(issuerKey),
		)
	}
	delegateKey, ok := certificate[2].([]byte)
	if !ok || len(delegateKey) != 64 {
		return fmt.Errorf(
			"invalid delegate verification key: got %T with length %d",
			certificate[2],
			len(delegateKey),
		)
	}
	certificateSignature, ok := certificate[3].([]byte)
	if !ok || len(certificateSignature) != 64 {
		return fmt.Errorf(
			"invalid certificate signature: got %T with length %d",
			certificate[3],
			len(certificateSignature),
		)
	}
	delegator, err := PBFTVerificationKeyHash(issuerKey)
	if err != nil {
		return fmt.Errorf("derive delegation issuer: %w", err)
	}
	if _, ok := s.allowedDelegators[delegator]; !ok {
		return fmt.Errorf(
			"delegation issuer %s is not a configured genesis key",
			delegator.String(),
		)
	}
	keyEpoch := pbftDelegationKeyEpoch{
		epoch:     delegationEpoch,
		delegator: delegator,
	}
	if _, exists := s.keyEpochDelegations[keyEpoch]; exists {
		return fmt.Errorf(
			"genesis issuer %s already delegated for epoch %d",
			delegator.String(),
			delegationEpoch,
		)
	}
	if s.securityParam > math.MaxUint64/2 ||
		currentSlot > math.MaxUint64-2*s.securityParam {
		return errors.New("Byron PBFT delegation activation slot overflows")
	}
	activationSlot := currentSlot + 2*s.securityParam
	for _, scheduled := range s.scheduledDelegations {
		if scheduled.delegator == delegator &&
			scheduled.activationSlot == activationSlot {
			return fmt.Errorf(
				"genesis issuer %s already delegated in slot %d",
				delegator.String(),
				currentSlot,
			)
		}
	}
	validator := NewHeaderValidator(ByronConfig{
		ProtocolMagic: s.protocolMagic,
	})
	if err := validator.validateDelegationCertSignature(
		issuerKey,
		delegateKey,
		certificateSignature,
		delegationEpoch,
	); err != nil {
		return fmt.Errorf("validate delegation certificate: %w", err)
	}
	delegate, err := PBFTVerificationKeyHash(delegateKey)
	if err != nil {
		return fmt.Errorf("derive delegation delegate: %w", err)
	}
	s.scheduledDelegations = append(
		s.scheduledDelegations,
		pbftScheduledDelegation{
			activationSlot: activationSlot,
			delegator:      delegator,
			delegate:       delegate,
		},
	)
	s.keyEpochDelegations[keyEpoch] = struct{}{}
	return nil
}

func (s *PBFTDelegationState) activate(
	delegation pbftScheduledDelegation,
) {
	previousSlot := s.delegationSlots[delegation.delegator]
	if s.delegateIsActiveForOther(
		delegation.delegator,
		delegation.delegate,
	) || (delegation.activationSlot != 0 &&
		previousSlot >= delegation.activationSlot) {
		return
	}
	s.activeDelegations[delegation.delegator] = delegation.delegate
	s.delegationSlots[delegation.delegator] = delegation.activationSlot
}

func (s PBFTDelegationState) delegateIsActiveForOther(
	delegator common.Blake2b224,
	delegate common.Blake2b224,
) bool {
	for activeDelegator, activeDelegate := range s.activeDelegations {
		if activeDelegator != delegator && activeDelegate == delegate {
			return true
		}
	}
	return false
}
