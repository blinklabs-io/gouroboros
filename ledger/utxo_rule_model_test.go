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

// A model-driven test of the UTXO rule from Shelley to Babbage: the
// validity interval, inputs, fee, value conservation with deposits, refunds,
// withdrawals and minting, the output minimum and maximum value size, the
// collateral rules with Babbage's collateral return and total collateral,
// and the IsValid declaration. The rules below restate the Utxo TLA+ model
// (specs/17-ledger-utxo in gouroboros-tla-models), which follows the
// reference rules in eras/<era>/impl/src/Cardano/Ledger/<Era>/Rules/Utxo.hs
// and Utxos.hs.
//
// For each era the test builds small UTxOs (two places, each empty or
// holding two or three ada-units, a token or not, under a key or a script)
// and five families of transactions around a base one, builds each as a
// real transaction of that era, runs the era's rule list, and checks that
// gOuroboros accepts exactly the transactions the model accepts.
//
// Rules outside the model (signatures, witnesses, metadata, script data
// hash, network) are run too, but none of their failures counts, not even
// one about an input missing from the UTxO: BadInputs, a restated rule,
// must reject that transaction first. Plutus scripts are not
// evaluated; the model's pass field stands for their result and is fed to
// common.ValidatePlutusScriptResult.

package ledger_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// umUnit is one model ada-unit in lovelace. It is far above the Alonzo and
// Babbage size-based output minimums, so in every era an output of one
// unit meets the minimum and an output of none does not, as in the model.
const (
	umUnit       = 1_000_000
	umCollPct    = 150
	umMaxColl    = 2
	umMaxValSize = 5000
	// umBigAssets is how many extra assets make an output's value larger
	// than both Mary's fixed 4000 bytes and umMaxValSize.
	umBigAssets = 200
)

type umEra int

const (
	umShelley umEra = iota
	umAllegra
	umMary
	umAlonzo
	umBabbage
)

func (e umEra) String() string {
	return [...]string{"Shelley", "Allegra", "Mary", "Alonzo", "Babbage"}[e]
}

func (e umEra) atLeast(o umEra) bool { return e >= o }

// umOut is an output: ada-units, a quantity of the one token, and whether
// a script locks it.
type umOut struct {
	coin   int
	tok    int
	script bool
}

// umState is a ledger state: places a and b (c is always empty), the slot,
// and the one stake credential's registration and reward balance.
type umState struct {
	utxo       [3]*umOut
	slot       uint64
	registered bool
	reward     int
}

const (
	umCertNone = iota
	umCertReg
	umCertDereg
)

type umRet struct{ coin, tok int }

// umTx is the model's transaction. ins, coll and refs are sets of places as
// bit masks (bit i is place i). delta and tdelta are how far the output is
// from balancing. lo and hi are the validity bounds, 0 for absent (Shelley:
// hi is the TTL). tcoll is the total collateral, -1 for absent.
type umTx struct {
	ins, coll, refs    uint8
	delta, tdelta      int
	fee, mint          int
	cert               int
	wdrl, big          bool
	lo, hi             uint64
	rdmrs, valid, pass bool
	ret                *umRet
	tcoll              int
}

func (t umTx) String() string {
	ret := "none"
	if t.ret != nil {
		ret = fmt.Sprintf("%d/%d", t.ret.coin, t.ret.tok)
	}
	return fmt.Sprintf(
		"ins=%03b coll=%03b refs=%03b delta=%d tdelta=%d fee=%d mint=%d cert=%d wdrl=%v big=%v lo=%d hi=%d rdmrs=%v valid=%v pass=%v ret=%s tcoll=%d",
		t.ins, t.coll, t.refs, t.delta, t.tdelta, t.fee, t.mint, t.cert,
		t.wdrl, t.big, t.lo, t.hi, t.rdmrs, t.valid, t.pass, ret, t.tcoll,
	)
}

// Sums over the places in a mask that hold an output.
func (s umState) coinOf(mask uint8) int {
	n := 0
	for i, o := range s.utxo {
		if mask&(1<<i) != 0 && o != nil {
			n += o.coin
		}
	}
	return n
}

