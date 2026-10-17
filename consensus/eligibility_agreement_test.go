// Copyright 2026 Blink Labs Software
// SPDX-License-Identifier: Apache-2.0

package consensus

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

func TestLeaderEligibilityElectionValidatorAgreement(t *testing.T) {
	// The existing strict reader checks every SHA256SUMS entry, including JSON.
	// No oracle generator, adjacent checkout, or self-derived expectation is used.
	_ = e34TestRows(t, "pallas/node-vectors.tsv")
	data, err := fixtures.EmbeddedFixtures().
		ReadFile("upstream/e34/jormanager/node-schedules.json")
	require.NoError(t, err)
	var corpus struct {
		Cases []struct {
			ActiveStake   string   `json:"activeStake"`
			AssignedSlots []uint64 `json:"assignedSlots"`
			VRFSkeyHex    string   `json:"vrfSkeyHex"`
			DDenominator  string   `json:"dDenominator"`
			DNumerator    string   `json:"dNumerator"`
			FDenominator  string   `json:"fDenominator"`
			FNumerator    string   `json:"fNumerator"`
			FirstSlot     uint64   `json:"firstSlot"`
			Mode          string   `json:"mode"`
			NonceHex      string   `json:"nonceHex"`
			PoolStake     string   `json:"poolStake"`
			SlotCount     uint64   `json:"slotCount"`
		} `json:"cases"`
		Metadata json.RawMessage `json:"metadata"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&corpus))
	require.ErrorIs(t, decoder.Decode(new(any)), io.EOF)
	require.Len(t, corpus.Cases, 21)
	modes := map[ConsensusMode]bool{}
	for _, c := range corpus.Cases {
		if c.SlotCount != 120 || c.PoolStake != "1" || c.ActiveStake != "3" ||
			c.FNumerator != "1" ||
			c.FDenominator != "20" {
			continue
		}
		require.Equal(t, "0", c.DNumerator)
		require.Equal(t, "1", c.DDenominator)
		mode := ConsensusModeCPraos
		switch c.Mode {
		case "praos":
		case "tpraos":
			mode = ConsensusModeTPraos
		default:
			t.Fatalf("unknown mode %q", c.Mode)
		}
		require.False(t, modes[mode])
		modes[mode] = true
		t.Run(c.Mode, func(t *testing.T) {
			secret, err := hex.DecodeString(c.VRFSkeyHex)
			require.NoError(t, err)
			require.Len(t, secret, 64)
			signer, err := NewSimpleVRFSigner(secret[:32])
			require.NoError(t, err)
			require.Equal(t, secret[32:], signer.PublicKey())
			nonce, err := hex.DecodeString(c.NonceHex)
			require.NoError(t, err)
			require.Len(t, nonce, 32)
			require.NotEmpty(t, c.AssignedSlots)
			membership := map[uint64]bool{}
			for _, slot := range c.AssignedSlots {
				require.GreaterOrEqual(t, slot, c.FirstSlot)
				require.Less(t, slot, c.FirstSlot+c.SlotCount)
				require.False(t, membership[slot])
				membership[slot] = true
			}
			unassigned := c.FirstSlot
			for membership[unassigned] {
				unassigned++
			}
			require.Less(t, unassigned, c.FirstSlot+c.SlotCount)
			f := big.NewRat(1, 20)
			config := NetworkConfig{
				ActiveSlotCoeff:   common.GenesisRat{Rat: f},
				SlotsPerKESPeriod: 129600,
				MaxKESEvolutions:  62,
			}
			validator := NewHeaderValidatorWithMode(config, mode)
			for _, slot := range []uint64{c.AssignedSlots[0], unassigned} {
				expected := membership[slot]
				t.Run(
					new(big.Int).SetUint64(slot).String(),
					func(t *testing.T) {
						election, err := IsSlotLeaderWithMode(
							slot,
							nonce,
							1,
							3,
							f,
							signer,
							mode,
						)
						require.NoError(t, err)
						require.Equal(t, expected, election.Eligible)
						require.Len(t, election.Proof, 80)
						require.Len(t, election.Output, 64)
						component, err := IsSlotLeaderFromComponentsWithMode(
							election.Output,
							1,
							3,
							f,
							mode,
						)
						require.NoError(t, err)
						require.Equal(t, expected, component)
						proofInput := &ValidateHeaderInput{
							Slot:       slot,
							EpochNonce: nonce,
							VrfKey:     signer.PublicKey(),
							VrfProof:   election.Proof,
							VrfOutput:  election.Output,
							PoolStake:  1,
							TotalStake: 3,
						}
						verified, err := validator.validateVRFProof(proofInput)
						require.NoError(t, err)
						require.Equal(t, election.Output, verified)
						leadershipErr := validator.validateLeadership(
							proofInput,
							verified,
						)
						if expected {
							require.NoError(t, leadershipErr)
						} else {
							require.ErrorContains(t, leadershipErr, "does not satisfy leader eligibility")
						}

						// Real depth produces the 448-byte KES signature enforced by validation.
						kesSecret, kesPublic, err := kes.KeyGen(
							kes.CardanoKesDepth,
							make([]byte, 32),
						)
						require.NoError(t, err)
						kesSigner := &SimpleKESSigner{
							sk:        kesSecret,
							publicKey: kesPublic,
						}
						coldPrivate := ed25519.NewKeyFromSeed(
							bytes.Repeat([]byte{1}, 32),
						)
						coldPublic := coldPrivate.Public().(ed25519.PublicKey)
						// Relative signer evolution is zero; its certificate starts now, not at
						// period zero (the fixture slots are well beyond the KES lifetime).
						certPeriod := slot / config.SlotsPerKESPeriod
						cert := &OperationalCert{
							HotVkey:        kesPublic,
							SequenceNumber: 1,
							KesPeriod:      certPeriod,
							Signature: ed25519.Sign(
								coldPrivate,
								common.OpCertSignableBytes(
									kesPublic,
									1,
									certPeriod,
								),
							),
						}
						builder := NewBlockBuilderWithMode(
							signer,
							kesSigner,
							cert,
							make([]byte, 28),
							coldPublic,
							f,
							mode,
						)
						buildInput := BuildHeaderInput{
							Slot:          slot,
							BlockNumber:   1,
							PrevHash:      make([]byte, 32),
							EpochNonce:    nonce,
							PoolStake:     1,
							TotalStake:    3,
							BlockBodyHash: make([]byte, 32),
							BlockBodySize: 1024,
							ProtoMajor:    2,
						}
						header, builtElection, err := builder.BuildHeader(
							buildInput,
						)
						if expected {
							require.NoError(t, err)
						} else {
							require.ErrorIs(t, err, ErrNotSlotLeader)
						}
						require.NotNil(t, builtElection)
						require.Equal(t, expected, builtElection.Eligible)
						if !expected {
							require.Nil(t, header)
							// Sign a structurally and cryptographically valid ineligible header.
							// Only the producer's local policy is overridden; the real validator
							// retains the independent fixture coefficient and must reject it.
							builder = NewBlockBuilderWithMode(
								signer,
								kesSigner,
								cert,
								make([]byte, 28),
								coldPublic,
								big.NewRat(1, 1),
								mode,
							)
							header, _, err = builder.BuildHeader(buildInput)
							require.NoError(t, err)
						}
						require.NotNil(t, header)
						body, err := builder.serializeHeaderBody(&header.Body)
						require.NoError(t, err)
						counter := uint64(0)
						input := &ValidateHeaderInput{
							Slot: header.Body.Slot, BlockNumber: header.Body.BlockNumber, PrevHash: header.Body.PrevHash,
							IssuerVkey: header.Body.IssuerVkey, VrfKey: header.Body.VrfKey, VrfProof: header.Body.VrfProof, VrfOutput: header.Body.VrfOutput,
							NonceVrfProof: header.Body.NonceVrfProof, NonceVrfOutput: header.Body.NonceVrfOutput,
							KesSignature: header.Signature, HeaderBodyCbor: body, OpCertHotVkey: header.Body.OpCertHotVkey,
							OpCertSequenceNumber: header.Body.OpCertSequenceNumber, OpCertKesPeriod: header.Body.OpCertKesPeriod, OpCertSignature: header.Body.OpCertSignature,
							OpCertCounterState: &counter, PrevHeaderHash: buildInput.PrevHash, EpochNonce: nonce, PoolStake: 1, TotalStake: 3,
							RegisteredVrfKeyHash: common.Blake2b256Hash(signer.PublicKey()).
								Bytes(),
						}
						require.NoError(
							t,
							validator.validateNonceVRFProof(input),
						)
						require.NoError(
							t,
							validator.validateKESSignature(input),
						)
						result := validator.ValidateHeader(input)
						require.Equal(
							t,
							expected,
							result.Valid,
							"validation errors: %v",
							result.Errors,
						)
						if expected {
							require.Equal(t, election.Output, result.VrfOutput)
						} else {
							require.Len(t, result.Errors, 1)
							require.Contains(t, result.Errors[0].Error(), "does not satisfy leader eligibility")
						}
					},
				)
			}
		})
	}
	require.Len(t, modes, 2)
}
