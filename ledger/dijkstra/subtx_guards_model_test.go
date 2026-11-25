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

// Guards across a Dijkstra batch's levels, the context a top-level guard
// script sees, and the Dijkstra-only proposal checks, each restated from
// cardano-ledger 1210d37fe6bd (Dijkstra Rules/Utxow.hs 263-334 and 476-508,
// Rules/SubUtxow.hs 274, UTxO.hs 237-246, TxBody.hs 1423, TxInfo.hs
// 829-899, PParams.hs 1150-1172; Conway Rules/Gov.hs actionWellFormed and
// ExpirationEpochTooSmall) and walked cell by cell.

import (
	"bytes"
	"fmt"
	"math/big"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

// subGuardAllNative is the native script `all []`, which always succeeds.
func subGuardAllNative(t *testing.T) common.NativeScript {
	t.Helper()
	raw, err := cbor.Encode([]any{uint(1), []any{}})
	require.NoError(t, err)
	var script common.NativeScript
	require.NoError(t, script.UnmarshalCBOR(raw))
	return script
}

// subGuardSpec is the reference's verdict on a batch's guard: required
// guards listed at the top, datums right on every level against the
// batch's scripts, the script provided exactly when a listed script guard
// needs it, and the signature when it is a listed key guard. It holds
// whatever IsValid says.
func subGuardSpec(kind string, listed, wit bool, avail string, reqs []string) bool {
	for _, r := range reqs {
		if r != "none" && !listed {
			return false
		}
		if r == "none" {
			continue
		}
		switch {
		case kind == "key":
			if r != "nodatum" {
				return false
			}
		case avail == "none":
		case kind == "native":
			if r != "nodatum" {
				return false
			}
		case kind == "plutus":
			if r != "datum" {
				return false
			}
		}
	}
	if (listed && kind != "key") != (avail != "none") {
		return false
	}
	return !listed || kind != "key" || wit
}

func TestSubTxModelGuards(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleRedeemerAndScriptWitnesses),
		modelPhasedRule(t, common.UtxoValidationRuleScriptWitnesses),
		modelPhasedRule(t, common.UtxoValidationRuleExtraneousRedeemers),
		modelPhasedRule(t, common.UtxoValidationRuleRequiredVKeyWitnesses),
	}
	pp := dijkstraGuardTestPParams()
	ls := mockledger.NewLedgerStateBuilder().Build()
	signer := []byte("batch-guard-signer")
	native := subGuardAllNative(t)
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	datum := &common.Datum{Data: data.NewInteger(big.NewInt(1))}
	reqs := []string{"none", "nodatum", "datum"}
	cells, accepted := 0, 0
	for _, kind := range []string{"key", "native", "plutus"} {
		var guard common.Credential
		var script common.Script
		switch kind {
		case "key":
			guard = common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224Hash(signer),
			}
		case "native":
			script = native
			guard = dijkstraGuardCredentialForScript(native)
		case "plutus":
			script = plutus
			guard = dijkstraGuardCredentialForScript(plutus)
		}
		witnesses := func(w *DijkstraTransactionWitnessSet) {
			switch s := script.(type) {
			case common.NativeScript:
				w.WsNativeScripts = cbor.NewSetType([]common.NativeScript{s}, false)
			case common.PlutusV4Script:
				w.WsPlutusV4Scripts = cbor.NewSetType([]common.PlutusV4Script{s}, false)
			}
		}
		required := func(r string) DijkstraRequiredTopLevelGuards {
			switch r {
			case "nodatum":
				return dijkstraRequiredGuards(guard, nil)
			case "datum":
				return dijkstraRequiredGuards(guard, datum)
			}
			return nil
		}
		avails := []string{"none"}
		if kind != "key" {
			avails = []string{"none", "top", "sub"}
		}
		for _, valid := range []bool{false, true} {
			for _, listed := range []bool{false, true} {
				for _, wit := range []bool{false, true} {
					for _, avail := range avails {
						for _, top := range reqs {
							for _, subs := range modelUpTo2(reqs) {
								if avail == "sub" && len(subs) == 0 {
									continue
								}
								built := make([]DijkstraSubTransaction, 0, len(subs))
								for k, r := range subs {
									sub := DijkstraSubTransaction{Body: DijkstraSubTransactionBody{
										TxOutputs: []DijkstraTransactionOutput{modelOutput(t, uint64(1_000_000+k))},
									}}
									sub.Body.TxRequiredTopLevelGuards = required(r)
									if avail == "sub" && k == 0 {
										witnesses(&sub.WitnessSet)
									}
									built = append(built, sub)
								}
								tx := &DijkstraTransaction{TxIsValid: valid}
								if len(built) > 0 {
									tx.Body.TxSubTransactions = cbor.NewSetType(built, true)
								}
								tx.Body.TxRequiredTopLevelGuards = required(top)
								if listed {
									tx.Body.TxGuards = &DijkstraGuards{
										Credentials: []common.Credential{guard},
									}
									if kind == "plutus" {
										tx.WitnessSet.WsRedeemers = dijkstraGuardRedeemers()
									}
								}
								if avail == "top" {
									witnesses(&tx.WitnessSet)
								}
								if wit {
									tx.WitnessSet.VkeyWitnesses = cbor.NewSetType(
										[]common.VkeyWitness{{Vkey: signer}}, true,
									)
								}
								err := modelFirstError(tx, ls, pp, rules)
								want := subGuardSpec(kind, listed, wit, avail, append(append([]string{}, subs...), top))
								if want != (err == nil) {
									t.Fatalf(
										"kind=%s valid=%v listed=%v wit=%v avail=%s top=%s subs=%v: reference accepts=%v, gOuroboros error=%v",
										kind, valid, listed, wit, avail, top, subs, want, err,
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
	t.Logf("guard cells: %d (%d accepted)", cells, accepted)
}

// ---------------------------------------------------------------------------
// The context a top-level guard script sees: the datum each level gives it,
// keyed by level id, and every credential any level requires, once.

func TestSubTxModelGuardContext(t *testing.T) {
	plutus := dijkstraGuardTestPlutus(t, lang.LanguageVersionV4, false)
	g := dijkstraGuardCredentialForScript(plutus)
	h := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.Blake2b224Hash([]byte("batch-guard-other")),
	}
	reqs := []string{"none", "g", "gd", "h", "hd"}
	required := func(r string, k int) DijkstraRequiredTopLevelGuards {
		datum := &common.Datum{Data: data.NewInteger(big.NewInt(int64(100 + k)))}
		switch r {
		case "g":
			return dijkstraRequiredGuards(g, nil)
		case "gd":
			return dijkstraRequiredGuards(g, datum)
		case "h":
			return dijkstraRequiredGuards(h, nil)
		case "hd":
			return dijkstraRequiredGuards(h, datum)
		}
		return nil
	}
	cells := 0
	for _, top := range reqs {
		for _, subs := range modelUpTo2(reqs) {
			built := make([]DijkstraSubTransaction, 0, len(subs))
			for k, r := range subs {
				sub := subCertSubTx(t, k)
				sub.Body.TxRequiredTopLevelGuards = required(r, k+1)
				built = append(built, sub)
			}
			tx := modelBatch(t, true, modelLevel{ins: []int{0}}, built)
			tx.Body.TxRequiredTopLevelGuards = required(top, 0)
			var batch []dijkstraScriptLevel
			for _, level := range dijkstraTransactionLevels(tx) {
				batch = append(batch, dijkstraScriptLevel{tx: level})
			}

			// Reference: one entry per level giving g a datum, ordered by
			// level id; each required credential once, in Credential order.
			type entry struct {
				id    common.Blake2b256
				datum int64
			}
			var want []entry
			names := append(append([]string{}, subs...), top)
			for idx, level := range dijkstraTransactionLevels(tx) {
				if names[idx] == "gd" {
					k := idx + 1
					if idx == len(names)-1 {
						k = 0
					}
					want = append(want, entry{level.Hash(), int64(100 + k)})
				}
			}
			slices.SortFunc(want, func(a, b entry) int { return bytes.Compare(a.id[:], b.id[:]) })
			pairs := make([][2]data.PlutusData, len(want))
			for idx, e := range want {
				pairs[idx] = [2]data.PlutusData{
					data.NewByteString(e.id.Bytes()),
					data.NewInteger(big.NewInt(e.datum)),
				}
			}
			gotDatums, err := dijkstraTopTxInfoGuardDatumsV4(batch, g.Credential)
			require.NoError(t, err)
			require.Equal(t, data.NewMap(pairs), gotDatums,
				"top=%s subs=%v: guard datums", top, subs)

			unique := map[*common.Credential]struct{}{}
			for _, c := range []struct {
				name string
				cred common.Credential
			}{{"g", g}, {"h", h}} {
				for _, n := range names {
					if n == c.name || n == c.name+"d" {
						cred := c.cred
						unique[&cred] = struct{}{}
						break
					}
				}
			}
			items := []data.PlutusData{}
			for _, cred := range sortedDijkstraCredentials(unique) {
				items = append(items, cred.ToPlutusData())
			}
			gotRequired, err := dijkstraBatchRequiredGuardsV4(batch)
			require.NoError(t, err)
			require.Equal(t, data.NewList(items...), gotRequired,
				"top=%s subs=%v: required guards", top, subs)
			cells++
		}
	}
	t.Logf("guard-context cells: %d", cells)
}

// ---------------------------------------------------------------------------
// Dijkstra-only proposal checks at any level. Cells whose update sets eMax
// to zero or the pledge leverage cap below 1 are left out: the reference
// refuses them and gOuroboros does not yet.

type subPropUpdate struct {
	maxTx, nOpt, eMax, coins string // "unset", "zero", "ok"
	lev                      string // "unset", "lt1", "ge1", "nocap"
}

func (u subPropUpdate) wellFormed() bool {
	nums := []string{u.maxTx, u.nOpt, u.eMax, u.coins}
	if !slices.ContainsFunc(nums, func(f string) bool { return f != "unset" }) && u.lev == "unset" {
		return false
	}
	return !slices.Contains(nums, "zero") && u.lev != "lt1"
}

func (u subPropUpdate) build() DijkstraProtocolParameterUpdate {
	var ret DijkstraProtocolParameterUpdate
	uintField := func(f string) *uint {
		switch f {
		case "zero":
			v := uint(0)
			return &v
		case "ok":
			v := uint(5)
			return &v
		}
		return nil
	}
	ret.MaxTxSize = uintField(u.maxTx)
	ret.NOpt = uintField(u.nOpt)
	ret.MaxEpoch = uintField(u.eMax)
	switch u.coins {
	case "zero":
		v := uint64(0)
		ret.AdaPerUtxoByte = &v
	case "ok":
		v := uint64(4310)
		ret.AdaPerUtxoByte = &v
	}
	switch u.lev {
	case "lt1":
		ret.MaxPledgeLeverage = &cbor.Rat{Rat: big.NewRat(1, 2)}
	case "ge1":
		ret.MaxPledgeLeverage = &cbor.Rat{Rat: big.NewRat(3, 1)}
	case "nocap":
		ret.MaxPledgeLeverageSet = true
	}
	return ret
}

func TestSubTxModelProposalChecks(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleGovActionWellFormedness),
		modelPhasedRule(t, common.UtxoValidationRuleProposalProcedures),
	}
	pp := subGovPParams()
	ls := dijkstraEpochState{LedgerState: mockledger.NewLedgerStateBuilder().Build(), epoch: 4}
	fields := []string{"unset", "zero", "ok"}
	var updates []subPropUpdate
	for _, maxTx := range fields {
		for _, nOpt := range fields {
			for _, eMax := range fields {
				for _, coins := range fields {
					for _, lev := range []string{"unset", "lt1", "ge1", "nocap"} {
						u := subPropUpdate{maxTx, nOpt, eMax, coins, lev}
						if eMax == "zero" || lev == "lt1" {
							continue
						}
						updates = append(updates, u)
					}
				}
			}
		}
	}
	member := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("batch-proposal-member")),
	}
	proposal := func(action common.GovAction) []DijkstraProposalProcedure {
		return []DijkstraProposalProcedure{{PPGovAction: DijkstraGovAction{Action: action}}}
	}
	type cell struct {
		name   string
		action common.GovAction
		ok     bool
	}
	var actions []cell
	for _, u := range updates {
		actions = append(actions, cell{
			name: fmt.Sprintf("%+v", u),
			action: &DijkstraParameterChangeGovAction{
				Type:        uint(common.GovActionTypeParameterChange),
				ParamUpdate: u.build(),
			},
			ok: u.wellFormed(),
		})
	}
	for exp, ok := range map[uint64]bool{3: false, 4: false, 5: true} {
		m := member
		actions = append(actions, cell{
			name: fmt.Sprintf("committee member until epoch %d (current 4)", exp),
			action: &common.UpdateCommitteeGovAction{
				Type:       uint(common.GovActionTypeUpdateCommittee),
				CredEpochs: map[*common.Credential]uint64{&m: exp},
				Quorum:     cbor.Rat{Rat: big.NewRat(1, 2)},
			},
			ok: ok,
		})
	}
	cells, accepted := 0, 0
	for _, valid := range []bool{false, true} {
		for _, at := range []string{"top", "sub"} {
			for _, c := range actions {
				tx := &DijkstraTransaction{TxIsValid: valid}
				if at == "top" {
					tx.Body.TxProposalProcedures = proposal(c.action)
				} else {
					tx.Body.TxSubTransactions = cbor.NewSetType([]DijkstraSubTransaction{{
						Body: DijkstraSubTransactionBody{TxProposalProcedures: proposal(c.action)},
					}}, true)
				}
				err := modelFirstError(tx, ls, pp, rules)
				want := !valid || c.ok
				if want != (err == nil) {
					t.Fatalf(
						"valid=%v at=%s %s: reference accepts=%v, gOuroboros error=%v",
						valid, at, c.name, want, err,
					)
				}
				cells++
				if want {
					accepted++
				}
			}
		}
	}
	t.Logf("proposal-check cells: %d (%d accepted)", cells, accepted)
}
