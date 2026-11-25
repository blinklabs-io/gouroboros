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

// The tests in this file enumerate the governance part of a Dijkstra
// batch's levels: proposals, votes, and DRep and committee certificates,
// each judged against the state the earlier levels left. Each restates a
// reference predicate in Go (cardano-ledger 1210d37fe6bd: Dijkstra
// Rules/SubLedger.hs 259-299 and Rules/Ledger.hs 430-478; Conway
// Rules/Gov.hs 460-620 and Rules/GovCert.hs conwayGovCertTransition),
// builds every cell as a real Dijkstra transaction and requires the
// production rules to accept exactly the cells the predicate accepts.

import (
	"fmt"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// subGovLedgerState carries the committee-voting capability, the
// future-VRF-key capability the POOL rule asks for, and the mock's recorded
// stake deposits. It does not unwrap: the committee-voting capability must
// be found on the outermost state.
type subGovLedgerState struct {
	dijkstraCommitteeVotingState
	mock *mockledger.MockLedgerState
}

func (subGovLedgerState) IsFutureVrfKeyInUse(
	common.Blake2b256,
) (bool, common.PoolKeyHash, error) {
	return false, common.PoolKeyHash{}, nil
}

func (s subGovLedgerState) StakeCredentialDeposit(
	cred common.Credential,
) (*uint64, error) {
	return s.mock.StakeCredentialDeposit(cred)
}

type subGovFixture struct {
	sub      subCertFixture
	drep     common.Credential
	c1, c2   common.Credential
	hot      common.Credential
	origId   common.GovActionId
	poolHash common.PoolKeyHash
}

func newSubGovFixture(t *testing.T) subGovFixture {
	t.Helper()
	cred := func(s string) common.Credential {
		return common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte(s)),
		}
	}
	return subGovFixture{
		sub:      newSubCertFixture(t),
		drep:     cred("batch-gov-drep"),
		c1:       cred("batch-gov-c1"),
		c2:       cred("batch-gov-c2"),
		hot:      cred("batch-gov-hot"),
		origId:   common.GovActionId{TransactionId: common.Blake2b256{0x0c}},
		poolHash: common.PoolKeyHash{subCertPoolOp},
	}
}

func subGovPParams() *DijkstraProtocolParameters {
	pp := subCertPParams()
	pp.ProtocolVersion.Major = common.ProtocolVersionDijkstra
	return pp
}

// ledger builds the original state. c1 is the seated member's state:
// "member", "auth" (hot key authorized) or "resigned".
func (f subGovFixture) ledger(
	t *testing.T,
	reg, dreg, pool bool,
	c1 string,
) common.LedgerState {
	t.Helper()
	member := common.CommitteeMember{ColdKey: f.c1.Credential, ExpiryEpoch: 20}
	switch c1 {
	case "auth":
		hot := f.hot.Credential
		member.HotKey = &hot
	case "resigned":
		member.Resigned = true
	}
	builder := mockledger.NewLedgerStateBuilder().
		WithNetworkId(0).
		WithCommitteeMembers([]common.CommitteeMember{member}).
		WithGovActions(map[string]*common.GovActionState{
			fmt.Sprintf("%x#%d", f.origId.TransactionId[:], f.origId.GovActionIdx): {
				ActionId:   f.origId,
				ActionType: common.GovActionTypeInfo,
				ExpirySlot: math.MaxUint64,
				Action:     &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)},
			},
		})
	if reg {
		builder = builder.
			WithRewardAccountCredentialBalance(f.sub.credential, 0).
			WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
				mockledger.NewRewardAccountKey(f.sub.credential): subCertKeyDep,
			})
	}
	if dreg {
		drepDeposit := uint64(modelDRepDep)
		builder = builder.WithDRepRegistrations([]common.DRepRegistration{
			{Credential: f.drep, Deposit: &drepDeposit},
		})
	}
	if pool {
		registered := crossLevelPoolCert(t, subCertPoolOp, subCertPoolVrf)
		builder = builder.WithPoolRegistrations([]common.PoolRegistrationCertificate{
			*registered.Certificate.(*common.PoolRegistrationCertificate),
		})
	}
	mock := builder.Build()
	return subGovLedgerState{
		dijkstraCommitteeVotingState: dijkstraCommitteeVotingState{LedgerState: mock},
		mock:                         mock,
	}
}

