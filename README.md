[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Benchmarks](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Two peers need to agree on the types and order of their messages. In `sess`, the **Cont** and **Expr** worlds give you two equivalent ways to write each role’s protocol in Go.

## Overview

`sess` represents each protocol action as an algebraic effect on [kont](https://code.hybscloud.com/kont). A role can send or receive a value, choose or follow a branch, and close its endpoint.

Use **Cont** (`kont.Eff[A]`) when you want the runner to handle retries. `RecvBind` passes the received value to a function that returns the rest of the role’s protocol. `Run` drives both roles. Use `Exec` for one endpoint while its peer runs separately.

Use **Expr** (`kont.Expr[A]`) when a driver such as an `io_uring` event loop needs to inspect operations or schedule retries. Continuation frames hold the remaining work. `Step` returns a result or a suspension, and `Advance` tries the suspended operation on the endpoint. If `Advance` returns `iox.ErrWouldBlock` on a receive, keep the same suspension and retry after the peer sends.

In both versions, the role receives an `int`, performs a local `Close`, and returns the value. Pair it with a sender using `Run` for Cont or `RunExpr` for Expr.

```go
contReceiver := sess.RecvBind(sess.CloseDone[int])
exprReceiver := sess.ExprRecvBind(sess.ExprCloseDone[int])
```

## Composition Boundary

The endpoints exchange payloads and branch choices through bounded queues. Enqueuing into a full queue or dequeuing from an empty one returns `iox.ErrWouldBlock`. A driver can use `takt` to match backend completions, `tempo` to track event history, and `cove` to check the current context before each attempt. The endpoint transport treats `iox.ErrMore` as a dispatcher error.

## Installation

```bash
go get code.hybscloud.com/sess
```

Requires Go 1.27+.

## Session Operations

Go checks each operation’s payload and reply types. Pair each send with a `Recv[T]` so that the type assertion `v.(T)` succeeds for every payload `v` at that step. For example, `Recv[[]byte]` can receive a `[]byte` sent with `Send[any]`. Pair each branch selection with an `Offer`. Payloads and branch choices travel in separate queues, so a run can finish even when the roles disagree on action order.

| Operation | Dual | Transport backpressure |
|-----------|------|-----------|
| `Send[T]` — send a value | `Recv[T]` — receive a value | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — choose a branch | `Offer` — follow the peer's choice | `iox.ErrWouldBlock` |
| `Close` — signal local close | `Close` | Never |

## Usage

Use `Run` to drive both roles on one goroutine, `Exec` for one externally managed endpoint, and `Step` with `Advance` when a driver chooses when to retry.

### Send and Receive

A successful send enqueues the payload. If the sender needs to know that the peer processed it, have the peer send an acknowledgment after processing.

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
	return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Expr equivalent: `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Branching

One side selects a branch; the dual side offers both branches and follows the selection.

```go
client := sess.SelectLThen(sess.SendThen(1, sess.CloseDone("left")))
server := sess.OfferBranch(
	func() kont.Eff[string] {
		return sess.RecvBind(func(n int) kont.Eff[string] {
			return sess.CloseDone(fmt.Sprintf("left %d", n))
		})
	},
	func() kont.Eff[string] { return sess.CloseDone("right") },
)
a, b := sess.Run(client, server)
```

### Recursive Protocols

Protocols that repeat use `Loop` with `Either`: `Left` continues the loop, `Right` terminates.

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
	if i >= 3 {
		return sess.CloseDone(kont.Right[int, string]("done"))
	}
	return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Delegation

A successful send puts the endpoint in transit until the receiver dequeues it. If the send returns `iox.ErrWouldBlock`, the sender keeps ownership.

```go
endpoint, peer := sess.New()
delegator := sess.SendThen(endpoint, sess.CloseDone(struct{}{}))
acceptor := sess.RecvBind(sess.CloseDone[*sess.Endpoint])
_, received := sess.Run(delegator, acceptor)

sess.Exec(received, sess.SendThen("hello", sess.CloseDone(struct{}{})))
message := sess.Exec(peer, sess.RecvBind(sess.CloseDone[string]))
fmt.Println(message) // hello
```

### Stepping

`Step` exposes the pending receive. `Advance` returns the same suspension while the queue is empty. After the peer sends, retrying that suspension obtains the value.

```go
sender, receiver := sess.New()
_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
_, pending, err := sess.Advance(receiver, h)
fmt.Println(iox.IsWouldBlock(err), pending == h) // true true

sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
value, next, err := sess.Advance(receiver, pending)
fmt.Println(value, next == nil, err) // 42 true <nil>
```

### Error Handling

An error-aware paired run ends when it dispatches a matching uncaught `Throw`. Check the returned `thrown` before interpreting the peer’s `Either`.

```go
client := kont.ExprThrowError[string, string]("boom")
server := sess.ExprRecvBind(func(v string) kont.Expr[string] {
	return sess.ExprCloseDone("recv: " + v)
})

clientResult, serverResult, thrown := sess.RunErrorExpr[string](client, server)
if thrown != nil {
	// Global session abort.
	fmt.Println("session aborted:", *thrown)
	// The peer-side Either may still be locally unresolved.
	_ = clientResult
	_ = serverResult
	return
}

// No uncaught session-wide throw: both Either values are final local outcomes.
fmt.Println(clientResult, serverResult)
```

In brief:

- `thrown == nil`: both `Either` values are final local outcomes.
- `thrown != nil`: the paired run aborted globally; `*thrown` is the uncaught throw, and a peer-side `Either` may still be unresolved.

## Execution Model

| Function | Description |
|----------|-------------|
| `Run` / `RunExpr` | Run both sides on one goroutine, creating an endpoint pair internally |
| `Exec` / `ExecExpr` | Run one side on a pre-created endpoint |
| `Step` + `Advance` | Evaluate one effect at a time for external event loops |

Compare allocation counts for complete protocols and for retrying a full queue. [Benchmark guide](BENCHMARKS.md)

## Contract

`sess` exposes a trusted-caller transport API. Each endpoint is intended for use by one goroutine at a time, and the hot path intentionally omits concurrent-use guards and post-`Close` checks.

If a payload type is an interface, the value must still carry a concrete dynamic type. Nil interface values such as `any(nil)` or `error(nil)` are outside the contract; if nil is semantically meaningful, use a nil value of a concrete type or wrap it explicitly.

Fused constructors such as `ExprSendThen` build single-use expressions. Build the role again for each run.

## API

| Category | Cont | Expr |
|----------|------|------|
| Constructors | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Recursion | `Loop` | `ExprLoop` |
| Execution | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| Error execution | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| Stepping | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| Bridge | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| Transport | `New` → `(*Endpoint, *Endpoint)` | |

## Practical Recipes

`RunErrorExpr` creates the endpoint pair. Change the request from `42` to `-1` to read the server’s rejection from `thrown`.

```go
type RequestError struct{ Reason string }

client := sess.ExprSendThen(42, sess.ExprRecvBind(sess.ExprCloseDone[string]))
server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
	if n < 0 {
		return kont.ExprThrowError[RequestError, string](RequestError{Reason: "negative request"})
	}
	return sess.ExprSendThen(fmt.Sprintf("got %d", n), sess.ExprCloseDone("ok"))
})

clientResult, serverResult, thrown := sess.RunErrorExpr[RequestError](client, server)
if thrown != nil {
	fmt.Println(thrown.Reason)
	return
}
reply, _ := clientResult.GetRight()
status, _ := serverResult.GetRight()
fmt.Println(reply, status) // got 42 ok
```

For socket input in a proactor loop, create the endpoint pair with `sess.New()` and pass any suspension from `StepError` to `AdvanceError`. If a receive returns `iox.ErrWouldBlock`, keep that suspension. Retry it after the adapter sends the decoded value through the peer endpoint.

## References

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dependencies

- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Atomic session identifiers and close counts
- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Delimited continuations and algebraic effects
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Non-blocking semantics (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Lock-free FIFO queues

## License

MIT — see [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
