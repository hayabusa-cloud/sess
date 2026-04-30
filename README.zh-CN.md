[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | **简体中文** | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

基于 [kont](https://code.hybscloud.com/kont) 代数效果的会话类型通信协议。

## 概述

会话类型为通信协议的每个步骤分配一个类型。每个操作 — 发送、接收、选择、提供、关闭 — 通过 Go 泛型实现独立的类型安全，单个端点内的协议组合也是类型安全的。对偶性（跨端点的操作匹配）是程序员的责任：程序员编写对偶协议，不匹配在运行时以类型断言失败或死锁的形式表现出来。

`sess` 将会话类型编码为由 [kont](https://code.hybscloud.com/kont) 效果系统求值的代数效果。每个协议步骤 — 发送、接收、选择、提供、关闭 — 是一个效果，它会挂起计算直到传输层完成操作。传输层在计算边界返回 `iox.ErrWouldBlock`，允许 proactor 事件循环（如 `io_uring`）在不阻塞线程的情况下多路复用执行。

提供两组等价的 API：Cont（基于闭包，便于直接组合）和 Expr（基于帧，在热路径上实现摊销零分配）。

## 组合边界

`sess` 拥有会话效果签名以及解释它的端点传输。它把 `iox.ErrWouldBlock` 用作有界队列上的非阻塞边界，但不拥有完整的 `iox`
结果代数；`takt` 拥有 proactor 风格的调度和完成事件关联；`cove` 拥有面向挂起组合的上下文证据。

## 安装

```bash
go get code.hybscloud.com/sess
```

需要 Go 1.26+。

## 会话操作

每个操作都有对偶。当一端执行某个操作时，另一端必须执行其对偶操作。

| 操作 | 对偶 | 挂起？ |
|------|------|--------|
| `Send[T]` — 发送一个值 | `Recv[T]` — 接收一个值 | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — 选择分支 | `Offer` — 跟随对端选择 | `iox.ErrWouldBlock` |
| `Close` — 结束会话 | `Close` | 从不 |

## 用法

使用 `Run` 进行协议原型设计与验证。对外部管理的端点使用 `Exec`。当需要步进控制，或希望在热路径上尽量减少分配开销时，使用
Expr API（`RunExpr`/`ExecExpr`）。

### 收发

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Expr 版本：`ExprSendThen`、`ExprRecvBind`、`ExprCloseDone`、`RunExpr`。

### 分支

一方选择分支；对偶方提供两个分支并跟随选择。

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

### 递归协议

重复的协议使用 `Loop` 和 `Either`：`Left` 继续循环，`Right` 终止。

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### 委托

通过发送端点将其转移给第三方；通过接收来接受委托。

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
    return sess.CloseDone("accepted")
})
```

### 步进

对于 proactor 事件循环（如 `io_uring`），`Step` 和 `Advance` 一次求值一个效果。与 `Run` 和 `Exec` — 同步等待进展 — 不同，步进 API 将 `iox.ErrWouldBlock` 返回给调用者，让事件循环重新调度。

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
// 在 proactor 事件循环（例如 io_uring）中，在边界处让出：
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
    return susp // 让出给事件循环，并在就绪后重新调度
}
susp = nextSusp
```

### 错误处理

可以将会话协议与错误效果组合。`Throw` 会立即中止成对执行。返回的 `thrown` 是会话级未捕获 `Throw` 的全局原因；在解释对端
`Either` 之前应先检查它。

```go
client := kont.ExprThrowError[string, string]("boom")
server := sess.ExprRecvBind(func(v string) kont.Expr[string] {
    return sess.ExprCloseDone("recv: " + v)
})

clientResult, serverResult, thrown := sess.RunErrorExpr[string](client, server)
if thrown != nil {
    // 会话整体已中止。
    fmt.Println("session aborted:", *thrown)
    // 对端 Either 仍可能在本地尚未解析完成。
    _ = clientResult
    _ = serverResult
    return
}

// 没有未捕获的全局 Throw：两个 Either 都是最终的本地结果。
fmt.Println(clientResult, serverResult)
```

简述：

- `thrown == nil`：两个 `Either` 都是最终的本地结果。
- `thrown != nil`：成对执行已经全局中止；`*thrown` 就是未捕获的 `Throw`，而对端 `Either` 仍可能尚未解析。

## 执行模型

| 函数 | 使用场景 |
|------|----------|
| `Run` / `RunExpr` | 在一个 goroutine 上运行双方 — 内部创建端点对 |
| `Exec` / `ExecExpr` | 在预创建的端点上运行一方 |
| `Step` + `Advance` | 面向外部事件循环的逐效果控制 |

**Cont 与 Expr 的选择**：Cont 基于闭包，组合简单直接。Expr 基于帧，摊销零分配，适合热路径。

## 契约

`sess` 提供的是面向受信调用方的传输 API。每个端点都假定在同一时刻只由一个 goroutine 使用，热路径有意不加入并发使用保护或
`Close` 之后的检查。

即使负载类型是接口，实际传递的值也必须带有具体的动态类型。像 `any(nil)` 或 `error(nil)` 这样的 nil 接口值不在契约内；如果
nil 本身具有语义，请使用带具体类型的 nil 值，或显式包一层。

## API

| 类别 | Cont | Expr |
|------|------|------|
| 构造器 | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| 递归 | `Loop` | `ExprLoop` |
| 执行 | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| 错误执行 | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| 步进 | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| 桥接 | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| 传输 | `New` → `(*Endpoint, *Endpoint)` | |

## 实用范式

成对的带错误处理执行会定义两侧对偶协议，并让 `RunErrorExpr` 在内部创建端点对：

```go
// 1. 用对偶操作分别定义两侧的协议。
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

// 2. 同步推进两侧并处理错误。
type Err struct{ Reason string }
left, right, thrown := sess.RunErrorExpr[Err](clientProg, serverProg)
if thrown != nil {
    // 会话被中止；left/right 可能仅承载部分结果。
    _ = thrown
}
_ = left; _ = right
```

用于 proactor 集成时，请用 `sess.New()` 创建端点，并自行驱动 `StepError` / `AdvanceError`：每当底层传输返回
`iox.ErrWouldBlock` 时挂起会让出执行权，事件循环在对端完成时再行恢复。

## 参考文献

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

## 依赖

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限界续体和代数效果
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 非阻塞语义（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — 无锁 FIFO 队列

## 许可证

MIT — 详见 [LICENSE](LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
