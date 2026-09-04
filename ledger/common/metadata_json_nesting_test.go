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

package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nestedNoSchemaJSON returns metadata JSON whose deepest container sits at
// nesting depth arrays, counting the mandatory top-level object as depth 0.
func nestedNoSchemaJSON(arrays int) []byte {
	return []byte(
		`{"1":` + strings.Repeat("[", arrays) +
			strings.Repeat("]", arrays) + `}`,
	)
}

func TestParseMetadataJSONNoSchemaAcceptsMaximumNesting(t *testing.T) {
	// The top-level object occupies depth 0, leaving one less array than the
	// limit before the limit is reached.
	metadata, err := ParseCardanoCLIMetadataJSONNoSchema(
		nestedNoSchemaJSON(MetadataJSONMaxNestingDepth - 1),
	)
	require.NoError(t, err)
	require.NotNil(t, metadata)
}

func TestParseMetadataJSONNoSchemaRejectsExcessiveNesting(t *testing.T) {
	for _, test := range []struct {
		name   string
		arrays int
	}{
		{name: "first invalid", arrays: MetadataJSONMaxNestingDepth},
		{name: "far beyond limit", arrays: 100_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A stack overflow is a fatal error that recover cannot catch, so
			// the limit has to reject before descending, not after.
			metadata, err := ParseCardanoCLIMetadataJSONNoSchema(
				nestedNoSchemaJSON(test.arrays),
			)
			require.Error(t, err)
			assert.Nil(t, metadata)
			assert.ErrorContains(t, err, "nesting depth")
		})
	}
}

func TestParseMetadataJSONDetailedSchemaRejectsExcessiveNesting(t *testing.T) {
	// Each detailed-schema list level costs two JSON containers: the wrapping
	// object and its array.
	const levels = 100_000
	payload := `{"1":` + strings.Repeat(`{"list":[`, levels) +
		`{"int":1}` + strings.Repeat(`]}`, levels) + `}`

	metadata, err := ParseCardanoCLIMetadataJSONDetailedSchema([]byte(payload))
	require.Error(t, err)
	assert.Nil(t, metadata)
	assert.ErrorContains(t, err, "nesting depth")
}

func TestParseMetadataJSONDetailedSchemaAcceptsModestNesting(t *testing.T) {
	const levels = 8
	payload := `{"1":` + strings.Repeat(`{"list":[`, levels) +
		`{"int":1}` + strings.Repeat(`]}`, levels) + `}`

	metadata, err := ParseCardanoCLIMetadataJSONDetailedSchema([]byte(payload))
	require.NoError(t, err)
	require.NotNil(t, metadata)
}
