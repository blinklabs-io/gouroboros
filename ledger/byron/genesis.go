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

package byron

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ByronGenesisFtsSeed struct {
	Value    string
	IsObject bool
}
type ByronGenesis struct {
	AvvmDistr            map[string]string                      `json:"avvmDistr"`
	BlockVersionData     ByronGenesisBlockVersionData           `json:"blockVersionData"`
	FtsSeed              ByronGenesisFtsSeed                    `json:"ftsSeed"`
	ProtocolConsts       ByronGenesisProtocolConsts             `json:"protocolConsts"`
	StartTime            int                                    `json:"startTime"`
	BootStakeholders     map[string]int                         `json:"bootStakeholders"`
	HeavyDelegation      map[string]ByronGenesisHeavyDelegation `json:"heavyDelegation"`
	NonAvvmBalances      map[string]string                      `json:"nonAvvmBalances"`
	VssCerts             map[string]ByronGenesisVssCert         `json:"vssCerts"`
	RequiresNetworkMagic string                                 `json:"requiresNetworkMagic"`
}

type ByronGenesisBlockVersionData struct {
	HeavyDelThd       int64                                    `json:"heavyDelThd,string"`
	MaxBlockSize      int                                      `json:"maxBlockSize,string"`
	MaxHeaderSize     int                                      `json:"maxHeaderSize,string"`
	MaxProposalSize   int                                      `json:"maxProposalSize,string"`
	MaxTxSize         int                                      `json:"maxTxSize,string"`
	MpcThd            int64                                    `json:"mpcThd,string"`
	ScriptVersion     int                                      `json:"scriptVersion"`
	SlotDuration      int                                      `json:"slotDuration,string"`
	SoftforkRule      ByronGenesisBlockVersionDataSoftforkRule `json:"softforkRule"`
	TxFeePolicy       ByronGenesisBlockVersionDataTxFeePolicy  `json:"txFeePolicy"`
	UnlockStakeEpoch  uint64                                   `json:"unlockStakeEpoch,string"`
	UpdateImplicit    int                                      `json:"updateImplicit,string"`
	UpdateProposalThd int64                                    `json:"updateProposalThd,string"`
	UpdateVoteThd     int64                                    `json:"updateVoteThd,string"`
}

type ByronGenesisBlockVersionDataSoftforkRule struct {
	InitThd      int64 `json:"initThd,string"`
	MinThd       int64 `json:"minThd,string"`
	ThdDecrement int64 `json:"thdDecrement,string"`
}

type ByronGenesisBlockVersionDataTxFeePolicy struct {
	Multiplier int64 `json:"multiplier,string"`
	Summand    int64 `json:"summand,string"`
}

type ByronGenesisProtocolConsts struct {
	K             int `json:"k"`
	ProtocolMagic int `json:"protocolMagic"`
	VssMinTTL     int `json:"vssMinTtl"`
	VssMaxTTL     int `json:"vssMaxTtl"`
}

type ByronGenesisHeavyDelegation struct {
	Cert       string `json:"cert"`
	DelegatePk string `json:"delegatePk"`
	IssuerPk   string `json:"issuerPk"`
	Omega      int    `json:"omega"`
}

type ByronGenesisVssCert struct {
	ExpiryEpoch int    `json:"expiryEpoch"`
	Signature   string `json:"signature"`
	SigningKey  string `json:"signingKey"`
	VssKey      string `json:"vssKey"`
}

func (g *ByronGenesis) GenesisUtxos() ([]common.Utxo, error) {
	avvmUtxos, err := g.avvmUtxos()
	if err != nil {
		return nil, err
	}
	nonAvvmUtxos, err := g.nonAvvmUtxos()
	if err != nil {
		return nil, err
	}
	ret := slices.Concat(
		avvmUtxos,
		nonAvvmUtxos,
	)
	seen := make(map[genesisUtxoRef]struct{}, len(ret))
	for _, utxo := range ret {
		ref := genesisUtxoRef{id: utxo.Id.Id(), index: utxo.Id.Index()}
		if _, exists := seen[ref]; exists {
			return nil, fmt.Errorf(
				"duplicate Byron genesis UTxO reference %s#%d",
				ref.id,
				ref.index,
			)
		}
		seen[ref] = struct{}{}
	}
	return ret, nil
}

type genesisUtxoRef struct {
	id    common.Blake2b256
	index uint32
}

