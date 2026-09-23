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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePoolRegistrationOwners(t *testing.T) {
	var first, second AddrKeyHash
	first[0] = 1
	second[0] = 2

	certificate := func(owners ...AddrKeyHash) CertificateWrapper {
		return CertificateWrapper{
			Type: uint(CertificateTypePoolRegistration),
			Certificate: &PoolRegistrationCertificate{
				PoolOwners: owners,
			},
		}
	}

	t.Run("duplicate full hash is rejected", func(t *testing.T) {
		err := ValidatePoolRegistrationOwners([]CertificateWrapper{
			certificate(first, second, first),
		})
		assert.ErrorContains(t, err, "duplicate owner")
	})

	t.Run("distinct hashes are accepted", func(t *testing.T) {
		require.NoError(t, ValidatePoolRegistrationOwners(
			[]CertificateWrapper{certificate(first, second)},
		))
	})

	t.Run("non-pool certificate is ignored", func(t *testing.T) {
		require.NoError(t, ValidatePoolRegistrationOwners([]CertificateWrapper{
			{Certificate: &StakeRegistrationCertificate{}},
		}))
	})
}
