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

// More cells of a Dijkstra batch's certificates: pool retirement across
// levels, several certificates in one level, and legacy-mode withdrawals
// with certificates. Each restates the reference (cardano-ledger
// 1210d37fe6bd: Dijkstra Rules/Pool.hs RetirePool, Rules/SubEntities.hs
// 252-282, Rules/Entities.hs 276-412; Conway Rules/Certs.hs and
// Rules/Deleg.hs 236-303) and requires the production rules to accept
// exactly the cells it accepts.

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// subCertEpochState adds the epoch capability the retirement bound needs.
type subCertEpochState struct {
	subCertLedgerState
}

func (subCertEpochState) EpochForSlot(uint64) (uint64, error) { return 0, nil }

func subCertEntityRules(t *testing.T) []common.UtxoValidationRuleFunc {
	t.Helper()
	return []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleBatchWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleAccountBalanceIntervals),
		modelPhasedRule(t, common.UtxoValidationRuleWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleCertificateDeposits),
		modelPhasedRule(t, common.UtxoValidationRuleDelegation),
		modelPhasedRule(t, common.UtxoValidationRulePoolCertificates),
	}
}

// ---------------------------------------------------------------------------
// Pool retirement: a retirement needs the pool registered after the earlier
// levels and leaves it registered for the rest of the batch.

func subCertSpecRet(pool, valid bool, levels []string) bool {
	if !valid {
		return true
	}
	for _, c := range levels {
		switch c {
		case "regP":
			pool = true
		case "retP", "delegS":
			if !pool {
				return false
			}
		}
	}
	return true
}

