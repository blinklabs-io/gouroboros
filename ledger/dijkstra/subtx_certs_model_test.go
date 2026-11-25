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

package dijkstra

// The tests in this file enumerate how the certificates, withdrawals and
// direct deposits of a Dijkstra batch's levels are threaded and
// aggregated. Each restates a reference predicate in Go (cardano-ledger
// 1210d37fe6bd, eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/:
// Rules/Ledger.hs 410-491, Rules/SubLedger.hs 259-299,
// Rules/SubEntities.hs 252-282, Rules/Entities.hs 276-389,
// Rules/SubCert.hs 197-215, Rules/Utxo.hs 361-386, Rules/Utxow.hs 512-527,
// UTxO.hs 115-137 and 187-203; Conway Rules/Deleg.hs 236-303), builds
// every cell as a real Dijkstra transaction against a mock ledger state,
// runs the production rules the predicate covers, and requires gOuroboros
// to accept exactly the cells the predicate accepts.

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

const (
	subCertKeyDep  = 2_000
	subCertPoolDep = 500_000
	subCertCoin    = 1_000
	subCertInCoins = 2_000_000
	subCertMinCost = 340_000_000
	subCertPoolOp  = 0x77
	subCertPoolVrf = 0xaa
)

// ---------------------------------------------------------------------------
// Entity effects: each level withdraws, then runs its certificate, then pays
// its direct deposit; the sub-transactions run first, then the top.

type subCertLevel struct {
	wd   string // "none", "zero", "one"
	cert string // "none", "regS", "unregS", "delegS", "regP"
	dd   bool
}

func (l subCertLevel) String() string {
	return fmt.Sprintf("{wd:%s cert:%s dd:%v}", l.wd, l.cert, l.dd)
}

func subCertLevels() []subCertLevel {
	var ret []subCertLevel
	for _, wd := range []string{"none", "zero", "one"} {
		for _, cert := range []string{"none", "regS", "unregS", "delegS", "regP"} {
			for _, dd := range []bool{false, true} {
				ret = append(ret, subCertLevel{wd: wd, cert: cert, dd: dd})
			}
		}
	}
	return ret
}

type subCertAccount struct {
	ok, reg, pool bool
	bal           int64
}

// subCertSpecLevel is one level under the reference: SUBENTITIES or
// ENTITIES withdraws, runs SUBCERTS or CERTS (Conway DELEG and the Dijkstra
// POOL transition), then pays direct deposits into accounts that exist
// after them.
func subCertSpecLevel(s subCertAccount, l subCertLevel) subCertAccount {
	if l.wd != "none" {
		s.ok = s.ok && s.reg
		if l.wd == "one" {
			s.bal--
		}
	}
	switch l.cert {
	case "regS":
		s.ok = s.ok && !s.reg
		s.reg, s.bal = true, 0
	case "unregS":
		s.ok = s.ok && s.reg && s.bal == 0
		s.reg = false
	case "delegS":
		s.ok = s.ok && s.reg && s.pool
	case "regP":
		s.pool = true
	}
	if l.dd {
		s.ok = s.ok && s.reg
		s.bal++
	}
	return s
}

// subCertSpecEnt is the reference's verdict: for a phase-2-valid batch the
// withdrawals may not exceed the original balance and need an account that
// existed originally (Entities.hs 341-389), and every level's steps must
// succeed in order. A phase-2-invalid batch runs none of this.
func subCertSpecEnt(
	reg bool,
	bal int64,
	pool bool,
	valid bool,
	top subCertLevel,
	subs []subCertLevel,
) bool {
	if !valid {
		return true
	}
	levels := append(append([]subCertLevel{}, subs...), top)
	var total int64
	anyWd := false
	s := subCertAccount{ok: true, reg: reg, bal: bal, pool: pool}
	for _, l := range levels {
		if l.wd != "none" {
			anyWd = true
		}
		if l.wd == "one" {
			total++
		}
		s = subCertSpecLevel(s, l)
	}
	return total <= bal && (!anyWd || reg) && s.ok
}

type subCertFixture struct {
	address    *common.Address
	credential common.Credential
	key        cbor.ByteString
	pool       common.PoolKeyHash
}

func newSubCertFixture(t *testing.T) subCertFixture {
	t.Helper()
	address := dijkstraIntervalAddress(t, 0x61)
	return subCertFixture{
		address:    address,
		credential: dijkstraIntervalCredential(t, address),
		key:        dijkstraIntervalKey(t, address),
		pool:       common.PoolKeyHash{subCertPoolOp},
	}
}

