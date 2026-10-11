// SPDX-License-Identifier: Apache-2.0

package sqlite

import "testing"

func TestMustSubPanicsOnAFolderOutsideTheTree(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()

	mustSub(migrations, "..")
}
