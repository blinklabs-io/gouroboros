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

type ScriptPurpose interface {
	isScriptPurpose()
	ScriptHash() lcommon.ScriptHash
	ToScriptInfo() ScriptInfo
	ToPlutusData
}

type ScriptInfo interface {
	isScriptInfo()
	ScriptHash() lcommon.ScriptHash
	ToPlutusData
}

type ScriptPurposeMinting struct {
	PolicyId lcommon.Blake2b224
}

func (ScriptPurposeMinting) isScriptPurpose() {}

func (s ScriptPurposeMinting) ScriptHash() lcommon.ScriptHash {
	return lcommon.ScriptHash(s.PolicyId)
}

func (s ScriptPurposeMinting) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		data.NewByteString(s.PolicyId.Bytes()),
	)
}

func (s ScriptPurposeMinting) ToScriptInfo() ScriptInfo {
	return ScriptInfoMinting{s}
}

type ScriptPurposeSpending struct {
	Input lcommon.Utxo
	Datum data.PlutusData
}

func (ScriptPurposeSpending) isScriptPurpose() {}

func (s ScriptPurposeSpending) ScriptHash() lcommon.ScriptHash {
	tmpAddr := s.Input.Output.Address()
	return lcommon.ScriptHash(tmpAddr.PaymentKeyHash())
}

func (s ScriptPurposeSpending) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		1,
		s.Input.Id.ToPlutusData(),
	)
}

func (s ScriptPurposeSpending) ToScriptInfo() ScriptInfo {
	return ScriptInfoSpending{s}
}

type ScriptPurposeRewarding struct {
	StakeCredential lcommon.Credential
}

func (ScriptPurposeRewarding) isScriptPurpose() {}

func (s ScriptPurposeRewarding) ScriptHash() lcommon.ScriptHash {
	return lcommon.ScriptHash(s.StakeCredential.Credential)
}

func (s ScriptPurposeRewarding) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		2,
		s.StakeCredential.ToPlutusData(),
	)
}

func (s ScriptPurposeRewarding) ToScriptInfo() ScriptInfo {
	return ScriptInfoRewarding{s}
}

type ScriptPurposeCertifying struct {
	Index       uint32
	Certificate lcommon.Certificate
}

func (ScriptPurposeCertifying) isScriptPurpose() {}

func (s ScriptPurposeCertifying) ScriptHash() lcommon.ScriptHash {
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
			return lcommon.ScriptHash(cred.Credential)
		}
	}
	return lcommon.ScriptHash{}
}

func (s ScriptPurposeCertifying) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		3,
		data.NewInteger(new(big.Int).SetUint64(uint64(s.Index))),
		certificateToPlutusData(s.Certificate),
	)
}

func (s ScriptPurposeCertifying) ToScriptInfo() ScriptInfo {
	return ScriptInfoCertifying{s}
}

type ScriptPurposeVoting struct {
	Voter lcommon.Voter
}

func (ScriptPurposeVoting) isScriptPurpose() {}

func (s ScriptPurposeVoting) ScriptHash() lcommon.ScriptHash {
	return lcommon.ScriptHash(lcommon.NewBlake2b224(s.Voter.Hash[:]))
}

func (s ScriptPurposeVoting) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		4,
		s.Voter.ToPlutusData(),
	)
}

func (s ScriptPurposeVoting) ToScriptInfo() ScriptInfo {
	return ScriptInfoVoting{s}
}

type ScriptPurposeProposing struct {
	Index             uint32
	ProposalProcedure lcommon.ProposalProcedure
}

func (ScriptPurposeProposing) isScriptPurpose() {}

func (s ScriptPurposeProposing) ScriptHash() lcommon.ScriptHash {
	// Use GovActionWithPolicy interface to get policy hash without importing conway
	if ga, ok := s.ProposalProcedure.GovAction().(lcommon.GovActionWithPolicy); ok {
		policyBytes := ga.GetPolicyHash()
		if len(policyBytes) == 28 {
			return lcommon.ScriptHash(lcommon.NewBlake2b224(policyBytes))
		}
	}
	return lcommon.ScriptHash{}
}

