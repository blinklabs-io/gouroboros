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

// A table test of the UTXOW witness rules from Shelley to Dijkstra:
// the key witnesses a transaction needs and whether they are supplied,
// native multisig and timelock scripts, needed versus supplied scripts and
// redeemers, required and supplemental datums, and the script integrity
// hash's language views. The rules below restate
// eras/<era>/impl/src/Cardano/Ledger/<Era>/Rules/Utxow.hs and the
// witsVKeyNeeded functions of cardano-ledger 1210d37f.
//
// Each family builds real transactions of each era, runs only the era's
// witness rules (selected by rule id from the era's own rule list, in its
// order), and checks that gOuroboros accepts exactly the transactions the
// restated reference rules accept. Signatures are real: every supplied vkey and bootstrap
// witness signs the transaction body hash, except the one a cell marks as
// bad. Plutus scripts are never run.

package ledger_test

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

type wmEra int

const (
	wmShelley wmEra = iota
	wmAllegra
	wmMary
	wmAlonzo
	wmBabbage
	wmConway
	wmDijkstra
)

var wmEras = []wmEra{
	wmShelley, wmAllegra, wmMary, wmAlonzo, wmBabbage, wmConway, wmDijkstra,
}

func (e wmEra) String() string {
	return [...]string{
		"Shelley", "Allegra", "Mary", "Alonzo", "Babbage", "Conway",
		"Dijkstra",
	}[e]
}

func (e wmEra) atLeast(o wmEra) bool { return e >= o }

// The test's three keys. v1 and v2 are vkeys: their hash is supplied by a
// vkey witness. b is a bootstrap key: its hash is the address root a
// bootstrap witness supplies.
const (
	wmV1 = iota
	wmV2
	wmB
	wmNumKeys
)

var wmKeyNames = [...]string{"v1", "v2", "b"}

type wmKey struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	hash common.Blake2b224
}

var (
	wmKeyset    [wmNumKeys]wmKey
	wmChainCode = make([]byte, 32)
	wmBootAttrs = []byte{0xa0}
	wmTxId      = "8f1e3a6a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccd"
	wmGenesis   = common.Blake2b224Hash([]byte("witness-model-genesis"))
	wmPool      = common.Blake2b224Hash([]byte("witness-model-pool"))
	wmScriptKey = common.Blake2b224Hash([]byte("witness-model-unrun-script"))
)

func init() {
	for i := range wmKeyset {
		seed := make([]byte, ed25519.SeedSize)
		seed[0] = byte(i + 1)
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		hash := common.Blake2b224Hash(pub)
		if i == wmB {
			root, err := common.BootstrapWitness{
				PublicKey:  pub,
				ChainCode:  wmChainCode,
				Attributes: wmBootAttrs,
			}.KeyHash()
			if err != nil {
				panic(err)
			}
			hash = root
		}
		wmKeyset[i] = wmKey{priv: priv, pub: pub, hash: hash}
	}
}

func wmHash(k int) common.Blake2b224 { return wmKeyset[k].hash }

func wmMaskString(mask uint8) string {
	s := "{"
	for k := range wmNumKeys {
		if mask&(1<<k) != 0 {
			if len(s) > 1 {
				s += ","
			}
			s += wmKeyNames[k]
		}
	}
	return s + "}"
}

// A transaction, era-neutral. Inputs are places: each names its output.
type wmOut struct {
	addr      common.Address
	datumHash *common.Blake2b256
	inline    *common.Datum
	scriptRef common.Script
}

type wmRedeemer struct {
	tag   common.RedeemerTag
	index uint32
}

type wmTx struct {
	ins, refs, coll []wmOut
	outs            []wmOut
	wdrls           []common.Credential
	certs           []common.Certificate
	signers         []common.Blake2b224
	voters          []common.Blake2b224
	scriptVoters    []common.Blake2b224
	update          bool
	mint            []common.Blake2b224
	lo              *uint64
	hi              *uint64
	wits            uint8 // keys whose witness is supplied
	badSig          bool  // add a v2 vkey witness with a wrong signature
	natives         []common.NativeScript
	plutus          []common.Script
	datums          []common.Datum
	redeemers       []wmRedeemer
	sdh             *common.Blake2b256
	valid           bool
}

func wmInput(i int) shelley.ShelleyTransactionInput {
	return shelley.MustNewShelleyTransactionInput(wmTxId, i)
}

// Places 0.. are spent inputs, 10.. reference inputs, 20.. collateral.
func wmInputs(base, n int) []shelley.ShelleyTransactionInput {
	ret := make([]shelley.ShelleyTransactionInput, 0, n)
	for i := range n {
		ret = append(ret, wmInput(base+i))
	}
	return ret
}

func wmKeyAddr(t *testing.T, h common.Blake2b224) common.Address {
	addr, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone, common.AddressNetworkTestnet, h[:], nil,
	)
	require.NoError(t, err)
	return addr
}

func wmByronAddr(t *testing.T, root common.Blake2b224) common.Address {
	addr, err := common.NewByronAddressFromParts(
		0, root[:], common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	return addr
}

func wmScriptAddr(t *testing.T, h common.ScriptHash) common.Address {
	addr, err := common.NewAddressFromParts(
		common.AddressTypeScriptNone, common.AddressNetworkTestnet, h[:], nil,
	)
	require.NoError(t, err)
	return addr
}

// wmLockedBy is a key-locked output: a Byron address for the bootstrap key,
// an enterprise address otherwise.
func wmLockedBy(t *testing.T, k int) wmOut {
	if k == wmB {
		return wmOut{addr: wmByronAddr(t, wmHash(k))}
	}
	return wmOut{addr: wmKeyAddr(t, wmHash(k))}
}

func wmRewardAddr(t *testing.T, c common.Credential) *common.Address {
	addrType := uint8(common.AddressTypeNoneKey)
	if c.CredType == common.CredentialTypeScriptHash {
		addrType = common.AddressTypeNoneScript
	}
	addr, err := common.NewAddressFromParts(
		addrType, common.AddressNetworkTestnet, nil, c.Credential[:],
	)
	require.NoError(t, err)
	return &addr
}

func wmKeyCred(h common.Blake2b224) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.CredentialHash(h),
	}
}

func wmScriptCred(h common.ScriptHash) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: common.CredentialHash(h),
	}
}

// wmBabbageOut is the ledger-state form of every output: the Babbage output
// carries a datum hash, an inline datum and a reference script, and the
// rules read outputs only through the common interface.
func wmBabbageOut(t *testing.T, o wmOut) *babbage.BabbageTransactionOutput {
	m := map[int]any{0: o.addr, 1: uint64(2_000_000)}
	switch {
	case o.datumHash != nil:
		m[2] = []any{0, o.datumHash[:]}
	case o.inline != nil:
		m[2] = []any{1, cbor.Tag{Number: 24, Content: o.inline.Cbor()}}
	}
	if o.scriptRef != nil {
		raw, err := cbor.Encode(
			[]any{wmScriptRefType(o.scriptRef), o.scriptRef},
		)
		require.NoError(t, err)
		m[3] = cbor.Tag{Number: 24, Content: raw}
	}
	raw, err := cbor.Encode(m)
	require.NoError(t, err)
	var out babbage.BabbageTransactionOutput
	_, err = cbor.Decode(raw, &out)
	require.NoError(t, err)
	return &out
}

