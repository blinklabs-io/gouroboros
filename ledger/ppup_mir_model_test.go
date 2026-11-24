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

// Tables for the Shelley-to-Babbage protocol parameter update (PPUP) and
// move instantaneous rewards (MIR) rules. Each cell is one transaction,
// carrying an update or a MIR certificate, against a ledger state. The
// expected verdict restates the reference (cardano-ledger Rules/Ppup.hs,
// Rules/Deleg.hs and Rules/Utxow.hs; Alonzo and Babbage Rules/Utxos.hs), and
// gOuroboros is expected to agree with it except where a go*Fixed constant
// below is false. The cells run each era's composed UtxoValidationRules, so
// the phase-2-valid gating is exercised too.

package ledger_test

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// Known differences from the reference. A false value means gOuroboros
// still deviates from the ledger in that way, and the cells it covers expect
// the deviation.
const (
	// goEmptyUpdateFixed: an update whose proposal map is empty still has its
	// target epoch checked (Rules/Ppup.hs ppupTransitionNonEmpty).
	goEmptyUpdateFixed = true
	// goPhase2InvalidPpupFixed: PPUP does not run for an IsValid false transaction
	// (Alonzo and Babbage Rules/Utxos.hs run it on the valid branch only).
	goPhase2InvalidPpupFixed = true
	// goMirQuorumPhase2InvalidFixed: the MIR genesis quorum is checked for every
	// transaction (UTXOW validateMIRInsufficientGenesisSigs).
	goMirQuorumPhase2InvalidFixed = true
)

// modelTx overrides what the composed rules read from a transaction:
// IsValid, and the update proposals in the era's own update type.
type modelTx struct {
	*shelley.ShelleyTransaction
	valid     bool
	hasUpdate bool
	epoch     uint64
	updates   map[common.Blake2b224]common.ProtocolParameterUpdate
}

func (t modelTx) IsValid() bool { return t.valid }

func (t modelTx) ProtocolParameterUpdates() (
	uint64,
	map[common.Blake2b224]common.ProtocolParameterUpdate,
) {
	if !t.hasUpdate {
		return 0, nil
	}
	return t.epoch, t.updates
}

type modelEra struct {
	name     string
	rules    []common.UtxoValidationRuleFunc
	phaseTwo bool
	// baseMajor is the era's protocol major version; the PPUP cells
	// propose versions relative to it.
	baseMajor uint
	pparams   func(major, minor uint) common.ProtocolParameters
	update    func(pv *common.ProtocolParametersProtocolVersion) common.ProtocolParameterUpdate
}

func modelEras() []modelEra {
	shelleyPP := func(major, minor uint) common.ProtocolParameters {
		p := mockledger.NewMockShelleyProtocolParams()
		p.ProtocolMajor, p.ProtocolMinor = major, minor
		return &p
	}
	shelleyUpdate := func(
		pv *common.ProtocolParametersProtocolVersion,
	) common.ProtocolParameterUpdate {
		return shelley.ShelleyProtocolParameterUpdate{ProtocolVersion: pv}
	}
	return []modelEra{
		{"Shelley", shelley.UtxoValidationRules, false, 2, shelleyPP, shelleyUpdate},
		{"Allegra", allegra.UtxoValidationRules, false, 3, shelleyPP, func(
			pv *common.ProtocolParametersProtocolVersion,
		) common.ProtocolParameterUpdate {
			return allegra.AllegraProtocolParameterUpdate{ProtocolVersion: pv}
		}},
		{"Mary", mary.UtxoValidationRules, false, 4, func(major, minor uint) common.ProtocolParameters {
			p := mockledger.NewMockMaryProtocolParams()
			p.ProtocolMajor, p.ProtocolMinor = major, minor
			return &p
		}, func(
			pv *common.ProtocolParametersProtocolVersion,
		) common.ProtocolParameterUpdate {
			return mary.MaryProtocolParameterUpdate{ProtocolVersion: pv}
		}},
		{"Alonzo", alonzo.UtxoValidationRules, true, 5, func(major, minor uint) common.ProtocolParameters {
			p := mockledger.NewMockAlonzoProtocolParams()
			p.ProtocolMajor, p.ProtocolMinor = major, minor
			return &p
		}, func(
			pv *common.ProtocolParametersProtocolVersion,
		) common.ProtocolParameterUpdate {
			return alonzo.AlonzoProtocolParameterUpdate{ProtocolVersion: pv}
		}},
		{"Babbage", babbage.UtxoValidationRules, true, 7, func(major, minor uint) common.ProtocolParameters {
			p := mockledger.NewMockBabbageProtocolParams()
			p.ProtocolMajor, p.ProtocolMinor = major, minor
			return &p
		}, func(
			pv *common.ProtocolParametersProtocolVersion,
		) common.ProtocolParameterUpdate {
			return babbage.BabbageProtocolParameterUpdate{ProtocolVersion: pv}
		}},
	}
}

