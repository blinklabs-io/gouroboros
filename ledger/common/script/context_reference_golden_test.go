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

package script_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/blinklabs-io/plutigo/data"
	fxcbor "github.com/fxamacker/cbor/v2"
)

// The cardano-ledger translation goldens (eras/<era>/impl/golden/
// translations.cbor) each hold a list of TranslationInstance records:
//
//	[protVer, language, utxo, tx, plutusPurpose, versionedTxInfo]
//
// built by Test.Cardano.Ledger.Alonzo.Translation.TranslationInstance. The
// expected TxInfo is not Plutus Data: it is the Generic-derived
// Codec.Serialise encoding of the plutus-ledger-api Haskell record, where a
// constructor is [constructorIndex, field...], a Haskell Maybe is [] or [x],
// a list is a CBOR array and an AssocMap is [0, [[k, v]...]]. The schema
// below rebuilds the ToData form of each record from that encoding, following
// the makeIsDataIndexed splices in plutus-ledger-api (PlutusLedgerApi/V1,
// V2 and V3 Contexts.hs, Tx.hs, DCert.hs, Interval.hs, Credential.hs). Every
// sum type there numbers its constructors in declaration order, which is the
// order Codec.Serialise's Generic encoding uses, so a constructor index
// carries across unchanged.

type refConv func(raw []byte) (data.PlutusData, error)

func refItems(raw []byte) ([]fxcbor.RawMessage, error) {
	var items []fxcbor.RawMessage
	if err := fxcbor.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("expected array: %w", err)
	}
	return items, nil
}

func refUint(raw []byte) (uint64, error) {
	var v uint64
	if err := fxcbor.Unmarshal(raw, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func refInteger(raw []byte) (data.PlutusData, error) {
	var v big.Int
	if err := fxcbor.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("integer: %w", err)
	}
	return data.NewInteger(&v), nil
}

func refBytes(raw []byte) (data.PlutusData, error) {
	var v []byte
	if err := fxcbor.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bytes: %w", err)
	}
	return data.NewByteString(v), nil
}

// refBool follows PlutusTx's Bool instance: False is Constr 0, True Constr 1.
func refBool(raw []byte) (data.PlutusData, error) {
	var v bool
	if err := fxcbor.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("bool: %w", err)
	}
	if v {
		return data.NewConstr(1), nil
	}
	return data.NewConstr(0), nil
}

func refFields(items []fxcbor.RawMessage, convs []refConv) ([]data.PlutusData, error) {
	if len(items) != len(convs) {
		return nil, fmt.Errorf(
			"expected %d fields, found %d",
			len(convs),
			len(items),
		)
	}
	ret := make([]data.PlutusData, len(items))
	for i, item := range items {
		pd, err := convs[i](item)
		if err != nil {
			return nil, fmt.Errorf("field %d: %w", i, err)
		}
		ret[i] = pd
	}
	return ret, nil
}

// refSum converts a constructor of a data type whose ToData index equals its
// declaration index; alts[i] lists the field converters of constructor i. A
// single-constructor record is a refSum with one alternative.
func refSum(alts ...[]refConv) refConv {
	return func(raw []byte) (data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, errors.New("constructor without index")
		}
		tag, err := refUint(items[0])
		if err != nil {
			return nil, err
		}
		if tag >= uint64(len(alts)) {
			return nil, fmt.Errorf("constructor index %d out of range", tag)
		}
		fields, err := refFields(items[1:], alts[tag])
		if err != nil {
			return nil, fmt.Errorf("constructor %d: %w", tag, err)
		}
		return data.NewConstr(tag, fields...), nil
	}
}

func refRecord(fields ...refConv) refConv {
	return refSum(fields)
}

// refNewtype unwraps a newtype whose ToData is derived through the newtype:
// the Generic encoding still wraps it as [0, field].
func refNewtype(inner refConv) refConv {
	return func(raw []byte) (data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return nil, err
		}
		if len(items) != 2 {
			return nil, fmt.Errorf("newtype with %d items", len(items))
		}
		if tag, err := refUint(items[0]); err != nil || tag != 0 {
			return nil, fmt.Errorf("newtype constructor %d: %w", tag, err)
		}
		return inner(items[1])
	}
}