func (s ScriptPurposeProposing) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		5,
		toPlutusData(uint64(s.Index)),
		s.ProposalProcedure.ToPlutusData(),
	)
}

func (s ScriptPurposeProposing) ToScriptInfo() ScriptInfo {
	return ScriptInfoProposing{s}
}

type ScriptInfoSpending struct {
	ScriptPurposeSpending
}

func (ScriptInfoSpending) isScriptInfo() {}

func (s ScriptInfoSpending) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		1,
		s.Input.Id.ToPlutusData(),
		Option[data.PlutusData]{s.Datum}.ToPlutusData(),
	)
}

type ScriptInfoMinting struct {
	ScriptPurposeMinting
}

func (ScriptInfoMinting) isScriptInfo() {}

type ScriptInfoRewarding struct {
	ScriptPurposeRewarding
}

func (ScriptInfoRewarding) isScriptInfo() {}

type ScriptInfoCertifying struct {
	ScriptPurposeCertifying
}

func (ScriptInfoCertifying) isScriptInfo() {}

type ScriptInfoVoting struct {
	ScriptPurposeVoting
}

func (ScriptInfoVoting) isScriptInfo() {}

type ScriptInfoProposing struct {
	ScriptPurposeProposing
}

func (ScriptInfoProposing) isScriptInfo() {}

type toScriptPurposeFunc func(lcommon.RedeemerKey) ScriptPurpose