func wmScriptRefType(s common.Script) uint {
	switch s.(type) {
	case common.NativeScript:
		return common.ScriptRefTypeNativeScript
	case common.PlutusV1Script:
		return common.ScriptRefTypePlutusV1
	case common.PlutusV2Script:
		return common.ScriptRefTypePlutusV2
	default:
		return common.ScriptRefTypePlutusV3
	}
}

func wmDatum(t *testing.T, n int64) common.Datum {
	raw, err := cbor.Encode(n)
	require.NoError(t, err)
	var d common.Datum
	_, err = cbor.Decode(raw, &d)
	require.NoError(t, err)
	return d
}

func wmDatumHash(t *testing.T, n int64) *common.Blake2b256 {
	d := wmDatum(t, n)
	h := common.Blake2b256(d.Hash())
	return &h
}

func wmLedgerState(
	t *testing.T,
	m wmTx,
	ls *mockledger.LedgerStateBuilder,
) *mockledger.LedgerStateBuilder {
	var utxos []common.Utxo
	add := func(base int, outs []wmOut) {
		for i, o := range outs {
			utxos = append(utxos, common.Utxo{
				Id: wmInput(base + i), Output: wmBabbageOut(t, o),
			})
		}
	}
	add(0, m.ins)
	add(10, m.refs)
	add(20, m.coll)
	if ls == nil {
		ls = mockledger.NewLedgerStateBuilder()
	}
	return ls.WithUtxos(utxos)
}

func wmCertWrappers(certs []common.Certificate) []common.CertificateWrapper {
	ret := make([]common.CertificateWrapper, 0, len(certs))
	for _, c := range certs {
		ret = append(
			ret,
			common.CertificateWrapper{Type: c.Type(), Certificate: c},
		)
	}
	return ret
}

func wmWithdrawals(
	t *testing.T,
	creds []common.Credential,
) map[*common.Address]uint64 {
	if len(creds) == 0 {
		return nil
	}
	ret := map[*common.Address]uint64{}
	for _, c := range creds {
		ret[wmRewardAddr(t, c)] = 0
	}
	return ret
}

func wmMint(
	policies []common.Blake2b224,
) *common.MultiAsset[common.MultiAssetTypeMint] {
	if len(policies) == 0 {
		return nil
	}
	data := map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{}
	for _, p := range policies {
		data[p] = map[cbor.ByteString]common.MultiAssetTypeMint{
			cbor.NewByteString([]byte("t")): big.NewInt(1),
		}
	}
	ma := common.NewMultiAsset[common.MultiAssetTypeMint](data)
	return &ma
}

func wmVotes(m wmTx) common.VotingProcedures {
	if len(m.voters)+len(m.scriptVoters) == 0 {
		return nil
	}
	ret := common.VotingProcedures{}
	add := func(typ uint8, h common.Blake2b224) {
		ret[&common.Voter{Type: typ, Hash: h}] = map[*common.GovActionId]common.VotingProcedure{
			{GovActionIdx: 0}: {Vote: 1},
		}
	}
	for _, h := range m.voters {
		add(common.VoterTypeDRepKeyHash, h)
	}
	for _, h := range m.scriptVoters {
		add(common.VoterTypeDRepScriptHash, h)
	}
	return ret
}

type wmWitnesses struct {
	vkeys []common.VkeyWitness
	boots []common.BootstrapWitness
}

func wmSign(m wmTx, msg []byte) wmWitnesses {
	var w wmWitnesses
	for _, k := range []int{wmV1, wmV2} {
		if m.wits&(1<<k) != 0 {
			w.vkeys = append(w.vkeys, common.VkeyWitness{
				Vkey:      wmKeyset[k].pub,
				Signature: ed25519.Sign(wmKeyset[k].priv, msg),
			})
		}
	}
	if m.badSig {
		w.vkeys = append(w.vkeys, common.VkeyWitness{
			Vkey: wmKeyset[wmV2].pub, Signature: make([]byte, 64),
		})
	}
	if m.wits&(1<<wmB) != 0 {
		w.boots = append(w.boots, common.BootstrapWitness{
			PublicKey:  wmKeyset[wmB].pub,
			Signature:  ed25519.Sign(wmKeyset[wmB].priv, msg),
			ChainCode:  wmChainCode,
			Attributes: wmBootAttrs,
		})
	}
	return w
}

func wmPlutusOf(
	m wmTx,
) (v1 []common.PlutusV1Script, v2 []common.PlutusV2Script, v3 []common.PlutusV3Script) {
	for _, s := range m.plutus {
		switch p := s.(type) {
		case common.PlutusV1Script:
			v1 = append(v1, p)
		case common.PlutusV2Script:
			v2 = append(v2, p)
		case common.PlutusV3Script:
			v3 = append(v3, p)
		}
	}
	return v1, v2, v3
}

func wmAlonzoRedeemers(m wmTx) alonzo.AlonzoRedeemers {
	var r alonzo.AlonzoRedeemers
	for _, rd := range m.redeemers {
		r.Redeemers = append(r.Redeemers, alonzo.AlonzoRedeemer{
			Tag: rd.tag, Index: rd.index,
		})
	}
	return r
}

func wmConwayRedeemers(m wmTx) conway.ConwayRedeemers {
	r := conway.ConwayRedeemers{}
	if len(m.redeemers) == 0 {
		return r
	}
	r.Redeemers = map[common.RedeemerKey]common.RedeemerValue{}
	for _, rd := range m.redeemers {
		r.Redeemers[common.RedeemerKey{Tag: rd.tag, Index: rd.index}] = common.RedeemerValue{}
	}
	return r
}

func wmAlonzoOutputs(
	t *testing.T,
	outs []wmOut,
) []alonzo.AlonzoTransactionOutput {
	var ret []alonzo.AlonzoTransactionOutput
	for _, o := range outs {
		ret = append(ret, alonzo.AlonzoTransactionOutput{
			OutputAddress:   o.addr,
			OutputAmount:    mary.MaryTransactionOutputValue{Amount: 2_000_000},
			OutputDatumHash: o.datumHash,
		})
	}
	return ret
}

func wmBabbageOutputs(
	t *testing.T,
	outs []wmOut,
) []babbage.BabbageTransactionOutput {
	var ret []babbage.BabbageTransactionOutput
	for _, o := range outs {
		ret = append(ret, *wmBabbageOut(t, o))
	}
	return ret
}

func wmUpdateEpoch() uint64 { return 0 }

func wmSet[T any](items []T) cbor.SetType[T] {
	return cbor.NewSetType(items, false)
}