func (g *ByronGenesis) avvmUtxos() ([]common.Utxo, error) {
	ret := []common.Utxo{}
	for pubkey, amount := range g.AvvmDistr {
		// Build address from redeem pubkey
		pubkeyBytes, err := base64.URLEncoding.DecodeString(pubkey)
		if err != nil {
			return nil, err
		}
		attributes := common.ByronAddressAttributes{}
		switch g.RequiresNetworkMagic {
		case "", "RequiresNoMagic":
		case "RequiresMagic":
			if g.ProtocolConsts.ProtocolMagic < 0 ||
				uint64(g.ProtocolConsts.ProtocolMagic) > uint64(^uint32(0)) {
				return nil, fmt.Errorf(
					"invalid Byron protocol magic %d",
					g.ProtocolConsts.ProtocolMagic,
				)
			}
			magic := uint32(g.ProtocolConsts.ProtocolMagic)
			attributes.Network = &magic
		default:
			return nil, fmt.Errorf(
				"invalid requiresNetworkMagic value %q",
				g.RequiresNetworkMagic,
			)
		}
		tmpAddr, err := common.NewByronAddressRedeem(pubkeyBytes, attributes)
		if err != nil {
			return nil, err
		}
		tmpAmount, err := strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return nil, err
		}
		addrBytes, err := tmpAddr.Bytes()
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			common.Utxo{
				Id: ByronTransactionInput{
					TxId:        common.Blake2b256Hash(addrBytes),
					OutputIndex: 0,
				},
				Output: ByronTransactionOutput{
					OutputAddress: tmpAddr,
					OutputAmount:  tmpAmount,
				},
			},
		)
	}
	return ret, nil
}

func (g *ByronGenesis) nonAvvmUtxos() ([]common.Utxo, error) {
	ret := []common.Utxo{}
	for address, amount := range g.NonAvvmBalances {
		tmpAddr, err := common.NewAddress(address)
		if err != nil {
			return nil, err
		}
		tmpAmount, err := strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return nil, err
		}
		addrBytes, err := tmpAddr.Bytes()
		if err != nil {
			return nil, err
		}
		ret = append(
			ret,
			common.Utxo{
				Id: ByronTransactionInput{
					TxId:        common.Blake2b256Hash(addrBytes),
					OutputIndex: 0,
				},
				Output: ByronTransactionOutput{
					OutputAddress: tmpAddr,
					OutputAmount:  tmpAmount,
				},
			},
		)
	}
	return ret, nil
}

func NewByronGenesisFromReader(r io.Reader) (ByronGenesis, error) {
	var ret ByronGenesis
	// Decode straight from r through a tee, rather than reading it to
	// completion with io.ReadAll first: r may stay open past the genesis
	// value (a long-lived connection, a multi-document stream), and the
	// original behavior here -- like encoding/json's own Decode -- reads
	// only the one JSON value, not until EOF. dec.InputOffset() after a
	// successful Decode gives the exact end of that value, so the checks
	// below run only over the bytes the value actually used, not any
	// read-ahead the decoder buffered past it.
	var raw bytes.Buffer
	dec := json.NewDecoder(io.TeeReader(r, &raw))
	if err := dec.Decode(&ret); err != nil {
		return ret, err
	}
	data := raw.Bytes()[:dec.InputOffset()]
	if err := rejectNonCanonicalJSONEscapes(data); err != nil {
		return ByronGenesis{}, err
	}
	if err := validateGenesisRequiredFields(data); err != nil {
		return ByronGenesis{}, err
	}
	if err := validateGenesisParameterDomains(ret); err != nil {
		return ByronGenesis{}, err
	}
	return ret, nil
}

