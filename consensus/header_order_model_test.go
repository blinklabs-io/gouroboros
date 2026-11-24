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

package consensus

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

// Header defects, named after the reference error each one causes
// (ouroboros-consensus HeaderValidation.hs and PolyPraos.hs, cardano-ledger
// TPraos Rules/Overlay.hs and Rules/OCert.hs).
const (
	orderBlockNo         = "BlockNo"
	orderSlot            = "Slot"
	orderPrevHash        = "PrevHash"
	orderKESBefore       = "KESBefore"
	orderKESAfter        = "KESAfter"
	orderOCertSig        = "OCertSig"
	orderKESSig          = "KESSig"
	orderNoCounter       = "NoCounter"
	orderCounterTooSmall = "CounterTooSmall"
	orderCounterOverInc  = "CounterOverInc"
	orderVRFUnknown      = "VRFUnknown"
	orderVRFWrongKey     = "VRFWrongKey"
	orderVRFBadNonce     = "VRFBadNonce"
	orderVRFBadProof     = "VRFBadProof"
	orderLeaderTooBig    = "LeaderTooBig"
)

const (
	orderSlotsPerKESPeriod = 100
	orderMaxKESEvolutions  = 2
	orderStartKESPeriod    = 2
	orderTotalStake        = uint64(1) << 40
)

func orderDefects(mode ConsensusMode) []string {
	defects := []string{
		orderBlockNo, orderSlot, orderPrevHash,
		orderKESBefore, orderKESAfter, orderOCertSig, orderKESSig,
		orderNoCounter, orderCounterTooSmall,
		orderVRFUnknown, orderVRFWrongKey, orderVRFBadProof, orderLeaderTooBig,
	}
	if mode == ConsensusModeCPraos {
		return append(defects, orderCounterOverInc)
	}
	return append(defects, orderVRFBadNonce)
}

func orderConsistent(d map[string]bool) bool {
	if d[orderKESBefore] && d[orderKESAfter] {
		return false
	}
	counters := 0
	for _, c := range []string{orderNoCounter, orderCounterTooSmall, orderCounterOverInc} {
		if d[c] {
			counters++
		}
	}
	if counters > 1 {
		return false
	}
	return !d[orderVRFUnknown] || (!d[orderVRFWrongKey] && !d[orderLeaderTooBig])
}

type orderRefCheck struct {
	defect string
	group  string
	onFail string // "halt", "skipGroup" or "continue"
}

// orderReferenceProgram lists the reference's checks in order: the envelope
// (HeaderValidation.hs validateEnvelope), then Praos's KES side and VRF side
// (PolyPraos.hs updateChainDepStatePolyPraos), or TPraos's VRF group and
// OCERT rule (cardano-ledger TPraos Rules/Overlay.hs and Rules/OCert.hs).
func orderReferenceProgram(mode ConsensusMode) []orderRefCheck {
	prog := []orderRefCheck{
		{orderBlockNo, "env", "halt"},
		{orderSlot, "env", "halt"},
		{orderPrevHash, "env", "halt"},
	}
	if mode == ConsensusModeCPraos {
		for _, d := range []string{
			orderKESBefore, orderKESAfter, orderOCertSig, orderKESSig,
			orderNoCounter, orderCounterTooSmall, orderCounterOverInc,
			orderVRFUnknown, orderVRFWrongKey, orderVRFBadProof, orderLeaderTooBig,
		} {
			prog = append(prog, orderRefCheck{d, "prot", "halt"})
		}
		return prog
	}
	for _, d := range []string{
		orderVRFUnknown, orderVRFWrongKey, orderVRFBadNonce, orderVRFBadProof,
		orderLeaderTooBig,
	} {
		prog = append(prog, orderRefCheck{d, "vrf", "skipGroup"})
	}
	for _, d := range []string{
		orderKESBefore, orderKESAfter, orderOCertSig, orderKESSig,
		orderNoCounter, orderCounterTooSmall,
	} {
		prog = append(prog, orderRefCheck{d, "ocert", "continue"})
	}
	return prog
}

func orderReferenceErrors(mode ConsensusMode, d map[string]bool) []string {
	var errs []string
	skip := ""
	for _, check := range orderReferenceProgram(mode) {
		if check.group == skip || !d[check.defect] {
			continue
		}
		errs = append(errs, check.defect)
		switch check.onFail {
		case "halt":
			return errs
		case "skipGroup":
			skip = check.group
		}
	}
	return errs
}

