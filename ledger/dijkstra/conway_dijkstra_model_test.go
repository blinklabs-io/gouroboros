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

// The tests in this file enumerate the transition cells of the TLA+ models
// ConwayLedger.tla and SubTx.tla (gouroboros-tla-models,
// specs/24-ledger-conway-dijkstra). Each test restates the model's
// reference predicate (Spec*) in Go, builds every cell as a real Conway or
// Dijkstra transaction against a mock ledger state, runs the rules the
// predicate covers, and requires gOuroboros to accept exactly the cells the
// predicate accepts.

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	// One model unit of reference-script size. Conway's fixed tier is
	// 25,600 bytes, two units, and its fixed per-transaction limit is
	// 204,800 bytes, sixteen units.
	modelRefUnit   = 12_800
	modelMaxRefTx  = 16
	modelTreasury  = 1_000
	modelInCoins   = 10
	modelKeyDep    = 2
	modelDRepDep   = 3
	modelGovDep    = 4
	modelWdrlUnits = 1_000_000
)

var (
	// minFeeRefScriptCostPerByte: a third of a lovelace, so that flooring
	// each tier separately differs from flooring the total once.
	modelRefBase = big.NewRat(1, 3)
	// Dijkstra's stride and multiplier are protocol parameters; the test
	// sets a multiplier other than Conway's fixed 6/5.
	modelDijkstraMult = big.NewRat(3, 2)
)

func modelInput(index int) shelley.ShelleyTransactionInput {
	return shelley.MustNewShelleyTransactionInput(
		"2424242424242424242424242424242424242424242424242424242424242424",
		index,
	)
}

func modelKeyAddress(t *testing.T) common.Address {
	t.Helper()
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x61}, common.AddressHashSize),
		bytes.Repeat([]byte{0x62}, common.AddressHashSize),
	)
	require.NoError(t, err)
	return address
}

func modelUtxo(
	t *testing.T,
	input shelley.ShelleyTransactionInput,
	coins uint64,
	scriptSize int,
) common.Utxo {
	t.Helper()
	output := babbage.BabbageTransactionOutput{
		OutputAddress: modelKeyAddress(t),
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: coins},
	}
	if scriptSize > 0 {
		output.TxOutScriptRef = &common.ScriptRef{
			Type:   common.ScriptRefTypePlutusV3,
			Script: common.PlutusV3Script(make([]byte, scriptSize)),
		}
	}
	return common.Utxo{Id: input, Output: output}
}

func modelOutput(t *testing.T, coins uint64) DijkstraTransactionOutput {
	t.Helper()
	return DijkstraTransactionOutput{
		Output: &babbage.BabbageTransactionOutput{
			OutputAddress: modelKeyAddress(t),
			OutputAmount:  mary.MaryTransactionOutputValue{Amount: coins},
		},
	}
}

func modelConwayOutput(t *testing.T, coins uint64) babbage.BabbageTransactionOutput {
	t.Helper()
	return babbage.BabbageTransactionOutput{
		OutputAddress: modelKeyAddress(t),
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: coins},
	}
}

func modelRewardAddress(t *testing.T, header byte, fill byte) *common.Address {
	t.Helper()
	raw := append(
		[]byte{header},
		bytes.Repeat([]byte{fill}, common.AddressHashSize)...)
	address, err := common.NewAddressFromBytes(raw)
	require.NoError(t, err)
	return &address
}

// The withdrawing key credential K and the script credential S.
func modelKeyCredential() common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.NewBlake2b224(bytes.Repeat([]byte{0x4b}, 28)),
	}
}

func modelScriptCredential() common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.NewBlake2b224(bytes.Repeat([]byte{0x53}, 28)),
	}
}

func modelDRepCredential(fill byte) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.NewBlake2b224(bytes.Repeat([]byte{fill}, 28)),
	}
}

// modelRefFee restates ConwayLedger.tla's RefFee: every tier exactly, one
// floor at the end.
func modelRefFee(size uint64, stride uint64, mult *big.Rat) uint64 {
	price := new(big.Rat).Set(modelRefBase)
	total := new(big.Rat)
	for size > 0 {
		tier := min(size, stride)
		total.Add(total, new(big.Rat).Mul(price, new(big.Rat).SetInt64(int64(tier))))
		size -= tier
		price.Mul(price, mult)
	}
	return new(big.Int).Quo(total.Num(), total.Denom()).Uint64()
}