func (s umState) tokOf(mask uint8) int {
	n := 0
	for i, o := range s.utxo {
		if mask&(1<<i) != 0 && o != nil {
			n += o.tok
		}
	}
	return n
}

func (s umState) live() uint8 {
	var m uint8
	for i, o := range s.utxo {
		if o != nil {
			m |= 1 << i
		}
	}
	return m
}

func (s umState) refund(t umTx) int {
	if t.cert == umCertDereg && s.registered {
		return 1
	}
	return 0
}

func umDeposit(t umTx) int {
	if t.cert == umCertReg {
		return 1
	}
	return 0
}

func (s umState) wdrl(t umTx) int {
	if t.wdrl {
		return s.reward
	}
	return 0
}

func (s umState) consumed(t umTx) int {
	return s.coinOf(t.ins) + s.wdrl(t) + s.refund(t)
}

func (s umState) outCoin(t umTx) int {
	return s.consumed(t) - t.fee - umDeposit(t) + t.delta
}

func (s umState) outTok(t umTx) int {
	return s.tokOf(t.ins) + t.mint + t.tdelta
}

func (s umState) collBal(t umTx) int {
	b := s.coinOf(t.coll)
	if t.ret != nil {
		b -= t.ret.coin
	}
	return b
}

func (t umTx) retTok() int {
	if t.ret == nil {
		return 0
	}
	return t.ret.tok
}

func (t umTx) scriptsPass() bool { return !t.rdmrs || t.pass }

// umSpecAccepts restates the model's SpecAccepts.
func umSpecAccepts(era umEra, s umState, t umTx) bool {
	// Shelley: ttl >= slot (inclusive). From Allegra: lo <= slot < hi.
	if era == umShelley {
		if t.hi < s.slot {
			return false
		}
	} else {
		if t.lo != 0 && s.slot < t.lo {
			return false
		}
		if t.hi != 0 && s.slot >= t.hi {
			return false
		}
	}
	if t.ins == 0 || t.fee < 1 {
		return false
	}
	// BadInputs: spent inputs; Alonzo adds collateral; Babbage adds
	// reference inputs.
	read := t.ins
	if era.atLeast(umAlonzo) {
		read |= t.coll
	}
	if era.atLeast(umBabbage) {
		read |= t.refs
	}
	if read&^s.live() != 0 {
		return false
	}
	if s.consumed(t) != s.outCoin(t)+t.fee+umDeposit(t) {
		return false
	}
	if era.atLeast(umMary) && s.tokOf(t.ins)+t.mint != s.outTok(t) {
		return false
	}
	// Output minimum: Mary's pointwise comparison exempts an output
	// holding a token.
	switch era {
	case umMary:
		if s.outCoin(t) < 1 && s.outTok(t) == 0 {
			return false
		}
	default:
		if s.outCoin(t) < 1 {
			return false
		}
	}
	if era == umBabbage && t.ret != nil && t.ret.coin < 1 {
		return false
	}
	if era.atLeast(umMary) && t.big {
		return false
	}
	// Collateral, only with redeemers.
	if era.atLeast(umAlonzo) && t.rdmrs {
		for i, o := range s.utxo {
			if t.coll&(1<<i) != 0 && o != nil && o.script {
				return false
			}
		}
		if s.collBal(t)*100 < t.fee*umCollPct {
			return false
		}
		if era == umAlonzo {
			if s.tokOf(t.coll) != 0 {
				return false
			}
		} else if !(s.tokOf(t.coll) == 0 && t.retTok() == 0) &&
			s.tokOf(t.coll)-t.retTok() != 0 {
			return false
		}
		if era == umBabbage && t.tcoll != -1 && t.tcoll != s.collBal(t) {
			return false
		}
		if t.coll&s.live() == 0 {
			return false
		}
	}
	if era.atLeast(umAlonzo) {
		n := 0
		for i := range 3 {
			if t.coll&(1<<i) != 0 {
				n++
			}
		}
		if n > umMaxColl {
			return false
		}
		if t.valid != t.scriptsPass() {
			return false
		}
	}
	// Certificates and withdrawals: only a phase-2-valid transaction runs
	// DELEGS.
	if t.valid {
		if t.wdrl && !s.registered {
			return false
		}
		if t.cert == umCertReg && s.registered {
			return false
		}
		if t.cert == umCertDereg && (!s.registered || s.reward-s.wdrl(t) != 0) {
			return false
		}
	}
	return true
}

