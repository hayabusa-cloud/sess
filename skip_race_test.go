// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build race

package sess_test

import "testing"

// skipRace skips tests that exercise lfq SPSC transport.
// The race detector tracks per-variable happens-before and cannot
// see SPSC's cross-variable memory ordering (store-release on data,
// load-acquire on index), producing false positives.
func skipRace(tb testing.TB) {
	tb.Helper()
	tb.Skip("skip: SPSC uses cross-variable memory ordering")
}
