// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build race

package sess_test

import "testing"

// The race detector does not observe lfq's atomix publication of queue slots.
// Only tests with a concurrent producer and consumer need this skip.
func skipRace(tb testing.TB) {
	tb.Helper()
	tb.Skip("race detector cannot observe concurrent lfq slot handoff")
}