func modelConwayPParams(pv uint) *conway.ConwayProtocolParameters {
	return &conway.ConwayProtocolParameters{
		ProtocolVersion:            common.ProtocolParametersProtocolVersion{Major: pv},
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: new(big.Rat).Set(modelRefBase)},
		KeyDeposit:                 modelKeyDep,
		DRepDeposit:                modelDRepDep,
		GovActionDeposit:           modelGovDep,
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(0, 1)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(0, 1)},
		},
	}
}

func modelDijkstraPParams() *DijkstraProtocolParameters {
	pp := dijkstraGuardTestPParams()
	pp.MinFeeRefScriptCostPerByte = &cbor.Rat{Rat: new(big.Rat).Set(modelRefBase)}
	pp.KeyDeposit = modelKeyDep
	pp.DRepDeposit = modelDRepDep
	pp.GovActionDeposit = modelGovDep
	pp.ExecutionCosts = common.ExUnitPrice{
		MemPrice:  &cbor.Rat{Rat: big.NewRat(0, 1)},
		StepPrice: &cbor.Rat{Rat: big.NewRat(0, 1)},
	}
	pp.RefScriptCostStride = uint32(conway.RefScriptCostStride)
	pp.RefScriptCostMultiplier = &cbor.Rat{Rat: new(big.Rat).Set(modelDijkstraMult)}
	pp.MaxRefScriptSizePerTx = modelMaxRefTx * modelRefUnit
	return pp
}

// modelPhasedRule returns the production Dijkstra rule for id, wrapped by
// its phase classification.
func modelPhasedRule(
	t *testing.T,
	id common.UtxoValidationRuleId,
) common.UtxoValidationRuleFunc {
	t.Helper()
	for idx, descriptor := range utxoValidationRuleDescriptors {
		if descriptor.Id == id {
			return UtxoValidationRules[idx]
		}
	}
	t.Fatalf("Dijkstra validation rule %q is not registered", id)
	return nil
}

