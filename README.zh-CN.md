[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | **简体中文** | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

基于 [kont](https://code.hybscloud.com/kont) 代数效果的会话类型通信协议。

## 概述

`sess` 提供由六种操作组成的类型化双向协议，每种操作作为代数效果在无锁端点对上分发。

- **双世界 API**：Cont（基于闭包）和 Expr（基于帧，零分配热路径）
- **步进**：逐个求值效果，便于 proactor 和事件循环集成
- **iox 非阻塞代数**：强制执行严格的进度模型，操作在计算边界处原生生成 `iox.ErrWouldBlock`，允许 proactor 事件循环（例如 io_uring）无缝复用执行，且不会阻塞系统线程

## 安装

```bash
go get code.hybscloud.com/sess
```

需要 Go 1.26+。

## 会话操作

| 操作 | 效果 | 挂起？ |
|------|------|--------|
| `Send[T]` | 发送一个值 | `iox.ErrWouldBlock` |
| `Recv[T]` | 接收一个值 | `iox.ErrWouldBlock` |
| `Close` | 结束会话 | 从不 |
| `SelectL` | 选择左分支 | `iox.ErrWouldBlock` |
| `SelectR` | 选择右分支 | `iox.ErrWouldBlock` |
| `Offer` | 等待对端选择 | `iox.ErrWouldBlock` |

通过发送端点进行委托，通过接收端点接受委托。

## 用法

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

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### 步进

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

## 依赖

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限界续体和代数效果
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 非阻塞语义（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — 无锁 FIFO 队列

## 许可证

MIT — 详见 [LICENSE](LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
