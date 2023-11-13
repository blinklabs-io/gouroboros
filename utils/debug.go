// Copyright 2023 Blink Labs Software
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

package utils

import (
	"bytes"
	"fmt"
)

// DumpCborStructure generates an indented string representing an arbitrary data structure for debugging purposes
func DumpCborStructure(data interface{}, prefix string) string {
	var ret bytes.Buffer
	switch v := data.(type) {
	case int, uint, int16, uint16, int32, uint32, int64, uint64:
		return fmt.Sprintf("%s0x%x (%d),\n", prefix, v, v)
	case []uint8:
		return fmt.Sprintf("%s<bytes> (length %d),\n", prefix, len(v))
	case []interface{}:
		ret.WriteString(fmt.Sprintf("%s[\n", prefix))
		newPrefix := prefix
		// Override original user-provided prefix
		// This assumes the original prefix won't start with a space
		if len(newPrefix) > 1 && newPrefix[0] != ' ' {
			newPrefix = ""
		}
		// Add 2 more spaces to the new prefix
		newPrefix = fmt.Sprintf("  %s", newPrefix)
		/*
			var lastOutput string
			var lastOutputCount uint32
		*/
		for _, val := range v {
			tmp := DumpCborStructure(val, newPrefix)
			/*
				if lastOutput == "" || lastOutput == tmp {
					lastOutputCount += 1
					if lastOutputCount == 5 {
						ret.WriteString(fmt.Sprintf("%s...\n", newPrefix))
						continue
					} else if lastOutputCount > 5 {
						lastOutput = tmp
						continue
					}
				}
				lastOutput = tmp
			*/
			ret.WriteString(tmp)
		}
		ret.WriteString(fmt.Sprintf("%s],\n", prefix))
	case map[interface{}]interface{}:
		ret.WriteString(fmt.Sprintf("%s{\n", prefix))
		newPrefix := prefix
		// Override original user-provided prefix
		// This assumes the original prefix won't start with a space
		if len(newPrefix) > 1 && newPrefix[0] != ' ' {
			newPrefix = ""
		}
		// Add 2 more spaces to the new prefix
		newPrefix = fmt.Sprintf("  %s", newPrefix)
		for key, val := range v {
			ret.WriteString(fmt.Sprintf("%s%#v => %#v,\n", newPrefix, key, val))
		}
		ret.WriteString(fmt.Sprintf("%s}\n", prefix))
	default:
		return fmt.Sprintf("%s%#v,\n", prefix, v)
	}
	return ret.String()
}