func modelFirstError(
	tx common.Transaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
	rules []common.UtxoValidationRuleFunc,
) error {
	for _, rule := range rules {
		if err := rule(tx, 0, ls, pp); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ConwayLedger.tla, GateTxs.

type modelGateTx struct {
	valid   bool
	treas   string // "none", "ok", "bad"
	ref     uint64 // reference-script size in model units
	short   bool
	wdrl    string // "none", "key", "script"
	overlap bool
}

type modelWorld struct {
	pv    uint
	reg   bool
	deleg string // "none", "drep", "abstain"
}

func (w modelWorld) String() string {
	return fmt.Sprintf("pv=%d reg=%v deleg=%s", w.pv, w.reg, w.deleg)
}

// modelSpecGate restates ConwayLedger.tla's SpecGate.
func modelSpecGate(dijkstra bool, w modelWorld, tx modelGateTx) bool {
	gated := dijkstra || tx.valid
	if gated && tx.treas == "bad" {
		return false
	}
	if gated && tx.ref > modelMaxRefTx {
		return false
	}
	if !dijkstra && tx.valid && w.pv != 9 && tx.wdrl == "key" && w.deleg == "none" {
		return false
	}
	if tx.valid && tx.wdrl != "none" && !w.reg {
		return false
	}
	if tx.overlap && !dijkstra && (w.pv == 9 || w.pv == 10) {
		return false
	}
	// The fee is RefFee less one when short: below the minimum by
	// construction.
	return !tx.short
}

func modelGateWorlds(dijkstra bool) []modelWorld {
	pvs := []uint{9, 10, 11}
	if dijkstra {
		pvs = []uint{12}
	}
	var worlds []modelWorld
	for _, pv := range pvs {
		for _, reg := range []bool{false, true} {
			for _, deleg := range []string{"none", "drep", "abstain"} {
				worlds = append(worlds, modelWorld{pv: pv, reg: reg, deleg: deleg})
			}
		}
	}
	return worlds
}

func modelGateTxs(stride uint64, mult *big.Rat) []modelGateTx {
	var txs []modelGateTx
	for _, valid := range []bool{false, true} {
		for _, treas := range []string{"none", "ok", "bad"} {
			for ref := uint64(0); ref <= modelMaxRefTx+1; ref++ {
				for _, short := range []bool{false, true} {
					if short && modelRefFee(ref*modelRefUnit, stride, mult) == 0 {
						continue
					}
					for _, wdrl := range []string{"none", "key", "script"} {
						for _, overlap := range []bool{false, true} {
							txs = append(txs, modelGateTx{
								valid: valid, treas: treas, ref: ref,
								short: short, wdrl: wdrl, overlap: overlap,
							})
						}
					}
				}
			}
		}
	}
	return txs
}

// modelGateLedger builds the world's state: input 0 (key-locked, no
// script), input 1 (holding a reference script of the cell's size), the
// treasury, and the key and script reward accounts, both registered or
// neither (the model's reg).
func modelGateLedger(t *testing.T, w modelWorld, ref uint64) common.LedgerState {
	t.Helper()
	key := modelKeyCredential()
	builder := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{
			modelUtxo(t, modelInput(0), modelInCoins, 0),
			modelUtxo(t, modelInput(1), modelInCoins, int(ref*modelRefUnit)),
		}).
		WithTreasuryAmount(modelTreasury).
		WithDRepDelegation(func(cred common.Credential) (*common.Drep, error) {
			if cred.CredType != key.CredType || cred.Credential != key.Credential {
				return nil, nil
			}
			switch w.deleg {
			case "drep":
				return &common.Drep{
					Type:       common.DrepTypeAddrKeyHash,
					Credential: bytes.Repeat([]byte{0x44}, 28),
				}, nil
			case "abstain":
				return &common.Drep{Type: common.DrepTypeAbstain}, nil
			}
			return nil, nil
		})
	if w.reg {
		builder = builder.
			WithRewardAccountCredentialBalance(key, modelWdrlUnits).
			WithRewardAccountCredentialBalance(modelScriptCredential(), modelWdrlUnits)
	}
	return builder.Build()
}

// modelGateBody fills the fields every era's body shares.
type modelGateFields struct {
	inputs     []shelley.ShelleyTransactionInput
	refs       []shelley.ShelleyTransactionInput
	fee        uint64
	withdrawal map[*common.Address]uint64
	treasury   *uint64
}

func modelGateFieldsFor(
	t *testing.T,
	tx modelGateTx,
	stride uint64,
	mult *big.Rat,
) modelGateFields {
	t.Helper()
	fee := modelRefFee(tx.ref*modelRefUnit, stride, mult)
	if tx.short {
		fee--
	}
	f := modelGateFields{
		inputs: []shelley.ShelleyTransactionInput{modelInput(0)},
		refs:   []shelley.ShelleyTransactionInput{modelInput(1)},
		fee:    fee,
	}
	if tx.overlap {
		// Spending the reference-script output too: the script must be
		// counted once (a set union of spent and reference inputs).
		f.inputs = append(f.inputs, modelInput(1))
	}
	switch tx.wdrl {
	case "key":
		f.withdrawal = map[*common.Address]uint64{
			modelRewardAddress(t, 0xe0, 0x4b): modelWdrlUnits,
		}
	case "script":
		f.withdrawal = map[*common.Address]uint64{
			modelRewardAddress(t, 0xf0, 0x53): modelWdrlUnits,
		}
	}
	switch tx.treas {
	case "ok":
		v := uint64(modelTreasury)
		f.treasury = &v
	case "bad":
		v := uint64(modelTreasury + 1)
		f.treasury = &v
	}
	return f
}

func modelConwayGateTx(t *testing.T, tx modelGateTx) *conway.ConwayTransaction {
	t.Helper()
	f := modelGateFieldsFor(t, tx, conway.RefScriptCostStride, big.NewRat(6, 5))
	ret := &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxInputs:          conway.NewConwayTransactionInputSet(f.inputs),
			TxReferenceInputs: cbor.NewSetType(f.refs, true),
			TxFee:             f.fee,
			TxWithdrawals:     f.withdrawal,
		},
		TxIsValid: tx.valid,
	}
	if f.treasury != nil {
		ret.Body.TxCurrentTreasuryValue = *f.treasury
		ret.Body.SetCurrentTreasuryValuePresence(true)
	}
	return ret
}