// umExpressible: the output holds no negative amount.
func umExpressible(s umState, t umTx) bool {
	return s.outCoin(t) >= 0 && s.outTok(t) >= 0
}

// Families of transactions, as in the model.

func umBase(era umEra) umTx {
	t := umTx{ins: 1, fee: 1, valid: true, pass: true, tcoll: -1}
	if era == umShelley {
		t.hi = 3
	}
	return t
}

func umBools(on bool) []bool {
	if on {
		return []bool{false, true}
	}
	return []bool{false}
}

func umRange(lo, hi int, on bool) []int {
	if !on {
		return []int{0}
	}
	var r []int
	for i := lo; i <= hi; i++ {
		r = append(r, i)
	}
	return r
}

func umMasks(on bool) []uint8 {
	if !on {
		return []uint8{0}
	}
	return []uint8{0, 1, 2, 3, 4, 5, 6, 7}
}

func umFamInputs(era umEra) []umTx {
	var txs []umTx
	for i := range uint8(8) {
		for _, c := range umMasks(era.atLeast(umAlonzo)) {
			for _, r := range umMasks(era.atLeast(umBabbage)) {
				for _, d := range umBools(era.atLeast(umAlonzo)) {
					t := umBase(era)
					t.ins, t.coll, t.refs, t.rdmrs = i, c, r, d
					txs = append(txs, t)
				}
			}
		}
	}
	return txs
}

func umFamBalance(era umEra) []umTx {
	var txs []umTx
	for _, d := range umRange(-1, 1, true) {
		for _, td := range umRange(-1, 1, era.atLeast(umMary)) {
			for f := 0; f <= 2; f++ {
				for _, m := range umRange(-1, 1, era.atLeast(umMary)) {
					for c := umCertNone; c <= umCertDereg; c++ {
						for _, w := range umBools(true) {
							t := umBase(era)
							t.delta, t.tdelta, t.fee, t.mint, t.cert, t.wdrl = d, td, f, m, c, w
							txs = append(txs, t)
						}
					}
				}
			}
		}
	}
	return txs
}

func umFamOutputs(era umEra) []umTx {
	var txs []umTx
	for _, i := range []uint8{1, 3} {
		for f := 1; f <= 3; f++ {
			for _, m := range umRange(-1, 1, era.atLeast(umMary)) {
				for _, b := range umBools(era.atLeast(umMary)) {
					t := umBase(era)
					t.ins, t.fee, t.mint, t.big = i, f, m, b
					txs = append(txs, t)
				}
			}
		}
	}
	return txs
}

func umFamInterval(era umEra) []umTx {
	var txs []umTx
	los, his := []uint64{0, 1, 2}, []uint64{0, 1, 2, 3}
	if era == umShelley {
		los, his = []uint64{0}, []uint64{1, 2, 3}
	}
	for _, l := range los {
		for _, h := range his {
			t := umBase(era)
			t.lo, t.hi = l, h
			txs = append(txs, t)
		}
	}
	return txs
}