func (f subCertFixture) certificate(
	t *testing.T,
	cert string,
) []common.CertificateWrapper {
	t.Helper()
	switch cert {
	case "regS":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeRegistration),
			Certificate: &common.RegistrationCertificate{
				CertType:        uint(common.CertificateTypeRegistration),
				StakeCredential: f.credential,
				Amount:          subCertKeyDep,
			},
		}}
	case "unregS":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeDeregistration),
			Certificate: &common.DeregistrationCertificate{
				CertType:        uint(common.CertificateTypeDeregistration),
				StakeCredential: f.credential,
				Amount:          subCertKeyDep,
			},
		}}
	case "delegS":
		credential := f.credential
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeStakeDelegation),
			Certificate: &common.StakeDelegationCertificate{
				CertType:        uint(common.CertificateTypeStakeDelegation),
				StakeCredential: &credential,
				PoolKeyHash:     f.pool,
			},
		}}
	case "regP":
		return []common.CertificateWrapper{
			crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf),
		}
	}
	return nil
}

func (f subCertFixture) withdrawals(wd string) map[*common.Address]uint64 {
	switch wd {
	case "zero":
		return map[*common.Address]uint64{f.address: 0}
	case "one":
		return map[*common.Address]uint64{f.address: subCertCoin}
	}
	return nil
}

func (f subCertFixture) directDeposits(dd bool) DijkstraDirectDeposits {
	if !dd {
		return nil
	}
	return DijkstraDirectDeposits{f.key: subCertCoin}
}

func (f subCertFixture) ledger(
	t *testing.T,
	reg bool,
	bal int64,
	pool bool,
	utxos ...common.Utxo,
) common.LedgerState {
	t.Helper()
	builder := mockledger.NewLedgerStateBuilder().WithNetworkId(0).WithUtxos(utxos)
	if reg {
		builder = builder.
			WithRewardAccountCredentialBalance(f.credential, uint64(bal)*subCertCoin).
			WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
				mockledger.NewRewardAccountKey(f.credential): subCertKeyDep,
			})
	}
	if pool {
		registered := crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf)
		builder = builder.WithPoolRegistrations([]common.PoolRegistrationCertificate{
			*registered.Certificate.(*common.PoolRegistrationCertificate),
		})
	}
	return subCertLedgerState{LedgerState: builder.Build()}
}

// subCertLedgerState adds the future-VRF-key capability the POOL rule asks
// for, and unwraps to the mock so its other capabilities stay reachable.
type subCertLedgerState struct {
	common.LedgerState
}

func (subCertLedgerState) IsFutureVrfKeyInUse(
	common.Blake2b256,
) (bool, common.PoolKeyHash, error) {
	return false, common.PoolKeyHash{}, nil
}

func (s subCertLedgerState) UnwrapLedgerState() common.LedgerState {
	return s.LedgerState
}

func subCertPParams() *DijkstraProtocolParameters {
	pp := modelDijkstraPParams()
	pp.KeyDeposit = subCertKeyDep
	pp.PoolDeposit = subCertPoolDep
	pp.MinPoolCost = subCertMinCost
	return pp
}

// subCertSubTx gives each sub-transaction an input and an output of its own
// so that two sub-transactions with the same effects are distinct.
func subCertSubTx(t *testing.T, k int) DijkstraSubTransaction {
	t.Helper()
	return DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
		TxInputs: conway.NewConwayTransactionInputSet(
			[]shelley.ShelleyTransactionInput{modelPlace(k + 1)},
		),
		TxOutputs: []DijkstraTransactionOutput{
			modelOutput(t, uint64(1_000_000+k)),
		},
	}}
}