// refMaybe converts a Haskell Maybe, which Codec.Serialise writes as [] or
// [x], into PlutusTx's Maybe: Just x is Constr 0 [x], Nothing is Constr 1 [].
func refMaybe(inner refConv) refConv {
	return func(raw []byte) (data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return nil, err
		}
		switch len(items) {
		case 0:
			return data.NewConstr(1), nil
		case 1:
			pd, err := inner(items[0])
			if err != nil {
				return nil, err
			}
			return data.NewConstr(0, pd), nil
		}
		return nil, fmt.Errorf("maybe with %d items", len(items))
	}
}

func refList(inner refConv) refConv {
	return func(raw []byte) (data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return nil, err
		}
		ret := make([]data.PlutusData, len(items))
		for i, item := range items {
			if ret[i], err = inner(item); err != nil {
				return nil, fmt.Errorf("list item %d: %w", i, err)
			}
		}
		return data.NewList(ret...), nil
	}
}

func refPairs(key, value refConv) func([]byte) ([][2]data.PlutusData, error) {
	return func(raw []byte) ([][2]data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return nil, err
		}
		ret := make([][2]data.PlutusData, len(items))
		for i, item := range items {
			pair, err := refItems(item)
			if err != nil {
				return nil, err
			}
			if len(pair) != 2 {
				return nil, fmt.Errorf("pair with %d items", len(pair))
			}
			if ret[i][0], err = key(pair[0]); err != nil {
				return nil, fmt.Errorf("pair %d key: %w", i, err)
			}
			if ret[i][1], err = value(pair[1]); err != nil {
				return nil, fmt.Errorf("pair %d value: %w", i, err)
			}
		}
		return ret, nil
	}
}

// refAssocMap converts PlutusTx.AssocMap.Map, a newtype over a list of pairs
// whose ToData is a Data map in list order.
func refAssocMap(key, value refConv) refConv {
	pairs := refPairs(key, value)
	return refNewtype(func(raw []byte) (data.PlutusData, error) {
		ret, err := pairs(raw)
		if err != nil {
			return nil, err
		}
		return data.NewMap(ret), nil
	})
}

// refTupleList converts a list of Haskell pairs, each of which PlutusTx
// encodes as Constr 0 [a, b] (the V1 txInfoWdrl and txInfoData fields).
func refTupleList(key, value refConv) refConv {
	pairs := refPairs(key, value)
	return func(raw []byte) (data.PlutusData, error) {
		ret, err := pairs(raw)
		if err != nil {
			return nil, err
		}
		items := make([]data.PlutusData, len(ret))
		for i, pair := range ret {
			items[i] = data.NewConstr(0, pair[0], pair[1])
		}
		return data.NewList(items...), nil
	}
}

// refBuiltinData converts BuiltinData, a Generic wrapper around the plutus
// Data whose own Serialise instance is the standard Plutus Data encoding.
func refBuiltinData(raw []byte) (data.PlutusData, error) {
	return refNewtype(func(inner []byte) (data.PlutusData, error) {
		return data.Decode(inner)
	})(raw)
}