func umFamPhase2(era umEra) []umTx {
	rets := []*umRet{nil}
	tcolls := []int{-1}
	if era == umBabbage {
		for c := 0; c <= 2; c++ {
			for tk := 0; tk <= 1; tk++ {
				rets = append(rets, &umRet{coin: c, tok: tk})
			}
		}
		tcolls = []int{-1, 1, 2, 3}
	}
	var txs []umTx
	for _, d := range umBools(era.atLeast(umAlonzo)) {
		for _, v := range umBools(true) {
			if !v && !era.atLeast(umAlonzo) {
				continue
			}
			for _, p := range umBools(era.atLeast(umAlonzo)) {
				for _, c := range umMasks(era.atLeast(umAlonzo)) {
					for _, r := range rets {
						for _, tc := range tcolls {
							for f := 1; f <= 2; f++ {
								t := umBase(era)
								t.rdmrs, t.valid, t.pass = d, v, p || !era.atLeast(umAlonzo)
								t.coll, t.ret, t.tcoll, t.fee = c, r, tc, f
								txs = append(txs, t)
							}
						}
					}
				}
			}
		}
	}
	return txs
}

// umOuts lists what a place can hold in an era: nothing, or 2 or 3 units,
// a token (from Mary), under a key or a script.
func umOuts(era umEra) []*umOut {
	outs := []*umOut{nil}
	for _, c := range []int{2, 3} {
		for _, tk := range umRange(0, 1, era.atLeast(umMary)) {
			for _, sc := range []bool{false, true} {
				outs = append(outs, &umOut{coin: c, tok: tk, script: sc})
			}
		}
	}
	return outs
}

var umEnvs = []struct {
	slot       uint64
	registered bool
	reward     int
}{{0, false, 0}, {1, true, 0}, {2, true, 1}}

// Building the transactions.

var (
	umTokenPolicy = common.Blake2b224Hash([]byte("utxo-model-token"))
	umBigPolicy   = common.Blake2b224Hash([]byte("utxo-model-big"))
	umStakeHash   = common.Blake2b224Hash([]byte("utxo-model-stake"))
	// umPayVkey is the payment key every key-locked output is locked by.
	// Each transaction carries a vkey witness for it (with no valid
	// signature: signatures are outside the model), so the witness rules
	// look past a key-locked collateral input to the next one.
	umPayVkey   = make([]byte, 32)
	umPayKey    = common.Blake2b224Hash(umPayVkey)
	umPayScript = common.Blake2b224Hash([]byte("utxo-model-payment-script"))
	umTxId      = "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
)

func umCredential() common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.CredentialHash(umStakeHash),
	}
}

func umAddress(t *testing.T, script bool) common.Address {
	addrType, payment := uint8(common.AddressTypeKeyNone), umPayKey
	if script {
		addrType, payment = common.AddressTypeScriptNone, umPayScript
	}
	addr, err := common.NewAddressFromParts(
		addrType, common.AddressNetworkTestnet, payment[:], nil,
	)
	require.NoError(t, err)
	return addr
}

func umRewardAddress(t *testing.T) *common.Address {
	addr, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey, common.AddressNetworkTestnet, nil, umStakeHash[:],
	)
	require.NoError(t, err)
	return &addr
}

func umInput(i int) shelley.ShelleyTransactionInput {
	return shelley.MustNewShelleyTransactionInput(umTxId, i)
}

func umInputs(mask uint8) []shelley.ShelleyTransactionInput {
	var ins []shelley.ShelleyTransactionInput
	for i := range 3 {
		if mask&(1<<i) != 0 {
			ins = append(ins, umInput(i))
		}
	}
	return ins
}

// umAssets is tok of the model token, plus the big assets when big.
func umAssets(tok int, isBig bool) *common.MultiAsset[common.MultiAssetTypeOutput] {
	if tok == 0 && !isBig {
		return nil
	}
	data := map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{}
	if tok > 0 {
		data[umTokenPolicy] = map[cbor.ByteString]common.MultiAssetTypeOutput{
			cbor.NewByteString([]byte("t")): big.NewInt(int64(tok)),
		}
	}
	if isBig {
		data[umBigPolicy] = umBigNames(func() common.MultiAssetTypeOutput { return big.NewInt(1) })
	}
	ma := common.NewMultiAsset[common.MultiAssetTypeOutput](data)
	return &ma
}

func umBigNames[T any](one func() T) map[cbor.ByteString]T {
	names := map[cbor.ByteString]T{}
	for i := range umBigAssets {
		names[cbor.NewByteString(fmt.Appendf(nil, "big-asset-%022d", i))] = one()
	}
	return names
}

