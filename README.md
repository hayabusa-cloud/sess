[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Session-typed communication protocols via algebraic effects on [kont](https://code.hybscloud.com/kont).

## Overview

Session types assign a type to each step of a communication protocol. Each operation — send, receive, select, offer, close — is individually well-typed via Go generics, and protocol composition within a single endpoint is type-safe. Duality (matching operations across endpoints) is a programmer responsibility: the programmer writes dual protocols, and mismatches manifest at runtime as type assertion failures or deadlocks.

`sess` encodes session types as algebraic effects evaluated by the [kont](https://code.hybscloud.com/kont) effect system. Each protocol step — send, receive, select, offer, close — is an effect that suspends the computation until the transport completes the operation. The transport returns `iox.ErrWouldBlock` at computational boundaries, allowing proactor event loops (e.g., `io_uring`) to multiplex execution without thread-blocking.

Two equivalent API families are available: Cont (closure-based, straightforward to compose) and Expr (frame-based,
amortized zero-allocation for hot paths).

## Composition Boundary

`sess` owns the session effect signature and the endpoint transport that interprets it. It uses `iox.ErrWouldBlock` as
the non-blocking boundary for bounded queues, but does not own the full `iox` outcome algebra; `takt` owns
proactor-style scheduling and completion correlation; `cove` owns contextual evidence for suspension-aware composition.

## Installation

```bash
go get code.hybscloud.com/sess
```

Requires Go 1.26+.

## Session Operations

Each operation has a dual. When one endpoint performs an operation, the other must perform its dual.

| Operation | Dual | Suspends? |
|-----------|------|-----------|
| `Send[T]` — send a value | `Recv[T]` — receive a value | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — choose a branch | `Offer` — follow the peer's choice | `iox.ErrWouldBlock` |
| `Close` — end the session | `Close` | Never |

## Usage

Use `Run` to prototype and validate protocols. Use `Exec` with externally managed endpoints. Use the Expr API (
`RunExpr`/`ExecExpr`) when you need stepping control or want to minimize allocation overhead on hot paths.

### Send and Receive

One side sends a value; the dual side receives it.

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

Transfer an endpoint to a third party by sending it; accept delegation by receiving it.

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
    return sess.CloseDone("accepted")
})
```

### Stepping

For proactor event loops (e.g., `io_uring`), `Step` and `Advance` evaluate one effect at a time. Unlike `Run` and `Exec` — which synchronously wait for progress — the stepping API yields `iox.ErrWouldBlock` to the caller, letting the event loop reschedule.

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
// In a proactor event loop (e.g., io_uring), yield on boundary:
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
    return susp // yield to event loop, reschedule when ready
}
susp = nextSusp
```

### Error Handling

Compose session protocols with error effects. `Throw` immediately aborts the paired run. The returned `thrown` value is
the session-global uncaught throw cause; check it before interpreting a peer-side `Either`.

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
- `thrown != nil`: the paired run aborted globally; `*thrown` is the uncaught throw, and a peer-side `Either` may still
  be unresolved.

## Execution Model

| Function | Description |
|----------|-------------|
| `Run` / `RunExpr` | Run both sides on one goroutine, creating an endpoint pair internally |
| `Exec` / `ExecExpr` | Run one side on a pre-created endpoint |
| `Step` + `Advance` | Evaluate one effect at a time for external event loops |

**Cont vs Expr**: Cont is closure-based and straightforward to compose. Expr is frame-based with amortized zero-allocation, suited for hot paths.

## Contract

`sess` exposes a trusted-caller transport API. Each endpoint is intended for use by one goroutine at a time, and the hot
path intentionally omits concurrent-use guards and post-`Close` checks.

If a payload type is an interface, the value must still carry a concrete dynamic type. Nil interface values such as
`any(nil)` or `error(nil)` are outside the contract; if nil is semantically meaningful, use a nil value of a concrete
type or wrap it explicitly.

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

A paired error-aware run defines dual protocols and lets `RunErrorExpr`
create the endpoint pair internally:

```go
// 1. Define the protocol on each side using the dual operations.
clientProg := sess.ExprSendThen(42, sess.ExprRecvBind(
    func(reply string) kont.Expr[string] {
        return sess.ExprCloseDone(reply)
    },
))
serverProg := sess.ExprRecvBind(func(n int) kont.Expr[string] {
    return sess.ExprSendThen(
        fmt.Sprintf("got %d", n),
        sess.ExprCloseDone[string]("ok"),
    )
})

// 2. Run both sides in lockstep with error handling.
type Err struct{ Reason string }
left, right, thrown := sess.RunErrorExpr[Err](clientProg, serverProg)
if thrown != nil {
    // The session aborted; left/right may carry only partial results.
    _ = thrown
}
_ = left; _ = right
```

For proactor integration, create endpoints with `sess.New()` and drive
`StepError` / `AdvanceError` yourself: the suspension yields whenever the
underlying transport returns `iox.ErrWouldBlock`, and the loop resumes it when
the matching endpoint completes.

## References

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (
  CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured
  Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381,
  122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014),
  384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM
  SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*.
  568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to
  Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session
  Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dependencies

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Delimited continuations and algebraic effects
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Non-blocking semantics (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Lock-free FIFO queues

## License

MIT — see [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