// rejectNonCanonicalJSONEscapes rejects a Byron genesis document containing
// a string escape outside the historical canonical-JSON grammar the Byron
// reference parses genesis with. That grammar permits only the quote (\")
// and backslash (\\) escapes inside a string; encoding/json additionally
// accepts and normalizes \/, \n, \r, \t, \b, \f, and \uXXXX, which would
// silently admit a genesis document the reference rejects before schema
// decoding.
//
// This is a byte-level scan for escape sequences within JSON string
// literals, not a full JSON parser: it tracks only whether the current byte
// is inside a string (and, if so, inside an escape sequence), which is
// enough to find every backslash a compliant JSON string can contain
// without needing to otherwise validate the document's structure -- a
// malformed document is left for the subsequent encoding/json decode to
// reject with its own error. UTF-8 continuation bytes are always >= 0x80,
// so a byte-level scan cannot misread a multi-byte character as a quote or
// backslash.
func rejectNonCanonicalJSONEscapes(data []byte) error {
	const (
		outsideString = iota
		insideString
		insideEscape
	)
	state := outsideString
	for _, b := range data {
		switch state {
		case outsideString:
			if b == '"' {
				state = insideString
			}
		case insideString:
			switch b {
			case '\\':
				state = insideEscape
			case '"':
				state = outsideString
			}
		case insideEscape:
			if b != '"' && b != '\\' {
				return fmt.Errorf(
					"byron genesis contains disallowed JSON escape \\%c: "+
						"the canonical-JSON grammar permits only \\\" and \\\\",
					b,
				)
			}
			state = insideString
		}
	}
	return nil
}

func validateGenesisRequiredFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if err := requireGenesisFields("", fields,
		"avvmDistr",
		"blockVersionData",
		"protocolConsts",
		"startTime",
		"bootStakeholders",
		"heavyDelegation",
		"nonAvvmBalances",
	); err != nil {
		return err
	}
	var blockVersionData map[string]json.RawMessage
	if err := json.Unmarshal(fields["blockVersionData"], &blockVersionData); err != nil {
		return fmt.Errorf("blockVersionData: %w", err)
	}
	if err := requireGenesisFields("blockVersionData", blockVersionData,
		"heavyDelThd",
		"maxBlockSize",
		"maxHeaderSize",
		"maxProposalSize",
		"maxTxSize",
		"mpcThd",
		"scriptVersion",
		"slotDuration",
		"softforkRule",
		"txFeePolicy",
		"unlockStakeEpoch",
		"updateImplicit",
		"updateProposalThd",
		"updateVoteThd",
	); err != nil {
		return err
	}
	var softforkRule map[string]json.RawMessage
	if err := json.Unmarshal(blockVersionData["softforkRule"], &softforkRule); err != nil {
		return fmt.Errorf("blockVersionData.softforkRule: %w", err)
	}
	if err := requireGenesisFields("blockVersionData.softforkRule", softforkRule,
		"initThd", "minThd", "thdDecrement",
	); err != nil {
		return err
	}
	var txFeePolicy map[string]json.RawMessage
	if err := json.Unmarshal(blockVersionData["txFeePolicy"], &txFeePolicy); err != nil {
		return fmt.Errorf("blockVersionData.txFeePolicy: %w", err)
	}
	if err := requireGenesisFields("blockVersionData.txFeePolicy", txFeePolicy,
		"multiplier", "summand",
	); err != nil {
		return err
	}
	var protocolConsts map[string]json.RawMessage
	if err := json.Unmarshal(fields["protocolConsts"], &protocolConsts); err != nil {
		return fmt.Errorf("protocolConsts: %w", err)
	}
	return requireGenesisFields("protocolConsts", protocolConsts,
		"k", "protocolMagic",
	)
}

func requireGenesisFields(
	object string,
	fields map[string]json.RawMessage,
	names ...string,
) error {
	for _, name := range names {
		value, ok := fields[name]
		field := name
		if object != "" {
			field = object + "." + name
		}
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("missing required Byron genesis field %s", field)
		}
	}
	return nil
}