func (f subGovFixture) certificate(
	t *testing.T,
	cert string,
) []common.CertificateWrapper {
	t.Helper()
	wrap := func(typ common.CertificateType, c common.Certificate) []common.CertificateWrapper {
		return []common.CertificateWrapper{{Type: uint(typ), Certificate: c}}
	}
	switch cert {
	case "regS", "regP":
		return f.sub.certificate(t, cert)
	case "regD":
		return wrap(common.CertificateTypeRegistrationDrep, &common.RegistrationDrepCertificate{
			CertType:       uint(common.CertificateTypeRegistrationDrep),
			DrepCredential: f.drep,
			Amount:         modelDRepDep,
		})
	case "unregD":
		return wrap(common.CertificateTypeDeregistrationDrep, &common.DeregistrationDrepCertificate{
			CertType:       uint(common.CertificateTypeDeregistrationDrep),
			DrepCredential: f.drep,
			Amount:         modelDRepDep,
		})
	case "updD":
		return wrap(common.CertificateTypeUpdateDrep, &common.UpdateDrepCertificate{
			CertType:       uint(common.CertificateTypeUpdateDrep),
			DrepCredential: f.drep,
		})
	case "authCC", "authC1", "authC2":
		cold := f.c1
		if cert == "authC2" {
			cold = f.c2
		}
		return wrap(common.CertificateTypeAuthCommitteeHot, &common.AuthCommitteeHotCertificate{
			CertType:       uint(common.CertificateTypeAuthCommitteeHot),
			ColdCredential: cold,
			HotCredential:  f.hot,
		})
	case "resignC1", "resignC2":
		cold := f.c1
		if cert == "resignC2" {
			cold = f.c2
		}
		return wrap(common.CertificateTypeResignCommitteeCold, &common.ResignCommitteeColdCertificate{
			CertType:       uint(common.CertificateTypeResignCommitteeCold),
			ColdCredential: cold,
		})
	}
	return nil
}

func (f subGovFixture) infoProposal() []DijkstraProposalProcedure {
	return []DijkstraProposalProcedure{{
		PPDeposit:       modelGovDep,
		PPRewardAccount: *f.sub.address,
		PPGovAction: DijkstraGovAction{
			Action: &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)},
		},
	}}
}

func (f subGovFixture) vote(
	voter string,
	target common.GovActionId,
) common.VotingProcedures {
	var v common.Voter
	switch voter {
	case "drep":
		v = common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: f.drep.Credential}
	case "cc":
		v = common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: f.hot.Credential,
		}
	case "spo":
		v = common.Voter{Type: common.VoterTypeStakingPoolKeyHash, Hash: f.poolHash}
	default:
		return nil
	}
	id := target
	return common.VotingProcedures{&v: {&id: {Vote: common.GovVoteYes}}}
}

func subGovRules(t *testing.T) []common.UtxoValidationRuleFunc {
	t.Helper()
	var rules []common.UtxoValidationRuleFunc
	for _, id := range []common.UtxoValidationRuleId{
		common.UtxoValidationRuleCertificateDeposits,
		common.UtxoValidationRuleDelegation,
		common.UtxoValidationRulePoolCertificates,
		common.UtxoValidationRuleCommitteeCertificates,
		common.UtxoValidationRuleGovActionWellFormedness,
		common.UtxoValidationRuleProposalProcedures,
		common.UtxoValidationRuleProposalDeposit,
		common.UtxoValidationRuleProposalNetworkIds,
		common.UtxoValidationRuleProposalReturnAccounts,
		common.UtxoValidationRuleProposalAncestry,
		common.UtxoValidationRuleUnknownVoters,
		common.UtxoValidationRuleUnknownGovActionIds,
		common.UtxoValidationRuleVotingOnExpiredGovAction,
		common.UtxoValidationRuleStakePoolVotingRestrictions,
		common.UtxoValidationRuleCCVotingRestrictions,
		common.UtxoValidationRuleUnelectedCommitteeVoters,
	} {
		rules = append(rules, modelPhasedRule(t, id))
	}
	return rules
}