// wmBuild builds m as a transaction of era, with the given witnesses.
func wmBuild(
	t *testing.T,
	era wmEra,
	m wmTx,
	w wmWitnesses,
) common.Transaction {
	ins := wmInputs(0, len(m.ins))
	refs := wmInputs(10, len(m.refs))
	coll := wmInputs(20, len(m.coll))
	certs := wmCertWrappers(m.certs)
	wdrls := wmWithdrawals(t, m.wdrls)
	var hi uint64
	if m.hi != nil {
		hi = *m.hi
	}
	var lo uint64
	if m.lo != nil {
		lo = *m.lo
	}
	v1, v2, v3 := wmPlutusOf(m)
	switch era {
	case wmShelley:
		body := shelley.ShelleyTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(ins),
			Ttl:      hi, TxCertificates: certs, TxWithdrawals: wdrls,
		}
		if m.update {
			body.Update = &shelley.ShelleyTransactionPparamUpdate{
				ProtocolParamUpdates: map[common.Blake2b224]shelley.ShelleyProtocolParameterUpdate{
					wmGenesis: {},
				},
				Epoch: wmUpdateEpoch(),
			}
		}
		return &shelley.ShelleyTransaction{
			Body: body,
			WitnessSet: shelley.ShelleyTransactionWitnessSet{
				VkeyWitnesses: w.vkeys, BootstrapWitnesses: w.boots,
				WsNativeScripts: m.natives,
			},
		}
	case wmAllegra:
		body := allegra.AllegraTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(ins),
			Ttl:      hi, TxValidityIntervalStart: lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
		}
		if m.lo != nil {
			body.SetValidityIntervalLowerBoundPresence(true)
		}
		if m.update {
			body.Update = &allegra.AllegraTransactionPparamUpdate{
				ProtocolParamUpdates: map[common.Blake2b224]allegra.AllegraProtocolParameterUpdate{
					wmGenesis: {},
				},
				Epoch: wmUpdateEpoch(),
			}
		}
		return &allegra.AllegraTransaction{
			Body: body,
			WitnessSet: shelley.ShelleyTransactionWitnessSet{
				VkeyWitnesses: w.vkeys, BootstrapWitnesses: w.boots,
				WsNativeScripts: m.natives,
			},
		}
	case wmMary:
		body := mary.MaryTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(ins),
			Ttl:      hi, TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: wmMint(m.mint),
		}
		if m.lo != nil {
			lo := *m.lo
			body.TxValidityIntervalStart = &lo
		}
		if m.update {
			body.Update = &mary.MaryTransactionPparamUpdate{
				ProtocolParamUpdates: map[common.Blake2b224]mary.MaryProtocolParameterUpdate{
					wmGenesis: {},
				},
				Epoch: wmUpdateEpoch(),
			}
		}
		return &mary.MaryTransaction{
			Body: body,
			WitnessSet: shelley.ShelleyTransactionWitnessSet{
				VkeyWitnesses: w.vkeys, BootstrapWitnesses: w.boots,
				WsNativeScripts: m.natives,
			},
		}
	case wmAlonzo:
		body := alonzo.AlonzoTransactionBody{
			TxInputs:  shelley.NewShelleyTransactionInputSet(ins),
			TxOutputs: wmAlonzoOutputs(t, m.outs),
			Ttl:       hi, TxValidityIntervalStart: lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: wmMint(m.mint), TxScriptDataHash: m.sdh,
			TxCollateral:      wmSet(coll),
			TxRequiredSigners: wmSet(m.signers),
		}
		if m.lo != nil {
			body.SetValidityIntervalLowerBoundPresence(true)
		}
		if m.update {
			body.Update = &alonzo.AlonzoTransactionPparamUpdate{
				ProtocolParamUpdates: map[common.Blake2b224]alonzo.AlonzoProtocolParameterUpdate{
					wmGenesis: {},
				},
				Epoch: wmUpdateEpoch(),
			}
		}
		wits := alonzo.AlonzoTransactionWitnessSet{
			VkeyWitnesses: w.vkeys, BootstrapWitnesses: w.boots,
			WsNativeScripts: m.natives, WsPlutusV1Scripts: v1,
			WsRedeemers: wmAlonzoRedeemers(m),
		}
		wits.WsPlutusData.Items = m.datums
		return &alonzo.AlonzoTransaction{
			Body:       body,
			WitnessSet: wits,
			TxIsValid:  m.valid,
		}
	case wmBabbage:
		body := babbage.BabbageTransactionBody{
			TxInputs:  shelley.NewShelleyTransactionInputSet(ins),
			TxOutputs: wmBabbageOutputs(t, m.outs),
			Ttl:       hi, TxValidityIntervalStart: lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: wmMint(m.mint), TxScriptDataHash: m.sdh,
			TxCollateral:      wmSet(coll),
			TxRequiredSigners: wmSet(m.signers),
			TxReferenceInputs: wmSet(refs),
		}
		if m.lo != nil {
			body.SetValidityIntervalLowerBoundPresence(true)
		}
		if m.update {
			body.Update = &babbage.BabbageTransactionPparamUpdate{
				ProtocolParamUpdates: map[common.Blake2b224]babbage.BabbageProtocolParameterUpdate{
					wmGenesis: {},
				},
				Epoch: wmUpdateEpoch(),
			}
		}
		wits := babbage.BabbageTransactionWitnessSet{
			VkeyWitnesses: w.vkeys, BootstrapWitnesses: w.boots,
			WsNativeScripts: m.natives, WsPlutusV1Scripts: v1,
			WsPlutusV2Scripts: v2, WsRedeemers: wmAlonzoRedeemers(m),
		}
		wits.WsPlutusData.Items = m.datums
		return &babbage.BabbageTransaction{
			Body:       body,
			WitnessSet: wits,
			TxIsValid:  m.valid,
		}
	case wmConway:
		body := conway.ConwayTransactionBody{
			TxInputs:  conway.NewConwayTransactionInputSet(ins),
			TxOutputs: wmBabbageOutputs(t, m.outs),
			Ttl:       hi, TxValidityIntervalStart: lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: wmMint(m.mint), TxScriptDataHash: m.sdh,
			TxCollateral:       wmSet(coll),
			TxRequiredSigners:  wmSet(m.signers),
			TxReferenceInputs:  wmSet(refs),
			TxVotingProcedures: wmVotes(m),
		}
		if m.lo != nil {
			body.SetValidityIntervalLowerBoundPresence(true)
		}
		wits := conway.ConwayTransactionWitnessSet{
			VkeyWitnesses:      wmSet(w.vkeys),
			BootstrapWitnesses: wmSet(w.boots),
			WsNativeScripts:    wmSet(m.natives),
			WsPlutusV1Scripts:  wmSet(v1),
			WsPlutusV2Scripts:  wmSet(v2),
			WsPlutusV3Scripts:  wmSet(v3),
			WsPlutusData:       wmSet(m.datums),
			WsRedeemers:        wmConwayRedeemers(m),
		}
		return &conway.ConwayTransaction{
			Body:       body,
			WitnessSet: wits,
			TxIsValid:  m.valid,
		}
	default:
		var outs []dijkstra.DijkstraTransactionOutput
		for _, o := range m.outs {
			outs = append(
				outs,
				dijkstra.DijkstraTransactionOutput{Output: wmBabbageOut(t, o)},
			)
		}
		body := dijkstra.DijkstraTransactionBody{
			TxInputs:  conway.NewConwayTransactionInputSet(ins),
			TxOutputs: outs,
			Ttl:       hi, TxValidityIntervalStart: lo,
			TxCertificates: certs, TxWithdrawals: wdrls,
			TxMint: wmMint(m.mint), TxScriptDataHash: m.sdh,
			TxCollateral:       wmSet(coll),
			TxReferenceInputs:  wmSet(refs),
			TxVotingProcedures: wmVotes(m),
		}
		if m.lo != nil {
			body.SetValidityIntervalLowerBoundPresence(true)
		}
		wits := dijkstra.DijkstraTransactionWitnessSet{
			VkeyWitnesses:      wmSet(w.vkeys),
			BootstrapWitnesses: wmSet(w.boots),
			WsNativeScripts:    wmSet(m.natives),
			WsPlutusV1Scripts:  wmSet(v1),
			WsPlutusV2Scripts:  wmSet(v2),
			WsPlutusV3Scripts:  wmSet(v3),
			WsPlutusData:       wmSet(m.datums),
			WsRedeemers: dijkstra.DijkstraRedeemers{
				Redeemers: wmConwayRedeemers(m).Redeemers,
			},
		}
		return &dijkstra.DijkstraTransaction{
			Body:       body,
			WitnessSet: wits,
			TxIsValid:  m.valid,
		}
	}
}