func validateGenesisParameterDomains(genesis ByronGenesis) error {
	const maxLovelacePortion = int64(1_000_000_000_000_000)
	const maxTxFeeSummand = int64(45_000_000_000_000_000)
	thresholds := []struct {
		name  string
		value int64
	}{
		{"blockVersionData.heavyDelThd", genesis.BlockVersionData.HeavyDelThd},
		{"blockVersionData.mpcThd", genesis.BlockVersionData.MpcThd},
		{"blockVersionData.updateProposalThd", genesis.BlockVersionData.UpdateProposalThd},
		{"blockVersionData.updateVoteThd", genesis.BlockVersionData.UpdateVoteThd},
		{"blockVersionData.softforkRule.initThd", genesis.BlockVersionData.SoftforkRule.InitThd},
		{"blockVersionData.softforkRule.minThd", genesis.BlockVersionData.SoftforkRule.MinThd},
		{"blockVersionData.softforkRule.thdDecrement", genesis.BlockVersionData.SoftforkRule.ThdDecrement},
	}
	for _, threshold := range thresholds {
		if threshold.value < 0 || threshold.value > maxLovelacePortion {
			return fmt.Errorf(
				"%s must be between 0 and %d, got %d",
				threshold.name,
				maxLovelacePortion,
				threshold.value,
			)
		}
	}
	if scriptVersion := genesis.BlockVersionData.ScriptVersion; scriptVersion < 0 || scriptVersion > 1<<16-1 {
		return fmt.Errorf(
			"blockVersionData.scriptVersion must be between 0 and %d, got %d",
			1<<16-1,
			scriptVersion,
		)
	}
	summand := genesis.BlockVersionData.TxFeePolicy.Summand
	if summand < 0 || summand > maxTxFeeSummand {
		return fmt.Errorf(
			"blockVersionData.txFeePolicy.summand must be between 0 and %d, got %d",
			maxTxFeeSummand,
			summand,
		)
	}

	unsigned := []struct {
		name  string
		value int
	}{
		{"blockVersionData.slotDuration", genesis.BlockVersionData.SlotDuration},
		{"blockVersionData.maxBlockSize", genesis.BlockVersionData.MaxBlockSize},
		{"blockVersionData.maxHeaderSize", genesis.BlockVersionData.MaxHeaderSize},
		{"blockVersionData.maxTxSize", genesis.BlockVersionData.MaxTxSize},
		{"blockVersionData.maxProposalSize", genesis.BlockVersionData.MaxProposalSize},
		{"blockVersionData.updateImplicit", genesis.BlockVersionData.UpdateImplicit},
	}
	for _, parameter := range unsigned {
		if parameter.value < 0 {
			return fmt.Errorf("%s must be non-negative, got %d", parameter.name, parameter.value)
		}
	}
	return nil
}

func NewByronGenesisFromFile(path string) (ByronGenesis, error) {
	f, err := os.Open(path)
	if err != nil {
		return ByronGenesis{}, err
	}
	defer f.Close()
	return NewByronGenesisFromReader(f)
}

// UnmarshalJSON accepts: "string", {}, or null
// Tries each expected shape and accepts the first that parses cleanly
func (f *ByronGenesisFtsSeed) UnmarshalJSON(b []byte) error {
	// Try string
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		f.Value = s
		f.IsObject = false
		return nil
	}

	// Try empty object
	var m map[string]any
	if err := json.Unmarshal(b, &m); err == nil {
		if len(m) == 0 {
			f.Value = ""
			f.IsObject = true
			return nil
		}
		return errors.New("ftsSeed: non-empty object not supported")
	}

	// Try null
	var v any
	if err := json.Unmarshal(b, &v); err == nil && v == nil {
		f.Value = ""
		f.IsObject = false
		return nil
	}

	return errors.New("ftsSeed: expected string, empty object, or null")
}

func (f ByronGenesisFtsSeed) MarshalJSON() ([]byte, error) {
	if f.IsObject {
		// serialize as empty object
		return []byte(`{}`), nil
	}
	if f.Value == "" {
		// serialize as null
		return []byte(`null`), nil
	}
	return json.Marshal(f.Value)
}

// GenesisDelegateKeyHashes returns the sorted Blake2b-224 boot stakeholder
// hashes. Heavy delegation certificates may change a stakeholder's active
// signing key, but do not define the genesis issuer set.
func (g *ByronGenesis) GenesisDelegateKeyHashes() ([]common.Blake2b224, error) {
	if len(g.BootStakeholders) == 0 {
		return nil, nil
	}

	// Sort the hex keys to make the issuer order deterministic.
	hexKeys := make([]string, 0, len(g.BootStakeholders))
	for keyHex := range g.BootStakeholders {
		hexKeys = append(hexKeys, keyHex)
	}

	// Sort by hex representation for deterministic ordering
	slices.Sort(hexKeys)

	// Parse each hex key into a Blake2b224
	result := make([]common.Blake2b224, len(hexKeys))
	for i, keyHex := range hexKeys {
		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid hex key in BootStakeholders: %s: %w",
				keyHex,
				err,
			)
		}
		if len(keyBytes) != common.Blake2b224Size {
			return nil, fmt.Errorf(
				"invalid key hash length in BootStakeholders: expected %d bytes, "+
					"got %d for key %s",
				common.Blake2b224Size,
				len(keyBytes),
				keyHex,
			)
		}
		result[i] = common.NewBlake2b224(keyBytes)
	}

	return result, nil
}