// ---------------------------------------------------------------------------
// Proposals and votes.

type subGovLevel struct {
	cert   string // "none", "regS", "regD", "authCC", "regP"
	prop   bool
	voter  string // "none", "drep", "cc", "spo"
	target string // "orig", "sub"
}

func subGovLevels(sub bool) []subGovLevel {
	var ret []subGovLevel
	for _, cert := range []string{"none", "regS", "regD", "authCC", "regP"} {
		for _, prop := range []bool{false, true} {
			ret = append(ret, subGovLevel{cert: cert, prop: prop, voter: "none", target: "orig"})
			for _, voter := range []string{"drep", "cc", "spo"} {
				for _, target := range []string{"orig", "sub"} {
					// A vote names an action by a transaction id that covers
					// the vote, so a sub-transaction cannot name its own
					// proposal.
					if sub && target == "sub" {
						continue
					}
					ret = append(ret, subGovLevel{cert: cert, prop: prop, voter: voter, target: target})
				}
			}
		}
	}
	return ret
}

type subGovState struct {
	ok, reg, dreg, cc, pool bool
	props                   map[string]bool
}

// subGovSpecLevel is one level under the reference: certificates, then
// proposals (return account registered after them), then votes, which may
// name an existing action or an earlier level's proposal and need a voter
// known after the level's certificates.
func subGovSpecLevel(s subGovState, l subGovLevel, me string) subGovState {
	switch l.cert {
	case "regS":
		s.ok = s.ok && !s.reg
		s.reg = true
	case "regD":
		s.ok = s.ok && !s.dreg
		s.dreg = true
	case "authCC":
		s.cc = true
	case "regP":
		s.pool = true
	}
	if l.prop {
		s.ok = s.ok && s.reg
		s.props[me] = true
	}
	if l.voter != "none" {
		known := map[string]bool{"drep": s.dreg, "cc": s.cc, "spo": s.pool}[l.voter]
		s.ok = s.ok && known && (l.target == "orig" || s.props[l.target])
	}
	return s
}

func subGovSpec(
	reg, dreg, cc, pool, valid bool,
	top subGovLevel,
	subs []subGovLevel,
) bool {
	if !valid {
		return true
	}
	s := subGovState{ok: true, reg: reg, dreg: dreg, cc: cc, pool: pool, props: map[string]bool{}}
	for _, l := range subs {
		s = subGovSpecLevel(s, l, "sub")
	}
	return subGovSpecLevel(s, top, "top").ok
}