// firstMatching runs every composed rule and returns the first error that
// match accepts. Other rules fail on these minimal transactions for reasons
// the cells do not cover; only the matched failures are the cell's verdict.
func firstMatching(
	rules []common.UtxoValidationRuleFunc,
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
	match func(error) bool,
) error {
	for _, rule := range rules {
		if err := rule(tx, slot, ls, pp); err != nil && match(err) {
			return err
		}
	}
	return nil
}

// signer is a genesis delegate key: its hash is what the rules compare.
type signer struct {
	vkey []byte
	hash common.Blake2b224
}

func newSigner(b byte) signer {
	vkey := make([]byte, 32)
	for i := range vkey {
		vkey[i] = b
	}
	return signer{vkey: vkey, hash: common.Blake2b224Hash(vkey)}
}

func witnessesOf(signers ...signer) shelley.ShelleyTransactionWitnessSet {
	var ws shelley.ShelleyTransactionWitnessSet
	for _, s := range signers {
		ws.VkeyWitnesses = append(ws.VkeyWitnesses, common.VkeyWitness{
			Vkey:      s.vkey,
			Signature: make([]byte, 64),
		})
	}
	return ws
}

// modelGenesis is the genesis delegation state both tables share: three
// genesis keys g1-g3 delegated to d1-d3, quorum two.
type modelGenesis struct {
	keys      [3]common.Blake2b224
	delegates [3]signer
}

func newModelGenesis() modelGenesis {
	var g modelGenesis
	for i := range g.keys {
		g.keys[i] = newSigner(byte(0x10 + i)).hash
		g.delegates[i] = newSigner(byte(0x20 + i))
	}
	return g
}

func (g modelGenesis) GenesisDelegateKeyHashes(uint64) ([]common.Blake2b224, error) {
	ret := make([]common.Blake2b224, 0, len(g.delegates))
	for _, d := range g.delegates {
		ret = append(ret, d.hash)
	}
	return ret, nil
}

func (g modelGenesis) GenesisDelegateForGenesisKey(
	key common.Blake2b224,
	_ uint64,
) (common.Blake2b224, bool, error) {
	for i, k := range g.keys {
		if k == key {
			return g.delegates[i].hash, true, nil
		}
	}
	return common.Blake2b224{}, false, nil
}

func (modelGenesis) GenesisUpdateQuorum() (uint, error) { return 2, nil }

// ppupModelState is the PPUP cells' ledger state: epoch 10, whose slot of
// no return is 50.
type ppupModelState struct {
	common.LedgerState
	modelGenesis
}

const (
	ppupEpoch        = 10
	ppupNoReturnSlot = 50
)

func (ppupModelState) ProtocolParameterUpdateWindow(uint64) (uint64, uint64, error) {
	return ppupEpoch, ppupNoReturnSlot, nil
}

// The test updates: "a" proposes no version; "a21", "a3" and "a4" a
// minor bump, a major bump and a double major bump of the base version.
var ppupModelUpdates = []string{"a", "a21", "a3", "a4"}

func ppupUpdatePV(u string, baseMajor uint) *common.ProtocolParametersProtocolVersion {
	switch u {
	case "a21":
		return &common.ProtocolParametersProtocolVersion{Major: baseMajor, Minor: 1}
	case "a3":
		return &common.ProtocolParametersProtocolVersion{Major: baseMajor + 1}
	case "a4":
		return &common.ProtocolParametersProtocolVersion{Major: baseMajor + 2}
	}
	return nil
}

