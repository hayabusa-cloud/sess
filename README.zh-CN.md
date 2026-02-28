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

两种等价的 API：Cont（基于闭包，直接组合）和 Expr（基于帧，热路径的摊销零分配）。

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

使用 `Run` 进行协议原型设计与验证。对于外部管理的端点，使用 `Exec`。当需要步进控制或在热路径上最小化分配开销时，使用 Expr API（`RunExpr`/`ExecExpr`）。

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
// In a proactor event loop (e.g., io_uring), yield on boundary:
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
    return susp // yield to event loop, reschedule when ready
}
susp = nextSusp
```

### 错误处理

将会话协议与错误效果组合。`Throw` 立即短路协议并丢弃挂起的暂停。

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: 成功时为 Right，Throw 时为 Left
```

## 执行模型

| 函数 | 使用场景 |
|------|----------|
| `Run` / `RunExpr` | 在一个 goroutine 上运行双方 — 内部创建端点对 |
| `Exec` / `ExecExpr` | 在预创建的端点上运行一方 |
| `Step` + `Advance` | 面向外部事件循环的逐效果控制 |

**Cont 与 Expr 的选择**：Cont 基于闭包，组合简单直接。Expr 基于帧，摊销零分配，适合热路径。

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

## References

- Kohei Honda. "Types for Dyadic Interaction." In *CONCUR 1993* (LNCS 715), pp. 509-523. Springer, 1993. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, Makoto Kubo. "Language Primitives and Type Discipline for Structured Communication-Based Programming." In *ESOP 1998* (LNCS 1381), pp. 122-138. Springer, 1998. https://doi.org/10.1007/BFb0053567
- Philip Wadler. "Propositions as Sessions." *Journal of Functional Programming* 24(2-3):384-418, 2014. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard, Nobuko Yoshida. "Effects as Sessions, Sessions as Effects." In *POPL 2016*, pp. 568-581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley, J. Garrett Morris. "Lightweight Functional Session Types." In *Behavioural Types: From Theory to Tools*, pp. 265-286, 2017 (first published year; DOI metadata date is 2022-09-01). https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, Sara Decova. "Exceptional Asynchronous Session Types: Session Types without Tiers." *Proc. ACM Program. Lang.* 3(POPL):28:1-28:29, 2019. https://doi.org/10.1145/3290341

## 依赖

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限界续体和代数效果
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 非阻塞语义（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — 无锁 FIFO 队列

## 许可证

MIT — 详见 [LICENSE](LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
