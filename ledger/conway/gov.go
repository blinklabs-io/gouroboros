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

package conway

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// ConwayVotingProcedure is a Conway voting procedure with its era-specific
// definite-array encoding requirement.
type ConwayVotingProcedure common.VotingProcedure

func (vp *ConwayVotingProcedure) UnmarshalCBOR(cborData []byte) error {
	if err := common.ValidateDefiniteCBORArrayLength(
		cborData,
		2,
		"Conway voting procedure",
	); err != nil {
		return err
	}
	var decoded common.VotingProcedure
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return err
	}
	*vp = ConwayVotingProcedure(decoded)
	return nil
}

func (vp ConwayVotingProcedure) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(common.VotingProcedure(vp))
}

type ConwayProposalProcedure struct {
	common.ProposalProcedureBase
	cbor.StructAsArray
	PPDeposit       uint64
	PPRewardAccount common.Address
	PPGovAction     ConwayGovAction
	PPAnchor        common.GovAnchor
}

func (p *ConwayProposalProcedure) UnmarshalCBOR(cborData []byte) error {
	if err := common.ValidateDefiniteCBORArrayLength(
		cborData,
		4,
		"Conway proposal procedure",
	); err != nil {
		return err
	}
	type tConwayProposalProcedure ConwayProposalProcedure
	var tmp tConwayProposalProcedure
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	if err := common.CheckAccountAddress(tmp.PPRewardAccount); err != nil {
		return err
	}
	*p = ConwayProposalProcedure(tmp)
	return nil
}

func (p ConwayProposalProcedure) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewInteger(new(big.Int).SetUint64(p.PPDeposit)),
		p.PPRewardAccount.ToPlutusData(),
		p.PPGovAction.ToPlutusData(),
	)
}

func (p ConwayProposalProcedure) Deposit() uint64 {
	return p.PPDeposit
}

func (p ConwayProposalProcedure) RewardAccount() common.Address {
	return p.PPRewardAccount
}

func (p ConwayProposalProcedure) GovAction() common.GovAction {
	return p.PPGovAction.Action
}

func (p ConwayProposalProcedure) Anchor() common.GovAnchor {
	return p.PPAnchor
}

type ConwayGovAction struct {
	Type   uint
	Action common.GovAction
}

func (g ConwayGovAction) ToPlutusData() data.PlutusData {
	return g.Action.ToPlutusData()
}

func (g *ConwayGovAction) UnmarshalCBOR(data []byte) error {
	// Determine action type
	actionType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	if actionType < 0 {
		return fmt.Errorf("invalid governance action type: %d", actionType)
	}
	arrayLength := 0
	optionalFieldIndexes := []int{}
	var tmpAction common.GovAction
	switch common.GovActionType(actionType) {
	case common.GovActionTypeParameterChange:
		arrayLength = 4
		optionalFieldIndexes = []int{1, 3}
		tmpAction = &ConwayParameterChangeGovAction{}
	case common.GovActionTypeHardForkInitiation:
		arrayLength = 3
		optionalFieldIndexes = []int{1}
		tmpAction = &common.HardForkInitiationGovAction{}
	case common.GovActionTypeTreasuryWithdrawal:
		arrayLength = 3
		optionalFieldIndexes = []int{2}
		tmpAction = &common.TreasuryWithdrawalGovAction{}
	case common.GovActionTypeNoConfidence:
		arrayLength = 2
		optionalFieldIndexes = []int{1}
		tmpAction = &common.NoConfidenceGovAction{}
	case common.GovActionTypeUpdateCommittee:
		arrayLength = 5
		optionalFieldIndexes = []int{1}
		tmpAction = &common.UpdateCommitteeGovAction{}
	case common.GovActionTypeNewConstitution:
		arrayLength = 3
		optionalFieldIndexes = []int{1}
		tmpAction = &common.NewConstitutionGovAction{}
	case common.GovActionTypeInfo:
		arrayLength = 1
		tmpAction = &common.InfoGovAction{}
	default:
		return fmt.Errorf("unknown governance action type: %d", actionType)
	}
	if err := common.ValidateCBORArrayLength(
		data,
		arrayLength,
		"Conway governance action",
	); err != nil {
		return err
	}
	fields := make([]cbor.RawMessage, arrayLength)
	if _, err := cbor.Decode(data, &fields); err != nil {
		return err
	}
	for _, fieldIndex := range optionalFieldIndexes {
		if len(fields[fieldIndex]) == 1 && fields[fieldIndex][0] == 0xf7 {
			return fmt.Errorf(
				"conway governance action optional field %d cannot be CBOR undefined",
				fieldIndex,
			)
		}
	}
	// Decode action
	if _, err := cbor.Decode(data, tmpAction); err != nil {
		return err
	}
	if hardFork, ok := tmpAction.(*common.HardForkInitiationGovAction); ok &&
		hardFork.ProtocolVersion.Major > 12 {
		return fmt.Errorf(
			"hard fork protocol version major %d exceeds Conway maximum 12",
			hardFork.ProtocolVersion.Major,
		)
	}
	// action type is known within uint range
	g.Type = uint(actionType) // #nosec G115
	g.Action = tmpAction
	return nil
}

func (a *ConwayParameterChangeGovAction) UnmarshalCBOR(cborData []byte) error {
	if err := common.ValidateCBORArrayLength(
		cborData,
		4,
		"parameter change governance action",
	); err != nil {
		return err
	}
	fields := make([]cbor.RawMessage, 4)
	if _, err := cbor.Decode(cborData, &fields); err != nil {
		return err
	}
	if len(fields[1]) == 1 && fields[1][0] == 0xf7 {
		return errors.New("parameter change previous action ID cannot be CBOR undefined")
	}
	if err := common.ValidateNullOrFixedLengthByteStringCBOR(
		fields[3],
		common.Blake2b224Size,
		"parameter change policy hash",
	); err != nil {
		return err
	}
	type tConwayParameterChangeGovAction ConwayParameterChangeGovAction
	var decoded tConwayParameterChangeGovAction
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return err
	}
	*a = ConwayParameterChangeGovAction(decoded)
	return nil
}

