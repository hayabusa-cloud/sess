// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package sess provides session-typed communication protocols via algebraic effects
// on [code.hybscloud.com/kont].
//
// Protocols are composed from typed operations, each dispatched as an algebraic
// effect on a session endpoint.
//
// # Design
//
//   - Lock-free bounded transport via [code.hybscloud.com/lfq] SPSC queues
//   - Non-blocking [code.hybscloud.com/iox.ErrWouldBlock] boundary semantics
//   - Dual-world API: Cont-world (closure-based) and Expr-world (defunctionalized)
//   - Structural interface dispatch for session operations
//
// # Non-Blocking Semantics
//
// Session operations are strictly non-blocking. Operations directly query the
// underlying transport and return [code.hybscloud.com/iox.ErrWouldBlock] when
// backpressure limits are reached:
//
//   - nil error: operation completed
//   - [code.hybscloud.com/iox.ErrWouldBlock]: queue not ready, retry after peer progress
//
// The stepping API ([Step], [Advance]) preserves this boundary.
// Blocking helpers ([Exec], [Run]) use [code.hybscloud.com/iox.Backoff]
// to wait past the boundary.
//
// # Session Operations
//
// Six operations define the protocol vocabulary. Each implements
// DispatchSession with non-blocking semantics:
//
//   - [Send]: Send a typed value to the peer
//   - [Recv]: Receive a typed value from the peer
//   - [Close]: Signal session termination (never blocks)
//   - [SelectL]: Choose the left branch
//   - [SelectR]: Choose the right branch
//   - [Offer]: Wait for the peer's branch choice
//
// Endpoint delegation is [Send]/[Recv] of [*Endpoint].
//
// # Cont-World API
//
// Fused constructors compose operations with continuation sequencing:
// [SendThen], [RecvBind], [CloseDone], [SelectLThen], [SelectRThen], [OfferBranch].
//
// # Expr-World API
//
// Defunctionalized constructors mirror the Cont-world API with pooled frames
// for amortized zero-allocation evaluation:
// [ExprSendThen], [ExprRecvBind], [ExprCloseDone], [ExprSelectLThen],
// [ExprSelectRThen], [ExprOfferBranch].
//
// # Recursive Protocols
//
// [Loop] and [ExprLoop] express recursive protocols via tailRecM. The step
// function returns Left(nextState) to continue or Right(result) to finish.
//
// # Execution
//
// Blocking helpers wait past [code.hybscloud.com/iox.ErrWouldBlock]
// via adaptive backoff:
//
//   - [Exec], [ExecExpr]: Run a protocol on a pre-created endpoint
//   - [Run], [RunExpr]: Create a pair and interleave both sides cooperatively
//
// # Stepping / Proactor Integration
//
// One-effect-at-a-time evaluation for external runtimes:
//
//   - [Step]: Evaluate a protocol until the first effect suspension
//   - [Advance]: Dispatch the suspended operation (non-blocking)
//
// Unconsumed suspensions from [code.hybscloud.com/iox.ErrWouldBlock] are retryable.
// [code.hybscloud.com/kont.Suspension.Op] returns the concrete operation
// for protocol-aware scheduling.
//
// # Error Handling
//
// Composed session+error dispatch: session operations are non-blocking,
// error operations ([code.hybscloud.com/kont.ThrowError],
// [code.hybscloud.com/kont.CatchError]) short-circuit.
// Results are [code.hybscloud.com/kont.Either] — Right on success, Left on Throw.
//
//   - [ExecError], [ExecErrorExpr]: Blocking error execution
//   - [RunError], [RunErrorExpr]: Blocking dual error execution
//   - [StepError]: [Step] with error support
//   - [AdvanceError]: [Advance] with error support
//
// # Cont ↔ Expr Bridge
//
//   - [Reify]: Cont-world → Expr-world
//   - [Reflect]: Expr-world → Cont-world
//
// # Transport
//
// [New] creates a connected [Endpoint] pair using four bounded SPSC queues
// (two data, two choice) from [code.hybscloud.com/lfq].
//
// # Serial
//
// Each session pair receives a monotonically increasing [Serial].
// Both endpoints share the same serial.
//
// # Example (Stepping)
//
//	epA, _ := sess.New()
//	protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
//	_, susp := sess.Step[struct{}](protocol)
//	for susp != nil {
//		var err error
//		_, susp, err = sess.Advance(epA, susp)
//		if err != nil {
//			continue // retry on ErrWouldBlock
//		}
//	}
package sess
