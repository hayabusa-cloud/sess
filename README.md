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

Two equivalent APIs: Cont (closure-based, straightforward composition) and Expr (frame-based, amortized zero-allocation for hot paths).

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

Use `Run` for protocol prototyping and validation. Use `Exec` for externally managed endpoints. Use the Expr API (`RunExpr`/`ExecExpr`) for stepping control or to minimize allocation overhead on hot paths.

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

Compose session protocols with error effects. `Throw` eagerly short-circuits the protocol and discards the pending suspension.

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right on success, Left on Throw
```

## Execution Model

| Function | Description |
|----------|-------------|
| `Run` / `RunExpr` | Run both sides on one goroutine, creating an endpoint pair internally |
| `Exec` / `ExecExpr` | Run one side on a pre-created endpoint |
| `Step` + `Advance` | Evaluate one effect at a time for external event loops |

**Cont vs Expr**: Cont is closure-based and straightforward to compose. Expr is frame-based with amortized zero-allocation, suited for hot paths.

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

## References

- Kohei Honda. "Types for Dyadic Interaction." In *CONCUR 1993* (LNCS 715), pp. 509-523. Springer, 1993. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, Makoto Kubo. "Language Primitives and Type Discipline for Structured Communication-Based Programming." In *ESOP 1998* (LNCS 1381), pp. 122-138. Springer, 1998. https://doi.org/10.1007/BFb0053567
- Philip Wadler. "Propositions as Sessions." *Journal of Functional Programming* 24(2-3):384-418, 2014. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard, Nobuko Yoshida. "Effects as Sessions, Sessions as Effects." In *POPL 2016*, pp. 568-581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley, J. Garrett Morris. "Lightweight Functional Session Types." In *Behavioural Types: From Theory to Tools*, pp. 265-286, 2017 (first published year; DOI metadata date is 2022-09-01). https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, Sara Decova. "Exceptional Asynchronous Session Types: Session Types without Tiers." *Proc. ACM Program. Lang.* 3(POPL):28:1-28:29, 2019. https://doi.org/10.1145/3290341

## Dependencies

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Delimited continuations and algebraic effects
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Non-blocking semantics (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Lock-free FIFO queues

## License

MIT — see [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