func modelDijkstraGateTx(t *testing.T, tx modelGateTx) *DijkstraTransaction {
	t.Helper()
	f := modelGateFieldsFor(t, tx, conway.RefScriptCostStride, modelDijkstraMult)
	ret := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs:          conway.NewConwayTransactionInputSet(f.inputs),
			TxReferenceInputs: cbor.NewSetType(f.refs, true),
			TxFee:             f.fee,
			TxWithdrawals:     f.withdrawal,
		},
		TxIsValid: tx.valid,
	}
	if f.treasury != nil {
		ret.Body.TxCurrentTreasuryValue = *f.treasury
		ret.Body.SetCurrentTreasuryValuePresence(true)
	}
	return ret
}

func TestConwayLedgerModelGates(t *testing.T) {
	conwayRules := []common.UtxoValidationRuleFunc{
		conway.UtxoValidateFeeTooSmallUtxo,
		common.UtxoValidateCurrentTreasuryValue,
		conway.UtxoValidateRefScriptSizePerTx,
		conway.UtxoValidateWithdrawals,
		conway.UtxoValidateDisjointRefInputs,
	}
	dijkstraRules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleFeeTooSmall),
		modelPhasedRule(t, common.UtxoValidationRuleCurrentTreasuryValue),
		modelPhasedRule(t, common.UtxoValidationRuleRefScriptSizePerTx),
		modelPhasedRule(t, common.UtxoValidationRuleBatchWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleWithdrawals),
	}
	for _, dijkstra := range []bool{false, true} {
		era := "Conway"
		mult := big.NewRat(6, 5)
		if dijkstra {
			era = "Dijkstra"
			mult = modelDijkstraMult
		}
		cells := 0
		for _, w := range modelGateWorlds(dijkstra) {
			for _, tx := range modelGateTxs(conway.RefScriptCostStride, mult) {
				ls := modelGateLedger(t, w, tx.ref)
				var err error
				if dijkstra {
					err = modelFirstError(
						modelDijkstraGateTx(t, tx), ls, modelDijkstraPParams(), dijkstraRules,
					)
				} else {
					err = modelFirstError(
						modelConwayGateTx(t, tx), ls, modelConwayPParams(w.pv), conwayRules,
					)
				}
				want := modelSpecGate(dijkstra, w, tx)
				if want != (err == nil) {
					t.Fatalf(
						"%s %s %+v: reference accepts=%v, gOuroboros error=%v",
						era, w, tx, want, err,
					)
				}
				cells++
			}
		}
		t.Logf("%s gate cells: %d", era, cells)
	}
}

// ---------------------------------------------------------------------------
// ConwayLedger.tla, BalTxs.

type modelBalTx struct {
	valid bool
	cert  string
	don   uint64
	delta int64
}

func modelSpecRefund(dijkstra bool, reg bool, tx modelBalTx) uint64 {
	switch tx.cert {
	case "unregStake":
		if dijkstra || reg {
			return modelKeyDep
		}
	case "unregDRep":
		return modelDRepDep
	}
	return 0
}

func modelSpecDeposit(tx modelBalTx) uint64 {
	switch tx.cert {
	case "regStake":
		return modelKeyDep
	case "regDRep":
		return modelDRepDep
	case "prop":
		return modelGovDep
	}
	return 0
}

// modelSpecBal restates ConwayLedger.tla's SpecBal. The output is built to
// balance by the reference's count and moved by delta, so conservation
// holds exactly when delta is zero.
func modelSpecBal(dijkstra bool, reg bool, tx modelBalTx) bool {
	if (!dijkstra || tx.valid) && tx.delta != 0 {
		return false
	}
	if tx.valid && tx.cert == "unregStake" && !reg {
		return false
	}
	if tx.valid && tx.cert == "regStake" && reg {
		return false
	}
	return true
}