// ppupCanFollow is the reference's pvCanFollow (Rules/Ppup.hs).
func ppupCanFollow(cur, next common.ProtocolParametersProtocolVersion) bool {
	return (next.Major == cur.Major+1 && next.Minor == 0) ||
		(next.Major == cur.Major && next.Minor == cur.Minor+1)
}

// ppupCell is one transaction carrying an update.
type ppupCell struct {
	keys   []string // "g1", "g2" or the outsider "x"
	update string
	late   bool
	next   bool // target the next epoch rather than this one
	valid  bool
	pv     common.ProtocolParametersProtocolVersion
}

// ppupRefOk is the reference's verdict: PPUP runs unless the transaction is
// IsValid false in an era with phase-2 validation, and then checks that
// every key is a genesis key, that a proposed version can follow (vacuous for
// an empty map), and that the target epoch is the one the slot allows.
func ppupRefOk(c ppupCell, phaseTwo bool, baseMajor uint) bool {
	if phaseTwo && !c.valid {
		return true
	}
	return ppupChecksOk(c, baseMajor)
}

func ppupChecksOk(c ppupCell, baseMajor uint) bool {
	for _, k := range c.keys {
		if k == "x" {
			return false
		}
	}
	if len(c.keys) > 0 {
		if pv := ppupUpdatePV(c.update, baseMajor); pv != nil &&
			!ppupCanFollow(c.pv, *pv) {
			return false
		}
	}
	return c.next == c.late
}

// ppupGoOk is the verdict expected from gOuroboros: ppupRefOk, with the
// differences a false go*Fixed constant keeps.
func ppupGoOk(c ppupCell, phaseTwo bool, baseMajor uint) bool {
	runs := !phaseTwo || c.valid || !goPhase2InvalidPpupFixed
	if !runs {
		return true
	}
	if len(c.keys) == 0 && !goEmptyUpdateFixed {
		return true
	}
	return ppupChecksOk(c, baseMajor)
}

func isPpupError(err error) bool {
	var (
		delegateErr common.ProtocolParameterUpdateDelegateError
		witnessErr  common.ProtocolParameterUpdateWitnessError
		epochErr    common.ProtocolParameterUpdateEpochError
		versionErr  common.ProtocolParameterUpdateVersionError
	)
	return errors.As(err, &delegateErr) || errors.As(err, &witnessErr) ||
		errors.As(err, &epochErr) || errors.As(err, &versionErr)
}