func umMint(mint int, isBig bool) *common.MultiAsset[common.MultiAssetTypeMint] {
	if mint == 0 && !isBig {
		return nil
	}
	data := map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{}
	if mint != 0 {
		data[umTokenPolicy] = map[cbor.ByteString]common.MultiAssetTypeMint{
			cbor.NewByteString([]byte("t")): big.NewInt(int64(mint)),
		}
	}
	if isBig {
		data[umBigPolicy] = umBigNames(func() common.MultiAssetTypeMint { return big.NewInt(1) })
	}
	ma := common.NewMultiAsset[common.MultiAssetTypeMint](data)
	return &ma
}

func umLovelace(units int) uint64 { return uint64(units) * umUnit } // #nosec G115 -- units >= 0

// umEraOutput builds an output of the era's own type.
func umEraOutput(
	t *testing.T, era umEra, coin, tok int, script, big bool,
) common.TransactionOutput {
	addr := umAddress(t, script)
	value := mary.MaryTransactionOutputValue{
		Amount: umLovelace(coin),
		Assets: umAssets(tok, big),
	}
	switch era {
	case umShelley, umAllegra:
		return &shelley.ShelleyTransactionOutput{OutputAddress: addr, OutputAmount: umLovelace(coin)}
	case umMary:
		return &mary.MaryTransactionOutput{OutputAddress: addr, OutputAmount: value}
	case umAlonzo:
		return &alonzo.AlonzoTransactionOutput{OutputAddress: addr, OutputAmount: value}
	default:
		return &babbage.BabbageTransactionOutput{OutputAddress: addr, OutputAmount: value}
	}
}

func umLedgerState(t *testing.T, era umEra, s umState) common.LedgerState {
	var utxos []common.Utxo
	for i, o := range s.utxo {
		if o == nil {
			continue
		}
		utxos = append(utxos, common.Utxo{
			Id:     umInput(i),
			Output: umEraOutput(t, era, o.coin, o.tok, o.script, false),
		})
	}
	b := mockledger.NewLedgerStateBuilder().WithUtxos(utxos)
	if s.registered {
		b = b.WithRewardAccountCredentialBalance(umCredential(), umLovelace(s.reward))
	}
	return b.Build()
}

func umPparams(era umEra) common.ProtocolParameters {
	switch era {
	case umShelley, umAllegra:
		return &shelley.ShelleyProtocolParameters{
			MinFeeB: umUnit, MaxTxSize: 1 << 20, KeyDeposit: umUnit,
			MinUtxoValue: umUnit, ProtocolMajor: uint(2 + era),
		}
	case umMary:
		return &mary.MaryProtocolParameters{
			MinFeeB: umUnit, MaxTxSize: 1 << 20, KeyDeposit: umUnit,
			MinUtxoValue: umUnit, ProtocolMajor: 4,
		}
	case umAlonzo:
		return &alonzo.AlonzoProtocolParameters{
			MinFeeB: umUnit, MaxTxSize: 1 << 20, KeyDeposit: umUnit,
			AdaPerUtxoByte: 1, MaxValueSize: umMaxValSize,
			CollateralPercentage: umCollPct, MaxCollateralInputs: umMaxColl,
			MaxTxExUnits:   common.ExUnits{Memory: 1 << 40, Steps: 1 << 40},
			ExecutionCosts: umFreeExecution(),
			ProtocolMajor:  6,
		}
	default:
		return &babbage.BabbageProtocolParameters{
			MinFeeB: umUnit, MaxTxSize: 1 << 20, KeyDeposit: umUnit,
			AdaPerUtxoByte: 1, MaxValueSize: umMaxValSize,
			CollateralPercentage: umCollPct, MaxCollateralInputs: umMaxColl,
			MaxTxExUnits:   common.ExUnits{Memory: 1 << 40, Steps: 1 << 40},
			ExecutionCosts: umFreeExecution(),
			ProtocolMajor:  7,
		}
	}
}