func TestSubTxCellsPoolRetirement(t *testing.T) {
	rules := subCertEntityRules(t)
	pp := subCertPParams()
	pp.MaxEpoch = 18
	f := newSubCertFixture(t)
	certificate := func(c string) []common.CertificateWrapper {
		if c != "retP" {
			return f.certificate(t, c)
		}
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypePoolRetirement),
			Certificate: &common.PoolRetirementCertificate{
				CertType:    uint(common.CertificateTypePoolRetirement),
				PoolKeyHash: f.pool,
				Epoch:       1,
			},
		}}
	}
	certs := []string{"none", "regP", "retP", "delegS"}
	cells, accepted := 0, 0
	for _, pool := range []bool{false, true} {
		ls := subCertEpochState{f.ledger(t, true, 0, pool).(subCertLedgerState)}
		for _, valid := range []bool{false, true} {
			for _, top := range certs {
				for _, subs := range modelUpTo2(certs) {
					built := make([]DijkstraSubTransaction, 0, len(subs))
					for k, c := range subs {
						sub := subCertSubTx(t, k)
						sub.Body.TxCertificates = certificate(c)
						built = append(built, sub)
					}
					tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, built)
					tx.Body.TxCertificates = certificate(top)
					err := modelFirstError(tx, ls, pp, rules)
					want := subCertSpecRet(pool, valid, append(append([]string{}, subs...), top))
					if want != (err == nil) {
						t.Fatalf(
							"pool=%v valid=%v top=%s subs=%v: reference accepts=%v, gOuroboros error=%v",
							pool, valid, top, subs, want, err,
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
	t.Logf("retirement cells: %d (%d accepted)", cells, accepted)
}

// ---------------------------------------------------------------------------
// Several certificates in one level, run in order after the level's
// withdrawal.

type subCertMultiLevel struct {
	wd    bool
	certs []string
}

func (l subCertMultiLevel) String() string {
	return fmt.Sprintf("{wd:%v certs:%v}", l.wd, l.certs)
}

func subCertSpecMulti(
	reg bool,
	bal int64,
	pool, valid bool,
	top subCertMultiLevel,
	subs []subCertMultiLevel,
) bool {
	if !valid {
		return true
	}
	s := subCertAccount{ok: true, reg: reg, bal: bal, pool: pool}
	var total int64
	for _, l := range append(append([]subCertMultiLevel{}, subs...), top) {
		wd := "none"
		if l.wd {
			wd = "one"
			total++
		}
		s = subCertSpecLevel(s, subCertLevel{wd: wd, cert: "none"})
		for _, c := range l.certs {
			s = subCertSpecLevel(s, subCertLevel{wd: "none", cert: c})
		}
	}
	return total <= bal && (total == 0 || reg) && s.ok
}

func TestSubTxCellsMultiCertificateLevels(t *testing.T) {
	rules := subCertEntityRules(t)
	pp := subCertPParams()
	f := newSubCertFixture(t)
	var levels []subCertMultiLevel
	for _, wd := range []bool{false, true} {
		for _, certs := range modelUpTo2([]string{"regS", "unregS", "delegS", "regP"}) {
			levels = append(levels, subCertMultiLevel{wd: wd, certs: certs})
		}
	}
	body := func(l subCertMultiLevel) ([]common.CertificateWrapper, map[*common.Address]uint64) {
		var certs []common.CertificateWrapper
		for _, c := range l.certs {
			certs = append(certs, f.certificate(t, c)...)
		}
		wd := "none"
		if l.wd {
			wd = "one"
		}
		return certs, f.withdrawals(wd)
	}
	cells, accepted := 0, 0
	for _, reg := range []bool{false, true} {
		bals := []int64{0}
		if reg {
			bals = []int64{0, 1}
		}
		for _, bal := range bals {
			for _, pool := range []bool{false, true} {
				ls := f.ledger(t, reg, bal, pool)
				for _, valid := range []bool{false, true} {
					for _, top := range levels {
						for _, subs := range cellsUpTo1(levels) {
							built := make([]DijkstraSubTransaction, 0, len(subs))
							for k, l := range subs {
								sub := subCertSubTx(t, k)
								sub.Body.TxCertificates, sub.Body.TxWithdrawals = body(l)
								built = append(built, sub)
							}
							tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, built)
							tx.Body.TxCertificates, tx.Body.TxWithdrawals = body(top)
							err := modelFirstError(tx, ls, pp, rules)
							want := subCertSpecMulti(reg, bal, pool, valid, top, subs)
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
			}
		}
	}
	t.Logf("multi-certificate cells: %d (%d accepted)", cells, accepted)
}

// ---------------------------------------------------------------------------
// Legacy mode with certificates: the batch bound covers only the
// sub-transactions' withdrawals; the top's withdrawal must exist in the
// current accounts and drain the balance the sub-transactions left, before
// its certificates run.

func subCertSpecLegacy(
	reg bool,
	bal int64,
	valid bool,
	top subCertLevel,
	topAmt int64,
	subs []subCertLevel,
) bool {
	if !valid {
		return true
	}
	s := subCertAccount{ok: true, reg: reg, bal: bal}
	var total int64
	for _, l := range subs {
		if l.wd != "none" {
			if !reg {
				return false
			}
			total++
		}
		s = subCertSpecLevel(s, l)
	}
	if total > bal {
		return false
	}
	if top.wd != "none" {
		s.ok = s.ok && s.reg && topAmt == s.bal
		s.bal = 0
	}
	s = subCertSpecLevel(s, subCertLevel{wd: "none", cert: top.cert, dd: top.dd})
	return s.ok
}

func TestSubTxCellsLegacyWithdrawalCertificates(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleBatchWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleAccountBalanceIntervals),
		modelPhasedRule(t, common.UtxoValidationRuleWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleCertificateDeposits),
	}
	pp := subCertPParams()
	f := newSubCertFixture(t)
	v3 := common.PlutusV3Script{0x31}
	certs := []string{"none", "regS", "unregS"}
	var subLevels []subCertLevel
	for _, wd := range []string{"none", "one"} {
		for _, c := range certs {
			for _, dd := range []bool{false, true} {
				subLevels = append(subLevels, subCertLevel{wd: wd, cert: c, dd: dd})
			}
		}
	}
	cells, accepted := 0, 0
	for _, reg := range []bool{false, true} {
		bals := []int64{0}
		if reg {
			bals = []int64{0, 1}
		}
		for _, bal := range bals {
			topInput, topUtxo := dijkstraScriptLockedInput(t, v3, 7)
			ls := f.ledger(t, reg, bal, false, topUtxo)
			for _, valid := range []bool{false, true} {
				for _, topAmt := range []int64{-1, 0, 1, 2} {
					for _, c := range certs {
						for _, dd := range []bool{false, true} {
							top := subCertLevel{wd: "none", cert: c, dd: dd}
							if topAmt >= 0 {
								top.wd = "amount"
							}
							for _, subs := range cellsUpTo1(subLevels) {
								built := make([]DijkstraSubTransaction, 0, len(subs))
								for k, l := range subs {
									sub := subCertSubTx(t, k)
									sub.Body.TxCertificates = f.certificate(t, l.cert)
									sub.Body.TxWithdrawals = f.withdrawals(l.wd)
									sub.Body.TxDirectDeposits = f.directDeposits(l.dd)
									built = append(built, sub)
								}
								tx := &DijkstraTransaction{
									Body: DijkstraTransactionBody{
										TxInputs: conway.NewConwayTransactionInputSet(
											[]shelley.ShelleyTransactionInput{topInput},
										),
									},
									TxIsValid: valid,
								}
								if len(built) > 0 {
									tx.Body.TxSubTransactions = cbor.NewSetType(built, true)
								}
								tx.Body.TxCertificates = f.certificate(t, c)
								tx.Body.TxDirectDeposits = f.directDeposits(dd)
								if topAmt >= 0 {
									tx.Body.TxWithdrawals = map[*common.Address]uint64{
										f.address: uint64(topAmt) * subCertCoin,
									}
								}
								tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
									[]common.PlutusV3Script{v3}, true,
								)
								err := modelFirstError(tx, ls, pp, rules)
								want := subCertSpecLegacy(reg, bal, valid, top, topAmt, subs)
								if want != (err == nil) {
									t.Fatalf(
										"reg=%v bal=%d valid=%v top=%v amount=%d subs=%v: reference accepts=%v, gOuroboros error=%v",
										reg, bal, valid, top, topAmt, subs, want, err,
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
	t.Logf("legacy cells: %d (%d accepted)", cells, accepted)
}

// ---------------------------------------------------------------------------
// Account balance intervals: a level's interval holds for the account as
// the earlier levels left it, at the level's start; the top's starting
// interval holds for the original account (Dijkstra Rules/SubEntities.hs
// 264, Rules/Entities.hs 287-288 and 485-517). A lower bound is inclusive
// and an upper bound exclusive (Scripts.hs 670-674).

type subCertIvLevel struct {
	subCertLevel
	iv, st string // "none", "exact0", "exact1", "lower1", "upper1"
}

func subCertInIv(iv string, x int64) bool {
	switch iv {
	case "exact0":
		return x == 0
	case "exact1":
		return x == 1
	case "lower1":
		return x >= 1
	case "upper1":
		return x < 1
	}
	return true
}

func subCertSpecIv(
	reg bool,
	bal int64,
	valid bool,
	top subCertIvLevel,
	subs []subCertIvLevel,
) bool {
	if !valid {
		return true
	}
	if top.st != "none" && !(reg && subCertInIv(top.st, bal)) {
		return false
	}
	s := subCertAccount{ok: true, reg: reg, bal: bal}
	var total int64
	anyWd := false
	for _, l := range append(append([]subCertIvLevel{}, subs...), top) {
		if l.iv != "none" {
			s.ok = s.ok && s.reg && subCertInIv(l.iv, s.bal)
		}
		if l.wd != "none" {
			anyWd = true
		}
		if l.wd == "one" {
			total++
		}
		s = subCertSpecLevel(s, l.subCertLevel)
	}
	return total <= bal && (!anyWd || reg) && s.ok
}

func TestSubTxCellsBalanceIntervals(t *testing.T) {
	rules := subCertEntityRules(t)
	pp := subCertPParams()
	f := newSubCertFixture(t)
	ivs := []string{"none", "exact0", "exact1", "lower1", "upper1"}
	interval := func(iv string) DijkstraAccountBalanceIntervals {
		one, zero := uint64(subCertCoin), uint64(0)
		var v *DijkstraAccountBalanceInterval
		switch iv {
		case "exact0":
			v = &DijkstraAccountBalanceInterval{Exact: &zero}
		case "exact1":
			v = &DijkstraAccountBalanceInterval{Exact: &one}
		case "lower1":
			v = &DijkstraAccountBalanceInterval{LowerBound: &one}
		case "upper1":
			v = &DijkstraAccountBalanceInterval{UpperBound: &one}
		default:
			return nil
		}
		return DijkstraAccountBalanceIntervals{f.key: v}
	}
	var subLevels []subCertIvLevel
	for _, wd := range []string{"none", "one"} {
		for _, c := range []string{"none", "regS", "unregS"} {
			for _, dd := range []bool{false, true} {
				for _, iv := range ivs {
					subLevels = append(subLevels, subCertIvLevel{
						subCertLevel: subCertLevel{wd: wd, cert: c, dd: dd},
						iv:           iv, st: "none",
					})
				}
			}
		}
	}
	var tops []subCertIvLevel
	for _, l := range subLevels {
		for _, st := range ivs {
			l.st = st
			tops = append(tops, l)
		}
	}
	cells, accepted := 0, 0
	for _, reg := range []bool{false, true} {
		bals := []int64{0}
		if reg {
			bals = []int64{0, 1}
		}
		for _, bal := range bals {
			ls := f.ledger(t, reg, bal, false)
			for _, valid := range []bool{false, true} {
				for _, top := range tops {
					for _, subs := range cellsUpTo1(subLevels) {
						built := make([]DijkstraSubTransaction, 0, len(subs))
						for k, l := range subs {
							sub := subCertSubTx(t, k)
							sub.Body.TxCertificates = f.certificate(t, l.cert)
							sub.Body.TxWithdrawals = f.withdrawals(l.wd)
							sub.Body.TxDirectDeposits = f.directDeposits(l.dd)
							sub.Body.TxAccountBalanceIntervals = interval(l.iv)
							built = append(built, sub)
						}
						tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, built)
						tx.Body.TxCertificates = f.certificate(t, top.cert)
						tx.Body.TxWithdrawals = f.withdrawals(top.wd)
						tx.Body.TxDirectDeposits = f.directDeposits(top.dd)
						tx.Body.TxBalanceIntervals = interval(top.iv)
						tx.Body.TxStartingBalanceIntervals = interval(top.st)
						err := modelFirstError(tx, ls, pp, rules)
						want := subCertSpecIv(reg, bal, valid, top, subs)
						if want != (err == nil) {
							t.Fatalf(
								"reg=%v bal=%d valid=%v top=%+v subs=%+v: reference accepts=%v, gOuroboros error=%v",
								reg, bal, valid, top, subs, want, err,
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
	t.Logf("balance-interval cells: %d (%d accepted)", cells, accepted)
}