// scriptPurposeBuilder creates a reusable function preloaded with information about a particular transaction
func scriptPurposeBuilder(
	resolvedInputs []lcommon.Utxo,
	inputs []lcommon.TransactionInput,
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
	certificates []lcommon.Certificate,
	withdrawals KeyValuePairs[*lcommon.Address, *big.Int],
	votes KeyValuePairs[*lcommon.Voter, KeyValuePairs[*lcommon.GovActionId, lcommon.VotingProcedure]],
	proposalProcedures []lcommon.ProposalProcedure,
) toScriptPurposeFunc {
	return func(redeemerKey lcommon.RedeemerKey) ScriptPurpose {
		// TODO: implement additional redeemer tags
		// https://github.com/aiken-lang/aiken/blob/af4e04b91e54dbba3340de03fc9e65a90f24a93b/crates/uplc/src/tx/script_context.rs#L771-L826
		switch redeemerKey.Tag {
		case lcommon.RedeemerTagSpend:
			if int(redeemerKey.Index) >= len(inputs) {
				return nil
			}
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
			return ScriptPurposeSpending{
				Input: resolvedInput,
				Datum: datum,
			}
		case lcommon.RedeemerTagMint:
			mintPolicies := mint.Policies()
			if int(redeemerKey.Index) >= len(mintPolicies) {
				return nil
			}
			slices.SortFunc(
				mintPolicies,
				func(a, b lcommon.Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
			)
			return ScriptPurposeMinting{
				PolicyId: mintPolicies[redeemerKey.Index],
			}
		case lcommon.RedeemerTagCert:
			if int(redeemerKey.Index) >= len(certificates) {
				return nil
			}
			return ScriptPurposeCertifying{
				Index:       redeemerKey.Index,
				Certificate: certificates[redeemerKey.Index],
			}
		case lcommon.RedeemerTagReward:
			if int(redeemerKey.Index) >= len(withdrawals) {
				return nil
			}
			return ScriptPurposeRewarding{
				StakeCredential: lcommon.Credential{
					CredType:   lcommon.CredentialTypeScriptHash,
					Credential: withdrawals[redeemerKey.Index].Key.StakeKeyHash(),
				},
			}
		case lcommon.RedeemerTagVoting:
			if int(redeemerKey.Index) >= len(votes) {
				return nil
			}
			return ScriptPurposeVoting{
				Voter: *(votes[redeemerKey.Index].Key),
			}
		case lcommon.RedeemerTagProposing:
			if int(redeemerKey.Index) >= len(proposalProcedures) {
				return nil
			}
			return ScriptPurposeProposing{
				Index:             redeemerKey.Index,
				ProposalProcedure: proposalProcedures[redeemerKey.Index],
			}
		}
		return nil
	}
}

// BuildScriptPurpose creates a ScriptPurpose from a redeemer key using map-based inputs.
// This variant accepts raw maps for withdrawals and votes and handles deterministic ordering internally.
// The witnessDatums parameter allows looking up datums from the transaction witness set
// for outputs that have a datum hash but no inline datum.
func BuildScriptPurpose(
	redeemerKey lcommon.RedeemerKey,
	resolvedInputs map[string]lcommon.Utxo,
	inputs []lcommon.TransactionInput,
	mint lcommon.MultiAsset[lcommon.MultiAssetTypeMint],
	certificates []lcommon.Certificate,
	withdrawals map[*lcommon.Address]*big.Int,
	votes lcommon.VotingProcedures,
	proposalProcedures []lcommon.ProposalProcedure,
	witnessDatums map[lcommon.Blake2b256]*lcommon.Datum,
) ScriptPurpose {
	switch redeemerKey.Tag {
	case lcommon.RedeemerTagSpend:
		if int(redeemerKey.Index) >= len(inputs) {
			return nil
		}
		tmpInput := inputs[redeemerKey.Index]
		utxo, ok := resolvedInputs[tmpInput.String()]
		if !ok {
			return nil
		}
		var datum data.PlutusData
		if utxo.Output != nil {
			if d := utxo.Output.Datum(); d != nil {
				// Inline datum - use it directly
				datum = d.Data
			} else if datumHash := utxo.Output.DatumHash(); datumHash != nil {
				// No inline datum - check witness datums by hash
				if witnessDatum, exists := witnessDatums[*datumHash]; exists && witnessDatum != nil {
					datum = witnessDatum.Data
				}
			}
		}
		return ScriptPurposeSpending{
			Input: utxo,
			Datum: datum,
		}
	case lcommon.RedeemerTagMint:
		policies := mint.Policies()
		if int(redeemerKey.Index) >= len(policies) {
			return nil
		}
		slices.SortFunc(
			policies,
			func(a, b lcommon.Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
		)
		return ScriptPurposeMinting{
			PolicyId: policies[redeemerKey.Index],
		}
	case lcommon.RedeemerTagCert:
		if int(redeemerKey.Index) >= len(certificates) {
			return nil
		}
		return ScriptPurposeCertifying{
			Index:       redeemerKey.Index,
			Certificate: certificates[redeemerKey.Index],
		}
	case lcommon.RedeemerTagReward:
		// Extract and sort withdrawal addresses for deterministic ordering
		sortedAddrs := make([]*lcommon.Address, 0, len(withdrawals))
		for addr := range withdrawals {
			sortedAddrs = append(sortedAddrs, addr)
		}
		slices.SortFunc(sortedAddrs, func(a, b *lcommon.Address) int {
			if a.String() < b.String() {
				return -1
			}
			if a.String() > b.String() {
				return 1
			}
			return 0
		})
		if int(redeemerKey.Index) >= len(sortedAddrs) {
			return nil
		}
		addr := sortedAddrs[redeemerKey.Index]
		return ScriptPurposeRewarding{
			StakeCredential: lcommon.Credential{
				CredType:   lcommon.CredentialTypeScriptHash,
				Credential: addr.StakeKeyHash(),
			},
		}
	case lcommon.RedeemerTagVoting:
		// Extract and sort voters for deterministic ordering
		sortedVoters := make([]*lcommon.Voter, 0, len(votes))
		for voter := range votes {
			sortedVoters = append(sortedVoters, voter)
		}
		slices.SortFunc(sortedVoters, func(a, b *lcommon.Voter) int {
			if a.String() < b.String() {
				return -1
			}
			if a.String() > b.String() {
				return 1
			}
			return 0
		})
		if int(redeemerKey.Index) >= len(sortedVoters) {
			return nil
		}
		return ScriptPurposeVoting{
			Voter: *sortedVoters[redeemerKey.Index],
		}
	case lcommon.RedeemerTagProposing:
		if int(redeemerKey.Index) >= len(proposalProcedures) {
			return nil
		}
		return ScriptPurposeProposing{
			Index:             redeemerKey.Index,
			ProposalProcedure: proposalProcedures[redeemerKey.Index],
		}
	}
	return nil
}