// umFreeExecution prices execution at zero, so the minimum fee is the
// model's constant whether or not the transaction has redeemers.
func umFreeExecution() common.ExUnitPrice {
	return common.ExUnitPrice{
		MemPrice:  &cbor.Rat{Rat: big.NewRat(0, 1)},
		StepPrice: &cbor.Rat{Rat: big.NewRat(0, 1)},
	}
}

func umCerts(cert int) []common.CertificateWrapper {
	var c common.Certificate
	switch cert {
	case umCertReg:
		c = &common.StakeRegistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeRegistration),
			StakeCredential: umCredential(),
		}
	case umCertDereg:
		c = &common.StakeDeregistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeDeregistration),
			StakeCredential: umCredential(),
		}
	default:
		return nil
	}
	return []common.CertificateWrapper{{Type: c.Type(), Certificate: c}}
}

func umWitnesses() []common.VkeyWitness {
	return []common.VkeyWitness{{Vkey: umPayVkey, Signature: make([]byte, 64)}}
}

func umTransaction(t *testing.T, era umEra, s umState, m umTx) common.Transaction {
	ins := shelley.NewShelleyTransactionInputSet(umInputs(m.ins))
	fee := umLovelace(m.fee)
	certs := umCerts(m.cert)
	var wdrls map[*common.Address]uint64
	if m.wdrl {
		wdrls = map[*common.Address]uint64{umRewardAddress(t): umLovelace(s.reward)}
	}
	out := umEraOutput(t, era, s.outCoin(m), s.outTok(m), false, m.big)
	var redeemers alonzo.AlonzoRedeemers
	if m.rdmrs {
		redeemers = alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{}}}
	}
	switch era {
	case umShelley:
		return &shelley.ShelleyTransaction{Body: shelley.ShelleyTransactionBody{
			TxInputs: ins, TxOutputs: []shelley.ShelleyTransactionOutput{*out.(*shelley.ShelleyTransactionOutput)},
			TxFee: fee, Ttl: m.hi, TxCertificates: certs, TxWithdrawals: wdrls,
		}}
	case umAllegra:
		return &allegra.AllegraTransaction{Body: allegra.AllegraTransactionBody{
			TxInputs: ins, TxOutputs: []shelley.ShelleyTransactionOutput{*out.(*shelley.ShelleyTransactionOutput)},
			TxFee: fee, Ttl: m.hi, TxValidityIntervalStart: m.lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
		}}
	case umMary:
		body := mary.MaryTransactionBody{
			TxInputs: ins, TxOutputs: []mary.MaryTransactionOutput{*out.(*mary.MaryTransactionOutput)},
			TxFee: fee, Ttl: m.hi, TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: umMint(m.mint, m.big),
		}
		if m.lo != 0 {
			lo := m.lo
			body.TxValidityIntervalStart = &lo
		}
		return &mary.MaryTransaction{Body: body}
	case umAlonzo:
		return &alonzo.AlonzoTransaction{
			Body: alonzo.AlonzoTransactionBody{
				TxInputs: ins, TxOutputs: []alonzo.AlonzoTransactionOutput{*out.(*alonzo.AlonzoTransactionOutput)},
				TxFee: fee, Ttl: m.hi, TxValidityIntervalStart: m.lo,
				TxCertificates: certs, TxWithdrawals: wdrls,
				TxMint:       umMint(m.mint, m.big),
				TxCollateral: cbor.NewSetType(umInputs(m.coll), false),
			},
			WitnessSet: alonzo.AlonzoTransactionWitnessSet{
				VkeyWitnesses: umWitnesses(), WsRedeemers: redeemers,
			},
			TxIsValid: m.valid,
		}
	default:
		body := babbage.BabbageTransactionBody{
			TxInputs: ins, TxOutputs: []babbage.BabbageTransactionOutput{*out.(*babbage.BabbageTransactionOutput)},
			TxFee: fee, Ttl: m.hi, TxValidityIntervalStart: m.lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint:            umMint(m.mint, m.big),
			TxCollateral:      cbor.NewSetType(umInputs(m.coll), false),
			TxReferenceInputs: cbor.NewSetType(umInputs(m.refs), false),
		}
		if m.ret != nil {
			ret := umEraOutput(t, era, m.ret.coin, m.ret.tok, false, false)
			body.TxCollateralReturn = ret.(*babbage.BabbageTransactionOutput)
		}
		if m.tcoll >= 0 {
			body.TxTotalCollateral = umLovelace(m.tcoll)
		}
		return &babbage.BabbageTransaction{
			Body: body,
			WitnessSet: babbage.BabbageTransactionWitnessSet{
				VkeyWitnesses: umWitnesses(), WsRedeemers: redeemers,
			},
			TxIsValid: m.valid,
		}
	}
}