func modelBalCerts(t *testing.T, tx modelBalTx) []common.CertificateWrapper {
	t.Helper()
	key := modelKeyCredential()
	switch tx.cert {
	case "regStake":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeRegistration),
			Certificate: &common.RegistrationCertificate{
				CertType:        uint(common.CertificateTypeRegistration),
				StakeCredential: key,
				Amount:          modelKeyDep,
			},
		}}
	case "unregStake":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeDeregistration),
			Certificate: &common.DeregistrationCertificate{
				CertType:        uint(common.CertificateTypeDeregistration),
				StakeCredential: key,
				Amount:          modelKeyDep,
			},
		}}
	case "regDRep":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeRegistrationDrep),
			Certificate: &common.RegistrationDrepCertificate{
				CertType:       uint(common.CertificateTypeRegistrationDrep),
				DrepCredential: modelDRepCredential(0x71),
				Amount:         modelDRepDep,
			},
		}}
	case "unregDRep":
		return []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeDeregistrationDrep),
			Certificate: &common.DeregistrationDrepCertificate{
				CertType:       uint(common.CertificateTypeDeregistrationDrep),
				DrepCredential: modelDRepCredential(0x72),
				Amount:         modelDRepDep,
			},
		}}
	}
	return nil
}

func modelBalLedger(t *testing.T, reg bool) common.LedgerState {
	t.Helper()
	key := modelKeyCredential()
	drepDeposit := uint64(modelDRepDep)
	builder := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{modelUtxo(t, modelInput(0), modelInCoins, 0)}).
		WithDRepRegistrations([]common.DRepRegistration{{
			Credential: modelDRepCredential(0x72),
			Deposit:    &drepDeposit,
		}})
	if reg {
		builder = builder.
			WithRewardAccountCredentialBalance(key, 0).
			WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
				mockledger.NewRewardAccountKey(key): modelKeyDep,
			})
	}
	return builder.Build()
}

func TestConwayLedgerModelBalance(t *testing.T) {
	conwayRules := []common.UtxoValidationRuleFunc{
		conway.UtxoValidateValueNotConservedUtxo,
		conway.UtxoValidateCertificateDeposits,
	}
	dijkstraRules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleValueNotConserved),
		modelPhasedRule(t, common.UtxoValidationRuleCertificateDeposits),
	}
	rewardAccount := *modelRewardAddress(t, 0xe0, 0x4b)
	for _, dijkstra := range []bool{false, true} {
		cells := 0
		for _, reg := range []bool{false, true} {
			ls := modelBalLedger(t, reg)
			for _, valid := range []bool{false, true} {
				for _, cert := range []string{"none", "regStake", "unregStake", "regDRep", "unregDRep", "prop"} {
					for _, don := range []uint64{0, 1} {
						for _, delta := range []int64{-1, 0, 1} {
							tx := modelBalTx{valid: valid, cert: cert, don: don, delta: delta}
							out := uint64(int64(modelInCoins+modelSpecRefund(dijkstra, reg, tx)-modelSpecDeposit(tx)-don) + delta)
							certs := modelBalCerts(t, tx)
							var err error
							if dijkstra {
								body := DijkstraTransactionBody{
									TxInputs:       conway.NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{modelInput(0)}),
									TxOutputs:      []DijkstraTransactionOutput{modelOutput(t, out)},
									TxCertificates: certs,
									TxDonation:     don,
								}
								if cert == "prop" {
									body.TxProposalProcedures = []DijkstraProposalProcedure{{
										PPDeposit:       modelGovDep,
										PPRewardAccount: rewardAccount,
										PPGovAction: DijkstraGovAction{
											Type:   uint(common.GovActionTypeInfo),
											Action: &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)},
										},
									}}
								}
								err = modelFirstError(
									&DijkstraTransaction{Body: body, TxIsValid: valid},
									ls, modelDijkstraPParams(), dijkstraRules,
								)
							} else {
								body := conway.ConwayTransactionBody{
									TxInputs:       conway.NewConwayTransactionInputSet([]shelley.ShelleyTransactionInput{modelInput(0)}),
									TxOutputs:      []babbage.BabbageTransactionOutput{modelConwayOutput(t, out)},
									TxCertificates: certs,
									TxDonation:     don,
								}
								if cert == "prop" {
									body.TxProposalProcedures = []conway.ConwayProposalProcedure{{
										PPDeposit:       modelGovDep,
										PPRewardAccount: rewardAccount,
										PPGovAction: conway.ConwayGovAction{
											Type:   uint(common.GovActionTypeInfo),
											Action: &common.InfoGovAction{Type: uint(common.GovActionTypeInfo)},
										},
									}}
								}
								err = modelFirstError(
									&conway.ConwayTransaction{Body: body, TxIsValid: valid},
									ls, modelConwayPParams(11), conwayRules,
								)
							}
							want := modelSpecBal(dijkstra, reg, tx)
							if want != (err == nil) {
								t.Fatalf(
									"dijkstra=%v reg=%v %+v: reference accepts=%v, gOuroboros error=%v",
									dijkstra, reg, tx, want, err,
								)
							}
							cells++
						}
					}
				}
			}
		}
		t.Logf("dijkstra=%v balance cells: %d", dijkstra, cells)
	}
}

