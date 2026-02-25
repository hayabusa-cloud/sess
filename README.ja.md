[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | **日本語** | [Français](README.fr.md)

# sess

[kont](https://code.hybscloud.com/kont) の代数的エフェクトによるセッション型通信プロトコル。

## 概要

`sess` は6つの操作から構成される型付き双方向プロトコルを提供します。各操作はロックフリーなエンドポイントペア上で代数的エフェクトとしてディスパッチされます。

- **デュアルワールドAPI**：Cont（クロージャベース）と Expr（フレームベース、ゼロアロケーションホットパス）
- **ステッピング**：エフェクトを一つずつ評価し、proactor やイベントループと統合可能
- **iox ノンブロッキング代数**：厳密な進行モデルを強制し、操作は計算境界でネイティブに `iox.ErrWouldBlock` を生成し、プロアクタのイベントループ (例: io_uring) がシステムスレッドをブロックすることなく実行をシームレスに多重化することを可能にする

## インストール

```bash
go get code.hybscloud.com/sess
```

Go 1.26+ が必要です。

## セッション操作

| 操作 | エフェクト | サスペンド？ |
|------|-----------|-------------|
| `Send[T]` | 値を送信 | `iox.ErrWouldBlock` |
| `Recv[T]` | 値を受信 | `iox.ErrWouldBlock` |
| `Close` | セッションを終了 | しない |
| `SelectL` | 左分岐を選択 | `iox.ErrWouldBlock` |
| `SelectR` | 右分岐を選択 | `iox.ErrWouldBlock` |
| `Offer` | ピアの選択を待つ | `iox.ErrWouldBlock` |

エンドポイントの委譲は送信で、受け入れは受信で行います。

## 使い方

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

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### ステッピング

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

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: 成功時は Right、Throw 時は Left
```

## 実行モデル

| 関数 | ユースケース |
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

## 依存関係

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — 限定継続と代数的エフェクト
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — ノンブロッキングセマンティクス（`ErrWouldBlock`、`Backoff`）
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — ロックフリー FIFO キュー

## ライセンス

MIT — 詳細は [LICENSE](LICENSE) を参照。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
