// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"code.hybscloud.com/kont"
)

// Step evaluates a session protocol until the first effect suspension.
// Returns (result, nil) on completion, or (zero, suspension) if pending.
func Step[R any](protocol kont.Expr[R]) (R, *kont.Suspension[R]) {
	return kont.StepExpr(protocol)
}

// Advance dispatches the suspended session operation on the endpoint.
// DispatchSession is non-blocking: returns iox.ErrWouldBlock when the
// bounded SPSC queue cannot make progress (the I/O boundary).
//
// On success (nil error), the suspension is consumed and the protocol
// advances to the next effect or completion.
// On iox.ErrWouldBlock, the suspension is unconsumed and may be retried
// after the peer makes progress.
func Advance[R any](ep *Endpoint, susp *kont.Suspension[R]) (R, *kont.Suspension[R], error) {
	sop, ok := susp.Op().(sessionDispatcher)
	if !ok {
		panic("sess: unhandled effect in Advance")
	}
	v, err := sop.DispatchSession(&ep.ctx)
	if err != nil {
		var zero R
		return zero, susp, err
	}
	result, next := susp.Resume(v)
	return result, next, nil
}