// ---------------------------------------------------------------------------
// SubTx.tla. Places "a" and "b" are model inputs 10 and 11 and exist; "c"
// is input 12 and does not.

type modelLevel struct {
	ins  []int // place indexes 0..2
	refs []int
}

func modelPlace(p int) shelley.ShelleyTransactionInput { return modelInput(10 + p) }

func modelPlaces(ps []int) []shelley.ShelleyTransactionInput {
	ret := make([]shelley.ShelleyTransactionInput, 0, len(ps))
	for _, p := range ps {
		ret = append(ret, modelPlace(p))
	}
	return ret
}

func modelSubsets() [][]int {
	var ret [][]int
	for mask := range 8 {
		var s []int
		for p := range 3 {
			if mask&(1<<p) != 0 {
				s = append(s, p)
			}
		}
		ret = append(ret, s)
	}
	return ret
}

func modelLevels() []modelLevel {
	var ret []modelLevel
	for _, ins := range modelSubsets() {
		for _, refs := range [][]int{nil, {0}, {2}} {
			ret = append(ret, modelLevel{ins: ins, refs: refs})
		}
	}
	return ret
}

func modelUpTo2[T any](items []T) [][]T {
	ret := [][]T{nil}
	for _, x := range items {
		ret = append(ret, []T{x})
	}
	for _, x := range items {
		for _, y := range items {
			ret = append(ret, []T{x, y})
		}
	}
	return ret
}

func modelIntersects(a, b []int) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// modelSpecInputs restates SubTx.tla's SpecInputs.
func modelSpecInputs(valid bool, top modelLevel, subs []modelLevel) bool {
	for _, l := range append(append([]modelLevel{}, subs...), top) {
		if len(l.ins) == 0 {
			return false
		}
		for _, p := range append(append([]int{}, l.ins...), l.refs...) {
			if p == 2 {
				return false
			}
		}
	}
	if !valid {
		return true
	}
	var spent []int
	for _, sub := range subs {
		if modelIntersects(spent, sub.ins) {
			return false
		}
		spent = append(spent, sub.ins...)
	}
	return !modelIntersects(top.ins, spent)
}

// modelSubTx builds a sub-transaction. Each carries an output of its own
// amount so that two sub-transactions with the same inputs are distinct.
func modelSubTx(
	t *testing.T,
	l modelLevel,
	k int,
) DijkstraSubTransaction {
	t.Helper()
	body := DijkstraSubTransactionBody{
		TxInputs:  conway.NewConwayTransactionInputSet(modelPlaces(l.ins)),
		TxOutputs: []DijkstraTransactionOutput{modelOutput(t, uint64(1_000_000+k))},
	}
	if len(l.refs) > 0 {
		body.TxReferenceInputs = cbor.NewSetType(modelPlaces(l.refs), true)
	}
	return DijkstraSubTransaction{Body: body}
}

func modelBatch(
	t *testing.T,
	valid bool,
	top modelLevel,
	subs []DijkstraSubTransaction,
) *DijkstraTransaction {
	t.Helper()
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxInputs: conway.NewConwayTransactionInputSet(modelPlaces(top.ins)),
		},
		TxIsValid: valid,
	}
	if len(top.refs) > 0 {
		tx.Body.TxReferenceInputs = cbor.NewSetType(modelPlaces(top.refs), true)
	}
	if len(subs) > 0 {
		tx.Body.TxSubTransactions = cbor.NewSetType(subs, true)
	}
	return tx
}

