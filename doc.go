// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package sess provides session-typed communication protocols via algebraic effects
// on [code.hybscloud.com/kont].
//
// Protocols are composed of typed operations dispatched on a session endpoint.
//
// # Architecture
//
//   - Transport: Lock-free bounded SPSC queues via [code.hybscloud.com/lfq]. [New] creates an [Endpoint] pair.
//   - Non-blocking: Operations return [code.hybscloud.com/iox.ErrWouldBlock] on backpressure.
//   - Execution: Dual-world API supporting closure-based (Cont-world) and defunctionalized (Expr-world) evaluation.
//   - Error Handling: Session operations are non-blocking, while error operations short-circuit returning [code.hybscloud.com/kont.Either].
//
// # API Topologies
//
//   - Operations: [Send], [Recv], [Close], [SelectL], [SelectR], [Offer]. Endpoint delegation is [Send]/[Recv] of [*Endpoint].
//   - Cont-world: [SendThen], [RecvBind], [CloseDone], [SelectLThen], [SelectRThen], [OfferBranch].
//   - Expr-world: Zero-allocation variants like [ExprSendThen], [ExprRecvBind], etc. Bridge via [Reify] and [Reflect].
//   - Recursive: [Loop] and [ExprLoop] for trampoline-based iterative protocols.
//
// # Integration
//
//   - Stepping: [Step] and [Advance] (or [StepError]/[AdvanceError]) evaluate computations one effect at a time, making them easy to integrate with a proactor loop.
//   - Blocking: [Exec], [Run] (and Error/Expr variants) wait past boundaries using adaptive backoff.
//
// # Example
//
//	epA, _ := sess.New()
//	protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
//	_, susp := sess.Step[struct{}](protocol)
//	for susp != nil {
//		var err error
//		if _, susp, err = sess.Advance(epA, susp); err != nil {
//			continue // retry on ErrWouldBlock
//		}
//	}
package sess
