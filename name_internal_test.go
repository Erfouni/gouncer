// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"strings"
	"testing"
)

func TestCleanNameAllocatesOnceForALongName(t *testing.T) {
	name := strings.Repeat("a ", 1<<16)

	if allocs := testing.AllocsPerRun(100, func() { _ = cleanName(name) }); allocs > 1 {
		t.Errorf("cleanName() allocations = %v, want at most 1", allocs)
	}
}
