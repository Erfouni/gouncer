// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"testing"

	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"
)

func TestTheBuildRunsTheLibcTheDriverWasMadeFor(t *testing.T) {
	t.Parallel()

	sqlitetest.CheckLibc(t)
}