var (
	refHash         = refNewtype(refBytes) // PubKeyHash, ScriptHash, DatumHash, CurrencySymbol, TokenName
	refCredential   = refSum([]refConv{refHash}, []refConv{refHash})
	refStakingCred  = refSum([]refConv{refCredential}, []refConv{refInteger, refInteger, refInteger})
	refAddress      = refRecord(refCredential, refMaybe(refStakingCred))
	refValue        = refNewtype(refAssocMap(refHash, refAssocMap(refHash, refInteger))) // Value and MintValue
	refTxIDV1       = refRecord(refBytes)                                                // V1/V2 TxId: makeIsDataIndexed, Constr 0 [B]
	refTxIDV3       = refNewtype(refBytes)
	refTxOutRefV1   = refRecord(refTxIDV1, refInteger)
	refTxOutRefV3   = refRecord(refTxIDV3, refInteger)
	refDatum        = data.Decode // Datum and Redeemer serialise as the Data itself
	refTxOutV1      = refRecord(refAddress, refValue, refMaybe(refHash))
	refOutputDatum  = refSum(nil, []refConv{refHash}, []refConv{refDatum})
	refTxOutV2      = refRecord(refAddress, refValue, refOutputDatum, refMaybe(refHash))
	refPOSIXTime    = refNewtype(refInteger)
	refExtended     = refSum(nil, []refConv{refPOSIXTime}, nil)
	refBound        = refRecord(refExtended, refBool)
	refInterval     = refRecord(refBound, refBound)
	refDCert        = refSum([]refConv{refStakingCred}, []refConv{refStakingCred}, []refConv{refStakingCred, refHash}, []refConv{refHash, refHash}, []refConv{refHash, refInteger}, nil, nil)
	refPurposeV1    = refSum([]refConv{refHash}, []refConv{refTxOutRefV1}, []refConv{refStakingCred}, []refConv{refDCert})
	refLovelace     = refNewtype(refInteger)
	refCommitteeKey = refNewtype(refCredential) // Cold/HotCommitteeCredential, DRepCredential
	refDRep         = refSum([]refConv{refCommitteeKey}, nil, nil)
	refDelegatee    = refSum([]refConv{refHash}, []refConv{refDRep}, []refConv{refHash, refDRep})
	refTxCert       = refSum(
		[]refConv{refCredential, refMaybe(refLovelace)},
		[]refConv{refCredential, refMaybe(refLovelace)},
		[]refConv{refCredential, refDelegatee},
		[]refConv{refCredential, refDelegatee, refLovelace},
		[]refConv{refCommitteeKey, refLovelace},
		[]refConv{refCommitteeKey},
		[]refConv{refCommitteeKey, refLovelace},
		[]refConv{refHash, refHash},
		[]refConv{refHash, refInteger},
		[]refConv{refCommitteeKey, refCommitteeKey},
		[]refConv{refCommitteeKey},
	)
	refVoter       = refSum([]refConv{refCommitteeKey}, []refConv{refCommitteeKey}, []refConv{refHash})
	refVote        = refSum(nil, nil, nil)
	refGovActionID = refRecord(refTxIDV3, refInteger)
	refRational    = refRecord(refInteger, refInteger) // ToData (n, d) is Constr 0 [n, d]
	refGovAction   = refSum(
		[]refConv{refMaybe(refGovActionID), refNewtype(refBuiltinData), refMaybe(refHash)},
		[]refConv{refMaybe(refGovActionID), refRecord(refInteger, refInteger)},
		[]refConv{refAssocMap(refCredential, refLovelace), refMaybe(refHash)},
		[]refConv{refMaybe(refGovActionID)},
		[]refConv{refMaybe(refGovActionID), refList(refCommitteeKey), refAssocMap(refCommitteeKey, refInteger), refRational},
		[]refConv{refMaybe(refGovActionID), refRecord(refMaybe(refHash))},
		nil,
	)
	refProposal  = refRecord(refLovelace, refCredential, refGovAction)
	refPurposeV3 = refSum(
		[]refConv{refHash},
		[]refConv{refTxOutRefV3},
		[]refConv{refCredential},
		[]refConv{refInteger, refTxCert},
		[]refConv{refVoter},
		[]refConv{refInteger, refProposal},
	)

	refTxInfoV1 = refRecord(
		refList(refRecord(refTxOutRefV1, refTxOutV1)),
		refList(refTxOutV1),
		refValue,
		refValue,
		refList(refDCert),
		refTupleList(refStakingCred, refInteger),
		refInterval,
		refList(refHash),
		refTupleList(refHash, refDatum),
		refTxIDV1,
	)
	refTxInfoV2 = refRecord(
		refList(refRecord(refTxOutRefV1, refTxOutV2)),
		refList(refRecord(refTxOutRefV1, refTxOutV2)),
		refList(refTxOutV2),
		refValue,
		refValue,
		refList(refDCert),
		refAssocMap(refStakingCred, refInteger),
		refInterval,
		refList(refHash),
		refAssocMap(refPurposeV1, refDatum),
		refAssocMap(refHash, refDatum),
		refTxIDV1,
	)
	refTxInfoV3 = refRecord(
		refList(refRecord(refTxOutRefV3, refTxOutV2)),
		refList(refRecord(refTxOutRefV3, refTxOutV2)),
		refList(refTxOutV2),
		refLovelace,
		refValue,
		refList(refTxCert),
		refAssocMap(refCredential, refLovelace),
		refInterval,
		refList(refHash),
		refAssocMap(refPurposeV3, refDatum),
		refAssocMap(refHash, refDatum),
		refTxIDV3,
		refAssocMap(refVoter, refAssocMap(refGovActionID, refVote)),
		refList(refProposal),
		refMaybe(refLovelace),
		refMaybe(refLovelace),
	)
	// VersionedTxInfo = TxInfoPV1 | TxInfoPV2 | TxInfoPV3 | TxInfoPV4. The
	// ToData of the result is the inner TxInfo, so the wrapper is dropped.
	refVersionedTxInfo = func(raw []byte) (uint64, data.PlutusData, error) {
		items, err := refItems(raw)
		if err != nil {
			return 0, nil, err
		}
		if len(items) != 2 {
			return 0, nil, fmt.Errorf("versioned TxInfo with %d items", len(items))
		}
		tag, err := refUint(items[0])
		if err != nil {
			return 0, nil, err
		}
		var conv refConv
		switch tag {
		case 0:
			conv = refTxInfoV1
		case 1:
			conv = refTxInfoV2
		case 2:
			conv = refTxInfoV3
		default:
			return tag, nil, fmt.Errorf("unsupported TxInfo version %d", tag)
		}
		pd, err := conv(items[1])
		return tag, pd, err
	}
)