func (g *ConwayGovAction) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(g.Action)
}

type ConwayParameterChangeGovAction struct {
	common.GovActionBase
	cbor.StructAsArray
	Type        uint
	ActionId    *common.GovActionId
	ParamUpdate ConwayProtocolParameterUpdate
	PolicyHash  []byte
}

var _ common.ParameterChangeGovAction = (*ConwayParameterChangeGovAction)(nil)

func (a *ConwayParameterChangeGovAction) ToPlutusData() data.PlutusData {
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	policyHash := data.NewConstr(1)
	if a.PolicyHash != nil {
		policyHash = data.NewConstr(
			0,
			data.NewByteString(a.PolicyHash),
		)
	}
	return data.NewConstr(0,
		actionId,
		a.ParamUpdate.ToPlutusData(),
		policyHash,
	)
}

// GetPolicyHash returns the policy script hash for this governance action
func (a *ConwayParameterChangeGovAction) GetPolicyHash() []byte {
	if a == nil {
		return nil
	}
	return a.PolicyHash
}

// PreviousGovActionId returns the parameter-change action this action follows.
func (a *ConwayParameterChangeGovAction) PreviousGovActionId() *common.GovActionId {
	if a == nil {
		return nil
	}
	return a.ActionId
}

// ProtocolParamUpdateUtxorpc converts the proposed parameter update.
func (a *ConwayParameterChangeGovAction) ProtocolParamUpdateUtxorpc() (*utxorpc.PParams, error) {
	if a == nil {
		return nil, nil
	}
	return a.ParamUpdate.Utxorpc()
}

// SecurityGroupFields returns the security-group parameters changed by this
// action.
func (a *ConwayParameterChangeGovAction) SecurityGroupFields() []string {
	if a == nil {
		return nil
	}
	return a.ParamUpdate.SecurityGroupFields()
}

// NewConwayParameterChangeGovAction builds a parameter change governance
// action. actionId is optional (nil means no parent action). policyHash is
// optional, but when provided must be a 28-byte script hash.
func NewConwayParameterChangeGovAction(
	actionId *common.GovActionId,
	paramUpdate ConwayProtocolParameterUpdate,
	policyHash []byte,
) (*ConwayParameterChangeGovAction, error) {
	if policyHash != nil && len(policyHash) != common.Blake2b224Size {
		return nil, fmt.Errorf(
			"invalid policy hash length: expected %d bytes, got %d",
			common.Blake2b224Size,
			len(policyHash),
		)
	}
	return &ConwayParameterChangeGovAction{
		Type:        uint(common.GovActionTypeParameterChange),
		ActionId:    actionId,
		ParamUpdate: paramUpdate,
		PolicyHash:  policyHash,
	}, nil
}

// NewConwayGovAction wraps a governance action and sets the (non-serialized)
// discriminant Type for in-memory consistency. A nil or unsupported action
// returns an error.
func NewConwayGovAction(action common.GovAction) (ConwayGovAction, error) {
	actionType, err := conwayGovActionType(action)
	if err != nil {
		return ConwayGovAction{}, err
	}
	return ConwayGovAction{
		Type:   actionType,
		Action: action,
	}, nil
}

// conwayGovActionType maps a concrete governance action to its discriminant.
// The returned value is only used for the in-memory ConwayGovAction.Type field,
// which is not serialized. An unknown or nil action returns an error rather than
// silently defaulting to discriminant 0, which would be indistinguishable from a
// valid parameter change action.
func conwayGovActionType(action common.GovAction) (uint, error) {
	if isNilGovAction(action) {
		return 0, errors.New("governance action cannot be nil")
	}
	switch action.(type) {
	case *ConwayParameterChangeGovAction:
		return uint(common.GovActionTypeParameterChange), nil
	case *common.HardForkInitiationGovAction:
		return uint(common.GovActionTypeHardForkInitiation), nil
	case *common.TreasuryWithdrawalGovAction:
		return uint(common.GovActionTypeTreasuryWithdrawal), nil
	case *common.NoConfidenceGovAction:
		return uint(common.GovActionTypeNoConfidence), nil
	case *common.UpdateCommitteeGovAction:
		return uint(common.GovActionTypeUpdateCommittee), nil
	case *common.NewConstitutionGovAction:
		return uint(common.GovActionTypeNewConstitution), nil
	case *common.InfoGovAction:
		return uint(common.GovActionTypeInfo), nil
	default:
		return 0, fmt.Errorf("unsupported governance action type: %T", action)
	}
}

// isNilGovAction reports whether action is nil, including an interface that
// contains a typed-nil pointer.
func isNilGovAction(action common.GovAction) bool {
	if action == nil {
		return true
	}
	rv := reflect.ValueOf(action)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// NewConwayProposalProcedure builds a Conway proposal procedure from a deposit,
// reward account, governance action, and anchor.
func NewConwayProposalProcedure(
	deposit uint64,
	rewardAccount common.Address,
	action common.GovAction,
	anchor common.GovAnchor,
) (*ConwayProposalProcedure, error) {
	govAction, err := NewConwayGovAction(action)
	if err != nil {
		return nil, err
	}
	return &ConwayProposalProcedure{
		PPDeposit:       deposit,
		PPRewardAccount: rewardAccount,
		PPGovAction:     govAction,
		PPAnchor:        anchor,
	}, nil
}
