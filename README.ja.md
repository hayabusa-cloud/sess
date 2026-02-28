[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | **日本語** | [Français](README.fr.md)

# sess

[kont](https://code.hybscloud.com/kont) の代数的エフェクトによるセッション型通信プロトコル。

## 概要

セッション型は通信プロトコルの各ステップに型を割り当てます。各操作 — 送信、受信、選択、提供、クローズ — は Go ジェネリクスにより個別に型安全であり、単一エンドポイント内のプロトコル合成も型安全です。双対性（エンドポイント間の操作の対応）はプログラマの責任です：プログラマが双対プロトコルを記述し、不一致は実行時に型アサーション失敗またはデッドロックとして現れます。

`sess` はセッション型を [kont](https://code.hybscloud.com/kont) エフェクトシステムで評価される代数的エフェクトとしてエンコードします。各プロトコルステップ — 送信、受信、選択、提供、クローズ — はトランスポートが操作を完了するまで計算を中断するエフェクトです。トランスポートは計算境界で `iox.ErrWouldBlock` を返し、proactor イベントループ（例：`io_uring`）がスレッドをブロックせずに実行を多重化できるようにします。

2つの等価な API：Cont（クロージャベース、直接的な合成）と Expr（フレームベース、ホットパス向けの償却ゼロアロケーション）。

## インストール

```bash
go get code.hybscloud.com/sess
```

Go 1.26+ が必要です。

## セッション操作

各操作には双対があります。一方のエンドポイントが操作を行うと、他方はその双対操作を行う必要があります。

| 操作 | 双対 | サスペンド？ |
|------|------|-------------|
| `Send[T]` — 値を送信 | `Recv[T]` — 値を受信 | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — 分岐を選択 | `Offer` — ピアの選択に従う | `iox.ErrWouldBlock` |
| `Close` — セッションを終了 | `Close` | しない |

## 使い方

プロトタイピングおよび検証には `Run` を使用します。外部管理されるエンドポイントには `Exec` を使用します。ステッピング制御が必要な場合、またはホットパスにおけるアロケーションのオーバーヘッドを最小化する場合は、Expr API（`RunExpr`/`ExecExpr`）を使用します。

### 送受信

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

proactor イベントループ（例：`io_uring`）向けに、`Step` と `Advance` は一度に1つのエフェクトを評価します。`Run` や `Exec` — 同期的に進行を待つ — とは異なり、ステッピング API は `iox.ErrWouldBlock` を呼び出し元に返し、イベントループが再スケジュールできるようにします。

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

### エラー処理

セッションプロトコルをエラーエフェクトと合成します。`Throw` はプロトコルを即座に短絡し、保留中のサスペンションを破棄します。

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: 成功時は Right、Throw 時は Left
```

## 実行モデル

| 関数 | 説明 |
|------|-------------|
| `Run` / `RunExpr` | 両側を1つのゴルーチンで実行 — 内部でエンドポイントペアを生成 |
| `Exec` / `ExecExpr` | 作成済みエンドポイント上で片側を実行 |
| `Step` + `Advance` | 外部イベントループ向けに、エフェクトを一つずつ評価 |

**Cont と Expr の使い分け**: Cont はクロージャベースで合成が容易。Expr はフレームベースで償却ゼロアロケーション、ホットパスに適する。

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

## References

- Kohei Honda. "Types for Dyadic Interaction." In *CONCUR 1993* (LNCS 715), pp. 509-523. Springer, 1993. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, Makoto Kubo. "Language Primitives and Type Discipline for Structured Communication-Based Programming." In *ESOP 1998* (LNCS 1381), pp. 122-138. Springer, 1998. https://doi.org/10.1007/BFb0053567
- Philip Wadler. "Propositions as Sessions." *Journal of Functional Programming* 24(2-3):384-418, 2014. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard, Nobuko Yoshida. "Effects as Sessions, Sessions as Effects." In *POPL 2016*, pp. 568-581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley, J. Garrett Morris. "Lightweight Functional Session Types." In *Behavioural Types: From Theory to Tools*, pp. 265-286, 2017 (first published year; DOI metadata date is 2022-09-01). https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, Sara Decova. "Exceptional Asynchronous Session Types: Session Types without Tiers." *Proc. ACM Program. Lang.* 3(POPL):28:1-28:29, 2019. https://doi.org/10.1145/3290341

## 依存関係

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限定継続と代数的エフェクト
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — ノンブロッキングセマンティクス（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — ロックフリー FIFO キュー

## ライセンス

MIT — 詳細は [LICENSE](LICENSE) を参照。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