func TestSubTxModelInputs(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleInputSetEmpty),
		modelPhasedRule(t, common.UtxoValidationRuleNoDuplicateInputs),
		modelPhasedRule(t, common.UtxoValidationRuleBadInputs),
	}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
		modelUtxo(t, modelPlace(0), modelInCoins, 0),
		modelUtxo(t, modelPlace(1), modelInCoins, 0),
	}).Build()
	pp := modelDijkstraPParams()
	cells := 0
	for _, valid := range []bool{false, true} {
		for _, top := range modelLevels() {
			for _, subs := range modelUpTo2(modelLevels()) {
				built := make([]DijkstraSubTransaction, 0, len(subs))
				for k, sub := range subs {
					built = append(built, modelSubTx(t, sub, k))
				}
				err := modelFirstError(modelBatch(t, valid, top, built), ls, pp, rules)
				want := modelSpecInputs(valid, top, subs)
				if want != (err == nil) {
					t.Fatalf(
						"valid=%v top=%+v subs=%+v: reference accepts=%v, gOuroboros error=%v",
						valid, top, subs, want, err,
					)
				}
				cells++
			}
		}
	}
	t.Logf("input cells: %d", cells)
}

func TestSubTxModelTreasury(t *testing.T) {
	rule := modelPhasedRule(t, common.UtxoValidationRuleCurrentTreasuryValue)
	ls := mockledger.NewLedgerStateBuilder().WithTreasuryAmount(modelTreasury).Build()
	pp := modelDijkstraPParams()
	states := []string{"none", "ok", "bad"}
	value := func(s string) uint64 {
		if s == "bad" {
			return modelTreasury + 1
		}
		return modelTreasury
	}
	cells := 0
	for _, valid := range []bool{false, true} {
		for _, top := range states {
			for _, subs := range modelUpTo2(states) {
				built := make([]DijkstraSubTransaction, 0, len(subs))
				want := top != "bad"
				for k, s := range subs {
					sub := modelSubTx(t, modelLevel{ins: []int{0}}, k)
					if s != "none" {
						sub.Body.TxCurrentTreasuryValue = value(s)
						sub.Body.SetCurrentTreasuryValuePresence(true)
					}
					want = want && s != "bad"
					built = append(built, sub)
				}
				tx := modelBatch(t, valid, modelLevel{ins: []int{1}}, built)
				if top != "none" {
					tx.Body.TxCurrentTreasuryValue = value(top)
					tx.Body.SetCurrentTreasuryValuePresence(true)
				}
				err := rule(tx, 0, ls, pp)
				if want != (err == nil) {
					t.Fatalf(
						"valid=%v top=%s subs=%v: reference accepts=%v, gOuroboros error=%v",
						valid, top, subs, want, err,
					)
				}
				cells++
			}
		}
	}
	t.Logf("treasury cells: %d", cells)
}

func TestSubTxModelRefScriptSize(t *testing.T) {
	// Each level reads its own reference input (model inputs 20, 21, 22)
	// holding a script of the cell's size, in units of 1,000 bytes against
	// a limit of 5 units.
	const unit = 1_000
	rule := modelPhasedRule(t, common.UtxoValidationRuleRefScriptSizePerTx)
	pp := modelDijkstraPParams()
	pp.MaxRefScriptSizePerTx = 5 * unit
	sizes := []int{0, 1, 2, 3}
	cells := 0
	for _, valid := range []bool{false, true} {
		for _, top := range sizes {
			for _, subs := range modelUpTo2(sizes) {
				utxos := []common.Utxo{
					modelUtxo(t, modelInput(20), modelInCoins, top*unit),
				}
				built := make([]DijkstraSubTransaction, 0, len(subs))
				total := top
				for k, s := range subs {
					utxos = append(utxos, modelUtxo(t, modelInput(21+k), modelInCoins, s*unit))
					sub := modelSubTx(t, modelLevel{ins: []int{0}}, k)
					sub.Body.TxReferenceInputs = cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{modelInput(21 + k)}, true,
					)
					built = append(built, sub)
					total += s
				}
				tx := modelBatch(t, valid, modelLevel{ins: []int{1}}, built)
				tx.Body.TxReferenceInputs = cbor.NewSetType(
					[]shelley.ShelleyTransactionInput{modelInput(20)}, true,
				)
				ls := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
				err := rule(tx, 0, ls, pp)
				want := total <= 5
				if want != (err == nil) {
					t.Fatalf(
						"valid=%v top=%d subs=%v: reference accepts=%v, gOuroboros error=%v",
						valid, top, subs, want, err,
					)
				}
				cells++
			}
		}
	}
	t.Logf("reference-script cells: %d", cells)
}