// wmSigned builds m and signs its body hash with every supplied key.
func wmSigned(t *testing.T, era wmEra, m wmTx) common.Transaction {
	unsigned := wmBuild(t, era, m, wmWitnesses{})
	h := unsigned.Hash()
	return wmBuild(t, era, m, wmSign(m, h[:]))
}

func wmDescriptors(era wmEra) []common.UtxoValidationRuleDescriptor {
	switch era {
	case wmShelley:
		return shelley.UtxoValidationRuleDescriptors()
	case wmAllegra:
		return allegra.UtxoValidationRuleDescriptors()
	case wmMary:
		return mary.UtxoValidationRuleDescriptors()
	case wmAlonzo:
		return alonzo.UtxoValidationRuleDescriptors()
	case wmBabbage:
		return babbage.UtxoValidationRuleDescriptors()
	case wmConway:
		return conway.UtxoValidationRuleDescriptors()
	default:
		return dijkstra.UtxoValidationRuleDescriptors()
	}
}

// wmRun runs the era's rules whose ids are in ids, in the era's order, and
// returns the first failure.
func wmRun(
	era wmEra,
	ids map[common.UtxoValidationRuleId]bool,
	tx common.Transaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
	slot uint64,
) error {
	for _, d := range wmDescriptors(era) {
		if !ids[d.Id] {
			continue
		}
		if err := d.Validator(tx, slot, ls, pp); err != nil {
			return fmt.Errorf("%s: %w", d.Id, err)
		}
	}
	return nil
}

var wmCostModel = make([]int64, 166)

func wmPparams(era wmEra) common.ProtocolParameters {
	costs := map[uint][]int64{0: wmCostModel, 1: wmCostModel, 2: wmCostModel}
	switch era {
	case wmShelley, wmAllegra:
		return &shelley.ShelleyProtocolParameters{
			MaxTxSize:     1 << 20,
			ProtocolMajor: uint(2 + era),
		}
	case wmMary:
		return &mary.MaryProtocolParameters{
			MaxTxSize:     1 << 20,
			ProtocolMajor: 4,
		}
	case wmAlonzo:
		return &alonzo.AlonzoProtocolParameters{
			MaxTxSize:     1 << 20,
			ProtocolMajor: 6,
			CostModels:    costs,
		}
	case wmBabbage:
		return &babbage.BabbageProtocolParameters{
			MaxTxSize:     1 << 20,
			ProtocolMajor: 8,
			CostModels:    costs,
		}
	case wmConway:
		return &conway.ConwayProtocolParameters{
			MaxTxSize: 1 << 20, CostModels: costs,
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 10,
			},
		}
	default:
		return &dijkstra.DijkstraProtocolParameters{
			ConwayProtocolParameters: conway.ConwayProtocolParameters{
				MaxTxSize: 1 << 20, CostModels: costs,
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: 12,
				},
			},
		}
	}
}

func wmCheck(t *testing.T, cell string, want bool, err error, failures *int) {
	t.Helper()
	if want == (err == nil) {
		return
	}
	*failures++
	if *failures <= 20 {
		t.Errorf("%s: model accepts=%v, gOuroboros err=%v", cell, want, err)
	}
}

func wmMasks() []uint8 {
	ret := make([]uint8, 0, 1<<wmNumKeys)
	for m := range uint8(1 << wmNumKeys) {
		ret = append(ret, m)
	}
	return ret
}

// Family 1: key witnesses. A source names one thing that needs a key
// witness (or, for reg0, does not), keyed by v1 or b.

// wmKeySources lists the sources an era can express: collateral and
// required signers from Alonzo, voters from Conway, and the genesis
// delegation, MIR and update proposals only before Conway. Dijkstra replaces
// required signers with guards, so it has no signer source.
func wmKeySources(era wmEra) []string {
	src := []string{"in", "wdrl", "deleg", "reg0", "pool", "owner", "retire"}
	if era.atLeast(wmAlonzo) {
		src = append(src, "coll")
	}
	if era.atLeast(wmAlonzo) && era != wmDijkstra {
		src = append(src, "signer")
	}
	if era.atLeast(wmConway) {
		src = append(src, "voter")
	} else {
		src = append(src, "gdeleg", "mir", "ppup")
	}
	return src
}

type wmKeyCell struct {
	src   string
	key   int
	wits  uint8
	bad   bool
	valid bool
}

func (c wmKeyCell) String() string {
	return fmt.Sprintf("src=%s key=%s wits=%s bad=%v valid=%v",
		c.src, wmKeyNames[c.key], wmMaskString(c.wits), c.bad, c.valid)
}

// wmKeyNeeded is witsVKeyNeeded for one source (Shelley/UTxO.hs
// getShelleyWitsVKeyNeeded, Alonzo/UTxO.hs getAlonzoWitsVKeyNeeded,
// Conway/UTxO.hs getConwayWitsVKeyNeededWith, Dijkstra/Rules/Utxow.hs
// validateDijkstraNeededWitnesses). The owner source's operator is v2.
func wmKeyNeeded(era wmEra, c wmKeyCell) uint8 {
	k := uint8(1) << c.key
	switch c.src {
	case "reg0":
		return 0
	case "owner":
		ownersNeeded := era != wmDijkstra || c.valid
		if ownersNeeded {
			return 1<<wmV2 | k
		}
		return 1 << wmV2
	case "mir":
		// Quorum 1 of the one genesis delegate, k.
		return k
	default:
		return k
	}
}

// wmKeySpec: every needed key hash is among the witness key hashes (vkey
// hashes and bootstrap roots alike, keyHashWitnessesTxWits), and every
// supplied signature verifies (validateVerifiedWits). This includes the MIR
// genesis quorum and the update-proposal delegates: the reference takes
// genSig and witsVKeyNeededGenDelegs over keyHashWitnessesTxWits too.
func wmKeySpec(era wmEra, c wmKeyCell) bool {
	need := wmKeyNeeded(era, c)
	return need&^c.wits == 0 && !c.bad
}