func umRules(era umEra) []common.UtxoValidationRuleDescriptor {
	switch era {
	case umShelley:
		return shelley.UtxoValidationRuleDescriptors()
	case umAllegra:
		return allegra.UtxoValidationRuleDescriptors()
	case umMary:
		return mary.UtxoValidationRuleDescriptors()
	case umAlonzo:
		return alonzo.UtxoValidationRuleDescriptors()
	default:
		return babbage.UtxoValidationRuleDescriptors()
	}
}

// umModelled are the rules the model restates; any failure of one is a
// rejection.
var umModelled = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleTimeToLive:               true,
	common.UtxoValidationRuleOutsideValidityInterval:  true,
	common.UtxoValidationRuleInputSetEmpty:            true,
	common.UtxoValidationRuleFeeTooSmall:              true,
	common.UtxoValidationRuleBadInputs:                true,
	common.UtxoValidationRuleValueNotConserved:        true,
	common.UtxoValidationRuleOutputTooSmall:           true,
	common.UtxoValidationRuleOutputTooBig:             true,
	common.UtxoValidationRuleDelegation:               true,
	common.UtxoValidationRuleWithdrawals:              true,
	common.UtxoValidationRuleInsufficientCollateral:   true,
	common.UtxoValidationRuleCollateralContainsNonAda: true,
	common.UtxoValidationRuleCollateralEqBalance:      true,
	common.UtxoValidationRuleNoCollateralInputs:       true,
	common.UtxoValidationRuleCollateralKeyLocked:      true,
	common.UtxoValidationRuleTooManyCollateralInputs:  true,
	common.UtxoValidationRuleIsValidFlag:              true,
	common.UtxoValidationRuleDisjointRefInputs:        true,
}

// umGoVerdict runs the era's rules on the transaction and returns whether
// gOuroboros accepts it, and the first rejecting rule.
func umGoVerdict(t *testing.T, era umEra, s umState, m umTx) (bool, string) {
	tx := umTransaction(t, era, s, m)
	ls := umLedgerState(t, era, s)
	pp := umPparams(era)
	for _, rule := range umRules(era) {
		if rule.Id == common.UtxoValidationRulePlutusScripts {
			var evalErr error
			if !m.scriptsPass() {
				evalErr = common.PlutusScriptFailedError{}
			}
			if err := common.ValidatePlutusScriptResult(tx, evalErr); err != nil {
				return false, fmt.Sprintf("%s: %v", rule.Id, err)
			}
			continue
		}
		err := rule.Validator(tx, s.slot, ls, pp)
		if err == nil {
			continue
		}
		if umModelled[rule.Id] {
			return false, fmt.Sprintf("%s: %v", rule.Id, err)
		}
	}
	return true, ""
}

// umCheck compares the model and gOuroboros on every expressible
// transaction of txs in state s, and returns how many cells it checked.
func umCheck(t *testing.T, era umEra, s umState, txs []umTx) int {
	t.Helper()
	n := 0
	for _, m := range txs {
		if !umExpressible(s, m) {
			continue
		}
		n++
		want := umSpecAccepts(era, s, m)
		got, why := umGoVerdict(t, era, s, m)
		if got != want {
			t.Fatalf(
				"%s state %+v slot=%d reg=%v reward=%d tx %v: model accepts=%v, gOuroboros accepts=%v (%s)",
				era, umStateString(s), s.slot, s.registered, s.reward, m, want, got, why,
			)
		}
	}
	return n
}

