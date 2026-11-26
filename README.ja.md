[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Benchmarks](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | **日本語** | [Français](README.fr.md)

# sess

通信する双方は、やり取りするメッセージの型と順序をそろえておく必要があります。`sess` には **Cont** と **Expr** という二つの世界があり、どちらでも同じロールのプロトコルを Go で記述できます。

## 概要

`sess` はプロトコルの各操作を [kont](https://code.hybscloud.com/kont) の代数的エフェクトとして表します。各ロールは値を送受信し、分岐を選んだり相手の選択に従ったりし、自分のエンドポイントを閉じることができます。

再試行をランナーに任せる場合は、**Cont**（`kont.Eff[A]`）を使います。`RecvBind` は受信した値を関数に渡し、その関数がロールの残りのプロトコルを返します。`Run` は両方のロールを実行します。相手側が別に動作している場合は、`Exec` で片方のエンドポイントを実行します。

`io_uring` を使うイベントループなどで、ドライバが操作の内容を調べたり再試行のタイミングを決めたりする場合は、**Expr**（`kont.Expr[A]`）を使います。残りの処理は継続フレームが保持します。`Step` は結果またはサスペンションを返し、`Advance` は保留中の操作をエンドポイントで試みます。受信時に `Advance` が `iox.ErrWouldBlock` を返したら、同じサスペンションを保持し、相手が値を送信してから再試行します。

次の二つは同じ受信ロールです。どちらも `int` を受信し、自分の側で `Close` を実行してから受信値を返します。送信ロールと組み合わせて、Cont では `Run`、Expr では `RunExpr` で実行します。

```go
contReceiver := sess.RecvBind(sess.CloseDone[int])
exprReceiver := sess.ExprRecvBind(sess.ExprCloseDone[int])
```

## 合成境界

エンドポイントは、容量に上限のあるキューを通じて値や分岐の選択をやり取りします。満杯のキューへの追加や空のキューからの取り出しを試みると、`iox.ErrWouldBlock` が返ります。ドライバは `takt` でバックエンドの完了を要求に対応付け、`tempo` でイベント履歴を追跡し、`cove` で試行のたびに現在のコンテキストを確認できます。エンドポイントのトランスポートでは `iox.ErrMore` をディスパッチャのエラーとして扱います。

## インストール

```bash
go get code.hybscloud.com/sess
```

Go 1.27+ が必要です。

## セッション操作

Go は各操作のペイロードと応答の型を検査します。各送信には、その段階で送り得るすべての値 `v` について型アサーション `v.(T)` が成功する `Recv[T]` を対応させます。たとえば、`Send[any]` で送った `[]byte` は `Recv[[]byte]` で受け取れます。分岐の選択には相手側の `Offer` を対応させます。データと分岐の選択は別々のキューを通るため、両者の操作順序が食い違っていても実行が完了することがあります。

| 操作 | 双対 | バックプレッシャー |
|------|------|-------------|
| `Send[T]` — 値を送信 | `Recv[T]` — 値を受信 | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — 分岐を選択 | `Offer` — ピアの選択に従う | `iox.ErrWouldBlock` |
| `Close` — 自分の側の終了を通知 | `Close` | しない |

## 使い方

両方のロールを 1 つのゴルーチンで実行する場合は `Run`、外部で管理する片側のエンドポイントには `Exec`、ドライバが再試行のタイミングを決める場合は `Step` と `Advance` を使用します。

### 送受信

送信に成功するとペイロードがキューに入ります。相手側で処理が終わったことを送信側が知る必要がある場合は、処理後に相手側から確認応答を返すようにします。

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
	return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Expr 版：`ExprSendThen`、`ExprRecvBind`、`ExprCloseDone`、`RunExpr`。

### 分岐

一方が分岐を選択し、デュアル側が両方の分岐を提供して選択に従います。

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

### 再帰プロトコル

繰り返すプロトコルは `Loop` と `Either` を使用します：`Left` はループを継続し、`Right` は終了します。

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
	if i >= 3 {
		return sess.CloseDone(kont.Right[int, string]("done"))
	}
	return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### 委譲

エンドポイントを送信して第三者に転送し、受信して委譲を受け入れます。

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
	return sess.CloseDone("accepted")
})
```

### ステッピング

`Step` は保留中の受信操作を返します。キューが空の間、`Advance` は同じサスペンションを返します。相手が送信した後、そのサスペンションで再試行すると値を受け取れます。

```go
sender, receiver := sess.New()
_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
_, pending, err := sess.Advance(receiver, h)
fmt.Println(iox.IsWouldBlock(err), pending == h) // true true

sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
value, next, err := sess.Advance(receiver, pending)
fmt.Println(value, next == nil, err) // 42 true <nil>
```

### エラー処理

エラー処理付きのペア実行は、対象の型に一致する未捕捉の `Throw` を処理すると終了します。相手側の `Either` を解釈する前に、返された `thrown` を確認してください。

```go
client := kont.ExprThrowError[string, string]("boom")
server := sess.ExprRecvBind(func(v string) kont.Expr[string] {
	return sess.ExprCloseDone("recv: " + v)
})

clientResult, serverResult, thrown := sess.RunErrorExpr[string](client, server)
if thrown != nil {
	// セッション全体の中断。
	fmt.Println("session aborted:", *thrown)
	// 相手側の Either はまだローカルに未解決のことがあります。
	_ = clientResult
	_ = serverResult
	return
}

// 未捕捉のセッション全体 Throw はなし。両方の Either が最終的なローカル結果です。
fmt.Println(clientResult, serverResult)
```

要点:

- `thrown == nil` のとき、両方の `Either` は最終的なローカル結果です。
- `thrown != nil` のとき、対になった実行はセッション全体として中断されています。`*thrown` が未捕捉 `Throw` であり、peer 側の `Either` はまだ未解決のことがあります。

## 実行モデル

| 関数 | 説明 |
|------|-------------|
| `Run` / `RunExpr` | 両側を1つのゴルーチンで実行 — 内部でエンドポイントペアを生成 |
| `Exec` / `ExecExpr` | 作成済みエンドポイント上で片側を実行 |
| `Step` + `Advance` | 外部イベントループ向けに、エフェクトを一つずつ評価 |

プロトコル全体を実行した場合と、満杯のキューに対して再試行した場合のメモリ割り当てを計測できます。[ベンチマークガイド](BENCHMARKS.md)

## 契約

`sess` は信頼された呼び出し側を前提とするトランスポート API です。各エンドポイントは同時に 1 つの goroutine だけが扱う前提であり、ホットパスには並行利用のガードや `Close` 後の検査を意図的に入れていません。

ペイロード型がインタフェースであっても、値は具体的な動的型を持っている必要があります。`any(nil)` や `error(nil)` のような nil インタフェース値は契約外です。nil 自体に意味がある場合は、具体型を持つ nil 値を使うか、明示的なラッパーで包んでください。

`ExprSendThen` などの融合コンストラクタが返す式は一度だけ実行できます。実行のたびに、そのロールの式を組み立て直してください。

## API

| カテゴリ | Cont | Expr |
|---------|------|------|
| コンストラクタ | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| 再帰 | `Loop` | `ExprLoop` |
| 実行 | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| エラー実行 | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| ステッピング | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| ブリッジ | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| トランスポート | `New` → `(*Endpoint, *Endpoint)` | |

## 実用レシピ

`RunErrorExpr` がエンドポイントのペアを作成します。リクエストの値を `42` から `-1` に変えると、サーバーがリクエストを拒否し、その理由が `thrown` に返ります。

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

プロアクターのイベントループでソケットからの入力を扱う場合は、`sess.New()` でペアを作り、`StepError` がサスペンションを返したら `AdvanceError` に渡します。受信時に `iox.ErrWouldBlock` が返ったら、そのサスペンションを保持します。アダプターがデコードした値を相手側のエンドポイントから送信できたら、同じサスペンションで受信を再試行します。

## 参考文献

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## 依存関係

- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — セッション識別子とクローズ回数のアトミックな更新
- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限定継続と代数的エフェクト
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — ノンブロッキングセマンティクス（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — ロックフリー FIFO キュー

## ライセンス

MIT — 詳細は [LICENSE](LICENSE) を参照。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