func wmKeyTx(t *testing.T, era wmEra, c wmKeyCell) wmTx {
	h := wmHash(c.key)
	m := wmTx{
		// A script-locked input: it needs no key, and no script rule runs.
		ins:    []wmOut{{addr: wmScriptAddr(t, wmScriptKey)}},
		wits:   c.wits,
		badSig: c.bad,
		valid:  c.valid,
	}
	switch c.src {
	case "in":
		m.ins = append(m.ins, wmLockedBy(t, c.key))
	case "coll":
		m.coll = []wmOut{wmLockedBy(t, c.key)}
	case "wdrl":
		m.wdrls = []common.Credential{wmKeyCred(h)}
	case "deleg":
		cred := wmKeyCred(h)
		m.certs = []common.Certificate{&common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &cred, PoolKeyHash: wmPool,
		}}
	case "reg0":
		m.certs = []common.Certificate{&common.StakeRegistrationCertificate{
			CertType:        uint(common.CertificateTypeStakeRegistration),
			StakeCredential: wmKeyCred(h),
		}}
	case "pool", "owner":
		cert := &common.PoolRegistrationCertificate{
			CertType:      uint(common.CertificateTypePoolRegistration),
			Operator:      h,
			Margin:        common.GenesisRat{Rat: big.NewRat(0, 1)},
			RewardAccount: wmHash(wmV2),
		}
		if c.src == "owner" {
			cert.Operator = wmHash(wmV2)
			cert.PoolOwners = []common.AddrKeyHash{h}
		}
		m.certs = []common.Certificate{cert}
	case "retire":
		m.certs = []common.Certificate{&common.PoolRetirementCertificate{
			CertType:    uint(common.CertificateTypePoolRetirement),
			PoolKeyHash: h, Epoch: 1,
		}}
	case "gdeleg":
		m.certs = []common.Certificate{&common.GenesisKeyDelegationCertificate{
			CertType: uint(
				common.CertificateTypeGenesisKeyDelegation,
			),
			GenesisHash:         h[:],
			GenesisDelegateHash: wmPool[:],
		}}
	case "signer":
		m.signers = []common.Blake2b224{h}
	case "voter":
		m.voters = []common.Blake2b224{h}
	case "mir":
		m.certs = []common.Certificate{
			&common.MoveInstantaneousRewardsCertificate{
				CertType: uint(common.CertificateTypeMoveInstantaneousRewards),
			},
		}
	case "ppup":
		m.update = true
	}
	return m
}

var wmKeyRules = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleRequiredVKeyWitnesses:    true,
	common.UtxoValidationRuleSignatures:               true,
	common.UtxoValidationRuleCollateralVKeyWitnesses:  true,
	common.UtxoValidationRuleMIRGenesisQuorum:         true,
	common.UtxoValidationRuleProtocolParameterUpdates: true,
}

func TestWitnessRuleModelKeys(t *testing.T) {
	for _, era := range wmEras {
		t.Run(era.String(), func(t *testing.T) {
			cells, failures := 0, 0
			pp := wmPparams(era)
			// IsValid exists from Alonzo on. Alonzo and Babbage send a
			// phase-2-invalid update proposal to a separate witness check,
			// and Dijkstra excuses an invalid transaction's pool owners.
			valids := []bool{true}
			if era.atLeast(wmAlonzo) {
				valids = append(valids, false)
			}
			for _, src := range wmKeySources(era) {
				for _, key := range []int{wmV1, wmB} {
					for _, wits := range wmMasks() {
						for _, bad := range []bool{false, true} {
							for _, valid := range valids {
								if bad && wits&(1<<wmV2) != 0 {
									continue // the bad witness is a second v2
								}
								c := wmKeyCell{
									src:   src,
									key:   key,
									wits:  wits,
									bad:   bad,
									valid: valid,
								}
								m := wmKeyTx(t, era, c)
								ls := wmLedgerState(t, m, nil).
									WithGenesisDelegation(map[common.Blake2b224]common.Blake2b224{
										wmGenesis: wmHash(key),
									}, 1).
									WithProtocolParameterUpdateWindow(func(uint64) (uint64, uint64, error) {
										return 0, 1 << 40, nil
									}).
									Build()
								tx := wmSigned(t, era, m)
								err := wmRun(era, wmKeyRules, tx, ls, pp, 1)
								wmCheck(
									t,
									era.String()+" "+c.String(),
									wmKeySpec(era, c),
									err,
									&failures,
								)
								cells++
							}
						}
					}
				}
			}
			t.Logf("%d cells", cells)
		})
	}
}

// Family 2: native scripts. A script is a leaf or one combinator over at
// most two leaves.

type wmLeaf struct {
	op   string // "sig", "start" or "expire"
	key  int
	slot uint64
}

type wmScript struct {
	op   string // "leaf", "all", "any" or "atleast"
	m    int64
	subs []wmLeaf
}

func (l wmLeaf) String() string {
	if l.op == "sig" {
		return "sig " + wmKeyNames[l.key]
	}
	return fmt.Sprintf("%s %d", l.op, l.slot)
}

func (s wmScript) String() string {
	if s.op == "leaf" {
		return s.subs[0].String()
	}
	return fmt.Sprintf("%s(%d)%v", s.op, s.m, s.subs)
}

func wmLeaves(era wmEra) []wmLeaf {
	leaves := []wmLeaf{
		{op: "sig", key: wmV1}, {op: "sig", key: wmV2}, {op: "sig", key: wmB},
	}
	if era.atLeast(wmAllegra) {
		leaves = append(leaves,
			wmLeaf{op: "start", slot: 0}, wmLeaf{op: "start", slot: 1},
			wmLeaf{op: "expire", slot: 2})
	}
	return leaves
}

func wmScripts(era wmEra) []wmScript {
	leaves := wmLeaves(era)
	var ret []wmScript
	for _, l := range leaves {
		ret = append(ret, wmScript{op: "leaf", subs: []wmLeaf{l}})
	}
	ret = append(ret,
		wmScript{op: "all"}, wmScript{op: "any"},
		wmScript{op: "atleast", m: 0}, wmScript{op: "atleast", m: 1})
	for i := range leaves {
		for j := i + 1; j < len(leaves); j++ {
			pair := []wmLeaf{leaves[i], leaves[j]}
			ret = append(
				ret,
				wmScript{
					op:   "all",
					subs: pair,
				},
				wmScript{op: "any", subs: pair},
			)
			for m := int64(0); m <= 3; m++ {
				ret = append(ret, wmScript{op: "atleast", m: m, subs: pair})
			}
		}
	}
	return ret
}

// wmEvalLeaf is evalTimelock's leaf (Allegra/Scripts.hs 514-517) and
// evalMultiSig's signature (Shelley/Scripts.hs 319): a signature counts
// only from a vkey witness (validateTimelock and validateMultiSig take
// addrTxWitsL, not the bootstrap witnesses); a start bound needs a present
// lower bound at or after it; an expiry needs a present upper bound at or
// before it.
func wmEvalLeaf(l wmLeaf, wits uint8, lo, hi *uint64) bool {
	switch l.op {
	case "sig":
		return l.key != wmB && wits&(1<<l.key) != 0
	case "start":
		return lo != nil && l.slot <= *lo
	default:
		return hi != nil && *hi <= l.slot
	}
}

func wmEvalScript(s wmScript, wits uint8, lo, hi *uint64) bool {
	n := int64(0)
	for _, l := range s.subs {
		if wmEvalLeaf(l, wits, lo, hi) {
			n++
		}
	}
	switch s.op {
	case "leaf", "all":
		return n == int64(len(s.subs))
	case "any":
		return n > 0
	default:
		return n >= s.m
	}
}

func wmLeafCbor(t *testing.T, l wmLeaf) cbor.RawMessage {
	var v []any
	switch l.op {
	case "sig":
		h := wmHash(l.key)
		v = []any{0, h[:]}
	case "start":
		v = []any{4, l.slot}
	default:
		v = []any{5, l.slot}
	}
	raw, err := cbor.Encode(v)
	require.NoError(t, err)
	return raw
}