// orderClassify names the reference error a ValidateHeader error stands for.
func orderClassify(t *testing.T, err error) string {
	t.Helper()
	msg := err.Error()
	for _, c := range []struct{ substr, defect string }{
		{"slot must be greater than previous slot", orderSlot},
		{"block number must be previous + 1", orderBlockNo},
		{"previous hash does not match", orderPrevHash},
		{"nonce VRF proof verification returned false", orderVRFBadNonce},
		{"VRF proof verification returned false", orderVRFBadProof},
		{"pool stake cannot be zero", orderVRFUnknown},
		{"registered VRF key hash is required", orderVRFUnknown},
		{"VRF key does not match registered key hash", orderVRFWrongKey},
		{"does not satisfy leader eligibility", orderLeaderTooBig},
		{"KES period is in the future", orderKESBefore},
		{"operational certificate has expired", orderKESAfter},
		{"KES signature verification failed", orderKESSig},
		{"OpCert signature verification failed", orderOCertSig},
		{"counter is required", orderNoCounter},
		{"counter rolled back", orderCounterTooSmall},
		{"counter skipped ahead", orderCounterOverInc},
	} {
		if strings.Contains(msg, c.substr) {
			return c.defect
		}
	}
	t.Fatalf("unclassified header validation error: %v", err)
	return ""
}

type orderFixture struct {
	mode       ConsensusMode
	validator  *HeaderValidator
	vrfSigner  *SimpleVRFSigner
	coldKey    ed25519.PrivateKey
	kesSeed    []byte
	epochNonce []byte
	slot       uint64
}

func newOrderFixture(t *testing.T, mode ConsensusMode) *orderFixture {
	t.Helper()
	config := NetworkConfig{
		Name:              "header-order-model",
		SecurityParam:     10,
		ActiveSlotCoeff:   common.GenesisRat{Rat: big.NewRat(1, 2)},
		SlotLength:        common.GenesisRat{Rat: big.NewRat(1, 1)},
		EpochLength:       1000,
		SlotsPerKESPeriod: orderSlotsPerKESPeriod,
		MaxKESEvolutions:  orderMaxKESEvolutions,
	}
	signer, err := NewSimpleVRFSigner([]byte("header_order_model_vrf_seed_0001"))
	require.NoError(t, err)
	f := &orderFixture{
		mode:       mode,
		validator:  NewHeaderValidatorWithMode(config, mode),
		vrfSigner:  signer,
		coldKey:    ed25519.NewKeyFromSeed([]byte("header_order_model_cold_seed_001")),
		kesSeed:    []byte("header_order_model_kes_seed_0001"),
		epochNonce: bytes.Repeat([]byte{7}, 32),
	}
	// The slot must elect the pool at full stake and not at a tiny stake, so
	// that LeaderTooBig is a defect of the stake alone.
	first := uint64(orderStartKESPeriod * orderSlotsPerKESPeriod)
	for slot := first; slot < first+orderSlotsPerKESPeriod; slot++ {
		full, err := IsSlotLeaderWithMode(slot, f.epochNonce, orderTotalStake,
			orderTotalStake, f.validator.activeSlotCoeff, signer, mode)
		require.NoError(t, err)
		tiny, err := IsSlotLeaderWithMode(slot, f.epochNonce, 1,
			orderTotalStake, f.validator.activeSlotCoeff, signer, mode)
		require.NoError(t, err)
		if full.Eligible && !tiny.Eligible {
			f.slot = slot
			return f
		}
	}
	t.Fatal("no slot in the KES period elects the full-stake pool only")
	return nil
}