func TestSubTxCellsGovernance(t *testing.T) {
	rules := subGovRules(t)
	pp := subGovPParams()
	f := newSubGovFixture(t)
	tops, subLevels := subGovLevels(false), subGovLevels(true)
	for _, reg := range []bool{false, true} {
		for _, dreg := range []bool{false, true} {
			for _, cc := range []bool{false, true} {
				for _, pool := range []bool{false, true} {
					t.Run(fmt.Sprintf("reg=%v/dreg=%v/cc=%v/pool=%v", reg, dreg, cc, pool), func(t *testing.T) {
						t.Parallel()
						cells, accepted := 0, 0
						c1 := "member"
						if cc {
							c1 = "auth"
						}
						ls := f.ledger(t, reg, dreg, pool, c1)
						for _, valid := range []bool{false, true} {
							for _, top := range tops {
								for _, subs := range cellsUpTo1(subLevels) {
									tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, nil)
									subId := common.GovActionId{}
									if len(subs) > 0 {
										sub := subCertSubTx(t, 0)
										sub.Body.TxCertificates = f.certificate(t, subs[0].cert)
										if subs[0].prop {
											sub.Body.TxProposalProcedures = f.infoProposal()
										}
										sub.Body.TxVotingProcedures = f.vote(subs[0].voter, f.origId)
										subId.TransactionId = sub.Body.Id()
										tx.Body.TxSubTransactions = cbor.NewSetType(
											[]DijkstraSubTransaction{sub}, true,
										)
									}
									tx.Body.TxCertificates = f.certificate(t, top.cert)
									if top.prop {
										tx.Body.TxProposalProcedures = f.infoProposal()
									}
									target := f.origId
									if top.target == "sub" {
										target = subId
									}
									tx.Body.TxVotingProcedures = f.vote(top.voter, target)
									err := modelFirstError(tx, ls, pp, rules)
									want := subGovSpec(reg, dreg, cc, pool, valid, top, subs)
									if want != (err == nil) {
										t.Fatalf(
											"reg=%v dreg=%v cc=%v pool=%v valid=%v top=%+v subs=%+v: reference accepts=%v, gOuroboros error=%v",
											reg, dreg, cc, pool, valid, top, subs, want, err,
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
}

func cellsUpTo1[T any](items []T) [][]T {
	ret := [][]T{nil}
	for _, item := range items {
		ret = append(ret, []T{item})
	}
	return ret
}

// ---------------------------------------------------------------------------
// DRep and committee certificates. Levels here carry no committee
// proposal, so C2 is never a member.

func subGovSpecCom(dreg bool, c1 string, valid bool, levels []string) bool {
	if !valid {
		return true
	}
	ok := true
	for _, c := range levels {
		switch c {
		case "regD":
			ok = ok && !dreg
			dreg = true
		case "unregD":
			ok = ok && dreg
			dreg = false
		case "updD":
			ok = ok && dreg
		case "authC1":
			ok = ok && c1 != "resigned"
			c1 = "auth"
		case "resignC1":
			ok = ok && c1 != "resigned"
			c1 = "resigned"
		case "authC2", "resignC2":
			ok = false
		}
	}
	return ok
}

func TestSubTxCellsDRepCommitteeCertificates(t *testing.T) {
	rules := subGovRules(t)
	pp := subGovPParams()
	f := newSubGovFixture(t)
	certs := []string{"none", "regD", "unregD", "updD", "authC1", "resignC1", "authC2", "resignC2"}
	cells, accepted := 0, 0
	for _, dreg := range []bool{false, true} {
		for _, c1 := range []string{"member", "auth", "resigned"} {
			ls := f.ledger(t, true, dreg, false, c1)
			for _, valid := range []bool{false, true} {
				for _, top := range certs {
					for _, subs := range modelUpTo2(certs) {
						built := make([]DijkstraSubTransaction, 0, len(subs))
						for k, c := range subs {
							sub := subCertSubTx(t, k)
							sub.Body.TxCertificates = f.certificate(t, c)
							built = append(built, sub)
						}
						tx := modelBatch(t, valid, modelLevel{ins: []int{0}}, built)
						tx.Body.TxCertificates = f.certificate(t, top)
						err := modelFirstError(tx, ls, pp, rules)
						levels := append(append([]string{}, subs...), top)
						want := subGovSpecCom(dreg, c1, valid, levels)
						if want != (err == nil) {
							t.Fatalf(
								"dreg=%v c1=%s valid=%v top=%s subs=%v: reference accepts=%v, gOuroboros error=%v",
								dreg, c1, valid, top, subs, want, err,
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
	t.Logf("DRep and committee cells: %d (%d accepted)", cells, accepted)
}