type modelWdrlBatch struct {
	valid, legacy bool
	wt, ws        int64 // -1: no withdrawal
	dd            uint64
}

func modelW(x int64) int64 {
	if x < 0 {
		return 0
	}
	return x
}

// modelSpecWdrl restates SubTx.tla's SpecWdrl.
func modelSpecWdrl(reg bool, bal int64, b modelWdrlBatch) bool {
	if !b.valid {
		return true
	}
	if (b.wt >= 0 || b.ws >= 0 || b.dd > 0) && !reg {
		return false
	}
	if !b.legacy {
		return modelW(b.wt)+modelW(b.ws) <= bal
	}
	if modelW(b.ws) > bal {
		return false
	}
	return b.wt < 0 || b.wt == bal-modelW(b.ws)+int64(b.dd)
}

func TestSubTxModelWithdrawals(t *testing.T) {
	rules := []common.UtxoValidationRuleFunc{
		modelPhasedRule(t, common.UtxoValidationRuleBatchWithdrawals),
		modelPhasedRule(t, common.UtxoValidationRuleAccountBalanceIntervals),
		modelPhasedRule(t, common.UtxoValidationRuleWithdrawals),
	}
	pp := modelDijkstraPParams()
	key := modelKeyCredential()
	account := modelRewardAddress(t, 0xe0, 0x4b)
	accountBytes, err := account.Bytes()
	require.NoError(t, err)
	v3 := common.PlutusV3Script{0x31}
	cells := 0
	for _, reg := range []bool{false, true} {
		for _, bal := range []int64{1, 2} {
			for _, valid := range []bool{false, true} {
				for _, legacy := range []bool{false, true} {
					for wt := int64(-1); wt <= 3; wt++ {
						for ws := int64(-1); ws <= 3; ws++ {
							for _, dd := range []uint64{0, 1} {
								b := modelWdrlBatch{valid: valid, legacy: legacy, wt: wt, ws: ws, dd: dd}
								topInput, topUtxo := modelPlace(0), modelUtxo(t, modelPlace(0), modelInCoins, 0)
								if legacy {
									topInput, topUtxo = dijkstraScriptLockedInput(t, v3, 7)
								}
								builder := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{
									topUtxo, modelUtxo(t, modelPlace(1), modelInCoins, 0),
								})
								if reg {
									builder = builder.WithRewardAccountCredentialBalance(
										key, uint64(bal)*modelWdrlUnits,
									)
								}
								ls := builder.Build()
								sub := modelSubTx(t, modelLevel{ins: []int{1}}, 0)
								if ws >= 0 {
									sub.Body.TxWithdrawals = map[*common.Address]uint64{
										account: uint64(ws) * modelWdrlUnits,
									}
								}
								if dd > 0 {
									sub.Body.TxDirectDeposits = DijkstraDirectDeposits{
										cbor.NewByteString(accountBytes): dd * modelWdrlUnits,
									}
								}
								tx := &DijkstraTransaction{
									Body: DijkstraTransactionBody{
										TxInputs: conway.NewConwayTransactionInputSet(
											[]shelley.ShelleyTransactionInput{topInput},
										),
										TxSubTransactions: cbor.NewSetType(
											[]DijkstraSubTransaction{sub}, true,
										),
									},
									TxIsValid: valid,
								}
								if wt >= 0 {
									tx.Body.TxWithdrawals = map[*common.Address]uint64{
										account: uint64(wt) * modelWdrlUnits,
									}
								}
								if legacy {
									tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
										[]common.PlutusV3Script{v3}, true,
									)
								}
								err := modelFirstError(tx, ls, pp, rules)
								want := modelSpecWdrl(reg, bal, b)
								if want != (err == nil) {
									t.Fatalf(
										"reg=%v bal=%d %+v: reference accepts=%v, gOuroboros error=%v",
										reg, bal, b, want, err,
									)
								}
								cells++
							}
						}
					}
				}
			}
		}
	}
	t.Logf("withdrawal cells: %d", cells)
}