func wmNative(t *testing.T, s wmScript) common.NativeScript {
	var raw []byte
	if s.op == "leaf" {
		raw = wmLeafCbor(t, s.subs[0])
	} else {
		subs := []cbor.RawMessage{}
		for _, l := range s.subs {
			subs = append(subs, wmLeafCbor(t, l))
		}
		var v []any
		switch s.op {
		case "all":
			v = []any{1, subs}
		case "any":
			v = []any{2, subs}
		default:
			v = []any{3, s.m, subs}
		}
		var err error
		raw, err = cbor.Encode(v)
		require.NoError(t, err)
	}
	var ns common.NativeScript
	_, err := cbor.Decode(raw, &ns)
	require.NoError(t, err)
	return ns
}

func wmU64(v uint64) *uint64 { return &v }

func wmBounds(era wmEra) (los, his []*uint64) {
	los = []*uint64{nil}
	his = []*uint64{nil}
	if era.atLeast(wmAllegra) {
		los = append(los, wmU64(0), wmU64(1), wmU64(2))
		his = append(his, wmU64(1), wmU64(2), wmU64(3))
	}
	return los, his
}

func wmBoundString(b *uint64) string {
	if b == nil {
		return "-"
	}
	return fmt.Sprint(*b)
}

var wmNativeRules = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleNativeScripts:   true,
	common.UtxoValidationRuleScriptWitnesses: true,
}

func TestWitnessRuleModelNative(t *testing.T) {
	for _, era := range wmEras {
		t.Run(era.String(), func(t *testing.T) {
			cells, failures := 0, 0
			pp := wmPparams(era)
			vias := []string{"wit"}
			if era.atLeast(wmBabbage) {
				vias = append(vias, "ref")
			}
			los, his := wmBounds(era)
			for _, s := range wmScripts(era) {
				ns := wmNative(t, s)
				for _, via := range vias {
					m := wmTx{
						ins:   []wmOut{{addr: wmScriptAddr(t, ns.Hash())}},
						valid: true,
					}
					if via == "wit" {
						m.natives = []common.NativeScript{ns}
					} else {
						m.refs = []wmOut{{addr: wmKeyAddr(t, wmHash(wmV1)), scriptRef: ns}}
					}
					ls := wmLedgerState(t, m, nil).Build()
					for _, wits := range wmMasks() {
						for _, lo := range los {
							for _, hi := range his {
								m.wits, m.lo, m.hi = wits, lo, hi
								// Signatures are outside this family, so the
								// witnesses carry the right keys unsigned.
								tx := wmBuild(
									t,
									era,
									m,
									wmSign(m, make([]byte, 32)),
								)
								err := wmRun(era, wmNativeRules, tx, ls, pp, 1)
								cell := fmt.Sprintf(
									"%s script=%v via=%s wits=%s lo=%s hi=%s",
									era,
									s,
									via,
									wmMaskString(wits),
									wmBoundString(lo),
									wmBoundString(hi),
								)
								wmCheck(
									t,
									cell,
									wmEvalScript(s, wits, lo, hi),
									err,
									&failures,
								)
								cells++
							}
						}
					}
				}
			}
			t.Logf("%d cells", cells)
		})
	}
}

// Family 3: needed versus supplied scripts and redeemers. One purpose needs
// one script, native (always true) or Plutus; the script is supplied in the
// witness set, by a reference input, both or neither; an unneeded script
// may be supplied too; a redeemer may point at the purpose or at the
// key-locked input, which is no purpose.

type wmScriptCell struct {
	purpose, kind string
	wit, ref      bool
	extra, rdmr   string
}

func (c wmScriptCell) String() string {
	return fmt.Sprintf("purpose=%s kind=%s wit=%v ref=%v extra=%s rdmr=%s",
		c.purpose, c.kind, c.wit, c.ref, c.extra, c.rdmr)
}

// wmScriptSpec is babbageMissingScripts (Babbage/Rules/Utxow.hs 206-221):
// missing = needed - refs - wits, extra = wits - (needed - refs); before
// Babbage refs is empty and this is validateMissingScripts
// (Shelley/Rules/Utxow.hs 389-399). The redeemers must be exactly the
// needed Plutus purposes (hasExactSetOfRedeemers, Alonzo/Rules/Utxow.hs
// 251-275).
func wmScriptSpec(c wmScriptCell) bool {
	supplied := c.wit || c.ref
	extraWit := c.wit && c.ref
	rdmrOK := c.rdmr == "none"
	if c.kind == "plutus" {
		rdmrOK = c.rdmr == "purpose"
	}
	return supplied && !extraWit && c.extra != "wit" && rdmrOK
}

var (
	wmNeededNative = func() common.NativeScript { return wmNativeFixed([]any{1, []any{}}) }()
	wmExtraNative  = func() common.NativeScript { return wmNativeFixed([]any{2, []any{wmSigLeaf()}}) }()
	wmNeededPlutus = common.PlutusV1Script(
		[]byte{
			0x4d,
			0x01,
			0x00,
			0x00,
			0x33,
			0x22,
			0x22,
			0x20,
			0x05,
			0x12,
			0x00,
			0x12,
			0x00,
			0x11,
		},
	)
)

func wmSigLeaf() []any { h := common.Blake2b224Hash([]byte("x")); return []any{0, h[:]} }

func wmNativeFixed(v any) common.NativeScript {
	raw, err := cbor.Encode(v)
	if err != nil {
		panic(err)
	}
	var ns common.NativeScript
	if _, err := cbor.Decode(raw, &ns); err != nil {
		panic(err)
	}
	return ns
}

func wmScriptPurposes(era wmEra) []string {
	p := []string{"spend", "wdrl", "cert"}
	if era.atLeast(wmMary) {
		p = append(p, "mint")
	}
	if era.atLeast(wmConway) {
		p = append(p, "vote")
	}
	return p
}

func wmScriptTx(t *testing.T, c wmScriptCell) wmTx {
	var script common.Script = wmNeededNative
	if c.kind == "plutus" {
		script = wmNeededPlutus
	}
	h := script.Hash()
	m := wmTx{ins: []wmOut{wmLockedBy(t, wmV1)}, valid: true}
	var tag common.RedeemerTag
	switch c.purpose {
	case "spend":
		m.ins = append(m.ins, wmOut{addr: wmScriptAddr(t, h)})
		tag = common.RedeemerTagSpend
	case "wdrl":
		m.wdrls = []common.Credential{wmScriptCred(h)}
		tag = common.RedeemerTagReward
	case "cert":
		cred := wmScriptCred(h)
		m.certs = []common.Certificate{&common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &cred, PoolKeyHash: wmPool,
		}}
		tag = common.RedeemerTagCert
	case "mint":
		m.mint = []common.Blake2b224{common.Blake2b224(h)}
		tag = common.RedeemerTagMint
	case "vote":
		m.scriptVoters = []common.Blake2b224{common.Blake2b224(h)}
		tag = common.RedeemerTagVoting
	}
	if c.wit {
		if c.kind == "plutus" {
			m.plutus = []common.Script{wmNeededPlutus}
		} else {
			m.natives = []common.NativeScript{wmNeededNative}
		}
	}
	if c.ref {
		m.refs = append(
			m.refs,
			wmOut{addr: wmKeyAddr(t, wmHash(wmV2)), scriptRef: script},
		)
	}
	switch c.extra {
	case "wit":
		m.natives = append(m.natives, wmExtraNative)
	case "ref":
		m.refs = append(
			m.refs,
			wmOut{addr: wmKeyAddr(t, wmHash(wmV2)), scriptRef: wmExtraNative},
		)
	}
	switch c.rdmr {
	case "purpose":
		index := uint32(0)
		if c.purpose == "spend" {
			index = 1 // the script input sorts after the key-locked one
		}
		m.redeemers = []wmRedeemer{{tag: tag, index: index}}
	case "stray":
		m.redeemers = []wmRedeemer{{tag: common.RedeemerTagSpend, index: 0}}
	}
	return m
}