func TestPpupModelTable(t *testing.T) {
	genesis := newModelGenesis()
	keyHash := map[string]common.Blake2b224{
		"g1": genesis.keys[0],
		"g2": genesis.keys[1],
		"x":  newSigner(0x7f).hash,
	}
	keySets := [][]string{{}, {"g1"}, {"x"}, {"g1", "g2"}}
	ls := ppupModelState{LedgerState: newMirFixtureLedger(), modelGenesis: genesis}
	cells := 0
	for _, era := range modelEras() {
		validFlags := []bool{true}
		if era.phaseTwo {
			validFlags = []bool{true, false}
		}
		// The parameters: the base version, and the version after
		// a major bump was adopted.
		versions := []common.ProtocolParametersProtocolVersion{
			{Major: era.baseMajor},
			{Major: era.baseMajor + 1},
		}
		for _, pv := range versions {
			for _, keys := range keySets {
				for _, u := range ppupModelUpdates {
					for _, late := range []bool{false, true} {
						for _, next := range []bool{false, true} {
							for _, valid := range validFlags {
								c := ppupCell{keys, u, late, next, valid, pv}
								updates := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
								for _, k := range keys {
									updates[keyHash[k]] = era.update(ppupUpdatePV(u, era.baseMajor))
								}
								epoch := uint64(ppupEpoch)
								if next {
									epoch++
								}
								slot := uint64(ppupNoReturnSlot - 1)
								if late {
									slot = ppupNoReturnSlot
								}
								tx := modelTx{
									ShelleyTransaction: &shelley.ShelleyTransaction{
										WitnessSet: witnessesOf(genesis.delegates[0], genesis.delegates[1]),
									},
									valid:     valid,
									hasUpdate: true,
									epoch:     epoch,
									updates:   updates,
								}
								err := firstMatching(era.rules, tx, slot, ls,
									era.pparams(pv.Major, pv.Minor), isPpupError)
								want := ppupGoOk(c, era.phaseTwo, era.baseMajor)
								if (err == nil) != want {
									t.Errorf("%s pv=%d.%d keys=%v update=%s late=%v next=%v valid=%v: accepted=%v want %v (ledger %v): %v",
										era.name, pv.Major, pv.Minor, keys, u, late, next, valid,
										err == nil, want, ppupRefOk(c, era.phaseTwo, era.baseMajor), err)
								}
								// A non-genesis key is NonGenesisUpdatePPUP, not
								// a missing witness: its delegate is unknown.
								var delegateErr common.ProtocolParameterUpdateDelegateError
								if err != nil && slices.Contains(keys, "x") &&
									!errors.As(err, &delegateErr) {
									t.Errorf("%s keys=%v: want a delegate error, got %v",
										era.name, keys, err)
								}
								cells++
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d PPUP cells", cells)
}

// mirModelState is the MIR cells' ledger state: the epoch ends at slot 1000
// with a stability window of 100, so the cutoff is slot 900. ir holds the
// pending credits ir[pot][credential], dpot the pending transfers.
type mirModelState struct {
	common.LedgerState
	modelGenesis
	ir   [2][2]int64
	dpot [2]int64
}

func (mirModelState) NextEpochFirstSlot(uint64) (uint64, error) { return 1000, nil }

func (mirModelState) StabilityWindow() (uint64, error) { return 100, nil }

func (s mirModelState) InstantaneousRewardsTotal(source uint) (*big.Int, error) {
	return big.NewInt(s.ir[source][0] + s.ir[source][1]), nil
}

func (s mirModelState) InstantaneousPotTransfer(source uint) (*big.Int, error) {
	return big.NewInt(s.dpot[source]), nil
}

func (s mirModelState) PendingInstantaneousRewards(
	source uint,
	cred common.Credential,
) (*big.Int, error) {
	if v := s.ir[source][cred.Credential[0]-1]; v != 0 {
		return big.NewInt(v), nil
	}
	return nil, nil
}

// mirCell is one transaction carrying a MIR certificate, with the state it
// is taken from.
type mirCell struct {
	pots     [2]int64 // reserves, treasury
	ir       [2][2]int64
	dpot     [2]int64
	transfer bool
	pot      int
	cred     int // 0: c1, 1: c2
	delta    int64
	amount   int64
	sigs     int
	valid    bool
	late     bool
}

func mirSum(xs [2]int64) int64 { return xs[0] + xs[1] }

// mirRefOk is the reference's verdict: the genesis quorum (Rules/Utxow.hs)
// for every transaction, then the MIR certificate rule (Rules/Deleg.hs) for
// a valid one.
func mirRefOk(c mirCell, alonzoRules bool) bool {
	if c.sigs < 2 {
		return false
	}
	if !c.valid {
		return true
	}
	return mirDelegOk(c, alonzoRules)
}

func mirDelegOk(c mirCell, alonzoRules bool) bool {
	if c.late {
		return false
	}
	avail := c.pots[c.pot] + c.dpot[c.pot]
	if c.transfer {
		return alonzoRules && c.amount <= avail-mirSum(c.ir[c.pot])
	}
	combined := c.ir[c.pot]
	if alonzoRules {
		combined[c.cred] += c.delta
		return combined[0] >= 0 && combined[1] >= 0 && mirSum(combined) <= avail
	}
	combined[c.cred] = c.delta
	return c.delta >= 0 && mirSum(combined) <= c.pots[c.pot]
}

// mirGoOk is the verdict expected from gOuroboros: mirRefOk, with the
// difference goMirQuorumPhase2InvalidFixed keeps while false.
func mirGoOk(c mirCell, alonzoRules bool) bool {
	checksQuorum := c.valid || goMirQuorumPhase2InvalidFixed
	if checksQuorum && c.sigs < 2 {
		return false
	}
	if !c.valid {
		return true
	}
	return mirDelegOk(c, alonzoRules)
}

func isMirError(err error) bool {
	var (
		quorum      common.MIRInsufficientGenesisSigsError
		late        shelley.MIRCertificateTooLateinEpochError
		transfer    shelley.MIRTransferNotCurrentlyAllowedError
		negative    shelley.MIRNegativesNotCurrentlyAllowedError
		produces    shelley.MIRProducesNegativeUpdateError
		rewards     shelley.InsufficientForInstantaneousRewardsError
		forTransfer shelley.InsufficientForTransferError
		unavailable shelley.PendingInstantaneousRewardsUnavailableError
	)
	return errors.As(err, &quorum) || errors.As(err, &late) ||
		errors.As(err, &transfer) || errors.As(err, &negative) ||
		errors.As(err, &produces) || errors.As(err, &rewards) ||
		errors.As(err, &forTransfer) || errors.As(err, &unavailable)
}

func TestMirModelTable(t *testing.T) {
	genesis := newModelGenesis()
	// The starting states, and states an epoch of certificates
	// can reach: a pending credit, and a pending transfer out of the
	// reserves.
	type start struct {
		pots [2]int64
		ir   [2][2]int64
		dpot [2]int64
	}
	starts := []start{
		{pots: [2]int64{3, 1}},
		{pots: [2]int64{3, 1}, ir: [2][2]int64{{1, 0}, {0, 0}}},
		{pots: [2]int64{3, 1}, ir: [2][2]int64{{2, 1}, {1, 0}}},
		{pots: [2]int64{3, 1}, ir: [2][2]int64{{1, 0}, {0, 0}}, dpot: [2]int64{-1, 1}},
		{pots: [2]int64{1, 1}},
	}
	cells := 0
	for _, era := range modelEras() {
		alonzoRules := era.phaseTwo
		validFlags := []bool{true}
		if era.phaseTwo {
			validFlags = []bool{true, false}
		}
		pp := era.pparams(era.baseMajor, 0)
		for _, st := range starts {
			if !alonzoRules && st.dpot != [2]int64{} {
				continue // no transfer exists before Alonzo
			}
			var certs []mirCell
			for pot := range 2 {
				for cred := range 2 {
					for _, d := range []int64{-1, 1, 2} {
						certs = append(certs, mirCell{pot: pot, cred: cred, delta: d})
					}
				}
				for _, a := range []int64{1, 2} {
					certs = append(certs, mirCell{transfer: true, pot: pot, amount: a})
				}
			}
			for _, cert := range certs {
				for _, sigs := range []int{1, 2} {
					for _, valid := range validFlags {
						for _, late := range []bool{false, true} {
							c := cert
							c.pots, c.ir, c.dpot = st.pots, st.ir, st.dpot
							c.sigs, c.valid, c.late = sigs, valid, late
							ls := mirModelState{
								LedgerState: mockledger.NewLedgerStateBuilder().
									WithAdaPots(common.AdaPots{
										Reserves: uint64(st.pots[0]),
										Treasury: uint64(st.pots[1]),
									}).
									Build(),
								modelGenesis: genesis,
								ir:           st.ir,
								dpot:         st.dpot,
							}
							var mir common.Certificate
							if c.transfer {
								mir = mirOppositePotCert(uint(c.pot), uint64(c.amount))
							} else {
								mir = mirCert(uint(c.pot), map[*common.Credential]*big.Int{
									mirStakeCredential(byte(c.cred + 1)): big.NewInt(c.delta),
								})
							}
							inner := poolCertTx(mir)
							inner.WitnessSet = witnessesOf(genesis.delegates[:sigs]...)
							tx := modelTx{ShelleyTransaction: inner, valid: valid}
							slot := uint64(899)
							if late {
								slot = 900
							}
							err := firstMatching(era.rules, tx, slot, ls, pp, isMirError)
							want := mirGoOk(c, alonzoRules)
							if (err == nil) != want {
								t.Errorf("%s %s: accepted=%v want %v (ledger %v): %v",
									era.name, mirCellString(c), err == nil, want,
									mirRefOk(c, alonzoRules), err)
							}
							cells++
						}
					}
				}
			}
		}
	}
	t.Logf("%d MIR cells", cells)
}

func mirCellString(c mirCell) string {
	kind := fmt.Sprintf("stake pot=%d cred=c%d delta=%d", c.pot, c.cred+1, c.delta)
	if c.transfer {
		kind = fmt.Sprintf("transfer pot=%d amount=%d", c.pot, c.amount)
	}
	return fmt.Sprintf("pots=%v ir=%v dpot=%v %s sigs=%d valid=%v late=%v",
		c.pots, c.ir, c.dpot, kind, c.sigs, c.valid, c.late)
}
