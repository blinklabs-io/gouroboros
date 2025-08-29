// Copyright 2025 Blink Labs Software
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

package script

import (
	"bytes"
	"math/big"
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

type ScriptInfo interface {
	isScriptInfo()
	ScriptHash() lcommon.ScriptHash
	ToPlutusData
}

type ScriptInfoMinting struct {
	PolicyId lcommon.Blake2b224
}

func (ScriptInfoMinting) isScriptInfo() {}

func (s ScriptInfoMinting) ScriptHash() lcommon.ScriptHash {
	return s.PolicyId
}

func (s ScriptInfoMinting) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		data.NewByteString(s.PolicyId.Bytes()),
	)
}

type ScriptInfoSpending struct {
	Input lcommon.Utxo
	Datum data.PlutusData
}

func (ScriptInfoSpending) isScriptInfo() {}

func (s ScriptInfoSpending) ScriptHash() lcommon.ScriptHash {
	tmpAddr := s.Input.Output.Address()
	return tmpAddr.PaymentKeyHash()
}

func (s ScriptInfoSpending) ToPlutusData() data.PlutusData {
	if s.Datum == nil {
		return data.NewConstr(
			1,
			s.Input.Id.ToPlutusData(),
		)
	}
	return data.NewConstr(
		1,
		s.Input.Id.ToPlutusData(),
		data.NewConstr(
			0,
			s.Datum,
		),
	)
}

type ScriptInfoRewarding struct {
	StakeCredential lcommon.Credential
}

func (ScriptInfoRewarding) isScriptInfo() {}

func (s ScriptInfoRewarding) ScriptHash() lcommon.ScriptHash {
	return lcommon.ScriptHash(s.StakeCredential.Credential)
}

func (s ScriptInfoRewarding) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		2,
		s.StakeCredential.ToPlutusData(),
	)
}

type ScriptInfoCertifying struct {
	Index       uint32
	Certificate lcommon.Certificate
}

func (ScriptInfoCertifying) isScriptInfo() {}

func (s ScriptInfoCertifying) ScriptHash() lcommon.ScriptHash {
	var cred *lcommon.Credential
	switch c := s.Certificate.(type) {
	case *lcommon.StakeDeregistrationCertificate:
		cred = &c.StakeCredential
	case *lcommon.RegistrationCertificate:
		cred = &c.StakeCredential
	case *lcommon.DeregistrationCertificate:
		cred = &c.StakeCredential
	case *lcommon.VoteDelegationCertificate:
		cred = &c.StakeCredential
	case *lcommon.VoteRegistrationDelegationCertificate:
		cred = &c.StakeCredential
	case *lcommon.StakeVoteDelegationCertificate:
		cred = &c.StakeCredential
	case *lcommon.StakeRegistrationDelegationCertificate:
		cred = &c.StakeCredential
	case *lcommon.StakeVoteRegistrationDelegationCertificate:
		cred = &c.StakeCredential
	case *lcommon.RegistrationDrepCertificate:
		cred = &c.DrepCredential
	case *lcommon.DeregistrationDrepCertificate:
		cred = &c.DrepCredential
	case *lcommon.UpdateDrepCertificate:
		cred = &c.DrepCredential
	case *lcommon.AuthCommitteeHotCertificate:
		cred = &c.ColdCredential
	case *lcommon.ResignCommitteeColdCertificate:
		cred = &c.ColdCredential
	case *lcommon.StakeDelegationCertificate:
		cred = c.StakeCredential
	}
	if cred != nil {
		if cred.CredType == lcommon.CredentialTypeScriptHash {
			return cred.Credential
		}
	}
	return lcommon.ScriptHash{}
}

func (s ScriptInfoCertifying) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		3,
		data.NewInteger(new(big.Int).SetUint64(uint64(s.Index))),
		certificateToPlutusData(s.Certificate),
	)
}

type ScriptInfoVoting struct {
	Voter lcommon.Voter
}

func (ScriptInfoVoting) isScriptInfo() {}

func (s ScriptInfoVoting) ScriptHash() lcommon.ScriptHash {
	// TODO
	return lcommon.ScriptHash{}
}

func (s ScriptInfoVoting) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type ScriptInfoProposing struct {
	Size              uint64
	ProposalProcedure lcommon.ProposalProcedure
}

func (ScriptInfoProposing) isScriptInfo() {}

func (s ScriptInfoProposing) ScriptHash() lcommon.ScriptHash {
	// TODO
	return lcommon.ScriptHash{}
}

func (s ScriptInfoProposing) ToPlutusData() data.PlutusData {
	// TODO
	return nil
}

type toScriptPurposeFunc func(lcommon.RedeemerKey) ScriptInfo

// scriptPurposeBuilder creates a reusable function preloaded with information about a particular transaction
func scriptPurposeBuilder(
	resolvedInputs []lcommon.Utxo,
	inputs []lcommon.TransactionInput,
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
	certificates []lcommon.Certificate,
	withdrawals KeyValuePairs[*lcommon.Address, uint64],
	// TODO: proposal procedures
	// TODO: votes
) toScriptPurposeFunc {
	return func(redeemerKey lcommon.RedeemerKey) ScriptInfo {
		// TODO: implement additional redeemer tags
		// https://github.com/aiken-lang/aiken/blob/af4e04b91e54dbba3340de03fc9e65a90f24a93b/crates/uplc/src/tx/script_context.rs#L771-L826
		switch redeemerKey.Tag {
		case lcommon.RedeemerTagSpend:
			var datum data.PlutusData
			tmpInput := inputs[redeemerKey.Index]
			var resolvedInput lcommon.Utxo
			for _, tmpResolvedInput := range resolvedInputs {
				if tmpResolvedInput.Id.String() == tmpInput.String() {
					resolvedInput = tmpResolvedInput
					if tmpDatum := resolvedInput.Output.Datum(); tmpDatum != nil {
						datum = tmpDatum.Data
					}
					break
				}
			}
			return ScriptInfoSpending{
				Input: resolvedInput,
				Datum: datum,
			}
		case lcommon.RedeemerTagMint:
			// TODO: fix this to work for more than one minted policy
			mintPolicies := mint.Policies()
			slices.SortFunc(
				mintPolicies,
				func(a, b lcommon.Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
			)
			return ScriptInfoMinting{
				PolicyId: mintPolicies[redeemerKey.Index],
			}
		case lcommon.RedeemerTagCert:
			return ScriptInfoCertifying{
				Index:       redeemerKey.Index,
				Certificate: certificates[redeemerKey.Index],
			}
		case lcommon.RedeemerTagReward:
			return ScriptInfoRewarding{
				StakeCredential: lcommon.Credential{
					CredType:   lcommon.CredentialTypeScriptHash,
					Credential: withdrawals[redeemerKey.Index].Key.StakeKeyHash(),
				},
			}
		case lcommon.RedeemerTagVoting:
			return nil
		case lcommon.RedeemerTagProposing:
			return nil
		}
		return nil
	}
}

func scriptPurposeStripDatum(purpose ScriptInfo) ScriptInfo {
	switch p := purpose.(type) {
	case ScriptInfoSpending:
		p.Datum = nil
		return p
	}
	return purpose
}
