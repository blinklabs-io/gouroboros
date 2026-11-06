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

package conway_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

// conwayDecodeWith decodes the mainnet fixture after setting the dotted path
// to value. A nil value deletes the key.
func conwayDecodeWith(
	t *testing.T,
	path string,
	value any,
) (conway.ConwayGenesis, error) {
	t.Helper()
	doc := loadConwayDoc(t, "testdata/mainnet-conway-genesis.json")
	parts := strings.Split(path, ".")
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		require.True(t, ok, "path element %q is not an object", part)
		cur = next
	}
	last := parts[len(parts)-1]
	if value == nil {
		delete(cur, last)
	} else {
		cur[last] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return conway.NewConwayGenesisFromReader(bytes.NewReader(data))
}

func firstMember(t *testing.T) string {
	t.Helper()
	doc := loadConwayDoc(t, "testdata/mainnet-conway-genesis.json")
	for key := range doc["committee"].(map[string]any)["members"].(map[string]any) {
		return key
	}
	t.Fatal("fixture has no committee member")
	return ""
}

func TestConwayGenesisIntegralNumberFormsDecode(t *testing.T) {
	t.Parallel()
	g, err := conwayDecodeWith(t, "committeeMinSize", json.Number("3.0"))
	require.NoError(t, err)
	require.Equal(t, uint(3), g.MinCommitteeSize)
	g, err = conwayDecodeWith(t, "govActionDeposit", json.Number("1e11"))
	require.NoError(t, err)
	require.Equal(t, uint64(100000000000), g.GovActionDeposit)
	g, err = conwayDecodeWith(t, "dRepActivity", json.Number("2.0e1"))
	require.NoError(t, err)
	require.Equal(t, uint64(20), g.DRepInactivityPeriod)
	member := firstMember(t)
	g, err = conwayDecodeWith(
		t,
		"committee.members."+member,
		json.Number("5.8e2"),
	)
	require.NoError(t, err)
	require.Equal(t, 580, g.Committee.Members[member])
	g, err = conwayDecodeWith(t, "committee.threshold",
		json.RawMessage(`{"numerator":2.0,"denominator":3e0}`))
	require.NoError(t, err)
	require.Equal(t, "2/3", g.Committee.Threshold.String())
	g, err = conwayDecodeWith(
		t,
		"dRepVotingThresholds.ppGovGroup",
		json.Number("6.7e-1"),
	)
	require.NoError(t, err)
	require.Equal(t, "67/100", g.DRepVotingThresholds.PpGovGroup.String())
	g, err = conwayDecodeWith(
		t,
		"minFeeRefScriptCostPerByte",
		json.Number("1.5e1"),
	)
	require.NoError(t, err)
	require.Equal(t, "15/1", g.MinFeeRefScriptCostPerByte.String())
}

func TestConwayGenesisRatioObjectCaseVariantCannotOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, value string }{
		{
			"committee.threshold",
			`{"numerator":0,"denominator":1,"NUMERATOR":2}`,
		},
		{
			"poolVotingThresholds.committeeNormal",
			`{"numerator":1,"Numerator":-1,"denominator":2}`,
		},
		{
			"dRepVotingThresholds.treasuryWithdrawal",
			`{"numerator":1,"denominator":2,"denominator ":0,"Denominator":0}`,
		},
		{
			"minFeeRefScriptCostPerByte",
			`{"numerator":1,"denominator":2,"Denominator":0}`,
		},
	} {
		_, err := conwayDecodeWith(t, tc.path, json.RawMessage(tc.value))
		require.ErrorContains(t, err, "differs only in case", tc.path)
		require.ErrorContains(t, err, tc.path)
	}
}

func TestConwayGenesisEveryValidatedObjectRejectsCaseVariants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, variant string }{
		{"", "CommitteeMinSize"},
		{"poolVotingThresholds", "CommitteeNormal"},
		{"dRepVotingThresholds", "PPGOVGROUP"},
		{"constitution", "Anchor"},
		{"constitution.anchor", "URL"},
		{"committee", "Members"},
	} {
		t.Run(tc.path+"+"+tc.variant, func(t *testing.T) {
			t.Parallel()
			doc := loadConwayDoc(t, "testdata/mainnet-conway-genesis.json")
			cur := doc
			if tc.path != "" {
				for _, part := range strings.Split(tc.path, ".") {
					cur = cur[part].(map[string]any)
				}
			}
			cur[tc.variant] = json.Number("0")
			data, err := json.Marshal(doc)
			require.NoError(t, err)
			_, err = conway.NewConwayGenesisFromReader(bytes.NewReader(data))
			require.ErrorContains(t, err, "differs only in case")
		})
	}
}

// committee.members is held in a Go int, which cannot hold the upper half of
// the Word64 the reference accepts; validation refuses it with the field named.
func TestConwayGenesisMemberExpiryBeyondGoIntIsValidationError(t *testing.T) {
	t.Parallel()
	maxInt := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize-1)),
		big.NewInt(1),
	)
	member := firstMember(t)
	path := "committee.members." + member
	g, err := conwayDecodeWith(t, path, json.Number(maxInt.String()))
	require.NoError(t, err)
	require.Equal(t, int(maxInt.Int64()), g.Committee.Members[member])
	_, err = conwayDecodeWith(t, path,
		json.Number(new(big.Int).Add(maxInt, big.NewInt(1)).String()))
	require.ErrorContains(t, err, "invalid Conway genesis")
	require.ErrorContains(t, err, path)
}

func TestConwayGenesisValidatedValuesAlwaysDecode(t *testing.T) {
	t.Parallel()
	member := "committee.members." + firstMember(t)
	paths := []string{
		"committeeMinSize", "committeeMaxTermLength", "govActionLifetime",
		"govActionDeposit", "dRepDeposit", "dRepActivity",
		"minFeeRefScriptCostPerByte", "committee.threshold", member,
		"poolVotingThresholds.motionNoConfidence",
		"dRepVotingThresholds.ppGovGroup",
	}
	literals := []string{
		"0", "-0", "1", "1.0", "1e3", "0.5", "5e-1",
		"9223372036854775807", "9223372036854775808",
		"18446744073709551615", "18446744073709551616",
		"4294967295", "4294967296", "65536",
		"0e999999999", "1e-1000000000", "1e5000",
		`{"numerator":1.0,"denominator":2}`,
		`{"numerator":9223372036854775808,"denominator":18446744073709551615}`,
		`{"numerator":-1,"denominator":-2}`,
		`null`, `"1"`, `1.5`, `[]`, `{}`,
	}
	for _, path := range paths {
		for _, lit := range literals {
			t.Run(path+"="+lit, func(t *testing.T) {
				t.Parallel()
				_, err := conwayDecodeWith(t, path, json.RawMessage(lit))
				if err != nil {
					require.ErrorContains(t, err, "invalid Conway genesis",
						"decode failed after validation passed")
				}
			})
		}
	}
}
