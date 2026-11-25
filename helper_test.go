// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
	"code.hybscloud.com/sess"
)

// execExpr drives a protocol to completion on ep via Step+Advance loop.
// Retries on iox.ErrWouldBlock (peer not ready yet).
// Used by stepping tests to exercise the non-blocking path.
func execExpr[R any](ep *sess.Endpoint, protocol kont.Expr[R]) R {
	result, susp := sess.Step[R](protocol)
	for susp != nil {
		var err error
		result, susp, err = sess.Advance(ep, susp)
		if err != nil && !iox.IsWouldBlock(err) {
			panic(err)
		}
	}
	return result
}

// advanceOne checks that a single ready operation completes on the first attempt.
func advanceOne[R any](t *testing.T, ep *sess.Endpoint, op kont.Expr[R]) R {
	t.Helper()
	_, h := sess.Step(op)
	if h == nil {
		t.Fatal("expected a pending operation")
	}
	value, next, err := sess.Advance(ep, h)
	if err != nil || next != nil {
		t.Fatalf("ready operation returned (%p, %v), want completion", next, err)
	}
	return value
}
