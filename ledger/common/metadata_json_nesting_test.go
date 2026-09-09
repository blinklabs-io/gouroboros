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

	"github.com/blinklabs-io/gouroboros/cbor"
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

// nestedNoSchemaObjectJSON is nestedNoSchemaJSON for the object reader. Both
// readers carry their own copy of the bound, so each needs its own boundary
// coverage; a no-schema object nests exactly like an array.
func nestedNoSchemaObjectJSON(objects int) []byte {
	return []byte(
		`{"1":` + strings.Repeat(`{"a":`, objects) + `1` +
			strings.Repeat("}", objects) + `}`,
	)
}

// nestedNoSchemaBuilders covers both container readers at the boundary.
var nestedNoSchemaBuilders = []struct {
	name  string
	build func(int) []byte
}{
	{name: "array", build: nestedNoSchemaJSON},
	{name: "object", build: nestedNoSchemaObjectJSON},
}

func TestParseMetadataJSONNoSchemaAcceptsMaximumNesting(t *testing.T) {
	// The top-level object is itself one of the containers counted by the
	// bound, so the deepest accepted document holds one fewer array.
	metadata, err := ParseCardanoCLIMetadataJSONNoSchema(
		nestedNoSchemaJSON(MetadataJSONMaxNestingDepth - 1),
	)
	require.NoError(t, err)
	require.NotNil(t, metadata)
}

// TestParseMetadataJSONNoSchemaAcceptBoundaryIsDecodable pins the accept
// boundary to what this repository's CBOR decoder accepts rather than to the
// constant alone. No-schema JSON containers map one to one onto CBOR nesting,
// so every document the parser accepts has to encode to auxiliary data that
// decodes again.
func TestParseMetadataJSONNoSchemaAcceptBoundaryIsDecodable(t *testing.T) {
	for _, builder := range nestedNoSchemaBuilders {
		t.Run(builder.name, func(t *testing.T) {
			deepestAccepted := 0
			for depth := MetadataJSONMaxNestingDepth - 2; depth <= MetadataJSONMaxNestingDepth+1; depth++ {
				metadata, err := ParseCardanoCLIMetadataJSONNoSchema(
					builder.build(depth),
				)
				if err != nil {
					continue
				}
				deepestAccepted = depth
				encoded, err := cbor.Encode(
					NewShelleyAuxiliaryData(metadata),
				)
				require.NoError(
					t,
					err,
					"%d %ss: encode accepted metadata",
					depth,
					builder.name,
				)

				var auxData ShelleyAuxiliaryData
				_, err = cbor.Decode(encoded, &auxData)
				require.NoError(
					t,
					err,
					"%d %ss: parser accepted metadata that cbor.Decode rejects",
					depth,
					builder.name,
				)

				_, err = DecodeMetadatumRaw(encoded)
				require.NoError(
					t,
					err,
					"%d %ss: parser accepted metadata that DecodeMetadatumRaw rejects",
					depth,
					builder.name,
				)
			}
			require.Equal(
				t,
				MetadataJSONMaxNestingDepth-1,
				deepestAccepted,
			)
		})
	}
}

func TestParseMetadataJSONNoSchemaRejectsExcessiveNesting(t *testing.T) {
	for _, builder := range nestedNoSchemaBuilders {
		t.Run(builder.name, func(t *testing.T) {
			for _, test := range []struct {
				name  string
				depth int
			}{
				{name: "first invalid", depth: MetadataJSONMaxNestingDepth},
				{name: "far beyond limit", depth: 100_000},
			} {
				t.Run(test.name, func(t *testing.T) {
					// The limit has to reject before descending, not after,
					// so the readers never recurse past it.
					metadata, err := ParseCardanoCLIMetadataJSONNoSchema(
						builder.build(test.depth),
					)
					require.Error(t, err)
					assert.Nil(t, metadata)
					assert.ErrorContains(t, err, "nesting depth")
				})
			}
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