func TestSubTxModelEntities(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleBatchWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleAccountBalanceIntervals),
		modelPhasedRule(t, common.UtxoValidationRuleWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleCertificateDeposits),
		modelPhasedRule(t, common.UtxoValidationRuleDelegation),
		modelPhasedRule(t, common.UtxoValidationRulePoolCertificates),
	}
	pp := subCertPParams()
	f := newSubCertFixture(t)
	levels := subCertLevels()
	for _, reg := range []bool{false, true} {
		bals := []int64{0}
		if reg {
			bals = []int64{0, 1}
		}
		for _, bal := range bals {
			for _, pool := range []bool{false, true} {
				t.Run(fmt.Sprintf("reg=%v/bal=%d/pool=%v", reg, bal, pool), func(t *testing.T) {
					t.Parallel()
					cells, accepted := 0, 0
					ls := f.ledger(t, reg, bal, pool)
					for _, valid := range []bool{false, true} {
						for _, top := range levels {
							for _, subs := range modelUpTo2(levels) {
								built := make([]DijkstraSubTransaction, 0, len(subs))
								for k, l := range subs {
									sub := subCertSubTx(t, k)
									sub.Body.TxCertificates = f.certificate(t, l.cert)
									sub.Body.TxWithdrawals = f.withdrawals(l.wd)
									sub.Body.TxDirectDeposits = f.directDeposits(l.dd)
									built = append(built, sub)
								}
								tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, built)
								tx.Body.TxCertificates = f.certificate(t, top.cert)
								tx.Body.TxWithdrawals = f.withdrawals(top.wd)
								tx.Body.TxDirectDeposits = f.directDeposits(top.dd)
								err := modelFirstError(tx, ls, pp, rules)
								want := subCertSpecEnt(reg, bal, pool, valid, top, subs)
								if want != (err == nil) {
									t.Fatalf(
										"reg=%v bal=%d pool=%v valid=%v top=%v subs=%v: reference accepts=%v, gOuroboros error=%v",
										reg, bal, pool, valid, top, subs, want, err,
									)
								}
								cells++
								if want {
									accepted++
								}
							}
						}
					}
					t.Logf("cells: %d (%d accepted)", cells, accepted)
				})
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Pool deposit: once per pool the original state lacks, over every level;
// in legacy mode the top level must also balance alone against the pool
// state the sub-transactions left.

type subCertDepBatch struct {
	valid, legacy bool
	top           bool   // the top registers the pool
	subs          []bool // each sub-transaction registers the pool
	dTop, dSub    uint64 // pool deposits the top and first sub leave room for
}

func subCertSpecDep(pool bool, b subCertDepBatch) bool {
	if !b.valid {
		return true
	}
	anySub := false
	for _, s := range b.subs {
		anySub = anySub || s
	}
	var batch, legacy uint64
	if !pool && (anySub || b.top) {
		batch = 1
	}
	if !pool && !anySub && b.top {
		legacy = 1
	}
	if b.dTop+b.dSub != batch {
		return false
	}
	return !b.legacy || b.dTop == legacy
}

func TestSubTxModelPoolDeposit(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleValueNotConserved),
		modelPhasedRule(t, common.UtxoValidationRuleValueNotConservedLegacyMode),
	}
	pp := subCertPParams()
	v3 := common.PlutusV3Script{0x31}
	cells, accepted := 0, 0
	for _, pool := range []bool{false, true} {
		for _, valid := range []bool{false, true} {
			for _, legacy := range []bool{false, true} {
				for _, top := range []bool{false, true} {
					for _, subs := range modelUpTo2([]bool{false, true}) {
						for dTop := uint64(0); dTop <= 1; dTop++ {
							for dSub := uint64(0); dSub <= 1; dSub++ {
								if len(subs) == 0 && dSub > 0 {
									continue
								}
								b := subCertDepBatch{
									valid: valid, legacy: legacy, top: top,
									subs: subs, dTop: dTop, dSub: dSub,
								}
								topInput := modelPlace(0)
								utxos := []common.Utxo{
									modelUtxo(t, topInput, subCertInCoins, 0),
								}
								if legacy {
									var topUtxo common.Utxo
									topInput, topUtxo = dijkstraScriptLockedInput(t, v3, 7)
									utxos[0] = topUtxo
								}
								built := make([]DijkstraSubTransaction, 0, len(subs))
								for k, registers := range subs {
									slack := uint64(0)
									if k == 0 {
										slack = dSub * subCertPoolDep
									}
									utxos = append(utxos, modelUtxo(
										t, modelPlace(k+1), subCertInCoins+uint64(k), 0,
									))
									sub := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
										TxInputs: conway.NewConwayTransactionInputSet(
											[]shelley.ShelleyTransactionInput{modelPlace(k + 1)},
										),
										TxOutputs: []DijkstraTransactionOutput{
											modelOutput(t, subCertInCoins+uint64(k)-slack),
										},
									}}
									if registers {
										sub.Body.TxCertificates = []common.CertificateWrapper{
											crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf),
										}
									}
									built = append(built, sub)
								}
								tx := &DijkstraTransaction{
									Body: DijkstraTransactionBody{
										TxInputs: conway.NewConwayTransactionInputSet(
											[]shelley.ShelleyTransactionInput{topInput},
										),
										TxOutputs: []DijkstraTransactionOutput{
											modelOutput(t, subCertInCoins-dTop*subCertPoolDep),
										},
									},
									TxIsValid: valid,
								}
								if len(built) > 0 {
									tx.Body.TxSubTransactions = cbor.NewSetType(built, true)
								}
								if top {
									tx.Body.TxCertificates = []common.CertificateWrapper{
										crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf),
									}
								}
								if legacy {
									tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
										[]common.PlutusV3Script{v3}, true,
									)
								}
								builder := mockledger.NewLedgerStateBuilder().
									WithNetworkId(0).
									WithUtxos(utxos)
								if pool {
									registered := crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf)
									builder = builder.WithPoolRegistrations(
										[]common.PoolRegistrationCertificate{
											*registered.Certificate.(*common.PoolRegistrationCertificate),
										},
									)
								}
								err := modelFirstError(tx, builder.Build(), pp, rules)
								want := subCertSpecDep(pool, b)
								if want != (err == nil) {
									t.Fatalf(
										"pool=%v %+v: reference accepts=%v, gOuroboros error=%v",
										pool, b, want, err,
									)
								}
								cells++
								if want {
									accepted++
								}
							}
						}
					}
				}
			}
		}
	}
	t.Logf("pool-deposit cells: %d (%d accepted)", cells, accepted)
}

