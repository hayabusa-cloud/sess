[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Session-typed communication protocols via algebraic effects on [kont](https://code.hybscloud.com/kont).

## Overview

`sess` provides typed, bidirectional protocols composed of six operations, each dispatched as an algebraic effect on a lock-free endpoint pair.

- **Dual-world API**: Cont (closure-based) and Expr (frame-based, zero-allocation hot paths)
- **Stepping**: Evaluate effects one at a time for proactor and event-loop integration
- **iox Non-blocking Algebra**: Enforces a strict progress model where operations natively yield `iox.ErrWouldBlock` at computational boundaries, allowing proactor event loops (e.g., io_uring) to seamlessly multiplex execution without thread-blocking

## Installation

```bash
go get code.hybscloud.com/sess
```

Requires Go 1.26+.

## Session Operations

| Operation | Effect | Suspends? |
|-----------|--------|-----------|
| `Send[T]` | Send a value | `iox.ErrWouldBlock` |
| `Recv[T]` | Receive a value | `iox.ErrWouldBlock` |
| `Close` | End the session | Never |
| `SelectL` | Choose the left branch | `iox.ErrWouldBlock` |
| `SelectR` | Choose the right branch | `iox.ErrWouldBlock` |
| `Offer` | Wait for the peer's choice | `iox.ErrWouldBlock` |

Delegate an endpoint by sending it; accept delegation by receiving it.

## Usage

### Send and Receive

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Expr equivalent: `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Branching

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

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Stepping

For real proactor event loops (e.g., `io_uring`), `sess` provides a `Step` and `Advance` mechanism. Unlike the `Run` and `Exec` helpers—which use `iox.Backoff` to synchronously wait for progress—the stepping API is the true non-blocking algebra that explicitly yields `iox.ErrWouldBlock` to the caller, allowing the event loop to seamlessly multiplex execution without thread-blocking.

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

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right on success, Left on Throw
```

## Execution Model

| Function | Use case |
|----------|----------|
| `Run` / `RunExpr` | Run both sides on one goroutine — creates an endpoint pair internally |
| `Exec` / `ExecExpr` | Run one side on a pre-created endpoint |
| `Step` + `Advance` | Evaluate one effect at a time, for external event loops |

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

## Dependencies

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Delimited continuations and algebraic effects
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Non-blocking semantics (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Lock-free FIFO queues

## License

MIT — see [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
