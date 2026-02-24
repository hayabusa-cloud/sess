[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | **Français**

# sess

Protocoles de communication a types de session via effets algebriques sur [kont](https://code.hybscloud.com/kont).

## Presentation

`sess` fournit des protocoles bidirectionnels types composes de six operations, chacune distribuee comme un effet algebrique sur une paire d'endpoints sans verrou.

- Deux styles d'API : Cont (a base de closures) et Expr (a base de cadres, zero allocation amortie)
- Pas a pas : evalue un effet a la fois pour l'integration proactor et boucle d'evenements
- Non bloquant : les operations retournent `iox.ErrWouldBlock` lorsqu'elles ne peuvent pas se terminer immediatement

## Installation

```bash
go get code.hybscloud.com/sess
```

Necessite Go 1.26+.

## Operations de Session

| Operation | Effet | Suspend ? |
|-----------|-------|-----------|
| `Send[T]` | Envoyer une valeur | `iox.ErrWouldBlock` |
| `Recv[T]` | Recevoir une valeur | `iox.ErrWouldBlock` |
| `Close` | Terminer la session | Jamais |
| `SelectL` | Choisir la branche gauche | `iox.ErrWouldBlock` |
| `SelectR` | Choisir la branche droite | `iox.ErrWouldBlock` |
| `Offer` | Attendre le choix du pair | `iox.ErrWouldBlock` |

Deleguez un endpoint en l'envoyant ; acceptez la delegation en le recevant.

## Utilisation

### Envoi et Reception

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Equivalent Expr : `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Branchement

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

### Protocoles Recursifs

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Pas a Pas

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
for susp != nil {
    var err error
    _, susp, err = sess.Advance(ep, susp)
    if err != nil {
        continue // reessayer sur iox.ErrWouldBlock
    }
}
```

### Gestion des Erreurs

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right en cas de succes, Left en cas de Throw
```

## Modele d'Execution

| Fonction | Cas d'utilisation |
|----------|-------------------|
| `Run` / `RunExpr` | Executer les deux cotes sur un goroutine — cree une paire d'endpoints en interne |
| `Exec` / `ExecExpr` | Executer un cote sur un endpoint pre-cree |
| `Step` + `Advance` | Evalue un effet a la fois, pour boucles d'evenements externes |

**Cont vs Expr** : Cont est a base de closures et facile a composer. Expr est a base de cadres avec zero allocation amortie, adapte aux chemins critiques.

## API

| Categorie | Cont | Expr |
|-----------|------|------|
| Constructeurs | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Recursion | `Loop` | `ExprLoop` |
| Execution | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| Execution avec erreurs | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| Pas a pas | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| Pont | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| Transport | `New` → `(*Endpoint, *Endpoint)` | |

## Dependances

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuations delimitees et effets algebriques
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantique non bloquante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Files FIFO sans verrou

## Licence

MIT — voir [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