var wmScriptRules = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleScriptWitnesses:            true,
	common.UtxoValidationRuleNativeScripts:              true,
	common.UtxoValidationRuleRedeemerAndScriptWitnesses: true,
	common.UtxoValidationRuleExtraneousRedeemers:        true,
	common.UtxoValidationRuleRequiredRedeemers:          true,
}

func TestWitnessRuleModelScripts(t *testing.T) {
	for _, era := range wmEras {
		t.Run(era.String(), func(t *testing.T) {
			cells, failures := 0, 0
			pp := wmPparams(era)
			kinds, rdmrs := []string{"native"}, []string{"none"}
			if era.atLeast(wmAlonzo) {
				kinds = append(kinds, "plutus")
				rdmrs = append(rdmrs, "purpose", "stray")
			}
			refs, extras := []bool{false}, []string{"none", "wit"}
			if era.atLeast(wmBabbage) {
				refs = append(refs, true)
				extras = append(extras, "ref")
			}
			for _, purpose := range wmScriptPurposes(era) {
				for _, kind := range kinds {
					for _, wit := range []bool{false, true} {
						for _, ref := range refs {
							for _, extra := range extras {
								for _, rdmr := range rdmrs {
									c := wmScriptCell{
										purpose,
										kind,
										wit,
										ref,
										extra,
										rdmr,
									}
									m := wmScriptTx(t, c)
									ls := wmLedgerState(t, m, nil).Build()
									tx := wmBuild(t, era, m, wmWitnesses{})
									err := wmRun(
										era,
										wmScriptRules,
										tx,
										ls,
										pp,
										1,
									)
									wmCheck(
										t,
										era.String()+" "+c.String(),
										wmScriptSpec(c),
										err,
										&failures,
									)
									cells++
								}
							}
						}
					}
				}
			}
			t.Logf("%d cells", cells)
		})
	}
}

// Family 4: datums, from Alonzo. One spent input is locked by a Plutus
// script of some version, or by a native script, and holds no datum, a
// datum hash or (from Babbage) an inline datum. The witness set carries any
// of: the input's datum, the datum of an output's datum hash, the datum of a
// reference input's datum hash (from Babbage; the reference input exists
// only then), and a stray datum.

const (
	wmDatIn = 1 << iota
	wmDatOut
	wmDatRef
	wmDatStray
)

type wmDatumCell struct {
	lock  string
	ver   int
	datum string
	wd    int
}

func (c wmDatumCell) String() string {
	return fmt.Sprintf(
		"lock=%s ver=%d datum=%s wd=%04b",
		c.lock,
		c.ver,
		c.datum,
		c.wd,
	)
}

// wmDatumSpec is missingRequiredDatums (Alonzo/Rules/Utxow.hs 222-247) with
// getInputDataHashesTxBody (Alonzo/UTxO.hs 280-311) and the supplemental
// hashes of Alonzo (outputs, UTxO.hs 246-255) and Babbage (outputs and
// reference inputs, Babbage/UTxO.hs 92-102). A PlutusV1 script cannot see
// an inline datum, nor in Babbage a reference input (the context errors of
// Babbage/TxInfo.hs 256-262).
func wmDatumSpec(era wmEra, c wmDatumCell) bool {
	plutus := c.lock == "plutus"
	required := 0
	if plutus && c.datum == "hash" {
		required = wmDatIn
	}
	if plutus && c.datum == "none" && c.ver < 3 {
		return false // UnspendableUTxONoDatumHash
	}
	allowed := required | wmDatOut
	if era.atLeast(wmBabbage) {
		allowed |= wmDatRef
	}
	if plutus && c.ver == 1 &&
		(c.datum == "inline" || (era == wmBabbage && c.wd&wmDatRef != 0)) {
		return false
	}
	return c.wd&required == required && c.wd&^allowed == 0
}

func wmPlutusVersion(v int) common.Script {
	body := []byte{
		0x4d,
		0x01,
		0x00,
		0x00,
		0x33,
		0x22,
		0x22,
		0x20,
		0x05,
		0x12,
		0x00,
		0x12,
		0x00,
		byte(v),
	}
	switch v {
	case 1:
		return common.PlutusV1Script(body)
	case 2:
		return common.PlutusV2Script(body)
	default:
		return common.PlutusV3Script(body)
	}
}

func wmDatumTx(t *testing.T, c wmDatumCell) wmTx {
	var script common.Script = wmNeededNative
	if c.lock == "plutus" {
		script = wmPlutusVersion(c.ver)
	}
	in := wmOut{addr: wmScriptAddr(t, script.Hash())}
	switch c.datum {
	case "hash":
		in.datumHash = wmDatumHash(t, 1)
	case "inline":
		d := wmDatum(t, 1)
		in.inline = &d
	}
	m := wmTx{
		ins: []wmOut{in},
		outs: []wmOut{
			{addr: wmKeyAddr(t, wmHash(wmV1)), datumHash: wmDatumHash(t, 2)},
		},
		valid: true,
	}
	if c.lock == "plutus" {
		m.plutus = []common.Script{script}
		m.redeemers = []wmRedeemer{{tag: common.RedeemerTagSpend, index: 0}}
	} else {
		m.natives = []common.NativeScript{wmNeededNative}
	}
	if c.wd&wmDatRef != 0 {
		m.refs = []wmOut{
			{addr: wmKeyAddr(t, wmHash(wmV2)), datumHash: wmDatumHash(t, 3)},
		}
	}
	for i, bit := range []int{wmDatIn, wmDatOut, wmDatRef, wmDatStray} {
		if c.wd&bit != 0 {
			m.datums = append(m.datums, wmDatum(t, int64(i+1)))
		}
	}
	return m
}

var wmDatumRules = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleSupplementalDatums:       true,
	common.UtxoValidationRuleInlineDatumsWithPlutusV1: true,
}

func TestWitnessRuleModelDatums(t *testing.T) {
	for _, era := range wmEras[wmAlonzo:] {
		t.Run(era.String(), func(t *testing.T) {
			cells, failures := 0, 0
			pp := wmPparams(era)
			vers, datums, wdMax := []int{1}, []string{"none", "hash"}, 1<<4
			if era.atLeast(wmBabbage) {
				vers = append(vers, 2)
				datums = append(datums, "inline")
			}
			if era.atLeast(wmConway) {
				vers = append(vers, 3)
			}
			for _, lock := range []string{"plutus", "native"} {
				for _, ver := range vers {
					if lock == "native" && ver != 1 {
						continue
					}
					for _, datum := range datums {
						for wd := range wdMax {
							if !era.atLeast(wmBabbage) && wd&wmDatRef != 0 {
								continue
							}
							c := wmDatumCell{lock, ver, datum, wd}
							m := wmDatumTx(t, c)
							ls := wmLedgerState(t, m, nil).Build()
							tx := wmBuild(t, era, m, wmWitnesses{})
							err := wmRun(era, wmDatumRules, tx, ls, pp, 1)
							wmCheck(
								t,
								era.String()+" "+c.String(),
								wmDatumSpec(era, c),
								err,
								&failures,
							)
							cells++
						}
					}
				}
			}
			t.Logf("%d cells", cells)
		})
	}
}