// referenceSlotState reproduces the slot-to-time conversion of the golden
// generator (Test.Cardano.Ledger.Alonzo.Translation.TranslatableGen):
// fixedEpochInfo with one-second slots and a system start of
// posixSecondsToUTCTime 1684445839000. The start is in seconds, as there.
type referenceSlotState struct{}

const referenceSystemStartSeconds = 1684445839000

func (referenceSlotState) SlotToTime(slot uint64) (time.Time, error) {
	return time.Unix(referenceSystemStartSeconds+int64(slot), 0), nil // nolint:gosec
}

func (referenceSlotState) TimeToSlot(t time.Time) (uint64, error) {
	return uint64(t.Unix() - referenceSystemStartSeconds), nil // nolint:gosec
}

type referenceTranslationEra struct {
	name      string
	newTx     func([]byte) (lcommon.Transaction, error)
	newOutput func([]byte) (lcommon.TransactionOutput, error)
	strict    bool
}

var referenceTranslationEras = []referenceTranslationEra{
	{
		name: "alonzo",
		newTx: func(b []byte) (lcommon.Transaction, error) {
			return alonzo.NewAlonzoTransactionFromCbor(b)
		},
		newOutput: func(b []byte) (lcommon.TransactionOutput, error) {
			return alonzo.NewAlonzoTransactionOutputFromCbor(b)
		},
	},
	{
		name: "babbage",
		newTx: func(b []byte) (lcommon.Transaction, error) {
			return babbage.NewBabbageTransactionFromCbor(b)
		},
		newOutput: func(b []byte) (lcommon.TransactionOutput, error) {
			return babbage.NewBabbageTransactionOutputFromCbor(b)
		},
	},
	{
		name: "conway",
		newTx: func(b []byte) (lcommon.Transaction, error) {
			return conway.NewConwayTransactionFromCbor(b)
		},
		newOutput: func(b []byte) (lcommon.TransactionOutput, error) {
			return babbage.NewBabbageTransactionOutputFromCbor(b)
		},
		strict: true,
	},
}

// decodeReferenceUtxo decodes a ledger UTxO map (TxIn -> TxOut). Its keys are
// arrays, so it is walked pair by pair rather than decoded into a Go map.
func decodeReferenceUtxo(
	raw []byte,
	newOutput func([]byte) (lcommon.TransactionOutput, error),
) ([]lcommon.Utxo, error) {
	if len(raw) == 0 || raw[0]>>5 != 5 {
		return nil, errors.New("UTxO is not a CBOR map")
	}
	count, headerLen, indefinite, err := mapHeader(raw)
	if err != nil {
		return nil, err
	}
	dec := fxcbor.NewDecoder(bytes.NewReader(raw[headerLen:]))
	var ret []lcommon.Utxo
	for i := 0; indefinite || i < count; i++ {
		if indefinite && dec.NumBytesRead() < len(raw)-headerLen &&
			raw[headerLen+dec.NumBytesRead()] == 0xff {
			break
		}
		var key, value fxcbor.RawMessage
		if err := dec.Decode(&key); err != nil {
			return nil, fmt.Errorf("UTxO key %d: %w", i, err)
		}
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("UTxO value %d: %w", i, err)
		}
		var input shelley.ShelleyTransactionInput
		if _, err := cbor.Decode(key, &input); err != nil {
			return nil, fmt.Errorf("UTxO key %d: %w", i, err)
		}
		output, err := newOutput(value)
		if err != nil {
			return nil, fmt.Errorf("UTxO value %d: %w", i, err)
		}
		ret = append(ret, lcommon.Utxo{Id: input, Output: output})
	}
	return ret, nil
}

