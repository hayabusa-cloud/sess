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
//     [code.hybscloud.com/iox.ErrMore] is outside the session transport domain
//     and is treated as an unexpected dispatcher failure.
//   - Execution: Dual-world API supporting closure-based (Cont-world) and defunctionalized (Expr-world) evaluation.
//   - Error Handling: Session operations are non-blocking, while error operations short-circuit into [code.hybscloud.com/kont.Either]. Paired error runners also return the first uncaught session-wide thrown cause as `*E`. When that `*E` is non-nil, treat it as the authoritative global outcome and check it before interpreting a peer [code.hybscloud.com/kont.Either], because the non-throwing side may still be locally unresolved.
//   - Contract: Each [Endpoint] is intended for single-goroutine use over an SPSC transport. Protocol duality and post-[Close] terminality remain caller responsibilities so the hot path stays validation-free. For interface-typed payloads, the value must carry a concrete dynamic type; nil interface payloads such as any(nil) are out of contract.
//
// # API Topologies
//
//   - Operations: [Send], [Recv], [Close], [SelectL], [SelectR], [Offer].
//     To delegate an [Endpoint], send its pointer.
//   - Cont-world: [SendThen], [RecvBind], [CloseDone], [SelectLThen], [SelectRThen], [OfferBranch].
//   - Expr-world: Frame-based constructors such as [ExprSendThen] and [ExprRecvBind]. Bridge via [Reify] and [Reflect].
//   - Recursive: [Loop] and [ExprLoop] for trampoline-based iterative protocols.
//
// [ExprSendThen], [ExprRecvBind], [ExprCloseDone], [ExprSelectLThen],
// [ExprSelectRThen], and [ExprOfferBranch] build single-use expressions.
// Construct the entire role, including any fused suffix, afresh for each run.
//
// # Integration
//
//   - Stepping: Start an Expr role with [Step] or a Cont role with [code.hybscloud.com/kont.Step].
//     Pass the resulting suspension to [Advance] to try its session operation.
//     For typed errors, start with [StepError]. Pass it an Expr directly or [Reify] a Cont first.
//   - Blocking: [Exec], [Run] (and Error/Expr variants) wait past boundaries using adaptive backoff.
//
// A driver can use takt for backend completions and tempo for event history.
// To guard a pending operation, call [code.hybscloud.com/cove.CheckSuspension]
// with the current context before each attempt. A failed check leaves the
// original suspension live for retry or discard. While the requirement holds,
// pass the view's Suspension to [Advance]. If it came from [StepError], use
// [AdvanceError] to handle a matching [code.hybscloud.com/kont.Throw] as Left.
//
// On [code.hybscloud.com/iox.ErrWouldBlock], retain the returned suspension and
// retry after the peer makes progress. A successful send places the payload in
// the queue. A protocol can acknowledge processing with a reply from the peer.
package sess
