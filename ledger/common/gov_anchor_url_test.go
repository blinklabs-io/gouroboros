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

package common_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

const govAnchorURLMaxBytes = 128

func govAnchorTestHash() []byte {
	hash, err := hex.DecodeString(
		"0000000000000000000000000000000000000000000000000000000000000001",
	)
	if err != nil {
		panic(err)
	}
	return hash
}

func govAnchorCBOR(t *testing.T, url string) []byte {
	t.Helper()
	encoded, err := cbor.Encode([]any{url, govAnchorTestHash()})
	require.NoError(t, err)
	return encoded
}

func govAnchorURL(prefix string, total int) string {
	return prefix + strings.Repeat("a", total-len(prefix))
}

// TestGovAnchorURLBoundAtDecode covers the CDDL rule
// url = text .size (0 .. 128) used by anchor = [anchor_url : url, ...].
func TestGovAnchorURLBoundAtDecode(t *testing.T) {
	atLimit := govAnchorURL("https://example.com/", govAnchorURLMaxBytes)
	require.Len(t, atLimit, govAnchorURLMaxBytes)

	var accepted common.GovAnchor
	require.NoError(t, accepted.UnmarshalCBOR(govAnchorCBOR(t, atLimit)))
	require.Equal(t, atLimit, accepted.Url)
	require.Equal(t, govAnchorTestHash(), accepted.DataHash[:])

	overLimit := govAnchorURL("https://example.com/", govAnchorURLMaxBytes+1)
	require.Len(t, overLimit, govAnchorURLMaxBytes+1)

	var rejected common.GovAnchor
	err := rejected.UnmarshalCBOR(govAnchorCBOR(t, overLimit))
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrGovAnchorURLTooLong)
}

func TestGovAnchorURLBoundAtConstructionAndEncoding(t *testing.T) {
	tooLong := govAnchorURL("https://example.com/", govAnchorURLMaxBytes+1)
	_, err := common.NewGovAnchor(tooLong, govAnchorTestHash())
	require.ErrorIs(t, err, common.ErrGovAnchorURLTooLong)

	anchor := common.GovAnchor{Url: tooLong}
	_, err = cbor.Encode(anchor)
	require.ErrorIs(t, err, common.ErrGovAnchorURLTooLong)

	atLimit := common.GovAnchor{
		Url:      govAnchorURL("https://example.com/", govAnchorURLMaxBytes),
		DataHash: [32]byte{1},
	}
	encoded, err := cbor.Encode(atLimit)
	require.NoError(t, err)
	var decoded common.GovAnchor
	require.NoError(t, decoded.UnmarshalCBOR(encoded))
	require.Equal(t, atLimit, decoded)
}

// TestGovAnchorURLBoundMeasuredInBytes checks that the limit counts UTF-8
// bytes rather than runes, matching cardano-ledger's lengthWord8 measure.
func TestGovAnchorURLBoundMeasuredInBytes(t *testing.T) {
	atLimit := strings.Repeat("é", govAnchorURLMaxBytes/2)
	require.Len(t, atLimit, govAnchorURLMaxBytes)
	var accepted common.GovAnchor
	require.NoError(t, accepted.UnmarshalCBOR(govAnchorCBOR(t, atLimit)))
	require.Equal(t, atLimit, accepted.Url)

	overLimit := strings.Repeat("é", govAnchorURLMaxBytes/2+1)
	var rejected common.GovAnchor
	require.ErrorIs(
		t,
		rejected.UnmarshalCBOR(govAnchorCBOR(t, overLimit)),
		common.ErrGovAnchorURLTooLong,
	)
}

// TestGovAnchorURLUnboundedInJSON records that the JSON representation stays
// unbounded, matching the reference ledger's Url FromJSON instance and the
// existing pool metadata behavior. Conway genesis carries a constitution
// anchor through this path.
func TestGovAnchorURLUnboundedInJSON(t *testing.T) {
	overLimit := govAnchorURL("https://example.com/", govAnchorURLMaxBytes+1)
	body := `{"url":"` + overLimit + `","dataHash":"` +
		hex.EncodeToString(govAnchorTestHash()) + `"}`

	var anchor common.GovAnchor
	require.NoError(t, anchor.UnmarshalJSON([]byte(body)))
	require.Equal(t, overLimit, anchor.Url)
}

// TestGovAnchorURLBoundAtEveryDecodeSite checks that the containers carrying
// an anchor reject an over-long URL through the shared GovAnchor decoder.
func TestGovAnchorURLBoundAtEveryDecodeSite(t *testing.T) {
	overLimit := govAnchorURL("https://example.com/", govAnchorURLMaxBytes+1)
	anchor := []any{overLimit, govAnchorTestHash()}
	credential := []any{uint(0), make([]byte, common.Blake2b224Size)}

	tests := []struct {
		name  string
		items []any
		value interface {
			UnmarshalCBOR([]byte) error
		}
	}{
		{
			name:  "registration_drep_cert",
			items: []any{uint(16), credential, uint64(0), anchor},
			value: &common.RegistrationDrepCertificate{},
		},
		{
			name:  "update_drep_cert",
			items: []any{uint(18), credential, anchor},
			value: &common.UpdateDrepCertificate{},
		},
		{
			name:  "committee_resignation_cert",
			items: []any{uint(15), credential, anchor},
			value: &common.ResignCommitteeColdCertificate{},
		},
		{
			name:  "voting_procedure",
			items: []any{uint8(0), anchor},
			value: &common.VotingProcedure{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := cbor.Encode(test.items)
			require.NoError(t, err)
			require.ErrorIs(
				t,
				test.value.UnmarshalCBOR(encoded),
				common.ErrGovAnchorURLTooLong,
			)
		})
	}

	proposalCBOR, err := cbor.Encode([]any{
		uint64(0),
		append([]byte{0x60}, make([]byte, 28)...),
		[]any{uint(6)}, anchor,
	})
	require.NoError(t, err)
	var proposal conway.ConwayProposalProcedure
	require.ErrorIs(
		t,
		proposal.UnmarshalCBOR(proposalCBOR),
		common.ErrGovAnchorURLTooLong,
	)

	constitutionCBOR, err := cbor.Encode([]any{
		uint(common.GovActionTypeNewConstitution), nil,
		[]any{anchor, nil},
	})
	require.NoError(t, err)
	var constitution common.NewConstitutionGovAction
	_, err = cbor.Decode(constitutionCBOR, &constitution)
	require.ErrorIs(
		t,
		err,
		common.ErrGovAnchorURLTooLong,
	)
}