func mapHeader(raw []byte) (int, int, bool, error) {
	info := raw[0] & 0x1f
	switch {
	case info < 24:
		return int(info), 1, false, nil
	case info == 24 && len(raw) > 1:
		return int(raw[1]), 2, false, nil
	case info == 25 && len(raw) > 2:
		return int(raw[1])<<8 | int(raw[2]), 3, false, nil
	case info == 31:
		return 0, 1, true, nil
	}
	return 0, 0, false, fmt.Errorf("unsupported map header 0x%02x", raw[0])
}

// translationGoldenPath is the fixture path of each era's golden inside the
// ouroboros-mock embedded corpus.
func translationGoldenPath(era string) string {
	return "upstream/cardano-ledger/eras/" + era + "/impl/golden/translations.cbor"
}

func readTranslationGolden(t *testing.T, era string) []byte {
	t.Helper()
	// A directory holding <era>-translations.cbor files checks a golden
	// from another cardano-ledger revision without changing the fixtures.
	if dir := os.Getenv("TRANSLATION_GOLDEN_DIR"); dir != "" {
		b, err := os.ReadFile(filepath.Join(dir, era+"-translations.cbor"))
		if err != nil {
			t.Fatalf("read %s golden: %v", era, err)
		}
		return b
	}
	b, err := fs.ReadFile(fixtures.EmbeddedFixtures(), translationGoldenPath(era))
	if err != nil {
		t.Fatalf("read %s golden: %v", era, err)
	}
	return b
}

// TestTxInfoMatchesLedgerTranslationGoldens rebuilds every TxInfo recorded in
// cardano-ledger's translation goldens with NewTxInfoV{1,2,3}FromTransaction
// and checks that its Plutus Data equals the ToData form of the expected
// plutus-ledger-api record.
func TestTxInfoMatchesLedgerTranslationGoldens(t *testing.T) {
	for _, era := range referenceTranslationEras {
		t.Run(era.name, func(t *testing.T) {
			checkTranslationGolden(t, era, readTranslationGolden(t, era.name))
		})
	}
}

