// Copyright 2023 Blink Labs, LLC.
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

package ouroboros_test

import (
	ouroboros "github.com/blinklabs-io/gouroboros"
	"testing"
)

// Ensure that we don't panic when closing the Ouroboros object after a failed Dial() call
func TestDialFailClose(t *testing.T) {
	oConn, err := ouroboros.New()
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	err = oConn.Dial("unix", "/path/does/not/exist")
	if err == nil {
		t.Fatalf("did not get expected failure on Dial()")
	}
	// Close Ouroboros connection
	oConn.Close()
}