func umOutString(o *umOut) string {
	if o == nil {
		return "-"
	}
	lock := "key"
	if o.script {
		lock = "script"
	}
	return fmt.Sprintf("%d/%d/%s", o.coin, o.tok, lock)
}

func umStateString(s umState) string {
	return fmt.Sprintf(
		"a:%s b:%s", umOutString(s.utxo[0]), umOutString(s.utxo[1]),
	)
}

// TestUtxoRuleModel walks the model's transaction families in its small
// ledger states, era by era. Families that read the UTxO beyond place a
// (inputs, collateral and phase 2, outputs) run in every UTxO; the balance
// family runs in every content of place a and every slot and credential
// state; the interval family in every slot.
func TestUtxoRuleModel(t *testing.T) {
	for _, era := range []umEra{umShelley, umAllegra, umMary, umAlonzo, umBabbage} {
		t.Run(era.String(), func(t *testing.T) {
			t.Parallel()
			outs := umOuts(era)
			// One subtest per content of place a, run in parallel.
			for _, a := range outs {
				name := "a:" + umOutString(a)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					cells := 0
					for _, b := range outs {
						s := umState{utxo: [3]*umOut{a, b, nil}}
						cells += umCheck(t, era, s, umFamInputs(era))
						cells += umCheck(t, era, s, umFamPhase2(era))
						cells += umCheck(t, era, s, umFamOutputs(era))
					}
					for _, env := range umEnvs {
						s := umState{
							utxo: [3]*umOut{a, nil, nil}, slot: env.slot,
							registered: env.registered, reward: env.reward,
						}
						cells += umCheck(t, era, s, umFamBalance(era))
						cells += umCheck(t, era, s, umFamInterval(era))
					}
					t.Logf("%s %s: %d cells agree", era, name, cells)
				})
			}
		})
	}
}

// TestUtxoRuleModelApply checks the model's ApplyValid and ApplyInvalid
// against what a transaction reports it consumes and produces, which
// BlockLedgerState.ApplyTransaction folds into the block's UTxO view. A
// phase-2-valid transaction spends its inputs (never its reference inputs)
// and creates its outputs; an invalid one spends only its collateral and
// creates only the Babbage collateral return, at the index after the last
// output.
func TestUtxoRuleModelApply(t *testing.T) {
	s := umState{utxo: [3]*umOut{{coin: 3}, {coin: 2}, nil}}
	for _, era := range []umEra{umAlonzo, umBabbage} {
		for _, valid := range []bool{true, false} {
			for _, withRet := range umBools(era == umBabbage) {
				m := umBase(era)
				m.ins, m.coll, m.rdmrs, m.valid, m.pass = 1, 2, true, valid, valid
				if era == umBabbage {
					m.refs = 2
				}
				if withRet {
					m.ret = &umRet{coin: 1}
				}
				name := fmt.Sprintf("%s valid=%v return=%v", era, valid, withRet)
				t.Run(name, func(t *testing.T) {
					tx := umTransaction(t, era, s, m)
					spent := map[uint32]bool{}
					for _, in := range tx.Consumed() {
						spent[in.Index()] = true
					}
					created := map[uint32]bool{}
					for _, u := range tx.Produced() {
						require.Equal(t, tx.Hash(), u.Id.Id())
						created[u.Id.Index()] = true
					}
					if valid {
						require.Equal(t, map[uint32]bool{0: true}, spent, "inputs only")
						require.Equal(t, map[uint32]bool{0: true}, created, "the one output")
						return
					}
					require.Equal(t, map[uint32]bool{1: true}, spent, "collateral only")
					if withRet {
						require.Equal(t, map[uint32]bool{1: true}, created, "the return, after the output")
					} else {
						require.Empty(t, created)
					}
				})
			}
		}
	}
}