func (f *orderFixture) input(t *testing.T, d map[string]bool) *ValidateHeaderInput {
	t.Helper()
	slot := f.slot
	opCertKESPeriod := uint64(orderStartKESPeriod)
	evolution := uint64(0)
	switch {
	case d[orderKESBefore]:
		opCertKESPeriod = orderStartKESPeriod + 1
	case d[orderKESAfter]:
		opCertKESPeriod = 0
		evolution = orderStartKESPeriod
	}
	seq, counter := uint64(1), uint64(1)
	switch {
	case d[orderCounterTooSmall]:
		counter = 2
	case d[orderCounterOverInc]:
		seq = 3
	}

	kesSk, kesPk, err := kes.KeyGen(kes.CardanoKesDepth, f.kesSeed)
	require.NoError(t, err)
	for range evolution {
		kesSk, err = kes.Update(kesSk)
		require.NoError(t, err)
	}
	body := []byte("header order model header body")
	kesSig, err := kes.Sign(kesSk, evolution, body)
	require.NoError(t, err)
	opCertSig := ed25519.Sign(
		f.coldKey,
		common.OpCertSignableBytes(kesPk, seq, opCertKESPeriod),
	)

	leaderInput, err := vrf.MkInputVrf(int64(slot), f.epochNonce) //nolint:gosec
	if f.mode == ConsensusModeTPraos {
		leaderInput, err = vrf.MkSeedTPraos(int64(slot), f.epochNonce, vrf.SeedL()) //nolint:gosec
	}
	require.NoError(t, err)
	vrfProof, vrfOutput, err := f.vrfSigner.Prove(leaderInput)
	require.NoError(t, err)
	var nonceProof, nonceOutput []byte
	if f.mode == ConsensusModeTPraos {
		nonceInput, err := vrf.MkSeedTPraos(int64(slot), f.epochNonce, vrf.SeedEta()) //nolint:gosec
		require.NoError(t, err)
		nonceProof, nonceOutput, err = f.vrfSigner.Prove(nonceInput)
		require.NoError(t, err)
	}

	prevHash := bytes.Repeat([]byte{1}, 32)
	vrfKeyHash := common.Blake2b256Hash(f.vrfSigner.PublicKey())
	in := &ValidateHeaderInput{
		Envelope:             testEnvelope(),
		Overlay:              testOverlayD0(),
		Slot:                 slot,
		BlockNumber:          10,
		PrevHash:             prevHash,
		IssuerVkey:           f.coldKey.Public().(ed25519.PublicKey),
		VrfKey:               f.vrfSigner.PublicKey(),
		VrfProof:             vrfProof,
		VrfOutput:            vrfOutput,
		NonceVrfProof:        nonceProof,
		NonceVrfOutput:       nonceOutput,
		KesSignature:         kesSig,
		HeaderBodyCbor:       body,
		OpCertHotVkey:        kesPk,
		OpCertSequenceNumber: seq,
		OpCertKesPeriod:      opCertKESPeriod,
		OpCertSignature:      opCertSig,
		OpCertCounterState:   &counter,
		PrevSlot:             slot - 1,
		PrevBlockNumber:      9,
		PrevHeaderHash:       slices.Clone(prevHash),
		EpochNonce:           f.epochNonce,
		PoolStake:            orderTotalStake,
		TotalStake:           orderTotalStake,
		RegisteredVrfKeyHash: vrfKeyHash.Bytes(),
	}
	if d[orderSlot] {
		in.PrevSlot = slot
	}
	if d[orderBlockNo] {
		in.BlockNumber = 12
	}
	if d[orderPrevHash] {
		in.PrevHeaderHash = bytes.Repeat([]byte{2}, 32)
	}
	if d[orderOCertSig] {
		in.OpCertSignature[0] ^= 0xff
	}
	if d[orderKESSig] {
		in.KesSignature[10] ^= 0xff
	}
	if d[orderNoCounter] {
		in.OpCertCounterState = nil
	}
	if d[orderVRFUnknown] {
		in.PoolStake = 0
		in.RegisteredVrfKeyHash = nil
	}
	if d[orderVRFWrongKey] {
		in.RegisteredVrfKeyHash = bytes.Repeat([]byte{0xaa}, 32)
	}
	if d[orderVRFBadProof] {
		in.VrfOutput[0] ^= 0xff
	}
	if d[orderVRFBadNonce] {
		in.NonceVrfOutput[0] ^= 0xff
	}
	if d[orderLeaderTooBig] {
		in.PoolStake = 1
	}
	return in
}

func orderDefectSets(mode ConsensusMode, maxDefects int) []map[string]bool {
	defects := orderDefects(mode)
	sets := []map[string]bool{{}}
	for i := range defects {
		sets = append(sets, map[string]bool{defects[i]: true})
		if maxDefects < 2 {
			continue
		}
		for j := i + 1; j < len(defects); j++ {
			sets = append(sets, map[string]bool{defects[i]: true, defects[j]: true})
		}
	}
	consistent := sets[:0]
	for _, d := range sets {
		if orderConsistent(d) {
			consistent = append(consistent, d)
		}
	}
	return consistent
}

func orderName(d map[string]bool) string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "valid"
	}
	return strings.Join(names, "+")
}

// TestValidateHeaderMatchesReferenceOrderTable runs ValidateHeader on every
// header with at most two consistent defects, in both modes, and checks it
// against the reference's checks in order: the verdict matches the
// reference, every reported error is a defect the header has, the reference's
// errors are all reported, the first error is the one the reference reports
// first, and no error is reported twice.
//
// A certificate before its start is left out of the coverage and duplicate
// checks: ValidateHeader reports it from both the KES period and the KES
// signature check, and then does not verify the signature.
func TestValidateHeaderMatchesReferenceOrderTable(t *testing.T) {
	for _, mode := range []ConsensusMode{ConsensusModeCPraos, ConsensusModeTPraos} {
		f := newOrderFixture(t, mode)
		for _, d := range orderDefectSets(mode, 2) {
			t.Run(fmt.Sprintf("mode%d/%s", mode, orderName(d)), func(t *testing.T) {
				result := f.validator.ValidateHeader(f.input(t, d))
				want := orderReferenceErrors(mode, d)
				got := make([]string, 0, len(result.Errors))
				for _, err := range result.Errors {
					got = append(got, orderClassify(t, err))
				}
				require.Equal(t, len(want) == 0, result.Valid, "verdict: got %v, reference %v", got, want)
				for _, g := range got {
					require.True(t, d[g], "reported %s, which the header does not have (%v)", g, got)
				}
				if len(want) > 0 {
					require.NotEmpty(t, got)
					require.Equal(t, want[0], got[0], "first error: got %v, reference %v", got, want)
				}
				if d[orderKESBefore] {
					return
				}
				for _, w := range want {
					require.Contains(t, got, w, "reference error missing: got %v, reference %v", got, want)
				}
				seen := map[string]bool{}
				for _, g := range got {
					require.False(t, seen[g], "%s reported twice: %v", g, got)
					seen[g] = true
				}
			})
		}
	}
}