func checkTranslationGolden(
	t *testing.T,
	era referenceTranslationEra,
	golden []byte,
) {
	var instances []fxcbor.RawMessage
	if err := fxcbor.Unmarshal(golden, &instances); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(instances) == 0 {
		t.Fatal("golden holds no translation instances")
	}
	for idx, raw := range instances {
		t.Run(fmt.Sprintf("instance-%03d", idx), func(t *testing.T) {
			fields, err := refItems(raw)
			// Older goldens have no plutusPurpose field; the TxInfo is
			// always last.
			if err != nil || (len(fields) != 5 && len(fields) != 6) {
				t.Fatalf("translation instance shape: %v (%d fields)", err, len(fields))
			}
			var protVer []uint64
			if err := fxcbor.Unmarshal(fields[0], &protVer); err != nil || len(protVer) != 2 {
				t.Fatalf("protocol version: %v", err)
			}
			lang, err := refUint(fields[1])
			if err != nil {
				t.Fatalf("language: %v", err)
			}
			utxos, err := decodeReferenceUtxo(fields[2], era.newOutput)
			if err != nil {
				t.Fatalf("utxo: %v", err)
			}
			tx, err := era.newTx(fields[3])
			if err != nil {
				// Older goldens were generated when Conway's ProtVerHigh
				// was 12, so they propose hard forks to 13. At the pinned
				// cardano-ledger revision decodeProtVer (BaseTypes.hs)
				// caps a Conway proposal at succVersion 11 = 12, which is
				// the bound the decoder enforces.
				if era.name == "conway" &&
					strings.Contains(err.Error(), "exceeds Conway maximum 12") {
					t.Skipf("hard fork proposal beyond the current Conway bound: %v", err)
				}
				t.Fatalf("transaction: %v", err)
			}
			version, expected, err := refVersionedTxInfo(fields[len(fields)-1])
			if err != nil {
				t.Fatalf("expected TxInfo: %v", err)
			}
			if version != lang {
				t.Fatalf("language %d with TxInfo version %d", lang, version)
			}
			// The Alonzo generator draws arbitrary redeemers, most of which
			// point at nothing. A PlutusV1 TxInfo has no redeemer field
			// (PlutusLedgerApi.V1.Contexts.TxInfo), so the reference
			// translation ignores them, whereas NewTxInfoV1FromTransaction
			// resolves each redeemer's purpose and rejects a dangling one,
			// as phase-1 validation would. Clear them so the comparison
			// covers the fields the TxInfo carries.
			if alonzoTx, ok := tx.(*alonzo.AlonzoTransaction); ok && lang == 0 {
				alonzoTx.WitnessSet.WsRedeemers = alonzo.AlonzoRedeemers{}
			}
			major := uint(protVer[0])
			var actual script.TxInfo
			switch lang {
			case 0:
				actual, err = script.NewTxInfoV1FromTransaction(referenceSlotState{}, tx, utxos, era.strict, major)
			case 1:
				actual, err = script.NewTxInfoV2FromTransaction(referenceSlotState{}, tx, utxos, era.strict, major)
			case 2:
				actual, err = script.NewTxInfoV3FromTransaction(referenceSlotState{}, tx, utxos, major)
			default:
				t.Fatalf("unsupported language %d", lang)
			}
			if err != nil {
				t.Fatalf("PlutusV%d TxInfo at protocol %d: %v", lang+1, major, err)
			}
			compareTxInfoData(t, expected, actual.ToPlutusData())
		})
	}
}

func compareTxInfoData(t *testing.T, expected, actual data.PlutusData) {
	t.Helper()
	if path, exp, act, ok := firstDataDifference("TxInfo", expected, actual); !ok {
		t.Errorf("%s differs:\nexpected %s\nactual   %s", path, exp, act)
	}
}

// dataEqual compares Plutus Data values, which is what a script observes;
// definite or indefinite CBOR framing does not take part.
func dataEqual(a, b data.PlutusData) bool {
	return a.Equal(b)
}

// firstDataDifference walks two Plutus Data values in step and reports the
// path of the outermost-leftmost node where they part.
func firstDataDifference(
	path string,
	expected, actual data.PlutusData,
) (string, data.PlutusData, data.PlutusData, bool) {
	if dataEqual(expected, actual) {
		return "", nil, nil, true
	}
	var expItems, actItems []data.PlutusData
	switch exp := expected.(type) {
	case *data.Constr:
		act, ok := actual.(*data.Constr)
		if !ok || exp.Tag.Cmp(act.Tag) != 0 {
			return path, expected, actual, false
		}
		expItems, actItems = exp.Fields(), act.Fields()
	case *data.List:
		act, ok := actual.(*data.List)
		if !ok {
			return path, expected, actual, false
		}
		expItems, actItems = exp.Items(), act.Items()
	case *data.Map:
		act, ok := actual.(*data.Map)
		if !ok {
			return path, expected, actual, false
		}
		for _, pair := range exp.Pairs() {
			expItems = append(expItems, pair[0], pair[1])
		}
		for _, pair := range act.Pairs() {
			actItems = append(actItems, pair[0], pair[1])
		}
	default:
		return path, expected, actual, false
	}
	if len(expItems) != len(actItems) {
		return fmt.Sprintf("%s (length %d vs %d)", path, len(expItems), len(actItems)), expected, actual, false
	}
	for i := range expItems {
		if p, e, a, ok := firstDataDifference(fmt.Sprintf("%s.%d", path, i), expItems[i], actItems[i]); !ok {
			return p, e, a, false
		}
	}
	return path, expected, actual, false
}