// ---------------------------------------------------------------------------
// Pool-owner witnesses: a registration needs its owners unless the pool is
// registered before the level and its latest parameters (pending ones
// first) equal the new ones apart from the BLS key; never in a
// phase-2-invalid batch.

type subCertOwnLevel struct {
	cert string // "none", "A", "A2" (A with a BLS key), "B" (another pledge)
	own  bool
}

func subCertOwnClass(c string) string {
	if c == "A2" {
		return "A"
	}
	return c
}

func subCertSpecOwn(
	cur, pend string,
	valid bool,
	top subCertOwnLevel,
	subs []subCertOwnLevel,
) bool {
	for _, l := range append(append([]subCertOwnLevel{}, subs...), top) {
		if l.cert == "none" {
			continue
		}
		latest := cur
		if pend != "none" {
			latest = pend
		}
		required := cur == "none" || subCertOwnClass(l.cert) != latest
		if valid && required && !l.own {
			return false
		}
		if cur == "none" {
			cur = subCertOwnClass(l.cert)
		} else {
			pend = subCertOwnClass(l.cert)
		}
	}
	return true
}

func TestSubTxModelPoolOwnerWitnesses(t *testing.T) {
	rule := modelPhasedRule(t, common.UtxoValidationRuleRequiredVKeyWitnesses)
	pp := subCertPParams()
	blsKey := &common.LeiosKey{PublicKey: []byte{0x0b}}
	params := func(class string) *common.PoolRegistrationCertificate {
		switch class {
		case "A":
			return poolOwnerTestCert(t, 100, nil)
		case "A2":
			return poolOwnerTestCert(t, 100, blsKey)
		case "B":
			return poolOwnerTestCert(t, 200, nil)
		}
		return nil
	}
	var levels []subCertOwnLevel
	for _, cert := range []string{"none", "A", "A2", "B"} {
		for _, own := range []bool{false, true} {
			levels = append(levels, subCertOwnLevel{cert: cert, own: own})
		}
	}
	level := func(l subCertOwnLevel) ([]common.CertificateWrapper, DijkstraTransactionWitnessSet) {
		if l.cert == "none" {
			return nil, DijkstraTransactionWitnessSet{}
		}
		keys := [][]byte{poolOwnerTestOperatorKey}
		if l.own {
			keys = append(keys, poolOwnerTestOwnerKey)
		}
		return []common.CertificateWrapper{{
			Type:        uint(common.CertificateTypePoolRegistration),
			Certificate: params(l.cert),
		}}, poolOwnerTestWitnesses(keys...)
	}
	cells, accepted := 0, 0
	for _, cur := range []string{"none", "A", "B"} {
		pends := []string{"none"}
		if cur != "none" {
			pends = []string{"none", "A", "B"}
		}
		for _, pend := range pends {
			ls := pendingPoolState{
				LedgerState: poolOwnerTestState(params(cur)),
				pending:     params(pend),
			}
			for _, valid := range []bool{false, true} {
				for _, top := range levels {
					for _, subs := range modelUpTo2(levels) {
						built := make([]DijkstraSubTransaction, 0, len(subs))
						for k, l := range subs {
							certs, wits := level(l)
							built = append(built, DijkstraSubTransaction{
								Body: DijkstraSubTransactionBody{
									TxCertificates: certs,
									TxOutputs: []DijkstraTransactionOutput{
										modelOutput(t, uint64(1_000_000+k)),
									},
								},
								WitnessSet: wits,
							})
						}
						certs, wits := level(top)
						tx := &DijkstraTransaction{
							Body:       DijkstraTransactionBody{TxCertificates: certs},
							WitnessSet: wits,
							TxIsValid:  valid,
						}
						if len(built) > 0 {
							tx.Body.TxSubTransactions = cbor.NewSetType(built, true)
						}
						err := rule(tx, 0, ls, pp)
						want := subCertSpecOwn(cur, pend, valid, top, subs)
						if want != (err == nil) {
							t.Fatalf(
								"cur=%s pend=%s valid=%v top=%+v subs=%+v: reference accepts=%v, gOuroboros error=%v",
								cur, pend, valid, top, subs, want, err,
							)
						}
						cells++
						if want {
							accepted++
						}
					}
				}
			}
		}
	}
	t.Logf("pool-owner cells: %d (%d accepted)", cells, accepted)
}
