[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Benchmarks](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | **简体中文** | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

通信双方需要约定消息的类型和收发顺序。`sess` 的 **Cont** 和 **Expr** 两个世界提供了两种等价的方式，都可以用 Go 描述每个角色的协议。

## 概述

`sess` 通过 [kont](https://code.hybscloud.com/kont) 的代数效果表示协议的每一步。每个角色都可以发送或接收值，选择分支或遵循对端的选择，也可以关闭自己的端点。

需要把重试交给运行器时，用 **Cont**（`kont.Eff[A]`）。`RecvBind` 把收到的值传给一个函数，由它返回该角色后续的协议。`Run` 驱动两个角色。如果对端单独运行，可以用 `Exec` 驱动本端。

当驱动程序需要查看操作或决定何时重试时，可以用 **Expr**（`kont.Expr[A]`），例如在基于 `io_uring` 的事件循环中。续延帧保存后续的计算。`Step` 返回结果或挂起句柄，`Advance` 在端点上尝试执行挂起的操作。如果接收时 `Advance` 返回 `iox.ErrWouldBlock`，就保留同一个挂起句柄，等对端发送数据后再重试。

下面是同一个接收角色的两种写法。两者都接收一个 `int`，在本端执行 `Close`，然后返回收到的值。将接收角色与发送角色配对后，Cont 用 `Run` 执行，Expr 用 `RunExpr` 执行。

```go
contReceiver := sess.RecvBind(sess.CloseDone[int])
exprReceiver := sess.ExprRecvBind(sess.ExprCloseDone[int])
```

## 组合边界

端点通过有界队列交换数据和分支选择。向已满的队列写入或从空队列读取时，操作会返回 `iox.ErrWouldBlock`。驱动程序可用 `takt` 将后端完成事件与请求对应，用 `tempo` 跟踪事件历史，并在每次尝试前用 `cove` 检查当前上下文。端点传输层将 `iox.ErrMore` 视为分派器错误。

## 安装

```bash
go get code.hybscloud.com/sess
```

需要 Go 1.27+。

## 会话操作

Go 检查每个操作的负载类型和应答类型。每次发送都应对应一个 `Recv[T]`，使该步可能发送的每个值 `v` 都能通过类型断言 `v.(T)`。例如，`Recv[[]byte]` 可以接收通过 `Send[any]` 发送的 `[]byte`。每次分支选择都应在对端对应一次 `Offer`。数据和分支选择使用独立的队列，因此即使两个角色的操作顺序不匹配，协议也可能执行完毕。

| 操作 | 对偶 | 传输背压 |
|------|------|--------|
| `Send[T]` — 发送一个值 | `Recv[T]` — 接收一个值 | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — 选择分支 | `Offer` — 跟随对端选择 | `iox.ErrWouldBlock` |
| `Close` — 标记本端结束 | `Close` | 从不 |

## 用法

用 `Run` 在一个 goroutine 上运行双方，用 `Exec` 驱动一个由外部管理的端点。若由驱动程序决定何时重试，可使用 `Step` 和 `Advance`。

### 收发

发送成功表示负载已入队。如果发送方需要知道对端已经处理了负载，应让对端在处理后发回确认。

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

`Step` 返回待执行的接收操作。队列为空时，`Advance` 返回同一个挂起对象。对端发送后，用该对象重试即可收到负载。

```go
sender, receiver := sess.New()
_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
_, pending, err := sess.Advance(receiver, h)
fmt.Println(iox.IsWouldBlock(err), pending == h) // true true

sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
value, next, err := sess.Advance(receiver, pending)
fmt.Println(value, next == nil, err) // 42 true <nil>
```

### 错误处理

带错误处理的成对运行在分派类型匹配且未被捕获的 `Throw` 时结束。解读对端的 `Either` 之前，先检查返回的 `thrown`。

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

可分别测量执行完整协议和在满队列上重试时的内存分配。[基准测试指南](BENCHMARKS.md)

## 契约

`sess` 提供的是面向受信调用方的传输 API。每个端点都假定在同一时刻只由一个 goroutine 使用，热路径有意不加入并发使用保护或 `Close` 之后的检查。

即使负载类型是接口，实际传递的值也必须带有具体的动态类型。像 `any(nil)` 或 `error(nil)` 这样的 nil 接口值不在契约内；如果 nil 本身具有语义，请使用带具体类型的 nil 值，或显式包一层。

`ExprSendThen` 等融合构造函数生成的表达式只能执行一次。每次运行前都要重新构造角色的表达式。

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

`RunErrorExpr` 创建一对端点。把请求值从 `42` 改为 `-1`，就能从 `thrown` 中读到服务端拒绝请求的原因。

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

在 proactor 事件循环中处理套接字输入时，用 `sess.New()` 创建一对端点。如果 `StepError` 返回挂起句柄，就将它传给 `AdvanceError`。若接收操作返回 `iox.ErrWouldBlock`，保留这个句柄。等适配器从对端成功发送解码后的值，再用同一个句柄重试接收。

## 参考文献

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## 依赖

- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — 原子更新会话标识符和关闭计数
- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限界续体和代数效果
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 非阻塞语义（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — 无锁 FIFO 队列

## 许可证

MIT — 详见 [LICENSE](LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