// Family 5: the script integrity hash, from Alonzo. The transaction may run
// a PlutusV1 spending script (with its redeemer), may carry a supplemental
// datum, and (from Babbage) may reference an input holding an unneeded
// PlutusV2 script. It declares no hash, or the hash over the redeemers,
// datums and the language views of a language set.

type wmIntegrityCell struct {
	rd, dat, unused bool
	declared        string // "none", "used", "used+v2" or "nolangs"
}

func (c wmIntegrityCell) String() string {
	return fmt.Sprintf(
		"rd=%v dat=%v unused=%v declared=%s",
		c.rd,
		c.dat,
		c.unused,
		c.declared,
	)
}

// wmIntegrityLangs is the language set a declaration covers. The used set
// is the languages of the needed Plutus scripts (asatPlutusLanguagesUsed,
// Alonzo.hs 168-169): V1 when the spending script runs.
func wmIntegrityLangs(c wmIntegrityCell) map[uint]struct{} {
	langs := map[uint]struct{}{}
	if c.declared != "nolangs" && c.rd {
		langs[0] = struct{}{}
	}
	if c.declared == "used+v2" {
		langs[1] = struct{}{}
	}
	return langs
}

// wmIntegritySpec is mkScriptIntegrity (Alonzo/Tx.hs 463-482) and
// checkScriptIntegrityHash (Alonzo/Rules/Utxow.hs 278-300): no redeemers,
// no datums and no languages mean no hash; otherwise the declared hash must
// be the one over the used languages.
func wmIntegritySpec(c wmIntegrityCell) bool {
	if !c.rd && !c.dat {
		return c.declared == "none"
	}
	switch c.declared {
	case "none":
		return false
	case "used":
		return true
	case "nolangs":
		return !c.rd
	default:
		return false
	}
}

func wmIntegrityTx(t *testing.T, c wmIntegrityCell) wmTx {
	m := wmTx{
		ins: []wmOut{wmLockedBy(t, wmV1)},
		outs: []wmOut{
			{addr: wmKeyAddr(t, wmHash(wmV1)), datumHash: wmDatumHash(t, 2)},
		},
		valid: true,
	}
	if c.rd {
		v1 := wmPlutusVersion(1)
		m.ins = []wmOut{
			{addr: wmScriptAddr(t, v1.Hash()), datumHash: wmDatumHash(t, 1)},
		}
		m.plutus = []common.Script{v1}
		m.redeemers = []wmRedeemer{{tag: common.RedeemerTagSpend, index: 0}}
		m.datums = []common.Datum{wmDatum(t, 1)}
	}
	if c.dat {
		m.datums = append(m.datums, wmDatum(t, 2))
	}
	if c.unused {
		m.refs = []wmOut{
			{addr: wmKeyAddr(t, wmHash(wmV2)), scriptRef: wmPlutusVersion(2)},
		}
	}
	return m
}

// wmIntegrityHash is the hash over tx's redeemers and datums, encoded as the
// era's rule encodes them, and the language views of langs.
func wmIntegrityHash(
	t *testing.T,
	era wmEra,
	tx common.Transaction,
	langs map[uint]struct{},
) common.Blake2b256 {
	var rd, dat []byte
	var err error
	ws := tx.Witnesses()
	require.NotNil(t, ws)
	switch w := ws.(type) {
	case *alonzo.AlonzoTransactionWitnessSet:
		require.NotNil(t, w)
		ws = *w
	case *babbage.BabbageTransactionWitnessSet:
		require.NotNil(t, w)
		ws = *w
	}
	switch w := ws.(type) {
	case alonzo.AlonzoTransactionWitnessSet:
		rd, dat = wmAlonzoFamilyIntegrity(t, w.WsRedeemers, w.WsPlutusData)
	case babbage.BabbageTransactionWitnessSet:
		rd, dat = wmAlonzoFamilyIntegrity(t, w.WsRedeemers, w.WsPlutusData)
	default:
		rd = []byte{0xa0}
		if r := ws.Redeemers(); r != nil {
			n := 0
			for range r.Iter() {
				n++
			}
			if n > 0 {
				rd, err = cbor.Encode(r)
				require.NoError(t, err)
			}
		}
		if d := ws.PlutusData(); len(d) > 0 {
			dat, err = cbor.Encode(wmSet(d))
			require.NoError(t, err)
		}
	}
	views, err := common.EncodeLangViews(
		langs,
		map[uint][]int64{0: wmCostModel, 1: wmCostModel, 2: wmCostModel},
	)
	require.NoError(t, err)
	return common.Blake2b256Hash(append(append(rd, dat...), views...))
}

func wmAlonzoFamilyIntegrity(
	t *testing.T,
	r alonzo.AlonzoRedeemers,
	d alonzo.PlutusDataList,
) ([]byte, []byte) {
	var rd, dat []byte
	var err error
	if r.Redeemers == nil {
		rd, err = cbor.Encode([]alonzo.AlonzoRedeemer{})
	} else {
		rd, err = cbor.Encode(r.Redeemers)
	}
	require.NoError(t, err)
	if len(d.Items) > 0 {
		dat, err = cbor.Encode(d.Items)
		require.NoError(t, err)
	}
	return rd, dat
}

var wmIntegrityRules = map[common.UtxoValidationRuleId]bool{
	common.UtxoValidationRuleScriptDataHash:             true,
	common.UtxoValidationRuleCostModelsPresent:          true,
	common.UtxoValidationRuleRedeemerAndScriptWitnesses: true,
}

// Dijkstra is left out: its rule hashes only the preserved CBOR of the
// witness set, which a transaction built in memory does not have.
func TestWitnessRuleModelIntegrity(t *testing.T) {
	for _, era := range wmEras[wmAlonzo:wmDijkstra] {
		t.Run(era.String(), func(t *testing.T) {
			cells, failures := 0, 0
			pp := wmPparams(era)
			unuseds := []bool{false}
			if era.atLeast(wmBabbage) {
				unuseds = append(unuseds, true)
			}
			for _, rd := range []bool{false, true} {
				for _, dat := range []bool{false, true} {
					for _, unused := range unuseds {
						for _, declared := range []string{"none", "used", "used+v2", "nolangs"} {
							c := wmIntegrityCell{rd, dat, unused, declared}
							m := wmIntegrityTx(t, c)
							if declared != "none" {
								h := wmIntegrityHash(
									t,
									era,
									wmBuild(t, era, m, wmWitnesses{}),
									wmIntegrityLangs(c),
								)
								m.sdh = &h
							}
							ls := wmLedgerState(t, m, nil).Build()
							tx := wmBuild(t, era, m, wmWitnesses{})
							err := wmRun(era, wmIntegrityRules, tx, ls, pp, 1)
							wmCheck(
								t,
								era.String()+" "+c.String(),
								wmIntegritySpec(c),
								err,
								&failures,
							)
							cells++
						}
					}
				}
			}
			t.Logf("%d cells", cells)
		})
	}
}
